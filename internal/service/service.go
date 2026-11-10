// Package service implements Acksy use-cases. Every entry point takes the
// authenticated *auth.Principal and enforces scopes and ownership itself, so
// all transports (REST, adapters, CLI) share the same rules.
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/oowliq/acksy/internal/auth"
	"github.com/oowliq/acksy/internal/broker"
	"github.com/oowliq/acksy/internal/domain"
	"github.com/oowliq/acksy/internal/store"
)

// Options tune service behaviour.
type Options struct {
	// StreamCheckInterval is how often realtime streams re-validate
	// credentials and poll the database for missed events (default 30s).
	StreamCheckInterval time.Duration
	// NewOIDCProvider builds the provider for active OIDC settings; nil
	// leaves OIDC sign-in unavailable. Settings live in the database (ADR 0006).
	NewOIDCProvider func(domain.OIDCSettings) OIDCProvider
	// OIDCRedirectURL is the callback registered at the provider (from
	// ACKSY_BASE_URL); OIDC can't be turned on without it.
	OIDCRedirectURL string
	// SignInTTL bounds how stale another instance's sign-in and server
	// settings changes can be here (default DefaultSignInTTL).
	SignInTTL time.Duration
}

// Service is the application core.
type Service struct {
	store  store.Store
	broker broker.Broker
	log    *slog.Logger
	opts   Options

	now   func() time.Time
	newID func() string

	publishLimit atomic.Pointer[publishLimiters]

	notifiers     []gatedNotifier
	pushSender    PushSender
	webhookSender WebhookSender

	actionRunner   ActionRunner
	actionLimit    *auth.RateLimiter
	actionsRunning actionsInFlight

	signIn   atomic.Pointer[SignInSnapshot]
	signInMu sync.Mutex

	server   atomic.Pointer[serverSnapshot]
	serverMu sync.Mutex
}

// New builds a Service.
func New(st store.Store, b broker.Broker, log *slog.Logger, opts Options) *Service {
	if opts.SignInTTL <= 0 {
		opts.SignInTTL = DefaultSignInTTL
	}
	return &Service{
		store: st, broker: b, log: log, opts: opts,
		now:   func() time.Time { return time.Now().UTC().Truncate(time.Millisecond) },
		newID: func() string { return ulid.Make().String() },

		actionLimit: auth.NewRateLimiter(actionLimitPerMinute, actionLimitPerMinute/3),
	}
}

// emitter records events inside a transaction for publishing after commit.
type emitter struct {
	q      store.Querier
	now    time.Time
	events []domain.Event
}

func (e *emitter) emit(ctx context.Context, userID string, typ domain.EventType, data any) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("encode event: %w", err)
	}
	seq, err := e.q.NextEventSeq(ctx, userID)
	if err != nil {
		return err
	}
	ev := domain.Event{UserID: userID, Seq: seq, Type: typ, Data: raw, CreatedAt: e.now}
	if err := e.q.InsertEvent(ctx, &ev); err != nil {
		return err
	}
	e.events = append(e.events, ev)
	return nil
}

// tx runs fn in a transaction and, after commit, fans its events out through
// the broker (invariant: persist first, then publish).
func (s *Service) tx(ctx context.Context, fn func(q store.Querier, em *emitter) error) error {
	em := &emitter{now: s.now()}
	err := s.store.InTx(ctx, func(q store.Querier) error {
		em.q, em.events = q, em.events[:0]
		return fn(q, em)
	})
	if err != nil {
		return err
	}
	s.publish(ctx, em.events)
	return nil
}

func (s *Service) publish(ctx context.Context, events []domain.Event) {
	for i := range events {
		ev := &events[i]
		payload, err := json.Marshal(ev)
		if err != nil {
			s.log.Error("encode event", "err", err)
			continue
		}
		// Best effort: clients recover missed events via ?since=<seq>.
		if err := s.broker.Publish(context.WithoutCancel(ctx), broker.UserChannel(ev.UserID), payload); err != nil {
			s.log.Warn("publish event", "user_id", ev.UserID, "seq", ev.Seq, "err", err)
		}
	}
}
