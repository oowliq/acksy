package httpapi

import (
	"net/http"
	"testing"
	"time"
)

func TestPushSubscriptionsAPI(t *testing.T) {
	h, svc := newAPIWith(t, Options{SessionTTL: time.Hour, PushPublicKey: "BPublicKey"})
	seedUsers(t, svc)
	root := login(t, h, "root", "password123")

	cfg := decode[struct {
		Enabled   bool   `json:"enabled"`
		PublicKey string `json:"public_key"`
	}](t, root.do(http.MethodGet, "/api/v1/push/config", nil))
	if !cfg.Enabled || cfg.PublicKey != "BPublicKey" {
		t.Fatalf("config: %+v", cfg)
	}

	// The body is what PushSubscription.toJSON() returns in a browser.
	browser := map[string]any{
		"endpoint":       "https://updates.push.services.mozilla.com/wpush/v2/gAAAA",
		"expirationTime": nil,
		"keys":           map[string]string{"p256dh": "BNcRd", "auth": "tBHI"},
		"name":           "Firefox on laptop",
		"min_priority":   4,
	}
	rec := root.do(http.MethodPost, "/api/v1/push/subscriptions", browser)
	expect(t, rec, http.StatusCreated)
	type subscription struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		MinPriority int    `json:"min_priority"`
		PushService string `json:"push_service"`
		Endpoint    string `json:"endpoint"`
		Auth        string `json:"auth"`
	}
	sub := decode[subscription](t, rec)
	if sub.ID == "" || sub.MinPriority != 4 || sub.PushService != "updates.push.services.mozilla.com" {
		t.Fatalf("created: %+v", sub)
	}
	if sub.Endpoint != "" || sub.Auth != "" {
		t.Fatalf("secrets leaked: %s", rec.Body)
	}

	list := decode[struct{ Items []subscription }](t, root.do(http.MethodGet, "/api/v1/push/subscriptions", nil))
	if len(list.Items) != 1 {
		t.Fatalf("list: %+v", list)
	}
	upd := decode[subscription](t, root.do(http.MethodPatch, "/api/v1/push/subscriptions/"+sub.ID, map[string]any{"min_priority": 5}))
	if upd.MinPriority != 5 || upd.Name != "Firefox on laptop" {
		t.Fatalf("update: %+v", upd)
	}
	// No sender in this router: the test push reports that clearly.
	expect(t, root.do(http.MethodPost, "/api/v1/push/subscriptions/"+sub.ID+"/test", nil), http.StatusConflict)

	// API tokens cannot manage push subscriptions.
	tok := &client{t: t, h: h, token: tokenFor(t, svc, "root", []string{"notifications:read"})}
	expect(t, tok.do(http.MethodGet, "/api/v1/push/subscriptions", nil), http.StatusForbidden)

	// Another user cannot see or delete it.
	bob := login(t, h, "bob", "password123")
	expect(t, bob.do(http.MethodDelete, "/api/v1/push/subscriptions/"+sub.ID, nil), http.StatusNotFound)
	expect(t, root.do(http.MethodDelete, "/api/v1/push/subscriptions/"+sub.ID, nil), http.StatusNoContent)

	browser["endpoint"] = "http://insecure.example/push"
	expect(t, root.do(http.MethodPost, "/api/v1/push/subscriptions", browser), http.StatusBadRequest)
}

func TestPushDisabled(t *testing.T) {
	h, svc := newAPI(t)
	seedUsers(t, svc)
	root := login(t, h, "root", "password123")
	cfg := decode[struct{ Enabled bool }](t, root.do(http.MethodGet, "/api/v1/push/config", nil))
	if cfg.Enabled {
		t.Fatal("push reported enabled without a key")
	}
	expect(t, root.do(http.MethodPost, "/api/v1/push/subscriptions", map[string]any{
		"endpoint": "https://push.example/x", "keys": map[string]string{"p256dh": "a", "auth": "b"},
	}), http.StatusConflict)
}
