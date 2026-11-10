package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/oowliq/acksy/internal/service"
)

// putServerSettings changes the server settings through the API as root.
func putServerSettings(t *testing.T, h http.Handler, mutate func(map[string]any)) {
	t.Helper()
	root := login(t, h, "root", "password123")
	rec := root.do(http.MethodGet, "/api/v1/settings/server", nil)
	expect(t, rec, http.StatusOK)
	var s map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &s); err != nil {
		t.Fatal(err)
	}
	delete(s, "updated_at")
	mutate(s)
	expect(t, root.do(http.MethodPut, "/api/v1/settings/server", s), http.StatusOK)
}

func TestServerSettingsAPI(t *testing.T) {
	h, svc := newAPIWithService(t, Options{PushPublicKey: "BPublicKey"}, service.Options{})
	seedUsers(t, svc)
	root := login(t, h, "root", "password123")
	bob := login(t, h, "bob", "password123")

	rec := root.do(http.MethodGet, "/api/v1/settings/server", nil)
	expect(t, rec, http.StatusOK)
	if body := rec.Body.String(); !strings.Contains(body, `"updated_at":null`) || !strings.Contains(body, `"session_days":30`) {
		t.Fatalf("defaults: %s", body)
	}
	expect(t, bob.do(http.MethodGet, "/api/v1/settings/server", nil), http.StatusForbidden)
	admin := &client{t: t, h: h, token: tokenFor(t, svc, "root", []string{"admin"})}
	expect(t, admin.do(http.MethodGet, "/api/v1/settings/server", nil), http.StatusForbidden)

	var s map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &s); err != nil {
		t.Fatal(err)
	}
	delete(s, "updated_at")
	s["session_days"] = 0
	expect(t, root.do(http.MethodPut, "/api/v1/settings/server", s), http.StatusBadRequest)
	expect(t, bob.do(http.MethodPut, "/api/v1/settings/server", s), http.StatusForbidden)

	tok := tokenFor(t, svc, "root", []string{"notifications:publish"})
	expect(t, raw(t, h, http.MethodPost, "/ntfy/alerts", "on", "Authorization", "Bearer "+tok), http.StatusOK)

	s["session_days"] = 2
	s["adapters"] = map[string]any{"ntfy": false, "gotify": true}
	s["push"] = map[string]any{"enabled": false, "subject": ""}
	put := root.do(http.MethodPut, "/api/v1/settings/server", s)
	expect(t, put, http.StatusOK)
	if body := put.Body.String(); strings.Contains(body, `"updated_at":null`) || !strings.Contains(body, `"ntfy":false`) {
		t.Fatalf("saved: %s", body)
	}

	// Effective at once, without a restart.
	expect(t, raw(t, h, http.MethodPost, "/ntfy/alerts", "off", "Authorization", "Bearer "+tok), http.StatusNotFound)
	if body := root.do(http.MethodGet, "/api/v1/push/config", nil).Body.String(); !strings.Contains(body, `"enabled":false`) {
		t.Fatalf("push config: %s", body)
	}
	rec = (&client{t: t, h: h}).do(http.MethodPost, "/api/v1/auth/login", map[string]string{"username": "bob", "password": "password123"})
	expect(t, rec, http.StatusOK)
	if c := rec.Header().Get("Set-Cookie"); !strings.Contains(c, "Max-Age=172800") {
		t.Fatalf("session cookie: %s", c)
	}
}
