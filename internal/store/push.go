package store

import (
	"context"
	"time"

	"github.com/oowliq/acksy/internal/domain"
)

type pushRow struct {
	ID            string `db:"id"`
	UserID        string `db:"user_id"`
	Endpoint      string `db:"endpoint"`
	P256dh        string `db:"p256dh"`
	Auth          string `db:"auth"`
	Name          string `db:"name"`
	MinPriority   int    `db:"min_priority"`
	CreatedAt     int64  `db:"created_at"`
	LastSuccessAt *int64 `db:"last_success_at"`
	Failures      int    `db:"failures"`
}

const pushCols = `id, user_id, endpoint, p256dh, auth, name, min_priority, created_at, last_success_at, failures`

func (r pushRow) toDomain() domain.PushSubscription {
	return domain.PushSubscription{
		ID: r.ID, UserID: r.UserID, Endpoint: r.Endpoint, P256dh: r.P256dh, Auth: r.Auth, Name: r.Name,
		MinPriority: r.MinPriority, CreatedAt: fromMs(r.CreatedAt), LastSuccessAt: fromMsPtr(r.LastSuccessAt),
		Failures: r.Failures, Service: domain.PushServiceHost(r.Endpoint),
	}
}

func (q queries) listPush(ctx context.Context, where string, args ...any) ([]domain.PushSubscription, error) {
	var rows []pushRow
	if err := q.sel(ctx, &rows, `SELECT `+pushCols+` FROM push_subscriptions WHERE `+where+` ORDER BY created_at, id`, args...); err != nil {
		return nil, err
	}
	out := make([]domain.PushSubscription, len(rows))
	for i, r := range rows {
		out[i] = r.toDomain()
	}
	return out, nil
}

// UpsertPushSubscription stores s, or — when the endpoint is already known —
// moves it to s.UserID with fresh keys and settings. A browser has one
// endpoint, so a second user signing in on it takes the subscription over.
// s.ID is set to the stored ID.
func (q queries) UpsertPushSubscription(ctx context.Context, s *domain.PushSubscription) error {
	_, err := q.exec(ctx, `INSERT INTO push_subscriptions (`+pushCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL, 0)
		ON CONFLICT (endpoint) DO UPDATE SET user_id = excluded.user_id, p256dh = excluded.p256dh,
			auth = excluded.auth, name = excluded.name, min_priority = excluded.min_priority, failures = 0`,
		s.ID, s.UserID, s.Endpoint, s.P256dh, s.Auth, s.Name, s.MinPriority, ms(s.CreatedAt))
	if err != nil {
		return err
	}
	var id string
	if err := q.get(ctx, &id, `SELECT id FROM push_subscriptions WHERE endpoint = ?`, s.Endpoint); err != nil {
		return err
	}
	s.ID = id
	return nil
}

func (q queries) GetPushSubscription(ctx context.Context, userID, id string) (*domain.PushSubscription, error) {
	var r pushRow
	err := q.get(ctx, &r, `SELECT `+pushCols+` FROM push_subscriptions WHERE user_id = ? AND id = ?`, userID, id)
	if err != nil {
		return nil, notFound(err, "push subscription")
	}
	s := r.toDomain()
	return &s, nil
}

func (q queries) ListPushSubscriptions(ctx context.Context, userID string) ([]domain.PushSubscription, error) {
	return q.listPush(ctx, `user_id = ?`, userID)
}

func (q queries) CountPushSubscriptions(ctx context.Context, userID string) (int, error) {
	var n int
	err := q.get(ctx, &n, `SELECT COUNT(*) FROM push_subscriptions WHERE user_id = ?`, userID)
	return n, err
}

func (q queries) ListPushTargets(ctx context.Context, userID string, priority int) ([]domain.PushSubscription, error) {
	return q.listPush(ctx, `user_id = ? AND min_priority <= ?`, userID, priority)
}

func (q queries) UpdatePushSubscription(ctx context.Context, s *domain.PushSubscription) error {
	return q.execOne(ctx, "push subscription", `UPDATE push_subscriptions SET name = ?, min_priority = ?
		WHERE user_id = ? AND id = ?`, s.Name, s.MinPriority, s.UserID, s.ID)
}

func (q queries) DeletePushSubscription(ctx context.Context, userID, id string) error {
	return q.execOne(ctx, "push subscription", `DELETE FROM push_subscriptions WHERE user_id = ? AND id = ?`, userID, id)
}

func (q queries) DeletePushSubscriptionByID(ctx context.Context, id string) error {
	_, err := q.exec(ctx, `DELETE FROM push_subscriptions WHERE id = ?`, id)
	return err
}

func (q queries) RecordPushResult(ctx context.Context, id string, ok bool, at time.Time) error {
	var err error
	if ok {
		_, err = q.exec(ctx, `UPDATE push_subscriptions SET last_success_at = ?, failures = 0 WHERE id = ?`, ms(at), id)
	} else {
		_, err = q.exec(ctx, `UPDATE push_subscriptions SET failures = failures + 1 WHERE id = ?`, id)
	}
	return err
}

// Instance settings.

func (q queries) GetSetting(ctx context.Context, key string) (string, error) {
	var v string
	if err := q.get(ctx, &v, `SELECT value FROM instance_settings WHERE key = ?`, key); err != nil {
		return "", notFound(err, "setting")
	}
	return v, nil
}

// GetSettingWithTime returns a setting and when it last changed.
func (q queries) GetSettingWithTime(ctx context.Context, key string) (string, time.Time, error) {
	var r struct {
		Value     string `db:"value"`
		UpdatedAt int64  `db:"updated_at"`
	}
	if err := q.get(ctx, &r, `SELECT value, updated_at FROM instance_settings WHERE key = ?`, key); err != nil {
		return "", time.Time{}, notFound(err, "setting")
	}
	return r.Value, time.UnixMilli(r.UpdatedAt).UTC(), nil
}

// PutSetting inserts or replaces a setting.
func (q queries) PutSetting(ctx context.Context, key, value string, at time.Time) error {
	_, err := q.exec(ctx, `INSERT INTO instance_settings (key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, key, value, ms(at))
	return err
}

func (q queries) InsertSettingIfAbsent(ctx context.Context, key, value string, at time.Time) error {
	_, err := q.exec(ctx, `INSERT INTO instance_settings (key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT (key) DO NOTHING`, key, value, ms(at))
	return err
}
