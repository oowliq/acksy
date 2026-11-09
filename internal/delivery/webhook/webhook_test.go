package webhook

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oowliq/acksy/internal/auth"
	"github.com/oowliq/acksy/internal/broker/memory"
	"github.com/oowliq/acksy/internal/domain"
	"github.com/oowliq/acksy/internal/service"
	"github.com/oowliq/acksy/internal/store"
)

type received struct {
	headers http.Header
	body    string
}

// receiver answers with the status for its path: /ok 204, /busy 503, /bad 400.
type receiver struct {
	*httptest.Server
	mu  sync.Mutex
	got []received
}

func newReceiver(t *testing.T) *receiver {
	t.Helper()
	r := &receiver{}
	r.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		r.mu.Lock()
		r.got = append(r.got, received{req.Header.Clone(), string(body)})
		r.mu.Unlock()
		switch {
		case strings.HasSuffix(req.URL.Path, "/ok"):
			w.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(req.URL.Path, "/busy"):
			w.Header().Set("Retry-After", "120")
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	t.Cleanup(r.Close)
	return r
}

func (r *receiver) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.got)
}

type env struct {
	svc   *service.Service
	user  *auth.Principal
	queue store.Store
}

func newEnv(t *testing.T) *env {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, "sqlite://:memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	b := memory.New()
	t.Cleanup(func() { _ = b.Close() })
	st := store.New(db)
	svc := service.New(st, b, slog.New(slog.NewTextHandler(io.Discard, nil)), service.Options{})
	u, err := svc.CreateUser(ctx, service.SystemPrincipal(), service.CreateUserInput{Username: "alice", Password: "password123"})
	if err != nil {
		t.Fatal(err)
	}
	return &env{svc: svc, user: service.UserPrincipal(u), queue: st}
}

func (e *env) hook(t *testing.T, url string, f domain.WebhookFormat) *service.CreatedWebhook {
	t.Helper()
	c, err := e.svc.CreateWebhook(context.Background(), e.user, service.WebhookInput{Name: &url, URL: &url, Format: &f})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (e *env) publish(t *testing.T, body string) {
	t.Helper()
	if _, err := e.svc.Publish(context.Background(), e.user, domain.Message{Title: "t", Body: body}); err != nil {
		t.Fatal(err)
	}
}

func (e *env) deliveries(t *testing.T, hookID string) []domain.WebhookDelivery {
	t.Helper()
	d, err := e.queue.ListWebhookDeliveries(context.Background(), hookID, 10)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func newWorker(e *env) *Worker {
	return New(e.svc, Config{BaseURL: "https://acksy.example", Client: http.DefaultClient, Poll: time.Hour},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestDeliveryOutcomes(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	rcv := newReceiver(t)
	ok := e.hook(t, rcv.URL+"/ok", domain.WebhookAcksy)
	busy := e.hook(t, rcv.URL+"/busy", domain.WebhookSlack)
	bad := e.hook(t, rcv.URL+"/bad", domain.WebhookDiscord)
	e.publish(t, "**disk** full")

	newWorker(e).drain(ctx)

	if rcv.count() != 3 {
		t.Fatalf("requests: %d", rcv.count())
	}
	// The receiver can verify the signature with the secret it was given.
	for _, r := range rcv.got {
		if r.headers.Get("X-Acksy-Event") != "notification.created" {
			continue
		}
		if strings.Contains(r.body, `"delivery_id"`) {
			sig := Sign(ok.Secret, r.headers.Get("X-Acksy-Timestamp"), []byte(r.body))
			if r.headers.Get("X-Acksy-Signature") != sig || !strings.HasPrefix(r.headers.Get("User-Agent"), "Acksy-Webhook/") {
				t.Fatalf("signature or UA: %v", r.headers)
			}
		}
	}
	if d := e.deliveries(t, ok.Webhook.ID); len(d) != 0 {
		t.Fatalf("delivered request still queued: %+v", d)
	}
	h, _ := e.svc.GetWebhook(ctx, e.user, ok.Webhook.ID)
	if h.LastSuccess == nil {
		t.Fatal("success not recorded")
	}

	// 503 with Retry-After: rescheduled, not failed.
	d := e.deliveries(t, busy.Webhook.ID)
	if len(d) != 1 || d[0].Attempts != 1 || d[0].LastStatus != 503 || d[0].FailedAt != nil || d[0].NextAttempt == nil ||
		d[0].NextAttempt.Before(time.Now().Add(110*time.Second)) {
		t.Fatalf("retry: %+v", d)
	}
	// 400: failed for good, no further attempts.
	d = e.deliveries(t, bad.Webhook.ID)
	if len(d) != 1 || d[0].FailedAt == nil || d[0].NextAttempt != nil || d[0].LastError != "HTTP 400" {
		t.Fatalf("permanent: %+v", d)
	}
	// Nothing is due any more.
	if due, _ := e.svc.DueWebhookDeliveries(ctx, 10); len(due) != 0 {
		t.Fatalf("still due: %+v", due)
	}
}

func TestDisabledAfterQueueingIsDropped(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	rcv := newReceiver(t)
	c := e.hook(t, rcv.URL+"/ok", domain.WebhookAcksy)
	e.publish(t, "x")
	off := false
	if _, err := e.svc.UpdateWebhook(ctx, e.user, c.Webhook.ID, service.WebhookInput{Enabled: &off}); err != nil {
		t.Fatal(err)
	}
	newWorker(e).drain(ctx)
	if rcv.count() != 0 || len(e.deliveries(t, c.Webhook.ID)) != 0 {
		t.Fatalf("disabled webhook: sent=%d queued=%d", rcv.count(), len(e.deliveries(t, c.Webhook.ID)))
	}
}

func TestRunWakesOnNotify(t *testing.T) {
	e := newEnv(t)
	rcv := newReceiver(t)
	e.hook(t, rcv.URL+"/ok", domain.WebhookAcksy)
	w := newWorker(e) // polls hourly: only Notify can wake it in time
	e.svc.AddNotifier(w)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { w.Run(ctx); close(done) }()

	e.publish(t, "wake up")
	deadline := time.Now().Add(5 * time.Second)
	for rcv.count() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("publish did not wake the worker")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop")
	}
}

func TestSendTestAndPrivateTargets(t *testing.T) {
	e := newEnv(t)
	rcv := newReceiver(t)
	c := e.hook(t, rcv.URL+"/ok", domain.WebhookAcksy)

	res := newWorker(e).SendTest(context.Background(), *c.Webhook)
	if !res.OK || res.Status != http.StatusNoContent || rcv.got[0].headers.Get("X-Acksy-Event") != "webhook.test" {
		t.Fatalf("test: %+v", res)
	}
	// The default client refuses loopback/private targets unless allowed.
	guarded := New(e.svc, Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	res = guarded.SendTest(context.Background(), *c.Webhook)
	if res.OK || !strings.Contains(res.Error, "ACKSY_WEBHOOK_ALLOW_PRIVATE") {
		t.Fatalf("private target: %+v", res)
	}
	allowed := New(e.svc, Config{AllowPrivate: true}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if res := allowed.SendTest(context.Background(), *c.Webhook); !res.OK {
		t.Fatalf("allow private: %+v", res)
	}
}
