package webpush

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oowliq/acksy/internal/auth"
	"github.com/oowliq/acksy/internal/domain"
)

type fakeTargets struct {
	mu        sync.Mutex
	subs      []domain.PushSubscription
	minPrio   int
	delivered map[string]bool
	gone      []string
}

func (f *fakeTargets) PushTargets(_ context.Context, _ string, prio int) ([]domain.PushSubscription, error) {
	if prio < f.minPrio {
		return nil, nil
	}
	return f.subs, nil
}

func (f *fakeTargets) PushDelivered(_ context.Context, id string, ok bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.delivered[id] = ok
	return nil
}

func (f *fakeTargets) PushGone(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gone = append(f.gone, id)
	return nil
}

// browserKeys returns a valid subscription key pair as a browser makes it.
func browserKeys(t *testing.T) (p256dh, authSecret string) {
	t.Helper()
	k, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	secret := make([]byte, 16)
	_, _ = rand.Read(secret)
	return base64.RawURLEncoding.EncodeToString(k.PublicKey().Bytes()), base64.RawURLEncoding.EncodeToString(secret)
}

type pushServer struct {
	*httptest.Server
	mu   sync.Mutex
	reqs []*http.Request
}

// newPushServer answers /ok with 201, /gone with 410 and /fail with 500.
func newPushServer(t *testing.T) *pushServer {
	t.Helper()
	ps := &pushServer{}
	ps.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		ps.mu.Lock()
		ps.reqs = append(ps.reqs, r)
		ps.mu.Unlock()
		switch {
		case strings.HasSuffix(r.URL.Path, "/ok"):
			w.WriteHeader(http.StatusCreated)
		case strings.HasSuffix(r.URL.Path, "/gone"):
			w.WriteHeader(http.StatusGone)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	t.Cleanup(ps.Close)
	return ps
}

func newDispatcher(t *testing.T, ft *fakeTargets) *Dispatcher {
	t.Helper()
	keys, err := auth.NewVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	// The push "service" listens on loopback, so use a plain client here;
	// the default netguard client is covered by its own tests.
	return New(ft, Config{Keys: keys, Subject: func(context.Context) string { return "https://acksy.example" }, Client: http.DefaultClient},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestDeliver(t *testing.T) {
	ps := newPushServer(t)
	p256dh, secret := browserKeys(t)
	sub := func(id, path string) domain.PushSubscription {
		return domain.PushSubscription{ID: id, Endpoint: ps.URL + path, P256dh: p256dh, Auth: secret}
	}
	ft := &fakeTargets{
		delivered: map[string]bool{},
		subs:      []domain.PushSubscription{sub("s-ok", "/ok"), sub("s-gone", "/gone"), sub("s-fail", "/fail")},
	}
	d := newDispatcher(t, ft)

	d.deliver(context.Background(), domain.Notification{ID: "n1", UserID: "u", Title: "Disk", Body: "**full**", Priority: 5})

	if len(ps.reqs) != 3 {
		t.Fatalf("requests: %d", len(ps.reqs))
	}
	r := ps.reqs[0]
	if r.Method != http.MethodPost || r.Header.Get("Content-Encoding") != "aes128gcm" ||
		r.Header.Get("Urgency") != "high" || r.Header.Get("TTL") != "86400" || len(r.Header.Get("Topic")) != 32 ||
		!strings.HasPrefix(r.Header.Get("Authorization"), "vapid t=") {
		t.Fatalf("headers: %v", r.Header)
	}
	if ok, seen := ft.delivered["s-ok"]; !seen || !ok {
		t.Fatalf("success not recorded: %v", ft.delivered)
	}
	if ok, seen := ft.delivered["s-fail"]; !seen || ok {
		t.Fatalf("failure not recorded: %v", ft.delivered)
	}
	if _, seen := ft.delivered["s-gone"]; seen || len(ft.gone) != 1 || ft.gone[0] != "s-gone" {
		t.Fatalf("gone subscription: delivered=%v gone=%v", ft.delivered, ft.gone)
	}

	// Below every device's threshold: nothing is sent.
	ft.minPrio = 4
	ps.reqs = nil
	d.deliver(context.Background(), domain.Notification{ID: "n2", UserID: "u", Body: "meh", Priority: 2})
	if len(ps.reqs) != 0 {
		t.Fatalf("low priority pushed: %d", len(ps.reqs))
	}
}

func TestRunAndSendTest(t *testing.T) {
	ps := newPushServer(t)
	p256dh, secret := browserKeys(t)
	ok := domain.PushSubscription{ID: "ok", Endpoint: ps.URL + "/ok", P256dh: p256dh, Auth: secret}
	ft := &fakeTargets{delivered: map[string]bool{}, subs: []domain.PushSubscription{ok}}
	d := newDispatcher(t, ft)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { d.Run(ctx); close(done) }()
	d.Notify([]domain.Notification{{ID: "n", UserID: "u", Body: "x", Priority: 3}})
	deadline := time.Now().Add(5 * time.Second)
	for {
		ft.mu.Lock()
		_, seen := ft.delivered["ok"]
		ft.mu.Unlock()
		if seen {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("queued push not delivered")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop on cancel")
	}

	if err := d.SendTest(context.Background(), ok); err != nil {
		t.Fatalf("test push: %v", err)
	}
	gone := ok
	gone.Endpoint = ps.URL + "/gone"
	if err := d.SendTest(context.Background(), gone); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("gone: %v", err)
	}
	fail := ok
	fail.Endpoint = ps.URL + "/fail"
	if err := d.SendTest(context.Background(), fail); !errors.Is(err, domain.ErrConflict) || !strings.Contains(err.Error(), "500") {
		t.Fatalf("fail: %v", err)
	}
}

func TestPayloadFor(t *testing.T) {
	long := strings.Repeat("word ", 100)
	p := PayloadFor(domain.Notification{
		ID: "n1", Body: "**Deploy** done\n\n" + long, Priority: 4, DedupKey: "deploy", RepeatCount: 2,
		Topic: &domain.TopicSummary{ID: "t1", Name: "ci"}, UpdatedAt: time.UnixMilli(1_800_000_000_000),
	})
	if p.Title != "ci" || !strings.HasPrefix(p.Body, "Deploy done\nword") || len([]rune(p.Body)) != previewLen ||
		!strings.HasSuffix(p.Body, "…") || p.Tag != "acksy:dedup:t1:deploy" || !p.Renotify || p.URL != "/?n=n1" ||
		p.Timestamp != 1_800_000_000_000 {
		t.Fatalf("payload: %+v", p)
	}
	if p := PayloadFor(domain.Notification{ID: "n2", Body: "x"}); p.Title != "Acksy" || p.Tag != "acksy:n2" || p.Renotify {
		t.Fatalf("fallbacks: %+v", p)
	}
}

func TestOptions(t *testing.T) {
	d := newDispatcher(t, &fakeTargets{})
	fixed := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	d.now = func() time.Time { return fixed }
	soon := fixed.Add(10 * time.Minute)
	o := d.options(domain.Notification{ID: "a", Priority: 1, ExpiresAt: &soon})
	if o.ttl != 600 || o.urgency != "low" {
		t.Fatalf("expiring low: %+v", o)
	}
	a := d.options(domain.Notification{ID: "x", DedupKey: "k"})
	b := d.options(domain.Notification{ID: "y", DedupKey: "k"})
	if a.topic != b.topic || a.topic == d.options(domain.Notification{ID: "x"}).topic {
		t.Fatal("repeats of one alert must share the push topic, distinct alerts must not")
	}
}
