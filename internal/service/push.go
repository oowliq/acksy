package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/oowliq/acksy/internal/auth"
	"github.com/oowliq/acksy/internal/domain"
	"github.com/oowliq/acksy/internal/store"
)

// Notifier receives notifications after their transaction committed, for
// delivery outside Acksy (Web Push now, webhooks later). It must not block:
// implementations queue the work.
type Notifier interface {
	Notify(ns []domain.Notification)
}

// PushSender delivers a single test message to one subscription.
type PushSender interface {
	SendTest(ctx context.Context, sub domain.PushSubscription) error
}

// AddNotifier registers n for committed notifications. Call during wiring,
// before serving requests.
func (s *Service) AddNotifier(n Notifier) { s.notifiers = append(s.notifiers, n) }

// SetPushSender enables test pushes.
func (s *Service) SetPushSender(p PushSender) { s.pushSender = p }

// notify hands committed notifications that may alert the user to the
// notifiers: unread, not snoozed, not expired.
func (s *Service) notify(ns []domain.Notification) {
	if len(s.notifiers) == 0 || len(ns) == 0 {
		return
	}
	now := s.now()
	out := make([]domain.Notification, 0, len(ns))
	for _, n := range ns {
		if n.ReadAt != nil || n.ArchivedAt != nil ||
			(n.SnoozedUntil != nil && n.SnoozedUntil.After(now)) ||
			(n.ExpiresAt != nil && !n.ExpiresAt.After(now)) {
			continue
		}
		out = append(out, n)
	}
	if len(out) == 0 {
		return
	}
	for _, nf := range s.notifiers {
		nf.Notify(out)
	}
}

// vapidSetting stores generated keys as one JSON value, so concurrent first
// starts of several instances cannot end up with mismatched halves.
const vapidSetting = "vapid_keys"

// EnsureVAPIDKeys returns the instance's Web Push keys, generating and
// storing them on first use.
func (s *Service) EnsureVAPIDKeys(ctx context.Context) (auth.VAPIDKeys, error) {
	var keys auth.VAPIDKeys
	raw, err := s.store.GetSetting(ctx, vapidSetting)
	if errors.Is(err, domain.ErrNotFound) {
		fresh, gerr := auth.NewVAPIDKeys()
		if gerr != nil {
			return keys, gerr
		}
		enc, merr := json.Marshal(fresh)
		if merr != nil {
			return keys, merr
		}
		if err := s.store.InsertSettingIfAbsent(ctx, vapidSetting, string(enc), s.now()); err != nil {
			return keys, err
		}
		raw, err = s.store.GetSetting(ctx, vapidSetting) // another instance may have won
	}
	if err != nil {
		return keys, err
	}
	if err := json.Unmarshal([]byte(raw), &keys); err != nil {
		return keys, fmt.Errorf("decode stored VAPID keys: %w", err)
	}
	return keys, keys.Validate()
}

func requireSession(p *auth.Principal, what string) error {
	if !p.IsSession() {
		return domain.Forbidden("%s requires a login session", what)
	}
	return nil
}

// SubscribePushInput is a browser's PushSubscription plus settings.
type SubscribePushInput struct {
	Endpoint    string
	P256dh      string
	Auth        string
	Name        string
	MinPriority int // 0 = default (3)
}

// SubscribePush registers (or refreshes) the caller's browser for Web Push.
// A known endpoint is re-keyed and, if another user owned it, taken over.
func (s *Service) SubscribePush(ctx context.Context, p *auth.Principal, in SubscribePushInput) (*domain.PushSubscription, error) {
	if err := requireSession(p, "push subscriptions"); err != nil {
		return nil, err
	}
	if in.MinPriority == 0 {
		in.MinPriority = domain.PriorityDefault
	}
	sub := &domain.PushSubscription{
		ID: s.newID(), UserID: p.User.ID, Endpoint: in.Endpoint, P256dh: in.P256dh, Auth: in.Auth,
		Name: in.Name, MinPriority: in.MinPriority, CreatedAt: s.now(),
	}
	if err := domain.ValidatePushSubscription(sub); err != nil {
		return nil, err
	}
	err := s.store.InTx(ctx, func(q store.Querier) error {
		existing, err := q.ListPushSubscriptions(ctx, p.User.ID)
		if err != nil {
			return err
		}
		known := false
		for _, e := range existing {
			known = known || e.Endpoint == sub.Endpoint
		}
		if !known && len(existing) >= domain.MaxPushSubscriptions {
			return domain.Conflict("at most %d push subscriptions per user; remove an old device first", domain.MaxPushSubscriptions)
		}
		if err := q.UpsertPushSubscription(ctx, sub); err != nil {
			return err
		}
		sub, err = q.GetPushSubscription(ctx, p.User.ID, sub.ID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return sub, nil
}

// ListPushSubscriptions lists the caller's devices.
func (s *Service) ListPushSubscriptions(ctx context.Context, p *auth.Principal) ([]domain.PushSubscription, error) {
	if err := requireSession(p, "push subscriptions"); err != nil {
		return nil, err
	}
	return s.store.ListPushSubscriptions(ctx, p.User.ID)
}

// UpdatePushInput changes a subscription; nil fields are unchanged.
type UpdatePushInput struct {
	Name        *string
	MinPriority *int
}

// UpdatePushSubscription changes the name or priority threshold of a device.
func (s *Service) UpdatePushSubscription(ctx context.Context, p *auth.Principal, id string, in UpdatePushInput) (*domain.PushSubscription, error) {
	if err := requireSession(p, "push subscriptions"); err != nil {
		return nil, err
	}
	sub, err := s.store.GetPushSubscription(ctx, p.User.ID, id)
	if err != nil {
		return nil, err
	}
	if in.Name != nil {
		sub.Name = *in.Name
	}
	if in.MinPriority != nil {
		sub.MinPriority = *in.MinPriority
	}
	if err := domain.ValidatePushSubscription(sub); err != nil {
		return nil, err
	}
	return sub, s.store.UpdatePushSubscription(ctx, sub)
}

// DeletePushSubscription removes one of the caller's devices.
func (s *Service) DeletePushSubscription(ctx context.Context, p *auth.Principal, id string) error {
	if err := requireSession(p, "push subscriptions"); err != nil {
		return err
	}
	return s.store.DeletePushSubscription(ctx, p.User.ID, id)
}

// TestPushSubscription sends a test message to one of the caller's devices
// right away and reports the push service's verdict.
func (s *Service) TestPushSubscription(ctx context.Context, p *auth.Principal, id string) error {
	if err := requireSession(p, "push subscriptions"); err != nil {
		return err
	}
	if s.pushSender == nil {
		return domain.Conflict("Web Push is not enabled on this server")
	}
	sub, err := s.store.GetPushSubscription(ctx, p.User.ID, id)
	if err != nil {
		return err
	}
	return s.pushSender.SendTest(ctx, *sub)
}

// The methods below serve delivery workers, which act for the system rather
// than for a principal.

// PushTargets returns the user's subscriptions that want this priority.
func (s *Service) PushTargets(ctx context.Context, userID string, priority int) ([]domain.PushSubscription, error) {
	return s.store.ListPushTargets(ctx, userID, priority)
}

// PushDelivered records the outcome of a push to a subscription.
func (s *Service) PushDelivered(ctx context.Context, id string, ok bool) error {
	return s.store.RecordPushResult(ctx, id, ok, s.now())
}

// PushGone removes a subscription the push service no longer knows
// (HTTP 404/410: the user revoked permission or the browser dropped it).
func (s *Service) PushGone(ctx context.Context, id string) error {
	return s.store.DeletePushSubscriptionByID(ctx, id)
}
