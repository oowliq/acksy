package store

import (
	"context"
	"time"

	"github.com/oowliq/acksy/internal/domain"
)

type webhookRow struct {
	ID          string `db:"id"`
	UserID      string `db:"user_id"`
	Name        string `db:"name"`
	URL         string `db:"url"`
	Format      string `db:"format"`
	Secret      string `db:"secret"`
	MinPriority int    `db:"min_priority"`
	DisabledAt  *int64 `db:"disabled_at"`
	CreatedAt   int64  `db:"created_at"`
	UpdatedAt   int64  `db:"updated_at"`
	LastSuccess *int64 `db:"last_success_at"`
	LastFailure *int64 `db:"last_failure_at"`
	LastError   string `db:"last_error"`
}

const webhookCols = `id, user_id, name, url, format, secret, min_priority, disabled_at, created_at, updated_at,
	last_success_at, last_failure_at, last_error`

func (r webhookRow) toDomain() domain.Webhook {
	return domain.Webhook{
		ID: r.ID, UserID: r.UserID, Name: r.Name, URL: r.URL, Format: domain.WebhookFormat(r.Format), Secret: r.Secret,
		MinPriority: r.MinPriority, Enabled: r.DisabledAt == nil, CreatedAt: fromMs(r.CreatedAt), UpdatedAt: fromMs(r.UpdatedAt),
		LastSuccess: fromMsPtr(r.LastSuccess), LastFailure: fromMsPtr(r.LastFailure), LastError: r.LastError,
		TopicIDs: []string{},
	}
}

func disabledAt(w *domain.Webhook) *int64 {
	if w.Enabled {
		return nil
	}
	v := ms(w.UpdatedAt)
	return &v
}

func (q queries) setWebhookTopics(ctx context.Context, w *domain.Webhook) error {
	if _, err := q.exec(ctx, `DELETE FROM webhook_topics WHERE webhook_id = ?`, w.ID); err != nil {
		return err
	}
	for _, topicID := range w.TopicIDs {
		if _, err := q.exec(ctx, `INSERT INTO webhook_topics (webhook_id, topic_id) VALUES (?, ?)`, w.ID, topicID); err != nil {
			return err
		}
	}
	return nil
}

func (q queries) loadWebhooks(ctx context.Context, where string, args ...any) ([]domain.Webhook, error) {
	var rows []webhookRow
	if err := q.sel(ctx, &rows, `SELECT `+webhookCols+` FROM webhooks WHERE `+where+` ORDER BY created_at, id`, args...); err != nil {
		return nil, err
	}
	out := make([]domain.Webhook, len(rows))
	for i, r := range rows {
		out[i] = r.toDomain()
		if err := q.sel(ctx, &out[i].TopicIDs, `SELECT topic_id FROM webhook_topics WHERE webhook_id = ? ORDER BY topic_id`, r.ID); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (q queries) CreateWebhook(ctx context.Context, w *domain.Webhook) error {
	_, err := q.exec(ctx, `INSERT INTO webhooks (`+webhookCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, '')`,
		w.ID, w.UserID, w.Name, w.URL, string(w.Format), w.Secret, w.MinPriority, disabledAt(w), ms(w.CreatedAt), ms(w.UpdatedAt))
	if err != nil {
		return err
	}
	return q.setWebhookTopics(ctx, w)
}

func (q queries) GetWebhook(ctx context.Context, userID, id string) (*domain.Webhook, error) {
	ws, err := q.loadWebhooks(ctx, `user_id = ? AND id = ?`, userID, id)
	if err != nil {
		return nil, err
	}
	if len(ws) == 0 {
		return nil, domain.NotFound("webhook")
	}
	return &ws[0], nil
}

func (q queries) GetWebhookByID(ctx context.Context, id string) (*domain.Webhook, error) {
	ws, err := q.loadWebhooks(ctx, `id = ?`, id)
	if err != nil {
		return nil, err
	}
	if len(ws) == 0 {
		return nil, domain.NotFound("webhook")
	}
	return &ws[0], nil
}

func (q queries) ListWebhooks(ctx context.Context, userID string) ([]domain.Webhook, error) {
	return q.loadWebhooks(ctx, `user_id = ?`, userID)
}

func (q queries) ListEnabledWebhooks(ctx context.Context, userID string) ([]domain.Webhook, error) {
	return q.loadWebhooks(ctx, `user_id = ? AND disabled_at IS NULL`, userID)
}

func (q queries) CountWebhooks(ctx context.Context, userID string) (int, error) {
	var n int
	err := q.get(ctx, &n, `SELECT COUNT(*) FROM webhooks WHERE user_id = ?`, userID)
	return n, err
}

func (q queries) UpdateWebhook(ctx context.Context, w *domain.Webhook) error {
	err := q.execOne(ctx, "webhook", `UPDATE webhooks SET name = ?, url = ?, format = ?, secret = ?, min_priority = ?,
		disabled_at = ?, updated_at = ? WHERE user_id = ? AND id = ?`,
		w.Name, w.URL, string(w.Format), w.Secret, w.MinPriority, disabledAt(w), ms(w.UpdatedAt), w.UserID, w.ID)
	if err != nil {
		return err
	}
	return q.setWebhookTopics(ctx, w)
}

func (q queries) DeleteWebhook(ctx context.Context, userID, id string) error {
	return q.execOne(ctx, "webhook", `DELETE FROM webhooks WHERE user_id = ? AND id = ?`, userID, id)
}

func (q queries) RecordWebhookResult(ctx context.Context, id string, ok bool, at time.Time, errMsg string) error {
	var err error
	if ok {
		_, err = q.exec(ctx, `UPDATE webhooks SET last_success_at = ?, last_error = '' WHERE id = ?`, ms(at), id)
	} else {
		_, err = q.exec(ctx, `UPDATE webhooks SET last_failure_at = ?, last_error = ? WHERE id = ?`, ms(at), errMsg, id)
	}
	return err
}

// Delivery queue.

type deliveryRow struct {
	ID          string `db:"id"`
	WebhookID   string `db:"webhook_id"`
	Event       string `db:"event"`
	Payload     string `db:"payload"`
	Attempts    int    `db:"attempts"`
	NextAttempt *int64 `db:"next_attempt_at"`
	LastStatus  int    `db:"last_status"`
	LastError   string `db:"last_error"`
	CreatedAt   int64  `db:"created_at"`
	FailedAt    *int64 `db:"failed_at"`
}

const deliveryCols = `id, webhook_id, event, payload, attempts, next_attempt_at, last_status, last_error, created_at, failed_at`

func (r deliveryRow) toDomain() domain.WebhookDelivery {
	return domain.WebhookDelivery{
		ID: r.ID, WebhookID: r.WebhookID, Event: domain.WebhookEvent(r.Event), Payload: r.Payload, Attempts: r.Attempts,
		NextAttempt: fromMsPtr(r.NextAttempt), LastStatus: r.LastStatus, LastError: r.LastError,
		CreatedAt: fromMs(r.CreatedAt), FailedAt: fromMsPtr(r.FailedAt),
	}
}

func (q queries) listDeliveries(ctx context.Context, query string, args ...any) ([]domain.WebhookDelivery, error) {
	var rows []deliveryRow
	if err := q.sel(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	out := make([]domain.WebhookDelivery, len(rows))
	for i, r := range rows {
		out[i] = r.toDomain()
	}
	return out, nil
}

func (q queries) EnqueueWebhookDelivery(ctx context.Context, d *domain.WebhookDelivery) error {
	_, err := q.exec(ctx, `INSERT INTO webhook_deliveries (`+deliveryCols+`) VALUES (?, ?, ?, ?, 0, ?, 0, '', ?, NULL)`,
		d.ID, d.WebhookID, string(d.Event), d.Payload, msPtr(d.NextAttempt), ms(d.CreatedAt))
	return err
}

func (q queries) ListDueWebhookDeliveries(ctx context.Context, now time.Time, limit int) ([]domain.WebhookDelivery, error) {
	return q.listDeliveries(ctx, `SELECT `+deliveryCols+` FROM webhook_deliveries
		WHERE next_attempt_at IS NOT NULL AND next_attempt_at <= ? ORDER BY next_attempt_at, id LIMIT ?`, ms(now), limit)
}

func (q queries) ClaimWebhookDelivery(ctx context.Context, id string, now, leaseUntil time.Time) (bool, error) {
	res, err := q.exec(ctx, `UPDATE webhook_deliveries SET next_attempt_at = ?, attempts = attempts + 1
		WHERE id = ? AND next_attempt_at IS NOT NULL AND next_attempt_at <= ?`, ms(leaseUntil), id, ms(now))
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

func (q queries) DeleteWebhookDelivery(ctx context.Context, id string) error {
	_, err := q.exec(ctx, `DELETE FROM webhook_deliveries WHERE id = ?`, id)
	return err
}

func (q queries) RescheduleWebhookDelivery(ctx context.Context, d *domain.WebhookDelivery) error {
	_, err := q.exec(ctx, `UPDATE webhook_deliveries SET next_attempt_at = ?, last_status = ?, last_error = ?, failed_at = ?
		WHERE id = ?`, msPtr(d.NextAttempt), d.LastStatus, d.LastError, msPtr(d.FailedAt), d.ID)
	return err
}

func (q queries) ListWebhookDeliveries(ctx context.Context, webhookID string, limit int) ([]domain.WebhookDelivery, error) {
	return q.listDeliveries(ctx, `SELECT `+deliveryCols+` FROM webhook_deliveries WHERE webhook_id = ?
		ORDER BY created_at DESC, id DESC LIMIT ?`, webhookID, limit)
}

func (q queries) DeleteFailedWebhookDeliveries(ctx context.Context, before time.Time) (int64, error) {
	res, err := q.exec(ctx, `DELETE FROM webhook_deliveries WHERE failed_at IS NOT NULL AND failed_at < ?`, ms(before))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
