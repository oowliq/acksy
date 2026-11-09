package app

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oowliq/acksy/internal/config"
)

// End-to-end wiring: config -> SQLite + migrations -> memory broker -> router.
func TestNewServesHealthAndAPI(t *testing.T) {
	cfg := config.Default()
	cfg.DatabaseURL = "sqlite://:memory:"

	a, err := New(context.Background(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Close)

	for _, tc := range []struct {
		path   string
		status int
		body   string
	}{
		{"/healthz", http.StatusOK, `"ok"`},
		{"/readyz", http.StatusOK, `"database":"ok"`},
		{"/api/v1/version", http.StatusOK, `"version"`},
	} {
		rec := httptest.NewRecorder()
		a.server.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if rec.Code != tc.status || !strings.Contains(rec.Body.String(), tc.body) {
			t.Errorf("%s: %d %s", tc.path, rec.Code, rec.Body)
		}
	}
}

func TestNewFailsOnBadDatabase(t *testing.T) {
	cfg := config.Default()
	cfg.DatabaseURL = "mysql://nope"
	if _, err := New(context.Background(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil))); err == nil {
		t.Fatal("expected error")
	}
}

// pipeListener serves HTTP over in-memory connections (no port binding).
type pipeListener struct {
	conns chan net.Conn
	done  chan struct{}
	once  sync.Once
}

func (l *pipeListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.conns:
		return c, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}

func (l *pipeListener) Close() error   { l.once.Do(func() { close(l.done) }); return nil }
func (l *pipeListener) Addr() net.Addr { return &net.TCPAddr{} }

func (l *pipeListener) dial(ctx context.Context, _, _ string) (net.Conn, error) {
	s, c := net.Pipe()
	select {
	case l.conns <- s:
		return c, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Shutdown must not wait for open event streams until the timeout.
func TestShutdownEndsStreams(t *testing.T) {
	cfg := config.Default()
	cfg.DatabaseURL = "sqlite://:memory:"
	cfg.ShutdownTimeout = 10 * time.Second
	cfg.Admin = config.AdminConfig{Username: "admin", Password: "password123"}

	a, err := New(context.Background(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Close)

	secret, _, err := a.Service().Login(context.Background(), "admin", "password123", "")
	if err != nil {
		t.Fatal(err)
	}

	l := &pipeListener{conns: make(chan net.Conn), done: make(chan struct{})}
	ctx, stop := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() { served <- a.Serve(ctx, l) }()

	client := &http.Client{Transport: &http.Transport{DialContext: l.dial}}
	req, _ := http.NewRequest(http.MethodGet, "http://acksy.test/api/v1/events", nil)
	req.AddCookie(&http.Cookie{Name: "acksy_session", Value: secret})
	resp, err := client.Do(req) //nolint:bodyclose // closed by defer below; drained in a goroutine
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	// Keep reading like a real client (net.Pipe has no kernel buffer).
	go func() { _, _ = io.Copy(io.Discard, resp.Body) }()

	start := time.Now()
	stop()
	select {
	case err := <-served:
		if err != nil {
			t.Fatalf("serve: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown blocked by open stream")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("shutdown took %v", d)
	}
}
