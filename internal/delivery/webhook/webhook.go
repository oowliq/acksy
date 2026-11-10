// Package webhook delivers queued webhook requests (see ADR 0003).
//
// Deliveries are written to the database in the publish transaction. The
// Worker picks up due ones, claims each atomically (safe with several
// instances), sends it and records the outcome: success deletes the row,
// failure schedules a retry or, after the last attempt or a permanent
// error, marks it failed. Publishing wakes the worker; it also polls, so
// retries and other instances' work are picked up.
package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/oowliq/acksy/internal/buildinfo"
	"github.com/oowliq/acksy/internal/domain"
	"github.com/oowliq/acksy/internal/netguard"
	"github.com/oowliq/acksy/internal/service"
)

// Queue is what the worker needs from the service.
type Queue interface {
	DueWebhookDeliveries(ctx context.Context, limit int) ([]domain.WebhookDelivery, error)
	ClaimWebhookDelivery(ctx context.Context, id string, lease time.Duration) (bool, error)
	WebhookByID(ctx context.Context, id string) (*domain.Webhook, error)
	WebhookDelivered(ctx context.Context, d *domain.WebhookDelivery) error
	WebhookDropped(ctx context.Context, d *domain.WebhookDelivery) error
	WebhookAttemptFailed(ctx context.Context, d *domain.WebhookDelivery, status int, errMsg string, permanent bool, retryAfter time.Duration) error
	// Settings are the server settings in effect: they switch delivery on
	// and off and allow private destinations.
	Settings(ctx context.Context) domain.ServerSettings
}

// Config tunes the worker.
type Config struct {
	// BaseURL builds links back to the app in payloads (optional).
	BaseURL string
	Workers int           // concurrent requests (default 4)
	Poll    time.Duration // queue poll interval (default 5s)
	Lease   time.Duration // how long a claimed delivery stays reserved (default 2m)
	// Client overrides the HTTP client for every request (tests). By default
	// netguard clients are used, one of them allowing private and loopback
	// targets (LAN services such as Home Assistant) when the server settings
	// say so; off by default to prevent SSRF.
	Client *http.Client
}

// Worker sends queued deliveries.
type Worker struct {
	q       Queue
	cfg     Config
	private *http.Client // used when the settings allow private targets
	log     *slog.Logger
	wake    chan struct{}
	now     func() time.Time
}

// batch bounds how many due deliveries one pass reads.
const batch = 100

// New builds a worker; call Run to start it.
func New(q Queue, cfg Config, log *slog.Logger) *Worker {
	if cfg.Workers <= 0 {
		cfg.Workers = 4
	}
	if cfg.Poll <= 0 {
		cfg.Poll = 5 * time.Second
	}
	if cfg.Lease <= 0 {
		cfg.Lease = 2 * time.Minute
	}
	private := cfg.Client
	if cfg.Client == nil {
		cfg.Client = netguard.NewClient(netguard.Options{Timeout: 30 * time.Second})
		private = netguard.NewClient(netguard.Options{AllowPrivate: true, Timeout: 30 * time.Second})
	}
	return &Worker{q: q, cfg: cfg, private: private, log: log, wake: make(chan struct{}, 1), now: time.Now}
}

// Notify implements service.Notifier: new deliveries were committed, so
// look at the queue now rather than at the next poll.
func (w *Worker) Notify([]domain.Notification) {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

// Run processes the queue until ctx is cancelled and returns after
// in-flight requests finish. Unsent deliveries stay queued.
func (w *Worker) Run(ctx context.Context) {
	tick := time.NewTicker(w.cfg.Poll)
	defer tick.Stop()
	for {
		w.drain(ctx)
		select {
		case <-ctx.Done():
			return
		case <-w.wake:
		case <-tick.C:
		}
	}
}

// drain sends every due delivery, a batch at a time.
func (w *Worker) drain(ctx context.Context) {
	if !w.q.Settings(ctx).Webhooks.Enabled {
		return // switched off: deliveries stay queued until it is back on
	}
	for ctx.Err() == nil {
		due, err := w.q.DueWebhookDeliveries(ctx, batch)
		if err != nil {
			w.log.Warn("webhook queue", "err", err)
			return
		}
		if len(due) == 0 {
			return
		}
		sem := make(chan struct{}, w.cfg.Workers)
		var wg sync.WaitGroup
		for i := range due {
			d := due[i]
			ok, err := w.q.ClaimWebhookDelivery(ctx, d.ID, w.cfg.Lease)
			if err != nil || !ok {
				continue // another worker has it
			}
			d.Attempts++
			sem <- struct{}{}
			wg.Go(func() {
				defer func() { <-sem }()
				w.process(ctx, &d)
			})
		}
		wg.Wait()
		if len(due) < batch {
			return
		}
	}
}

// outcome of one attempt.
type outcome struct {
	status     int
	err        string
	permanent  bool
	retryAfter time.Duration
}

func (o outcome) ok() bool { return o.err == "" }

func (w *Worker) process(ctx context.Context, d *domain.WebhookDelivery) {
	hook, err := w.q.WebhookByID(ctx, d.WebhookID)
	if err != nil {
		w.log.Warn("webhook lookup", "webhook_id", d.WebhookID, "err", err)
		return // lease expires; retried later
	}
	if hook == nil {
		return // deleted; its deliveries went with it
	}
	if !hook.Enabled {
		if err := w.q.WebhookDropped(ctx, d); err != nil {
			w.log.Warn("drop webhook delivery", "delivery_id", d.ID, "err", err)
		}
		return
	}
	var n domain.Notification
	if err := json.Unmarshal([]byte(d.Payload), &n); err != nil {
		w.finish(ctx, d, hook, outcome{err: "corrupt payload", permanent: true})
		return
	}
	w.finish(ctx, d, hook, w.attempt(ctx, hook, d.Event, d.ID, &n))
}

func (w *Worker) finish(ctx context.Context, d *domain.WebhookDelivery, hook *domain.Webhook, o outcome) {
	var err error
	if o.ok() {
		err = w.q.WebhookDelivered(ctx, d)
	} else {
		w.log.Info("webhook attempt failed", "webhook_id", hook.ID, "delivery_id", d.ID, "attempt", d.Attempts,
			"status", o.status, "err", o.err, "permanent", o.permanent)
		err = w.q.WebhookAttemptFailed(ctx, d, o.status, o.err, o.permanent, o.retryAfter)
	}
	if err != nil {
		w.log.Warn("record webhook result", "delivery_id", d.ID, "err", err)
	}
}

// attempt sends one request and classifies the result.
func (w *Worker) attempt(ctx context.Context, hook *domain.Webhook, event domain.WebhookEvent, deliveryID string, n *domain.Notification) outcome {
	req, err := Render(hook, event, deliveryID, n, w.cfg.BaseURL, w.now())
	if err != nil {
		return outcome{err: err.Error(), permanent: true}
	}
	hr, err := http.NewRequestWithContext(ctx, http.MethodPost, hook.URL, bytes.NewReader(req.Body))
	if err != nil {
		return outcome{err: "invalid URL", permanent: true}
	}
	for k, v := range req.Headers {
		hr.Header.Set(k, v)
	}
	hr.Header.Set("User-Agent", "Acksy-Webhook/"+buildinfo.Version)

	client := w.cfg.Client
	if w.q.Settings(ctx).Webhooks.AllowPrivate {
		client = w.private
	}
	resp, err := client.Do(hr)
	if err != nil {
		if netguard.Blocked(err) {
			return outcome{err: "destination is a private address; allow private addresses for webhooks in the server settings to reach LAN targets", permanent: true}
		}
		return outcome{err: netguard.Describe(err)}
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64*1024))

	o := outcome{status: resp.StatusCode}
	switch s := resp.StatusCode; {
	case s >= 200 && s < 300:
		return o
	case s == http.StatusRequestTimeout || s == http.StatusTooEarly || s == http.StatusTooManyRequests || s >= 500:
		o.retryAfter = parseRetryAfter(resp.Header.Get("Retry-After"), w.now())
	default:
		o.permanent = true // other 4xx: the receiver will not accept it on retry
	}
	o.err = "HTTP " + strconv.Itoa(resp.StatusCode)
	return o
}

func parseRetryAfter(v string, now time.Time) time.Duration {
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil && t.After(now) {
		return t.Sub(now)
	}
	return 0
}

// SendTest implements service.WebhookSender: one request with a sample
// notification, no queue and no retries.
func (w *Worker) SendTest(ctx context.Context, hook domain.Webhook) service.WebhookTestResult {
	now := w.now()
	sample := &domain.Notification{
		ID: "test", Title: "Test from Acksy", Body: "If you can read this, the webhook **works**.",
		Priority: domain.PriorityDefault, Tags: []string{}, Actions: []domain.Action{}, Source: "acksy",
		CreatedAt: now, UpdatedAt: now,
	}
	o := w.attempt(ctx, &hook, domain.WebhookTest, fmt.Sprintf("test-%d", now.UnixMilli()), sample)
	return service.WebhookTestResult{Status: o.status, OK: o.ok(), Error: o.err}
}
