package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

func TestWebhooksAPI(t *testing.T) {
	h, svc := newAPI(t)
	seedUsers(t, svc)
	root := login(t, h, "root", "password123")
	expect(t, root.do(http.MethodPost, "/api/v1/topics", map[string]string{"name": "ci"}), http.StatusCreated)

	rec := root.do(http.MethodPost, "/api/v1/webhooks", map[string]any{
		"name": "Discord #alerts", "url": "https://discord.com/api/webhooks/123/SECRET-TOKEN",
		"format": "discord", "topics": []string{"ci"}, "min_priority": 4,
	})
	expect(t, rec, http.StatusCreated)
	type hook struct {
		ID          string   `json:"id"`
		Name        string   `json:"name"`
		URL         string   `json:"url"`
		Format      string   `json:"format"`
		TopicIDs    []string `json:"topic_ids"`
		MinPriority int      `json:"min_priority"`
		Enabled     bool     `json:"enabled"`
	}
	created := decode[struct {
		Webhook hook   `json:"webhook"`
		Secret  string `json:"secret"`
	}](t, rec)
	w := created.Webhook
	if !strings.HasPrefix(created.Secret, "whsec_") || w.Format != "discord" || len(w.TopicIDs) != 1 || w.MinPriority != 4 || !w.Enabled {
		t.Fatalf("created: %s", rec.Body)
	}
	// The URL's credential and the signing secret never come back.
	if w.URL != "https://discord.com/…" || strings.Contains(rec.Body.String(), "SECRET-TOKEN") {
		t.Fatalf("url leaked: %s", rec.Body)
	}
	rec = root.do(http.MethodGet, "/api/v1/webhooks", nil)
	expect(t, rec, http.StatusOK)
	if strings.Contains(rec.Body.String(), created.Secret) || strings.Contains(rec.Body.String(), "SECRET-TOKEN") {
		t.Fatalf("list leaks secrets: %s", rec.Body)
	}

	upd := decode[hook](t, root.do(http.MethodPatch, "/api/v1/webhooks/"+w.ID, map[string]any{"enabled": false, "topics": []string{}}))
	if upd.Enabled || len(upd.TopicIDs) != 0 || upd.Name != "Discord #alerts" {
		t.Fatalf("update: %+v", upd)
	}
	expect(t, root.do(http.MethodPatch, "/api/v1/webhooks/"+w.ID, map[string]any{"enabled": true}), http.StatusOK)
	rot := decode[struct{ Secret string }](t, root.do(http.MethodPost, "/api/v1/webhooks/"+w.ID+"/rotate-secret", nil))
	if rot.Secret == "" || rot.Secret == created.Secret {
		t.Fatalf("rotate: %+v", rot)
	}

	// A P5 publish queues a delivery; the history shows it.
	expect(t, root.do(http.MethodPost, "/api/v1/notifications", map[string]any{"body": "down", "priority": 5}), http.StatusCreated)
	hist := decode[struct {
		Items []struct {
			Event    string `json:"event"`
			Attempts int    `json:"attempts"`
		} `json:"items"`
	}](t, root.do(http.MethodGet, "/api/v1/webhooks/"+w.ID+"/deliveries", nil))
	if len(hist.Items) != 1 || hist.Items[0].Event != "notification.created" {
		t.Fatalf("deliveries: %+v", hist)
	}
	if strings.Contains(root.do(http.MethodGet, "/api/v1/webhooks/"+w.ID+"/deliveries", nil).Body.String(), "payload") {
		t.Fatal("delivery payload exposed")
	}

	// No worker in this router: the test endpoint says so.
	expect(t, root.do(http.MethodPost, "/api/v1/webhooks/"+w.ID+"/test", nil), http.StatusConflict)

	bob := login(t, h, "bob", "password123")
	expect(t, bob.do(http.MethodGet, "/api/v1/webhooks/"+w.ID, nil), http.StatusNotFound)
	tok := &client{t: t, h: h, token: tokenFor(t, svc, "root", []string{"notifications:read"})}
	expect(t, tok.do(http.MethodGet, "/api/v1/webhooks", nil), http.StatusForbidden)
	expect(t, root.do(http.MethodPost, "/api/v1/webhooks", map[string]any{"name": "x", "url": "ftp://x"}), http.StatusBadRequest)

	expect(t, root.do(http.MethodDelete, "/api/v1/webhooks/"+w.ID, nil), http.StatusNoContent)
	expect(t, root.do(http.MethodGet, "/api/v1/webhooks/"+w.ID, nil), http.StatusNotFound)
}

func TestTopicRetentionAPI(t *testing.T) {
	h, svc := newAPI(t)
	seedUsers(t, svc)
	root := login(t, h, "root", "password123")
	type topic struct {
		ID               string `json:"id"`
		RetentionDays    *int   `json:"retention_days"`
		MaxNotifications *int   `json:"max_notifications"`
	}
	rec := root.do(http.MethodPost, "/api/v1/topics", map[string]any{"name": "ci", "retention_days": 7, "max_notifications": 500})
	expect(t, rec, http.StatusCreated)
	tp := decode[topic](t, rec)
	if tp.RetentionDays == nil || *tp.RetentionDays != 7 || tp.MaxNotifications == nil || *tp.MaxNotifications != 500 {
		t.Fatalf("created: %s", rec.Body)
	}
	// null clears an override, absent leaves it alone.
	upd := decode[topic](t, root.do(http.MethodPatch, "/api/v1/topics/"+tp.ID, map[string]any{"retention_days": nil}))
	if upd.RetentionDays != nil || upd.MaxNotifications == nil {
		t.Fatalf("patch: %+v", upd)
	}
	expect(t, root.do(http.MethodPatch, "/api/v1/topics/"+tp.ID, map[string]any{"max_notifications": 3}), http.StatusBadRequest)
}

func TestRunActionAPI(t *testing.T) {
	h, svc := newAPI(t)
	seedUsers(t, svc)
	root := login(t, h, "root", "password123")
	rec := root.do(http.MethodPost, "/api/v1/notifications", map[string]any{"body": "gate", "actions": []map[string]any{
		{"type": "http", "label": "Open", "url": "https://ha.lan/api/open?key=K3Y", "headers": map[string]string{"Authorization": "Bearer S3CRET"}, "clear": true},
	}})
	expect(t, rec, http.StatusCreated)
	id := decode[struct {
		NotificationID string `json:"notification_id"`
	}](t, rec).NotificationID

	rec = root.do(http.MethodGet, "/api/v1/notifications/"+id, nil)
	if body := rec.Body.String(); strings.Contains(body, "S3CRET") || strings.Contains(body, "K3Y") ||
		!strings.Contains(body, `"url":"https://ha.lan/api/open"`) || !strings.Contains(body, `"method":"POST"`) {
		t.Fatalf("notification JSON: %s", body)
	}
	// No runner wired in this router: the endpoint says so; bad index is 404.
	expect(t, root.do(http.MethodPost, "/api/v1/notifications/"+id+"/actions/0", nil), http.StatusConflict)
	expect(t, root.do(http.MethodPost, "/api/v1/notifications/"+id+"/actions/x", nil), http.StatusNotFound)
}
