package store

import (
	"context"
	"strings"

	"github.com/oowliq/acksy/internal/domain"
)

func (q queries) ListRetentionTopics(ctx context.Context) ([]domain.Topic, error) {
	var rows []struct {
		ID            string `db:"id"`
		RetentionDays *int   `db:"retention_days"`
		MaxCount      *int   `db:"max_notifications"`
	}
	err := q.sel(ctx, &rows, `SELECT id, retention_days, max_notifications FROM topics
		WHERE retention_days IS NOT NULL OR max_notifications IS NOT NULL ORDER BY id`)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Topic, len(rows))
	for i, r := range rows {
		out[i] = domain.Topic{ID: r.ID, RetentionDays: r.RetentionDays, MaxNotifications: r.MaxCount}
	}
	return out, nil
}

type refRow struct {
	UserID string `db:"user_id"`
	ID     string `db:"id"`
}

func toRefs(rows []refRow) []domain.NotificationRef {
	out := make([]domain.NotificationRef, len(rows))
	for i, r := range rows {
		out[i] = domain.NotificationRef{UserID: r.UserID, ID: r.ID}
	}
	return out
}

func (q queries) ListExpiredNotificationRefs(ctx context.Context, e domain.ExpiryQuery, limit int) ([]domain.NotificationRef, error) {
	var (
		where []string
		args  []any
	)
	switch e.State {
	case domain.ReadOnly:
		where = append(where, "read_at IS NOT NULL")
	case domain.UnreadOnly:
		where = append(where, "read_at IS NULL")
	}
	if !e.UpdatedBefore.IsZero() {
		where, args = append(where, "updated_at < ?"), append(args, ms(e.UpdatedBefore))
	}
	if !e.ExpiresBefore.IsZero() {
		where, args = append(where, "expires_at IS NOT NULL AND expires_at < ?"), append(args, ms(e.ExpiresBefore))
	}
	if e.Priority != 0 {
		where, args = append(where, "priority = ?"), append(args, e.Priority)
	}
	if e.TopicID != "" {
		where, args = append(where, "topic_id = ?"), append(args, e.TopicID)
	}
	if e.SkipTopicOverrides {
		where = append(where, "(topic_id IS NULL OR topic_id NOT IN (SELECT id FROM topics WHERE retention_days IS NOT NULL))")
	}
	if len(where) == 0 {
		return nil, domain.Invalid("expiry query without conditions")
	}
	var rows []refRow
	err := q.sel(ctx, &rows, `SELECT user_id, id FROM notifications WHERE `+strings.Join(where, " AND ")+
		` ORDER BY id LIMIT ?`, append(args, limit)...)
	return toRefs(rows), err
}

func (q queries) ListUsersOverNotificationLimit(ctx context.Context, topicID string, maxCount int) ([]string, error) {
	var ids []string
	var err error
	if topicID == "" {
		err = q.sel(ctx, &ids, `SELECT user_id FROM notifications GROUP BY user_id HAVING COUNT(*) > ? ORDER BY user_id`, maxCount)
	} else {
		err = q.sel(ctx, &ids, `SELECT user_id FROM notifications WHERE topic_id = ?
			GROUP BY user_id HAVING COUNT(*) > ? ORDER BY user_id`, topicID, maxCount)
	}
	return ids, err
}

func (q queries) ListOverflowNotificationRefs(ctx context.Context, userID, topicID string, keep, limit int) ([]domain.NotificationRef, error) {
	scope, args := "user_id = ?", []any{userID}
	if topicID != "" {
		scope, args = scope+" AND topic_id = ?", append(args, topicID)
	}
	var total int
	if err := q.get(ctx, &total, `SELECT COUNT(*) FROM notifications WHERE `+scope, args...); err != nil {
		return nil, err
	}
	excess := min(total-keep, limit)
	if excess <= 0 {
		return nil, nil
	}
	// Read notifications go first, oldest first (IDs are time-ordered ULIDs).
	var rows []refRow
	err := q.sel(ctx, &rows, `SELECT user_id, id FROM notifications WHERE `+scope+`
		ORDER BY CASE WHEN read_at IS NULL THEN 1 ELSE 0 END, id LIMIT ?`, append(args, excess)...)
	return toRefs(rows), err
}
