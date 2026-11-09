package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/oowliq/acksy/internal/domain"
	"github.com/oowliq/acksy/internal/service"
)

// pipeListener serves HTTP over in-memory connections, so streaming tests
// exercise a real http.Server without binding a port.
type pipeListener struct {
	conns chan net.Conn
	once  sync.Once
	done  chan struct{}
}

func newPipeListener() *pipeListener {
	return &pipeListener{conns: make(chan net.Conn), done: make(chan struct{})}
}

func (l *pipeListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.conns:
		return c, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}

func (l *pipeListener) Close() error {
	l.once.Do(func() { close(l.done) })
	return nil
}

func (l *pipeListener) Addr() net.Addr { return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 80} }

func (l *pipeListener) dial(ctx context.Context, _, _ string) (net.Conn, error) {
	server, client := net.Pipe()
	select {
	case l.conns <- server:
		return client, nil
	case <-l.done:
		return nil, net.ErrClosed
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type liveServer struct {
	svc    *service.Service
	client *http.Client
	cookie *http.Cookie
}

func startLive(t *testing.T) *liveServer {
	t.Helper()
	h, svc := newAPIWith(t, Options{SessionTTL: time.Hour, KeepAlive: 50 * time.Millisecond})
	seedUsers(t, svc)

	l := newPipeListener()
	srv := &http.Server{Handler: h, ReadHeaderTimeout: time.Second}
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})

	ls := &liveServer{svc: svc, client: &http.Client{Transport: &http.Transport{DialContext: l.dial}}}
	ls.cookie = login(t, h, "bob", "password123").cookie
	return ls
}

// publishAsBob publishes through the service as bob's session principal.
func (ls *liveServer) publishAsBob(t *testing.T, body string) {
	t.Helper()
	p, err := ls.svc.AuthenticateSession(context.Background(), ls.cookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ls.svc.Publish(context.Background(), p, domain.Message{Body: body}); err != nil {
		t.Fatal(err)
	}
}

type sseFrame struct {
	id   string
	data string
}

func readSSE(t *testing.T, r *bufio.Reader) sseFrame {
	t.Helper()
	var f sseFrame
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("read sse: %v", err)
		}
		line = strings.TrimRight(line, "\n")
		switch {
		case line == "":
			if f.data != "" {
				return f
			}
		case strings.HasPrefix(line, "id: "):
			f.id = strings.TrimPrefix(line, "id: ")
		case strings.HasPrefix(line, "data: "):
			f.data = strings.TrimPrefix(line, "data: ")
		}
		// "retry:" and ": ping" lines are skipped.
	}
}

func TestSSEStream(t *testing.T) {
	ls := startLive(t)
	ls.publishAsBob(t, "before") // seq 1

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://acksy.test/api/v1/events", nil)
	req.AddCookie(ls.cookie)
	req.Header.Set("Last-Event-ID", "0") // resume from the beginning
	resp, err := ls.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("status %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	br := bufio.NewReader(resp.Body)

	f := readSSE(t, br)
	var ev domain.Event
	if err := json.Unmarshal([]byte(f.data), &ev); err != nil || f.id != "1" || ev.Type != domain.EventNotificationCreated {
		t.Fatalf("backlog frame: %+v", f)
	}
	if f := readSSE(t, br); f.id != "1" || !strings.Contains(f.data, `"type":"ready"`) {
		t.Fatalf("ready frame: %+v", f)
	}

	ls.publishAsBob(t, "live")
	f = readSSE(t, br)
	if f.id != "2" || !strings.Contains(f.data, `"live"`) {
		t.Fatalf("live frame: %+v", f)
	}
}

func TestSSERejectsBadRequests(t *testing.T) {
	h, svc := newAPI(t)
	seedUsers(t, svc)
	anon := &client{t: t, h: h}
	expect(t, anon.do(http.MethodGet, "/api/v1/events", nil), http.StatusUnauthorized)

	bob := login(t, h, "bob", "password123")
	expect(t, bob.do(http.MethodGet, "/api/v1/events?since=abc", nil), http.StatusBadRequest)
	expect(t, bob.do(http.MethodGet, "/api/v1/ws?since=-1", nil), http.StatusBadRequest)
}

func TestWebSocketStream(t *testing.T) {
	ls := startLive(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	hdr := http.Header{}
	hdr.Set("Cookie", ls.cookie.String())
	c, _, err := websocket.Dial(ctx, "ws://acksy.test/api/v1/ws", &websocket.DialOptions{HTTPClient: ls.client, HTTPHeader: hdr}) //nolint:bodyclose // the library owns the handshake response body
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.CloseNow() }()

	read := func() map[string]any {
		t.Helper()
		_, data, err := c.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	if m := read(); m["type"] != "ready" || m["seq"] != float64(0) {
		t.Fatalf("ready: %v", m)
	}
	ls.publishAsBob(t, "hello")
	if m := read(); m["type"] != string(domain.EventNotificationCreated) || m["seq"] != float64(1) {
		t.Fatalf("event: %v", m)
	}

	// Logging out ends the stream once the credential check runs; here we just
	// verify a clean client close is handled.
	if err := c.Close(websocket.StatusNormalClosure, ""); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("close: %v", err)
	}
}

func TestWebSocketRejectsCrossOrigin(t *testing.T) {
	ls := startLive(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	hdr := http.Header{}
	hdr.Set("Cookie", ls.cookie.String())
	hdr.Set("Origin", "https://evil.example")
	_, resp, err := websocket.Dial(ctx, "ws://acksy.test/api/v1/ws", &websocket.DialOptions{HTTPClient: ls.client, HTTPHeader: hdr}) //nolint:bodyclose // the library owns the handshake response body
	if err == nil {
		t.Fatal("cross-origin handshake accepted")
	}
	if resp != nil && resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status %d", resp.StatusCode)
	}
}
