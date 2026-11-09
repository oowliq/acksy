package httpapi

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/oowliq/acksy/internal/domain"
	"github.com/oowliq/acksy/internal/service"
)

func TestAPISearch(t *testing.T) {
	h, svc := newAPI(t)
	seedUsers(t, svc)
	root := login(t, h, "root", "password123")

	publish := func(title string, prio int) {
		t.Helper()
		expect(t, root.do(http.MethodPost, "/api/v1/notifications", map[string]any{"title": title, "priority": prio}), http.StatusCreated)
	}
	publish("Бэкап завершён", 3)
	publish("Disk full", 5)

	page := decode[service.Page](t, root.do(http.MethodGet, "/api/v1/notifications?q="+url.QueryEscape("БЭКАП"), nil))
	if len(page.Items) != 1 || page.Items[0].Title != "Бэкап завершён" {
		t.Fatalf("search: %+v", page.Items)
	}
	expect(t, root.do(http.MethodGet, "/api/v1/notifications?q="+strings.Repeat("x", 201), nil), http.StatusBadRequest)

	// Mark all read narrowed by the search touches only the matches.
	res := decode[struct{ Affected int }](t, root.do(http.MethodPost, "/api/v1/notifications/bulk",
		map[string]any{"action": "read", "all": true, "q": "disk"}))
	if res.Affected != 1 {
		t.Fatalf("bulk q affected %d", res.Affected)
	}
	if c := decode[domain.Counts](t, root.do(http.MethodGet, "/api/v1/notifications/counts", nil)); c.Unread != 1 {
		t.Fatalf("after bulk q: %+v", c)
	}

	// ... and by priority.
	publish("Load high", 5)
	res = decode[struct{ Affected int }](t, root.do(http.MethodPost, "/api/v1/notifications/bulk",
		map[string]any{"action": "read", "all": true, "min_priority": 5}))
	if res.Affected != 1 {
		t.Fatalf("bulk min_priority affected %d", res.Affected)
	}

	// Snoozing shows up in the counts.
	id := decode[service.Page](t, root.do(http.MethodGet, "/api/v1/notifications?q=бэкап", nil)).Items[0].ID
	until := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	expect(t, root.do(http.MethodPatch, "/api/v1/notifications/"+id, map[string]any{"snoozed_until": until}), http.StatusOK)
	if c := decode[domain.Counts](t, root.do(http.MethodGet, "/api/v1/notifications/counts", nil)); c.Snoozed != 1 || c.Unread != 0 {
		t.Fatalf("counts after snooze: %+v", c)
	}
}
