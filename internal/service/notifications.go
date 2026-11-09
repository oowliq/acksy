package service

import (
	"context"
	"slices"
	"time"

	"github.com/oowliq/acksy/internal/auth"
	"github.com/oowliq/acksy/internal/domain"
	"github.com/oowliq/acksy/internal/store"
)

// PublishResult summarizes a publish.
type PublishResult struct {
	MessageID    string `json:"message_id"`
	Recipients   int    `json:"recipients"`
	Deduplicated int    `json:"deduplicated"`
	// NotificationID is the caller's own copy, when the caller is a recipient.
	NotificationID string `json:"notification_id,omitempty"`
}

// Publish delivers a message: to the caller when no topic is given, or to
// every member of the topic. Dedup keys update existing notifications.
func (s *Service) Publish(ctx context.Context, p *auth.Principal, m domain.Message) (*PublishResult, error) {
	if err := p.Require(domain.ScopePublish); err != nil {
		return nil, err
	}
	if err := s.checkPublishRate(p); err != nil {
		return nil, err
	}
	now := s.now()
	if err := m.Normalize(now); err != nil {
		return nil, err
	}
	tokenTopicID := ""
	if r := p.TopicRestriction(); m.Topic == "" && m.TopicFromToken && len(r) == 1 {
		tokenTopicID = r[0]
	}
	if m.Topic == "" && tokenTopicID == "" && p.TopicRestriction() != nil {
		return nil, domain.Forbidden("this token can only publish to its topics")
	}

	res := &PublishResult{MessageID: s.newID()}
	var delivered []domain.Notification
	err := s.tx(ctx, func(q store.Querier, em *emitter) error {
		delivered = delivered[:0]
		wq := &webhookQueue{byUser: map[string][]domain.Webhook{}}
		var topic *domain.Topic
		recipients := []string{p.User.ID}
		if tokenTopicID != "" {
			ref, err := topicRefByID(ctx, q, tokenTopicID)
			if err != nil {
				return err
			}
			m.Topic = ref
		}
		if m.Topic != "" {
			var err error
			if topic, err = s.resolveTopic(ctx, q, p, m.Topic, true, em); err != nil {
				return err
			}
			if recipients, err = q.ListRecipientIDs(ctx, topic.ID); err != nil {
				return err
			}
		}

		prio := m.Priority
		if prio == 0 {
			prio = domain.PriorityDefault
			if topic != nil && topic.DefaultPriority != nil {
				prio = *topic.DefaultPriority
			}
		}
		var tokenID string
		if p.Token != nil {
			tokenID = p.Token.ID
		}

		res.Recipients = len(recipients)
		for _, userID := range recipients {
			n := &domain.Notification{
				ID: s.newID(), UserID: userID, MessageID: res.MessageID, Title: m.Title, Body: m.Body,
				Priority: prio, Tags: m.Tags, ClickURL: m.ClickURL, Actions: m.Actions,
				GroupKey: m.GroupKey, DedupKey: m.DedupKey, Source: m.Source, TokenID: tokenID,
				CreatedAt: now, UpdatedAt: now, ExpiresAt: m.ExpiresAt,
			}
			if topic != nil {
				n.Topic = &domain.TopicSummary{ID: topic.ID, Name: topic.Name}
			}
			stored, deduped, err := s.storeNotification(ctx, q, em, wq, n)
			if err != nil {
				return err
			}
			if deduped {
				res.Deduplicated++
			}
			delivered = append(delivered, *stored)
			if userID == p.User.ID {
				res.NotificationID = stored.ID
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.notify(delivered) // after commit, like the broker fan-out
	return res, nil
}

// storeNotification writes one recipient's notification inside a transaction, with its
// event and webhook deliveries. It returns the stored copy and whether it
// was a dedup repeat. Callers hand the stored copies to s.notify after commit.
func (s *Service) storeNotification(ctx context.Context, q store.Querier, em *emitter, wq *webhookQueue, n *domain.Notification) (*domain.Notification, bool, error) {
	deduped, err := q.UpsertNotification(ctx, n)
	if err != nil {
		return nil, false, err
	}
	stored, err := q.GetNotification(ctx, n.UserID, n.ID)
	if err != nil {
		return nil, false, err
	}
	typ := domain.EventNotificationCreated
	if deduped {
		typ = domain.EventNotificationUpdated
	}
	if err := em.emit(ctx, n.UserID, typ, stored); err != nil {
		return nil, false, err
	}
	if err := s.enqueueWebhooks(ctx, q, wq, stored, typ); err != nil {
		return nil, false, err
	}
	return stored, deduped, nil
}

// checkPublishRate enforces the per-token and per-user publish limits. It
// lives in the service so every ingest path (REST, ntfy, gotify) shares it.
func (s *Service) checkPublishRate(p *auth.Principal) error {
	if p.User == nil || p.User.ID == "" {
		return nil // system principal (CLI, bootstrap)
	}
	now := s.now()
	if p.Token != nil {
		if ok, wait := s.tokenLimit.Allow(p.Token.ID, now); !ok {
			return &domain.RateLimitError{Msg: "publish rate limit exceeded for this token", RetryAfter: wait}
		}
	}
	if ok, wait := s.userLimit.Allow(p.User.ID, now); !ok {
		if p.Token != nil {
			s.tokenLimit.Refund(p.Token.ID, now) // the request did not go through
		}
		return &domain.RateLimitError{Msg: "publish rate limit exceeded for this user", RetryAfter: wait}
	}
	return nil
}

// topicRefByID returns the fully qualified reference (owner/name) of a topic,
// so it can go through resolveTopic and its membership checks.
func topicRefByID(ctx context.Context, q store.Querier, id string) (string, error) {
	t, err := q.GetTopic(ctx, id)
	if err != nil {
		return "", err
	}
	owner, err := q.GetUser(ctx, t.OwnerID)
	if err != nil {
		return "", err
	}
	return owner.Username + "/" + t.Name, nil
}

// KeyResult reports how many recipients' notifications a keyed action changed.
type KeyResult struct {
	Affected int
}

// ApplyToKey deletes (BulkDelete) or marks read (BulkRead) every recipient's
// notification in a topic that carries dedupKey. This backs ntfy's
// delete/clear by sequence ID. It needs the publish scope and the right to
// publish to the topic: the same callers can already replace the message by
// publishing with the same key.
func (s *Service) ApplyToKey(ctx context.Context, p *auth.Principal, topicRef, dedupKey string, action domain.BulkAction) (*KeyResult, error) {
	if err := p.Require(domain.ScopePublish); err != nil {
		return nil, err
	}
	if err := s.checkPublishRate(p); err != nil {
		return nil, err
	}
	if action != domain.BulkDelete && action != domain.BulkRead {
		return nil, domain.Invalid("only delete and read can be applied by key")
	}
	if dedupKey == "" || len(dedupKey) > domain.MaxKeyLen {
		return nil, domain.Invalid("key must be 1-%d bytes", domain.MaxKeyLen)
	}
	res := &KeyResult{}
	err := s.tx(ctx, func(q store.Querier, em *emitter) error {
		topic, err := s.resolveTopic(ctx, q, p, topicRef, false, nil)
		if err != nil {
			return err
		}
		refs, err := q.ListNotificationRefsByKey(ctx, topic.ID, dedupKey)
		if err != nil {
			return err
		}
		byUser := map[string][]string{}
		var users []string
		for _, r := range refs {
			if _, ok := byUser[r.UserID]; !ok {
				users = append(users, r.UserID)
			}
			byUser[r.UserID] = append(byUser[r.UserID], r.ID)
		}
		now := s.now()
		for _, userID := range users {
			ids := byUser[userID]
			if err := q.BulkUpdateNotifications(ctx, userID, ids, action, now); err != nil {
				return err
			}
			if err := em.emit(ctx, userID, domain.EventNotificationsBulk, domain.BulkEventData{Action: action, IDs: ids, At: now}); err != nil {
				return err
			}
		}
		res.Affected = len(refs)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// ListQuery is a notification list request.
type ListQuery struct {
	View        domain.View
	UnreadOnly  bool
	Topic       string // topic reference
	NoTopic     bool
	MinPriority int
	GroupKey    string
	Search      string // free text; see domain.SearchTerms
	Before      string
	Limit       int
}

// Page is one page of notifications.
type Page struct {
	Items      []domain.Notification `json:"items"`
	NextCursor string                `json:"next_cursor,omitempty"`
}

// Paging limits.
const (
	DefaultPageSize = 50
	MaxPageSize     = 200
	maxBulk         = 10_000
)

func (s *Service) buildFilter(ctx context.Context, p *auth.Principal, lq ListQuery) (domain.NotificationFilter, error) {
	if lq.View == "" {
		lq.View = domain.ViewInbox
	}
	if !lq.View.Valid() {
		return domain.NotificationFilter{}, domain.Invalid("view must be inbox, archived, snoozed or all")
	}
	if lq.MinPriority < 0 || lq.MinPriority > domain.PriorityMax {
		return domain.NotificationFilter{}, domain.Invalid("min_priority must be between 1 and 5")
	}
	f := domain.NotificationFilter{
		UserID: p.User.ID, View: lq.View, UnreadOnly: lq.UnreadOnly, NoTopic: lq.NoTopic,
		MinPriority: lq.MinPriority, GroupKey: lq.GroupKey, Before: lq.Before, Now: s.now(),
		TopicIDs: p.TopicRestriction(),
	}
	terms, err := domain.SearchTerms(lq.Search)
	if err != nil {
		return f, err
	}
	f.Search = terms
	if lq.Topic != "" {
		t, err := s.resolveTopic(ctx, s.store, p, lq.Topic, false, nil)
		if err != nil {
			return f, err
		}
		f.TopicIDs = []string{t.ID}
	}
	if f.TopicIDs != nil && lq.NoTopic {
		f.TopicIDs = []string{} // restricted tokens never see topic-less notifications
	}
	return f, nil
}

// ListNotifications returns a page of the caller's notifications, newest first.
func (s *Service) ListNotifications(ctx context.Context, p *auth.Principal, lq ListQuery) (*Page, error) {
	if err := p.Require(domain.ScopeRead); err != nil {
		return nil, err
	}
	f, err := s.buildFilter(ctx, p, lq)
	if err != nil {
		return nil, err
	}
	limit := lq.Limit
	if limit <= 0 {
		limit = DefaultPageSize
	}
	f.Limit = min(limit, MaxPageSize) + 1 // one extra to detect the next page

	items, err := s.store.ListNotifications(ctx, f)
	if err != nil {
		return nil, err
	}
	page := &Page{Items: items}
	if len(items) == f.Limit {
		page.Items = items[:f.Limit-1]
		page.NextCursor = page.Items[len(page.Items)-1].ID
	}
	return page, nil
}

// visibleNotification loads one of the caller's notifications, honouring
// token topic restrictions.
func (s *Service) visibleNotification(ctx context.Context, q store.Querier, p *auth.Principal, id string) (*domain.Notification, error) {
	n, err := q.GetNotification(ctx, p.User.ID, id)
	if err != nil {
		return nil, err
	}
	if p.TopicRestriction() != nil && (n.Topic == nil || !p.CanUseTopic(n.Topic.ID)) {
		return nil, domain.NotFound("notification")
	}
	return n, nil
}

// GetNotification returns one of the caller's notifications.
func (s *Service) GetNotification(ctx context.Context, p *auth.Principal, id string) (*domain.Notification, error) {
	if err := p.Require(domain.ScopeRead); err != nil {
		return nil, err
	}
	return s.visibleNotification(ctx, s.store, p, id)
}

// StatePatch changes read state; nil fields are unchanged. SnoozedUntil is
// applied when SetSnooze is true (nil clears the snooze).
type StatePatch struct {
	Read         *bool
	Archived     *bool
	SnoozedUntil *time.Time
	SetSnooze    bool
}

// UpdateNotificationState changes the read state of one notification.
func (s *Service) UpdateNotificationState(ctx context.Context, p *auth.Principal, id string, patch StatePatch) (*domain.Notification, error) {
	if err := p.Require(domain.ScopeWrite); err != nil {
		return nil, err
	}
	var out *domain.Notification
	err := s.tx(ctx, func(q store.Querier, em *emitter) error {
		n, err := s.visibleNotification(ctx, q, p, id)
		if err != nil {
			return err
		}
		now := s.now()
		if patch.Read != nil {
			n.ReadAt = toggleTime(n.ReadAt, *patch.Read, now)
		}
		if patch.Archived != nil {
			n.ArchivedAt = toggleTime(n.ArchivedAt, *patch.Archived, now)
			if *patch.Archived && n.ReadAt == nil {
				n.ReadAt = &now // archiving acknowledges
			}
		}
		if patch.SetSnooze {
			if patch.SnoozedUntil != nil && !patch.SnoozedUntil.After(now) {
				return domain.Invalid("snoozed_until must be in the future")
			}
			n.SnoozedUntil = patch.SnoozedUntil
		}
		n.UpdatedAt = now
		if err := q.SetNotificationState(ctx, n); err != nil {
			return err
		}
		out = n
		return em.emit(ctx, p.User.ID, domain.EventNotificationUpdated, n)
	})
	return out, err
}

func toggleTime(cur *time.Time, on bool, now time.Time) *time.Time {
	switch {
	case on && cur == nil:
		return &now
	case on:
		return cur
	default:
		return nil
	}
}

// DeleteNotification deletes one of the caller's notifications.
func (s *Service) DeleteNotification(ctx context.Context, p *auth.Principal, id string) error {
	if err := p.Require(domain.ScopeWrite); err != nil {
		return err
	}
	return s.tx(ctx, func(q store.Querier, em *emitter) error {
		n, err := s.visibleNotification(ctx, q, p, id)
		if err != nil {
			return err
		}
		if err := q.BulkUpdateNotifications(ctx, p.User.ID, []string{n.ID}, domain.BulkDelete, s.now()); err != nil {
			return err
		}
		return em.emit(ctx, p.User.ID, domain.EventNotificationDeleted, domain.IDEventData{ID: n.ID})
	})
}

// BulkSelector chooses notifications for a bulk action. At least one of IDs,
// Topic, GroupKey or All must be set; they combine with AND.
type BulkSelector struct {
	IDs      []string
	Topic    string
	NoTopic  bool
	GroupKey string
	View     domain.View
	All      bool
	// Search and MinPriority narrow the selection; they don't select on their own.
	Search      string
	MinPriority int
}

// BulkResult reports how many notifications changed.
type BulkResult struct {
	Affected int `json:"affected"`
}

// Bulk applies action to the selected notifications.
func (s *Service) Bulk(ctx context.Context, p *auth.Principal, action domain.BulkAction, sel BulkSelector) (*BulkResult, error) {
	if err := p.Require(domain.ScopeWrite); err != nil {
		return nil, err
	}
	if !action.Valid() {
		return nil, domain.Invalid("action must be read, unread, archive, unarchive or delete")
	}
	if len(sel.IDs) == 0 && sel.Topic == "" && !sel.NoTopic && sel.GroupKey == "" && !sel.All {
		return nil, domain.Invalid("select notifications with ids, topic, no_topic, group_key or all")
	}
	if len(sel.IDs) > maxBulk {
		return nil, domain.Invalid("at most %d ids per request", maxBulk)
	}
	view := sel.View
	if view == "" {
		view = domain.ViewAll
		if action == domain.BulkRead || action == domain.BulkArchive {
			view = domain.ViewInbox
		}
	}
	f, err := s.buildFilter(ctx, p, ListQuery{
		View: view, Topic: sel.Topic, NoTopic: sel.NoTopic, GroupKey: sel.GroupKey,
		Search: sel.Search, MinPriority: sel.MinPriority,
	})
	if err != nil {
		return nil, err
	}
	f.IDs, f.Limit = sel.IDs, maxBulk
	if action == domain.BulkRead {
		f.UnreadOnly = true
	}

	res := &BulkResult{}
	err = s.tx(ctx, func(q store.Querier, em *emitter) error {
		ids, err := q.ListNotificationIDs(ctx, f)
		if err != nil || len(ids) == 0 {
			return err
		}
		now := s.now()
		if err := q.BulkUpdateNotifications(ctx, p.User.ID, ids, action, now); err != nil {
			return err
		}
		res.Affected = len(ids)
		slices.Sort(ids)
		return em.emit(ctx, p.User.ID, domain.EventNotificationsBulk, domain.BulkEventData{Action: action, IDs: ids, At: now})
	})
	return res, err
}

// Counts returns unread inbox counts for the caller.
func (s *Service) Counts(ctx context.Context, p *auth.Principal) (domain.Counts, error) {
	if err := p.Require(domain.ScopeRead); err != nil {
		return domain.Counts{}, err
	}
	return s.store.Counts(ctx, p.User.ID, p.TopicRestriction(), s.now())
}
