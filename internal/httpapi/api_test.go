package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/oowliq/acksy/internal/adapter"
	"github.com/oowliq/acksy/internal/adapter/gotify"
	"github.com/oowliq/acksy/internal/adapter/ntfy"
	"github.com/oowliq/acksy/internal/broker/memory"
	"github.com/oowliq/acksy/internal/domain"
	"github.com/oowliq/acksy/internal/service"
	"github.com/oowliq/acksy/internal/store"
)

type client struct {
	t      *testing.T
	h      http.Handler
	cookie *http.Cookie
	token  string
}

func newAPI(t *testing.T) (http.Handler, *service.Service) {
	t.Helper()
	return newAPIWith(t, Options{})
}

func newAPIWith(t *testing.T, opts Options) (http.Handler, *service.Service) {
	t.Helper()
	return newAPIWithService(t, opts, service.Options{})
}

func newAPIWithService(t *testing.T, opts Options, svcOpts service.Options) (http.Handler, *service.Service) {
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
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := service.New(store.New(db), b, log, svcOpts)
	h := NewRouter(Deps{
		Logger: log, Service: svc, Options: opts, OpenAPI: []byte("openapi: 3.1.0"),
		Adapters: []adapter.Adapter{ntfy.New(), gotify.New()},
	})
	return h, svc
}

func (c *client) do(method, path string, body any, hdr ...string) *httptest.ResponseRecorder {
	c.t.Helper()
	var r io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		r = strings.NewReader(b)
	default:
		raw, err := json.Marshal(b)
		if err != nil {
			c.t.Fatal(err)
		}
		r = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.cookie != nil {
		req.AddCookie(c.cookie)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	rec := httptest.NewRecorder()
	c.h.ServeHTTP(rec, req)
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return v
}

func expect(t *testing.T, rec *httptest.ResponseRecorder, status int) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status %d, want %d: %s", rec.Code, status, rec.Body)
	}
}

func login(t *testing.T, h http.Handler, username, password string) *client {
	t.Helper()
	c := &client{t: t, h: h}
	rec := c.do(http.MethodPost, "/api/v1/auth/login", map[string]string{"username": username, "password": password})
	expect(t, rec, http.StatusOK)
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == SessionCookie {
			if !ck.HttpOnly || ck.SameSite != http.SameSiteLaxMode {
				t.Fatalf("insecure cookie: %+v", ck)
			}
			c.cookie = ck
		}
	}
	if c.cookie == nil {
		t.Fatal("no session cookie")
	}
	return c
}

func seedUsers(t *testing.T, svc *service.Service) {
	t.Helper()
	ctx := context.Background()
	if _, err := svc.BootstrapAdmin(ctx, "root", "password123"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateUser(ctx, service.SystemPrincipal(), service.CreateUserInput{Username: "bob", Password: "password123"}); err != nil {
		t.Fatal(err)
	}
}

func TestAPIEndToEnd(t *testing.T) {
	h, svc := newAPI(t)
	seedUsers(t, svc)

	anon := &client{t: t, h: h}
	rec := anon.do(http.MethodGet, "/api/v1/notifications", nil)
	expect(t, rec, http.StatusUnauthorized)
	if rec.Header().Get("WWW-Authenticate") == "" {
		t.Fatal("missing WWW-Authenticate")
	}
	expect(t, anon.do(http.MethodGet, "/api/v1/version", nil), http.StatusOK)
	expect(t, anon.do(http.MethodGet, "/api/v1/openapi.yaml", nil), http.StatusOK)

	root := login(t, h, "root", "password123")
	me := decode[meResponse](t, root.do(http.MethodGet, "/api/v1/me", nil))
	if me.User.Username != "root" || me.Auth.Type != "session" || !me.Auth.Scopes.Has(domain.ScopeAdmin) {
		t.Fatalf("me: %+v", me)
	}
	if !me.Auth.HasPassword {
		t.Fatal("root has a password")
	}

	// Topic shared with bob.
	rec = root.do(http.MethodPost, "/api/v1/topics", map[string]any{"name": "ci", "default_priority": 4})
	expect(t, rec, http.StatusCreated)
	topic := decode[domain.Topic](t, rec)
	expect(t, root.do(http.MethodPost, "/api/v1/topics/"+topic.ID+"/invites", map[string]string{"username": "bob"}), http.StatusCreated)
	expect(t, root.do(http.MethodPost, "/api/v1/topics/"+topic.ID+"/invites", map[string]string{"username": "nobody"}), http.StatusNotFound)
	invitee := login(t, h, "bob", "password123")
	invs := decode[struct{ Items []domain.TopicInvite }](t, invitee.do(http.MethodGet, "/api/v1/invitations", nil))
	if len(invs.Items) != 1 || invs.Items[0].TopicID != topic.ID || invs.Items[0].OwnerUsername != "root" {
		t.Fatalf("invitations: %+v", invs)
	}
	pending := decode[struct{ Items []domain.TopicInvite }](t, root.do(http.MethodGet, "/api/v1/topics/"+topic.ID+"/invites", nil))
	if len(pending.Items) != 1 || pending.Items[0].Username != "bob" {
		t.Fatalf("pending invites: %+v", pending)
	}
	expect(t, invitee.do(http.MethodPost, "/api/v1/invitations/"+topic.ID+"/accept", nil), http.StatusOK)
	expect(t, invitee.do(http.MethodDelete, "/api/v1/invitations/"+topic.ID, nil), http.StatusNotFound)

	// Publish-only token restricted to the topic.
	rec = root.do(http.MethodPost, "/api/v1/tokens", map[string]any{
		"name": "ci", "scopes": []string{"notifications:publish"}, "topics": []string{"ci"},
	})
	expect(t, rec, http.StatusCreated)
	created := decode[struct {
		Token  domain.APIToken `json:"token"`
		Secret string          `json:"secret"`
	}](t, rec)
	if !strings.HasPrefix(created.Secret, "ack_") || created.Token.Prefix == "" {
		t.Fatalf("token: %+v", created)
	}
	listed := root.do(http.MethodGet, "/api/v1/tokens", nil).Body.String()
	if strings.Contains(listed, created.Secret) || strings.Contains(listed, "secret_hash") {
		t.Fatal("token list leaks secrets")
	}

	ci := &client{t: t, h: h, token: created.Secret}
	rec = ci.do(http.MethodPost, "/api/v1/notifications", map[string]any{
		"topic": "ci", "title": "Build #42", "body": "**passed**", "tags": []string{"ci"},
	})
	expect(t, rec, http.StatusCreated)
	if res := decode[service.PublishResult](t, rec); res.Recipients != 2 {
		t.Fatalf("publish: %+v", res)
	}
	expect(t, ci.do(http.MethodGet, "/api/v1/notifications", nil), http.StatusForbidden)
	expect(t, ci.do(http.MethodGet, "/api/v1/tokens", nil), http.StatusForbidden)

	// Bob sees it, with the topic's default priority.
	bob := login(t, h, "bob", "password123")
	page := decode[service.Page](t, bob.do(http.MethodGet, "/api/v1/notifications?unread=true&topic=root/ci", nil))
	if len(page.Items) != 1 || page.Items[0].Priority != 4 || page.Items[0].Title != "Build #42" {
		t.Fatalf("bob list: %+v", page)
	}
	id := page.Items[0].ID

	// Snooze, then clear the snooze with an explicit null.
	until := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	rec = bob.do(http.MethodPatch, "/api/v1/notifications/"+id, map[string]any{"read": true, "snoozed_until": until})
	expect(t, rec, http.StatusOK)
	if n := decode[domain.Notification](t, rec); n.ReadAt == nil || n.SnoozedUntil == nil {
		t.Fatalf("patch: %+v", n)
	}
	rec = bob.do(http.MethodPatch, "/api/v1/notifications/"+id, `{"snoozed_until": null}`)
	if n := decode[domain.Notification](t, rec); n.SnoozedUntil != nil || n.ReadAt == nil {
		t.Fatalf("clear snooze: %+v", n)
	}

	// Root's copy is unaffected; bulk read by topic.
	if c := decode[domain.Counts](t, root.do(http.MethodGet, "/api/v1/notifications/counts", nil)); c.Unread != 1 {
		t.Fatalf("root counts: %+v", c)
	}
	rec = root.do(http.MethodPost, "/api/v1/notifications/bulk", map[string]any{"action": "read", "topic": "ci"})
	expect(t, rec, http.StatusOK)
	if r := decode[service.BulkResult](t, rec); r.Affected != 1 {
		t.Fatalf("bulk: %+v", r)
	}

	// Bob cannot see root's notification by ID or reach admin endpoints.
	rootItems := decode[service.Page](t, root.do(http.MethodGet, "/api/v1/notifications?view=all", nil)).Items
	expect(t, bob.do(http.MethodGet, "/api/v1/notifications/"+rootItems[0].ID, nil), http.StatusNotFound)
	expect(t, bob.do(http.MethodGet, "/api/v1/users", nil), http.StatusForbidden)
	expect(t, root.do(http.MethodGet, "/api/v1/users", nil), http.StatusOK)

	// Logout invalidates the session cookie.
	expect(t, bob.do(http.MethodPost, "/api/v1/auth/logout", nil), http.StatusNoContent)
	expect(t, bob.do(http.MethodGet, "/api/v1/me", nil), http.StatusUnauthorized)
}

func TestAPIValidationAndErrors(t *testing.T) {
	h, svc := newAPI(t)
	seedUsers(t, svc)
	root := login(t, h, "root", "password123")

	cases := []struct {
		name   string
		method string
		path   string
		body   any
		status int
		code   string
	}{
		{"unknown field", http.MethodPost, "/api/v1/notifications", `{"title":"x","colour":"red"}`, 400, "invalid"},
		{"bad json", http.MethodPost, "/api/v1/notifications", `{"title":`, 400, "invalid"},
		{"empty body", http.MethodPost, "/api/v1/notifications", "", 400, "invalid"},
		{"empty message", http.MethodPost, "/api/v1/notifications", map[string]any{}, 400, "invalid"},
		{"bad priority", http.MethodPost, "/api/v1/notifications", map[string]any{"title": "x", "priority": 9}, 400, "invalid"},
		{"bad view", http.MethodGet, "/api/v1/notifications?view=trash", nil, 400, "invalid"},
		{"bad bool", http.MethodGet, "/api/v1/notifications?unread=maybe", nil, 400, "invalid"},
		{"missing", http.MethodGet, "/api/v1/notifications/nope", nil, 404, "not_found"},
		{"unknown route", http.MethodGet, "/api/v1/nope", nil, 404, "not_found"},
		{"bad method", http.MethodPut, "/api/v1/notifications", nil, 405, "method_not_allowed"},
		{"dup topic", http.MethodPost, "/api/v1/topics", map[string]string{"name": "x"}, 201, ""},
		{"dup topic again", http.MethodPost, "/api/v1/topics", map[string]string{"name": "x"}, 409, "conflict"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := root.do(tc.method, tc.path, tc.body)
			expect(t, rec, tc.status)
			if tc.code != "" {
				if got := decode[errorBody](t, rec).Error.Code; got != tc.code {
					t.Fatalf("code %q, want %q", got, tc.code)
				}
			}
		})
	}

	big := map[string]string{"body": strings.Repeat("x", maxBodyBytes)}
	expect(t, root.do(http.MethodPost, "/api/v1/notifications", big), http.StatusBadRequest)

	bad := &client{t: t, h: h, token: "ack_" + strings.Repeat("a", 40)}
	expect(t, bad.do(http.MethodGet, "/api/v1/me", nil), http.StatusUnauthorized)
	basic := &client{t: t, h: h}
	expect(t, basic.do(http.MethodGet, "/api/v1/me", nil, "Authorization", "Basic Zm9vOmJhcg=="), http.StatusUnauthorized)
}

func TestAPICrossOriginProtection(t *testing.T) {
	h, svc := newAPI(t)
	seedUsers(t, svc)
	root := login(t, h, "root", "password123")

	rec := root.do(http.MethodPost, "/api/v1/notifications", map[string]string{"title": "x"},
		"Sec-Fetch-Site", "cross-site", "Origin", "https://evil.example")
	expect(t, rec, http.StatusForbidden)

	rec = root.do(http.MethodPost, "/api/v1/notifications", map[string]string{"title": "x"}, "Sec-Fetch-Site", "same-origin")
	expect(t, rec, http.StatusCreated)
	// Safe methods are always allowed.
	expect(t, root.do(http.MethodGet, "/api/v1/me", nil, "Sec-Fetch-Site", "cross-site"), http.StatusOK)
}

func TestAPILoginRateLimit(t *testing.T) {
	h, svc := newAPI(t)
	seedUsers(t, svc)
	c := &client{t: t, h: h}

	for range 10 {
		expect(t, c.do(http.MethodPost, "/api/v1/auth/login", map[string]string{"username": "bob", "password": "wrong"}), http.StatusUnauthorized)
	}
	rec := c.do(http.MethodPost, "/api/v1/auth/login", map[string]string{"username": "bob", "password": "password123"})
	expect(t, rec, http.StatusTooManyRequests)
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("missing Retry-After")
	}
	// Other accounts are unaffected.
	login(t, h, "root", "password123")
}
