package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oowliq/acksy/internal/service"
)

// raw sends a request with an arbitrary body and headers.
func raw(t *testing.T, h http.Handler, method, target, body string, hdr ...string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func tokenFor(t *testing.T, svc *service.Service, username string, scopes []string, topics ...string) string {
	t.Helper()
	ctx := context.Background()
	u, err := svc.GetUserByUsername(ctx, service.SystemPrincipal(), username)
	if err != nil {
		t.Fatal(err)
	}
	ct, err := svc.CreateToken(ctx, service.UserPrincipal(u), service.CreateTokenInput{Name: "t", Scopes: scopes, Topics: topics})
	if err != nil {
		t.Fatal(err)
	}
	return ct.Secret
}

type inboxPage struct {
	Items []struct {
		ID       string   `json:"id"`
		Title    string   `json:"title"`
		Body     string   `json:"body"`
		Priority int      `json:"priority"`
		Tags     []string `json:"tags"`
		Source   string   `json:"source"`
		ReadAt   *string  `json:"read_at"`
		Topic    *struct {
			Name string `json:"name"`
		} `json:"topic"`
	} `json:"items"`
}

func TestNtfyAdapter(t *testing.T) {
	h, svc := newAPI(t)
	seedUsers(t, svc)
	root := login(t, h, "root", "password123")
	tok := tokenFor(t, svc, "root", []string{"notifications:publish"})

	// Publish the ntfy way; the topic is created on the fly as in ntfy.
	rec := raw(t, h, http.MethodPost, "/ntfy/alerts", "Disk almost full",
		"Authorization", "Bearer "+tok, "Title", "Backup", "Priority", "high", "Tags", "warning,disk")
	expect(t, rec, http.StatusOK)
	var pub struct {
		ID, Event, Topic, Message string
		Priority                  int
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &pub); err != nil {
		t.Fatal(err)
	}
	if pub.ID == "" || pub.Event != "message" || pub.Topic != "alerts" || pub.Priority != 4 {
		t.Fatalf("ntfy response: %s", rec.Body)
	}
	page := decode[inboxPage](t, root.do(http.MethodGet, "/api/v1/notifications", nil))
	if len(page.Items) != 1 || page.Items[0].Title != "Backup" || page.Items[0].Source != "ntfy" ||
		page.Items[0].Priority != 4 || page.Items[0].Topic == nil || page.Items[0].Topic.Name != "alerts" {
		t.Fatalf("inbox: %+v", page.Items)
	}

	// The returned ID works as a sequence ID: update in place, then clear, then delete.
	expect(t, raw(t, h, http.MethodPut, "/ntfy/alerts/"+pub.ID, "Disk cleaned up", "Authorization", "Bearer "+tok), http.StatusOK)
	page = decode[inboxPage](t, root.do(http.MethodGet, "/api/v1/notifications", nil))
	if len(page.Items) != 1 || page.Items[0].Body != "Disk cleaned up" {
		t.Fatalf("update by id: %+v", page.Items)
	}
	rec = raw(t, h, http.MethodPut, "/ntfy/alerts/"+pub.ID+"/clear", "", "Authorization", "Bearer "+tok)
	expect(t, rec, http.StatusOK)
	if !strings.Contains(rec.Body.String(), `"event":"message_clear"`) {
		t.Fatalf("clear response: %s", rec.Body)
	}
	page = decode[inboxPage](t, root.do(http.MethodGet, "/api/v1/notifications?view=all", nil))
	if page.Items[0].ReadAt == nil {
		t.Fatal("clear did not mark read")
	}
	// curl -u :token style Basic auth.
	expect(t, raw(t, h, http.MethodDelete, "/ntfy/alerts/"+pub.ID, "", "Authorization", basic("", tok)), http.StatusOK)
	page = decode[inboxPage](t, root.do(http.MethodGet, "/api/v1/notifications?view=all", nil))
	if len(page.Items) != 0 {
		t.Fatalf("delete left: %+v", page.Items)
	}
}

func TestNtfyAdapterAuth(t *testing.T) {
	h, svc := newAPI(t)
	seedUsers(t, svc)
	root := login(t, h, "root", "password123")

	rec := raw(t, h, http.MethodPost, "/ntfy/alerts", "x")
	expect(t, rec, http.StatusUnauthorized)
	if !strings.Contains(rec.Body.String(), `"http":401`) {
		t.Fatalf("ntfy error format: %s", rec.Body)
	}
	// A user's password is never accepted, only tokens.
	expect(t, raw(t, h, http.MethodPost, "/ntfy/alerts", "x", "Authorization", basic("root", "password123")), http.StatusUnauthorized)
	// Session cookies are ignored by adapters (no CSRF surface).
	req := httptest.NewRequest(http.MethodPost, "/ntfy/alerts", strings.NewReader("x"))
	req.AddCookie(root.cookie)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	expect(t, rec, http.StatusUnauthorized)

	// A read-only token cannot publish; a topic-restricted one cannot leave its topic.
	ro := tokenFor(t, svc, "root", []string{"notifications:read"})
	expect(t, raw(t, h, http.MethodPost, "/ntfy/alerts", "x", "Authorization", "Bearer "+ro), http.StatusForbidden)
	must := func(code int) {
		t.Helper()
		if code != http.StatusOK {
			t.Fatalf("setup status %d", code)
		}
	}
	anyTok := tokenFor(t, svc, "root", []string{"notifications:publish"})
	must(raw(t, h, http.MethodPost, "/ntfy/backups", "x", "Authorization", "Bearer "+anyTok).Code)
	must(raw(t, h, http.MethodPost, "/ntfy/other", "x", "Authorization", "Bearer "+anyTok).Code)
	scoped := tokenFor(t, svc, "root", []string{"notifications:publish"}, "backups")
	expect(t, raw(t, h, http.MethodPost, "/ntfy/other", "x", "Authorization", "Bearer "+scoped), http.StatusForbidden)
	expect(t, raw(t, h, http.MethodPost, "/ntfy/backups", "x", "Authorization", "Bearer "+scoped), http.StatusOK)
	// Unsupported features are explicit 400s in ntfy's error format.
	rec = raw(t, h, http.MethodPost, "/ntfy/backups", "x", "Authorization", "Bearer "+anyTok, "X-Delay", "1h")
	expect(t, rec, http.StatusBadRequest)
	if !strings.Contains(rec.Body.String(), "X-Delay") {
		t.Fatalf("delay error: %s", rec.Body)
	}
}

func TestGotifyAdapter(t *testing.T) {
	h, svc := newAPI(t)
	seedUsers(t, svc)
	root, bob := login(t, h, "root", "password123"), login(t, h, "bob", "password123")

	// A token restricted to one topic acts like a gotify application.
	tp := decode[struct{ ID string }](t, root.do(http.MethodPost, "/api/v1/topics", map[string]string{"name": "backups"}))
	expect(t, root.do(http.MethodPost, "/api/v1/topics/"+tp.ID+"/invites", map[string]string{"username": "bob"}), http.StatusCreated)
	expect(t, bob.do(http.MethodPost, "/api/v1/invitations/"+tp.ID+"/accept", nil), http.StatusOK)
	app := tokenFor(t, svc, "root", []string{"notifications:publish"}, "backups")

	rec := raw(t, h, http.MethodPost, "/gotify/message?token="+app, "title=Backup&message=done&priority=8",
		"Content-Type", "application/x-www-form-urlencoded")
	expect(t, rec, http.StatusOK)
	var msg struct {
		ID       int64
		Message  string
		Priority int
		Date     string
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &msg); err != nil || msg.ID == 0 || msg.Message != "done" || msg.Priority != 8 || msg.Date == "" {
		t.Fatalf("gotify response %s: %v", rec.Body, err)
	}
	page := decode[inboxPage](t, bob.do(http.MethodGet, "/api/v1/notifications", nil))
	if len(page.Items) != 1 || page.Items[0].Topic == nil || page.Items[0].Topic.Name != "backups" ||
		page.Items[0].Priority != 4 || page.Items[0].Source != "gotify" {
		t.Fatalf("bob's inbox: %+v", page.Items)
	}

	// An unrestricted token posts to the owner's own inbox.
	free := tokenFor(t, svc, "bob", []string{"notifications:publish"})
	expect(t, raw(t, h, http.MethodPost, "/gotify/message", `{"message":"to me"}`,
		"X-Gotify-Key", free, "Content-Type", "application/json"), http.StatusOK)
	page = decode[inboxPage](t, bob.do(http.MethodGet, "/api/v1/notifications", nil))
	if len(page.Items) != 2 || page.Items[0].Body != "to me" || page.Items[0].Topic != nil {
		t.Fatalf("own inbox: %+v", page.Items)
	}

	rec = raw(t, h, http.MethodPost, "/gotify/message", `{"message":"x"}`, "Content-Type", "application/json")
	expect(t, rec, http.StatusUnauthorized)
	if !strings.Contains(rec.Body.String(), `"errorCode":401`) {
		t.Fatalf("gotify error format: %s", rec.Body)
	}
}

func TestAdaptersCanBeDisabled(t *testing.T) {
	_, svc := newAPI(t)
	h := NewRouter(Deps{Logger: discardLog(), Service: svc, Options: Options{}})
	expect(t, raw(t, h, http.MethodPost, "/ntfy/alerts", "x"), http.StatusNotFound)
	expect(t, raw(t, h, http.MethodPost, "/gotify/message", "{}"), http.StatusNotFound)
}

func basic(user, password string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+password))
}

func discardLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestPublishRateLimitHTTP(t *testing.T) {
	h, svc := newAPIWithService(t, Options{}, service.Options{PublishLimit: service.PublishLimit{PerToken: 3}})
	seedUsers(t, svc)
	tok := tokenFor(t, svc, "root", []string{"notifications:publish"})
	c := &client{t: t, h: h, token: tok}

	// One shared budget for the token, whichever protocol spends it.
	expect(t, c.do(http.MethodPost, "/api/v1/notifications", map[string]string{"body": "native"}), http.StatusCreated)
	expect(t, raw(t, h, http.MethodPost, "/ntfy/alerts", "ntfy", "Authorization", "Bearer "+tok), http.StatusOK)
	expect(t, raw(t, h, http.MethodPost, "/gotify/message", `{"message":"gotify"}`,
		"X-Gotify-Key", tok, "Content-Type", "application/json"), http.StatusOK)

	rec := c.do(http.MethodPost, "/api/v1/notifications", map[string]string{"body": "over"})
	expect(t, rec, http.StatusTooManyRequests)
	if rec.Header().Get("Retry-After") == "" || !strings.Contains(rec.Body.String(), `"rate_limited"`) {
		t.Fatalf("native 429: %v %s", rec.Header(), rec.Body)
	}
	rec = raw(t, h, http.MethodPost, "/ntfy/alerts", "over", "Authorization", "Bearer "+tok)
	expect(t, rec, http.StatusTooManyRequests)
	if rec.Header().Get("Retry-After") == "" || !strings.Contains(rec.Body.String(), `"http":429`) {
		t.Fatalf("ntfy 429: %v %s", rec.Header(), rec.Body)
	}
	rec = raw(t, h, http.MethodPost, "/gotify/message", `{"message":"over"}`, "X-Gotify-Key", tok, "Content-Type", "application/json")
	expect(t, rec, http.StatusTooManyRequests)
	if rec.Header().Get("Retry-After") == "" || !strings.Contains(rec.Body.String(), `"errorCode":429`) {
		t.Fatalf("gotify 429: %v %s", rec.Header(), rec.Body)
	}

	// Another token of the same user has its own budget (no user limit set here).
	other := &client{t: t, h: h, token: tokenFor(t, svc, "root", []string{"notifications:publish"})}
	expect(t, other.do(http.MethodPost, "/api/v1/notifications", map[string]string{"body": "fine"}), http.StatusCreated)
}
