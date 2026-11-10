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
	e.svc.AddNotifier(w, nil)
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
	// The default clients refuse loopback/private targets unless the server
	// settings allow them; the switch takes effect without a new worker.
	guarded := New(e.svc, Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	res = guarded.SendTest(context.Background(), *c.Webhook)
	if res.OK || !strings.Contains(res.Error, "allow private addresses") {
		t.Fatalf("private target: %+v", res)
	}
	e.settings(t, func(s *domain.ServerSettings) { s.Webhooks.AllowPrivate = true })
	if res := guarded.SendTest(context.Background(), *c.Webhook); !res.OK {
		t.Fatalf("allow private: %+v", res)
	}
}

func TestDisabledWebhooksWait(t *testing.T) {
	e := newEnv(t)
	rcv := newReceiver(t)
	c := e.hook(t, rcv.URL+"/ok", domain.WebhookAcksy)
	e.publish(t, "queued")
	e.settings(t, func(s *domain.ServerSettings) { s.Webhooks.Enabled = false })
	newWorker(e).drain(context.Background())
	if rcv.count() != 0 || len(e.deliveries(t, c.Webhook.ID)) != 1 {
		t.Fatalf("switched off: sent=%d queued=%d", rcv.count(), len(e.deliveries(t, c.Webhook.ID)))
	}
	e.publish(t, "not queued while off")
	if n := len(e.deliveries(t, c.Webhook.ID)); n != 1 {
		t.Fatalf("queued while off: %d", n)
	}
	e.settings(t, func(s *domain.ServerSettings) { s.Webhooks.Enabled = true })
	newWorker(e).drain(context.Background())
	if rcv.count() != 1 {
		t.Fatalf("back on: sent=%d", rcv.count())
	}
}

// settings changes the server settings as an admin in a login session.
func (e *env) settings(t *testing.T, mutate func(*domain.ServerSettings)) {
	t.Helper()
	admin := &auth.Principal{User: &domain.User{ID: "admin", Role: domain.RoleAdmin}, SessionID: "s", Scopes: domain.Scopes{domain.ScopeAdmin}}
	s, _, err := e.svc.ServerSettings(context.Background(), admin)
	if err != nil {
		t.Fatal(err)
	}
	mutate(&s)
	if _, _, err := e.svc.SaveServerSettings(context.Background(), admin, s); err != nil {
		t.Fatal(err)
	}
}

type telegramTransport func(*http.Request) (*http.Response, error)

func (f telegramTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestTelegramDelivery(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	endpoint := "https://api.telegram.org/bot123:test/sendMessage"
	hook := e.hook(t, endpoint+"?chat_id=-123", domain.WebhookTelegram).Webhook
	status, response := http.StatusOK, `{"ok":true}`
	w := New(e.svc, Config{Client: &http.Client{Transport: telegramTransport(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost || req.URL.String() != endpoint {
			t.Error("unexpected Telegram request target")
		}
		defer req.Body.Close()
		body, err := io.ReadAll(req.Body)
		if err != nil || !strings.Contains(string(body), `"chat_id":"-123"`) {
			t.Error("missing chat in request body")
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(response))}, nil
	})}}, slog.Default())
	if result := w.SendTest(ctx, *hook); !result.OK {
		t.Fatalf("test send: %+v", result)
	}
	status, response = http.StatusTooManyRequests, `{"ok":false,"parameters":{"retry_after":120}}`
	e.publish(t, "backup finished")
	w.drain(ctx)
	rows := e.deliveries(t, hook.ID)
	if len(rows) != 1 || rows[0].NextAttempt == nil || rows[0].NextAttempt.Sub(rows[0].CreatedAt) < 120*time.Second {
		t.Fatal("Telegram retry interval was not preserved")
	}
}
