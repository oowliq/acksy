// Package webpush delivers notifications to browsers with Web Push
// (RFC 8030, payload encryption RFC 8291, VAPID RFC 8292).
//
// The service calls Notify after a publish commits; Notify only queues.
// Workers started by Run look up the recipient's subscriptions whose
// min_priority admits the notification and send to each. Delivery is
// best effort: a full queue drops work, failed sends are counted on the
// subscription, and subscriptions the push service reports as gone are
// removed. Clients recover state through the API on open.
package webpush

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	wp "github.com/SherClockHolmes/webpush-go"

	"github.com/oowliq/acksy/internal/auth"
	"github.com/oowliq/acksy/internal/domain"
	"github.com/oowliq/acksy/internal/markdown"
	"github.com/oowliq/acksy/internal/netguard"
)

// Targets is what the dispatcher needs from the service.
type Targets interface {
	PushTargets(ctx context.Context, userID string, priority int) ([]domain.PushSubscription, error)
	PushDelivered(ctx context.Context, id string, ok bool) error
	PushGone(ctx context.Context, id string) error
}

// Config tunes the dispatcher.
type Config struct {
	Keys auth.VAPIDKeys
	// Subject returns the VAPID "sub": an https URL or an e-mail address
	// (from the server settings). Empty falls back to acksy@localhost.
	Subject   func(context.Context) string
	Workers   int           // default 4
	QueueSize int           // default 1024
	TTL       time.Duration // how long push services keep undelivered messages (default 24h)
	// Client sends to push services; default is a netguard client that
	// refuses private addresses (endpoints are user-supplied URLs).
	Client *http.Client
}

// Dispatcher queues and sends pushes.
type Dispatcher struct {
	targets Targets
	cfg     Config
	log     *slog.Logger
	queue   chan domain.Notification
	now     func() time.Time
}

// New builds a dispatcher; call Run to start its workers.
func New(t Targets, cfg Config, log *slog.Logger) *Dispatcher {
	if cfg.Workers <= 0 {
		cfg.Workers = 4
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 1024
	}
	if cfg.TTL <= 0 {
		cfg.TTL = 24 * time.Hour
	}
	if cfg.Client == nil {
		cfg.Client = netguard.NewClient(netguard.Options{})
	}
	return &Dispatcher{targets: t, cfg: cfg, log: log, queue: make(chan domain.Notification, cfg.QueueSize), now: time.Now}
}

// Notify queues notifications for delivery without blocking.
func (d *Dispatcher) Notify(ns []domain.Notification) {
	for _, n := range ns {
		select {
		case d.queue <- n:
		default:
			d.log.Warn("web push queue full, dropping", "notification_id", n.ID, "user_id", n.UserID)
		}
	}
}

// Run sends queued pushes until ctx is cancelled; it returns after its
// workers have stopped. Pending work is dropped on shutdown.
func (d *Dispatcher) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for range d.cfg.Workers {
		wg.Go(func() {
			for {
				select {
				case <-ctx.Done():
					return
				case n := <-d.queue:
					d.deliver(ctx, n)
				}
			}
		})
	}
	wg.Wait()
}

func (d *Dispatcher) deliver(ctx context.Context, n domain.Notification) {
	subs, err := d.targets.PushTargets(ctx, n.UserID, n.Priority)
	if err != nil {
		d.log.Warn("web push targets", "user_id", n.UserID, "err", err)
		return
	}
	if len(subs) == 0 {
		return
	}
	payload, err := json.Marshal(PayloadFor(n))
	if err != nil {
		d.log.Error("web push payload", "err", err)
		return
	}
	opts := d.options(n)
	for _, s := range subs {
		_, _ = d.send(ctx, s, payload, opts)
	}
}

// options derives per-message headers: urgency from priority, a topic that
// collapses undelivered repeats of the same alert, and a TTL that never
// outlives expires_at.
func (d *Dispatcher) options(n domain.Notification) messageOptions {
	ttl := d.cfg.TTL
	if n.ExpiresAt != nil {
		ttl = min(ttl, n.ExpiresAt.Sub(d.now()))
	}
	sum := sha256.Sum256([]byte(tagFor(n)))
	return messageOptions{
		urgency: urgencyFor(n.Priority),
		topic:   base64.RawURLEncoding.EncodeToString(sum[:])[:32],
		ttl:     max(int(ttl.Seconds()), 0),
	}
}

type messageOptions struct {
	urgency wp.Urgency
	topic   string
	ttl     int
}

func urgencyFor(priority int) wp.Urgency {
	switch {
	case priority >= 4:
		return wp.UrgencyHigh
	case priority == 3:
		return wp.UrgencyNormal
	default:
		return wp.UrgencyLow
	}
}

func (d *Dispatcher) subject(ctx context.Context) string {
	if d.cfg.Subject != nil {
		if s := d.cfg.Subject(ctx); s != "" {
			return s
		}
	}
	return "acksy@localhost"
}

// outcome of one send.
type outcome int

const (
	delivered outcome = iota
	gone
	failed
)

// send pushes to one subscription and records the result.
func (d *Dispatcher) send(ctx context.Context, s domain.PushSubscription, payload []byte, o messageOptions) (outcome, int) {
	resp, err := wp.SendNotificationWithContext(ctx, payload,
		&wp.Subscription{Endpoint: s.Endpoint, Keys: wp.Keys{P256dh: s.P256dh, Auth: s.Auth}},
		&wp.Options{
			HTTPClient: d.cfg.Client, Subscriber: d.subject(ctx),
			VAPIDPublicKey: d.cfg.Keys.Public, VAPIDPrivateKey: d.cfg.Keys.Private,
			TTL: o.ttl, Urgency: o.urgency, Topic: o.topic,
		})
	status := 0
	if resp != nil {
		status = resp.StatusCode
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
	}
	// The endpoint is a secret capability URL: log only the push service.
	logAttrs := []any{"subscription_id", s.ID, "push_service", s.Service, "status", status}
	var out outcome
	switch {
	case err == nil && status >= 200 && status < 300:
		out = delivered
	case err == nil && (status == http.StatusNotFound || status == http.StatusGone):
		out = gone
		d.log.Info("web push subscription gone, removing", logAttrs...)
		if err := d.targets.PushGone(ctx, s.ID); err != nil {
			d.log.Warn("remove push subscription", "subscription_id", s.ID, "err", err)
		}
		return out, status
	default:
		out = failed
		d.log.Warn("web push failed", append(logAttrs, "err", err)...)
	}
	if err := d.targets.PushDelivered(ctx, s.ID, out == delivered); err != nil {
		d.log.Warn("record push result", "subscription_id", s.ID, "err", err)
	}
	return out, status
}

// SendTest pushes a test message to one subscription synchronously.
func (d *Dispatcher) SendTest(ctx context.Context, s domain.PushSubscription) error {
	payload, err := json.Marshal(Payload{
		ID: "test", Title: "Acksy", Body: "Push notifications work on this device.",
		Priority: domain.PriorityDefault, Tag: "acksy:test", URL: "/", Timestamp: d.now().UnixMilli(),
	})
	if err != nil {
		return err
	}
	switch out, status := d.send(ctx, s, payload, messageOptions{urgency: wp.UrgencyHigh, ttl: 60}); out {
	case delivered:
		return nil
	case gone:
		return domain.NotFound("push subscription (the browser revoked it; enable notifications again)")
	default:
		if status == 0 {
			return domain.Conflict("could not reach the push service")
		}
		return domain.Conflict("the push service rejected the message (HTTP %d)", status)
	}
}

// Payload is the JSON the service worker receives.
type Payload struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Body     string `json:"body"`
	Topic    string `json:"topic,omitempty"`
	Priority int    `json:"priority"`
	// Tag groups OS notifications: a repeat (same dedup key) replaces the
	// previous one instead of stacking.
	Tag      string `json:"tag"`
	Renotify bool   `json:"renotify"`
	// URL opens the notification in the app (relative to the app origin).
	URL       string `json:"url"`
	Timestamp int64  `json:"ts"`
}

// previewLen bounds the body preview; push payloads must stay under ~4 KiB.
const previewLen = 200

// PayloadFor builds the push payload: title, a plain-text preview of the
// markdown body, and enough to route a click.
func PayloadFor(n domain.Notification) Payload {
	p := Payload{
		ID: n.ID, Title: n.Title, Body: markdown.Truncate(markdown.Plain(n.Body), previewLen),
		Priority: n.Priority, Tag: tagFor(n), Renotify: n.RepeatCount > 1,
		URL: "/?n=" + n.ID, Timestamp: n.UpdatedAt.UnixMilli(),
	}
	if n.Topic != nil {
		p.Topic = n.Topic.Name
	}
	if p.Title == "" {
		p.Title = p.Topic
	}
	if p.Title == "" {
		p.Title = "Acksy"
	}
	return p
}

func tagFor(n domain.Notification) string {
	if n.DedupKey != "" {
		// Keys are scoped to the topic, so the tag is too: a repeat in one
		// topic must not replace an alert from another.
		if n.Topic != nil {
			return "acksy:dedup:" + n.Topic.ID + ":" + n.DedupKey
		}
		return "acksy:dedup:" + n.DedupKey
	}
	return "acksy:" + n.ID
}

// String helps logs and tests.
func (o outcome) String() string {
	return [...]string{"delivered", "gone", "failed"}[o]
}

var _ fmt.Stringer = outcome(0)
