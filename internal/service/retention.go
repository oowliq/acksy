package service

import (
	"context"
	"slices"
	"time"

	"github.com/oowliq/acksy/internal/domain"
	"github.com/oowliq/acksy/internal/store"
)

// Retention batch sizes: each batch is one transaction per affected user,
// and one run stops after maxRetentionPerRun deletions (the janitor comes
// back an hour later), so a large backlog never blocks the database long.
const (
	retentionBatch     = 500
	maxRetentionPerRun = 50_000
)

// RetentionResult reports what a retention run deleted.
type RetentionResult struct {
	Deleted int
}

// ApplyRetention deletes notifications according to the instance policy
// and topic overrides (see domain.RetentionPolicy). Deletions are ordinary
// bulk deletes: they emit notifications.bulk events, so open clients drop
// the items immediately (invariant 3).
//
// Order of rules: expired (past expires_at), topic TTLs, read TTL by
// priority, unread TTL, topic count limits, user count limit.
func (s *Service) ApplyRetention(ctx context.Context) (*RetentionResult, error) {
	pol := s.Settings(ctx).RetentionPolicy()
	now := s.now()
	res := &RetentionResult{}

	topics, err := s.store.ListRetentionTopics(ctx)
	if err != nil {
		return res, err
	}

	queries := []domain.ExpiryQuery{{ExpiresBefore: now.Add(-domain.ExpiredGrace)}}
	for _, t := range topics {
		if t.RetentionDays != nil {
			queries = append(queries, domain.ExpiryQuery{
				State: domain.ReadOnly, TopicID: t.ID, UpdatedBefore: now.Add(-time.Duration(*t.RetentionDays) * 24 * time.Hour),
			})
		}
	}
	for prio := domain.PriorityMin; prio <= domain.PriorityMax; prio++ {
		if ttl := pol.ReadTTLFor(prio); ttl > 0 {
			queries = append(queries, domain.ExpiryQuery{
				State: domain.ReadOnly, Priority: prio, UpdatedBefore: now.Add(-ttl), SkipTopicOverrides: true,
			})
		}
	}
	if pol.UnreadTTL > 0 {
		queries = append(queries, domain.ExpiryQuery{State: domain.UnreadOnly, UpdatedBefore: now.Add(-pol.UnreadTTL)})
	}

	for _, q := range queries {
		if err := s.drainRefs(ctx, res, func(limit int) ([]domain.NotificationRef, error) {
			return s.store.ListExpiredNotificationRefs(ctx, q, limit)
		}); err != nil {
			return res, err
		}
	}

	var limits []countLimit
	for _, t := range topics {
		if t.MaxNotifications != nil {
			limits = append(limits, countLimit{topicID: t.ID, keep: *t.MaxNotifications})
		}
	}
	if pol.MaxPerUser > 0 {
		limits = append(limits, countLimit{keep: pol.MaxPerUser})
	}
	for _, l := range limits {
		users, err := s.store.ListUsersOverNotificationLimit(ctx, l.topicID, l.keep)
		if err != nil {
			return res, err
		}
		for _, userID := range users {
			if err := s.drainRefs(ctx, res, func(limit int) ([]domain.NotificationRef, error) {
				return s.store.ListOverflowNotificationRefs(ctx, userID, l.topicID, l.keep, limit)
			}); err != nil {
				return res, err
			}
		}
	}
	return res, nil
}

// countLimit caps each user's notifications in a topic ("" = in total).
type countLimit struct {
	topicID string
	keep    int
}

// drainRefs deletes what next returns, batch by batch, until it runs dry or
// the per-run budget is spent.
func (s *Service) drainRefs(ctx context.Context, res *RetentionResult, next func(limit int) ([]domain.NotificationRef, error)) error {
	for res.Deleted < maxRetentionPerRun {
		if err := ctx.Err(); err != nil {
			return err
		}
		refs, err := next(min(retentionBatch, maxRetentionPerRun-res.Deleted))
		if err != nil || len(refs) == 0 {
			return err
		}
		if err := s.deleteRefs(ctx, refs); err != nil {
			return err
		}
		res.Deleted += len(refs)
		if len(refs) < retentionBatch {
			return nil
		}
	}
	return nil
}

// deleteRefs deletes notifications, one transaction and one bulk event per
// user.
func (s *Service) deleteRefs(ctx context.Context, refs []domain.NotificationRef) error {
	byUser := map[string][]string{}
	for _, r := range refs {
		byUser[r.UserID] = append(byUser[r.UserID], r.ID)
	}
	for userID, ids := range byUser {
		slices.Sort(ids)
		err := s.tx(ctx, func(q store.Querier, em *emitter) error {
			now := s.now()
			if err := q.BulkUpdateNotifications(ctx, userID, ids, domain.BulkDelete, now); err != nil {
				return err
			}
			return em.emit(ctx, userID, domain.EventNotificationsBulk, domain.BulkEventData{Action: domain.BulkDelete, IDs: ids, At: now})
		})
		if err != nil {
			return err
		}
	}
	return nil
}
