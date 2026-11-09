package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/oowliq/acksy/internal/auth"
	"github.com/oowliq/acksy/internal/domain"
	"github.com/oowliq/acksy/internal/store"
)

// WebhookTestResult is what the receiver answered to a test request.
type WebhookTestResult struct {
	Status int    `json:"status"` // HTTP status, 0 when no response
	OK     bool   `json:"ok"`
	Error  string `json:"error,omitempty"`
}

// WebhookSender sends a test request to a webhook synchronously.
type WebhookSender interface {
	SendTest(ctx context.Context, w domain.Webhook) WebhookTestResult
}

// SetWebhookSender enables test requests.
func (s *Service) SetWebhookSender(ws WebhookSender) { s.webhookSender = ws }

// WebhookInput creates or changes a webhook; nil fields keep their value
// (or take the default on creation).
type WebhookInput struct {
	Name        *string
	URL         *string
	Format      *domain.WebhookFormat
	Topics      *[]string // topic references; empty list = all notifications
	MinPriority *int
	Enabled     *bool
}

// CreatedWebhook carries the signing secret, shown only once.
type CreatedWebhook struct {
	Webhook *domain.Webhook `json:"webhook"`
	Secret  string          `json:"secret"`
}

func (s *Service) applyWebhookInput(ctx context.Context, q store.Querier, p *auth.Principal, w *domain.Webhook, in WebhookInput) error {
	if in.Name != nil {
		w.Name = *in.Name
	}
	if in.URL != nil {
		w.URL = *in.URL
	}
	if in.Format != nil {
		w.Format = *in.Format
	}
	if in.MinPriority != nil {
		w.MinPriority = *in.MinPriority
	}
	if in.Enabled != nil {
		w.Enabled = *in.Enabled
	}
	if in.Topics != nil {
		ids := make([]string, 0, len(*in.Topics))
		for _, ref := range *in.Topics {
			t, err := s.resolveTopic(ctx, q, p, ref, false, nil)
			if err != nil {
				return err
			}
			ids = append(ids, t.ID)
		}
		w.TopicIDs = ids
	}
	return domain.ValidateWebhook(w)
}

// CreateWebhook adds a webhook for the caller's notifications.
func (s *Service) CreateWebhook(ctx context.Context, p *auth.Principal, in WebhookInput) (*CreatedWebhook, error) {
	if err := requireSession(p, "webhooks"); err != nil {
		return nil, err
	}
	now := s.now()
	w := &domain.Webhook{
		ID: s.newID(), UserID: p.User.ID, Format: domain.WebhookAcksy, MinPriority: domain.PriorityMin,
		Enabled: true, Secret: auth.NewWebhookSecret(), TopicIDs: []string{}, CreatedAt: now, UpdatedAt: now,
	}
	err := s.store.InTx(ctx, func(q store.Querier) error {
		n, err := q.CountWebhooks(ctx, p.User.ID)
		if err != nil {
			return err
		}
		if n >= domain.MaxWebhooks {
			return domain.Conflict("at most %d webhooks per user", domain.MaxWebhooks)
		}
		if err := s.applyWebhookInput(ctx, q, p, w, in); err != nil {
			return err
		}
		return q.CreateWebhook(ctx, w)
	})
	if err != nil {
		return nil, err
	}
	return &CreatedWebhook{Webhook: w, Secret: w.Secret}, nil
}

// ListWebhooks lists the caller's webhooks.
func (s *Service) ListWebhooks(ctx context.Context, p *auth.Principal) ([]domain.Webhook, error) {
	if err := requireSession(p, "webhooks"); err != nil {
		return nil, err
	}
	return s.store.ListWebhooks(ctx, p.User.ID)
}

// GetWebhook returns one of the caller's webhooks.
func (s *Service) GetWebhook(ctx context.Context, p *auth.Principal, id string) (*domain.Webhook, error) {
	if err := requireSession(p, "webhooks"); err != nil {
		return nil, err
	}
	return s.store.GetWebhook(ctx, p.User.ID, id)
}

// UpdateWebhook changes one of the caller's webhooks.
func (s *Service) UpdateWebhook(ctx context.Context, p *auth.Principal, id string, in WebhookInput) (*domain.Webhook, error) {
	if err := requireSession(p, "webhooks"); err != nil {
		return nil, err
	}
	var out *domain.Webhook
	err := s.store.InTx(ctx, func(q store.Querier) error {
		w, err := q.GetWebhook(ctx, p.User.ID, id)
		if err != nil {
			return err
		}
		if err := s.applyWebhookInput(ctx, q, p, w, in); err != nil {
			return err
		}
		w.UpdatedAt = s.now()
		out = w
		return q.UpdateWebhook(ctx, w)
	})
	return out, err
}

// RotateWebhookSecret replaces the signing secret and returns the new one.
func (s *Service) RotateWebhookSecret(ctx context.Context, p *auth.Principal, id string) (string, error) {
	if err := requireSession(p, "webhooks"); err != nil {
		return "", err
	}
	var secret string
	err := s.store.InTx(ctx, func(q store.Querier) error {
		w, err := q.GetWebhook(ctx, p.User.ID, id)
		if err != nil {
			return err
		}
		w.Secret, w.UpdatedAt = auth.NewWebhookSecret(), s.now()
		secret = w.Secret
		return q.UpdateWebhook(ctx, w)
	})
	return secret, err
}

// DeleteWebhook removes a webhook and its queued deliveries.
func (s *Service) DeleteWebhook(ctx context.Context, p *auth.Principal, id string) error {
	if err := requireSession(p, "webhooks"); err != nil {
		return err
	}
	return s.store.DeleteWebhook(ctx, p.User.ID, id)
}

// maxDeliveriesListed bounds the delivery history returned by the API.
const maxDeliveriesListed = 50

// ListWebhookDeliveries shows a webhook's pending and failed deliveries.
func (s *Service) ListWebhookDeliveries(ctx context.Context, p *auth.Principal, id string) ([]domain.WebhookDelivery, error) {
	if _, err := s.GetWebhook(ctx, p, id); err != nil {
		return nil, err
	}
	return s.store.ListWebhookDeliveries(ctx, id, maxDeliveriesListed)
}

// TestWebhook sends a sample notification right away.
func (s *Service) TestWebhook(ctx context.Context, p *auth.Principal, id string) (*WebhookTestResult, error) {
	w, err := s.GetWebhook(ctx, p, id)
	if err != nil {
		return nil, err
	}
	if s.webhookSender == nil {
		return nil, domain.Conflict("webhooks are not enabled on this server")
	}
	res := s.webhookSender.SendTest(ctx, *w)
	return &res, nil
}

// webhookQueue collects deliveries inside a publish transaction.
type webhookQueue struct {
	byUser map[string][]domain.Webhook
}

// enqueueWebhooks queues a delivery for every enabled webhook of the
// recipient that matches the stored notification. It runs in the publish
// transaction, so deliveries exist exactly when the notification does.
func (s *Service) enqueueWebhooks(ctx context.Context, q store.Querier, wq *webhookQueue, n *domain.Notification, typ domain.EventType) error {
	hooks, ok := wq.byUser[n.UserID]
	if !ok {
		var err error
		if hooks, err = q.ListEnabledWebhooks(ctx, n.UserID); err != nil {
			return err
		}
		wq.byUser[n.UserID] = hooks
	}
	if len(hooks) == 0 {
		return nil
	}
	event := domain.WebhookCreated
	if typ == domain.EventNotificationUpdated {
		event = domain.WebhookUpdated
	}
	payload, err := json.Marshal(n)
	if err != nil {
		return fmt.Errorf("encode webhook payload: %w", err)
	}
	now := s.now()
	for i := range hooks {
		if !hooks[i].Matches(n) {
			continue
		}
		d := &domain.WebhookDelivery{
			ID: s.newID(), WebhookID: hooks[i].ID, Event: event, Payload: string(payload), NextAttempt: &now, CreatedAt: now,
		}
		if err := q.EnqueueWebhookDelivery(ctx, d); err != nil {
			return err
		}
	}
	return nil
}

// The methods below serve the delivery worker.

// DueWebhookDeliveries returns deliveries ready to be attempted.
func (s *Service) DueWebhookDeliveries(ctx context.Context, limit int) ([]domain.WebhookDelivery, error) {
	return s.store.ListDueWebhookDeliveries(ctx, s.now(), limit)
}

// ClaimWebhookDelivery reserves a due delivery for lease; false means another
// worker (possibly on another instance) got it first.
func (s *Service) ClaimWebhookDelivery(ctx context.Context, id string, lease time.Duration) (bool, error) {
	now := s.now()
	return s.store.ClaimWebhookDelivery(ctx, id, now, now.Add(lease))
}

// WebhookByID returns a webhook for the worker; nil when it was deleted.
func (s *Service) WebhookByID(ctx context.Context, id string) (*domain.Webhook, error) {
	w, err := s.store.GetWebhookByID(ctx, id)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, nil
	}
	return w, err
}

// WebhookDelivered removes a delivered request from the queue.
func (s *Service) WebhookDelivered(ctx context.Context, d *domain.WebhookDelivery) error {
	if err := s.store.DeleteWebhookDelivery(ctx, d.ID); err != nil {
		return err
	}
	return s.store.RecordWebhookResult(ctx, d.WebhookID, true, s.now(), "")
}

// WebhookDropped removes a delivery that should not be sent any more (the
// webhook was disabled after it was queued).
func (s *Service) WebhookDropped(ctx context.Context, d *domain.WebhookDelivery) error {
	return s.store.DeleteWebhookDelivery(ctx, d.ID)
}

// WebhookAttemptFailed records a failed attempt. d.Attempts already counts
// it. Unless permanent, the delivery is retried on domain.WebhookRetry's
// schedule (or after retryAfter, when the receiver asked for longer); after
// the last attempt it is marked failed.
func (s *Service) WebhookAttemptFailed(ctx context.Context, d *domain.WebhookDelivery, status int, errMsg string, permanent bool, retryAfter time.Duration) error {
	now := s.now()
	d.LastStatus, d.LastError = status, errMsg
	if permanent || d.Attempts >= domain.MaxWebhookAttempts {
		d.NextAttempt, d.FailedAt = nil, &now
	} else {
		wait := max(domain.WebhookRetry[min(d.Attempts, len(domain.WebhookRetry))-1], min(retryAfter, time.Hour))
		next := now.Add(wait)
		d.NextAttempt, d.FailedAt = &next, nil
	}
	if err := s.store.RescheduleWebhookDelivery(ctx, d); err != nil {
		return err
	}
	return s.store.RecordWebhookResult(ctx, d.WebhookID, false, now, errMsg)
}

// PruneWebhookDeliveries deletes failed deliveries older than retention.
func (s *Service) PruneWebhookDeliveries(ctx context.Context, retention time.Duration) (int64, error) {
	return s.store.DeleteFailedWebhookDeliveries(ctx, s.now().Add(-retention))
}
