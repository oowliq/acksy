package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/oowliq/acksy/internal/domain"
)

func TestWebhookCRUD(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	alice, bob := e.user("alice", domain.RoleUser), e.user("bob", domain.RoleUser)
	must(e.svc.CreateTopic(ctx, alice, CreateTopicInput{Name: "ci"}))
	must(e.svc.CreateTopic(ctx, bob, CreateTopicInput{Name: "secret"}))

	created, err := e.svc.CreateWebhook(ctx, alice, WebhookInput{
		Name: ptr("HA"), URL: ptr("http://192.168.1.5:8123/api/webhook/acksy"), Topics: ptr([]string{"ci"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	w := created.Webhook
	if !strings.HasPrefix(created.Secret, "whsec_") || w.Format != domain.WebhookAcksy || w.MinPriority != 1 ||
		!w.Enabled || len(w.TopicIDs) != 1 {
		t.Fatalf("defaults: %+v %q", w, created.Secret)
	}

	// Someone else's topic cannot be referenced; it looks like it does not exist.
	if _, err := e.svc.CreateWebhook(ctx, alice, WebhookInput{
		Name: ptr("x"), URL: ptr("https://x.example"),
		Topics: ptr([]string{"bob/secret"}),
	}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("foreign topic: %v", err)
	}
	for _, bad := range []WebhookInput{
		{Name: ptr(""), URL: ptr("https://x.example")},
		{Name: ptr("x"), URL: ptr("ftp://x.example")},
		{Name: ptr("x"), URL: ptr("https://x.example"), Format: ptr(domain.WebhookFormat("teams"))},
		{Name: ptr("x"), URL: ptr("https://x.example"), MinPriority: ptr(0)},
	} {
		if _, err := e.svc.CreateWebhook(ctx, alice, bad); !errors.Is(err, domain.ErrInvalid) {
			t.Fatalf("invalid %+v: %v", bad, err)
		}
	}
	tok := e.token(alice, []string{"notifications:read"})
	if _, err := e.svc.ListWebhooks(ctx, tok); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("token: %v", err)
	}
	if _, err := e.svc.GetWebhook(ctx, bob, w.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("foreign get: %v", err)
	}

	upd, err := e.svc.UpdateWebhook(ctx, alice, w.ID, WebhookInput{Format: ptr(domain.WebhookDiscord), Topics: ptr([]string{})})
	if err != nil || upd.Format != domain.WebhookDiscord || len(upd.TopicIDs) != 0 || upd.Name != "HA" {
		t.Fatalf("update: %+v %v", upd, err)
	}
	secret := must(e.svc.RotateWebhookSecret(ctx, alice, w.ID))
	if secret == created.Secret || !strings.HasPrefix(secret, "whsec_") {
		t.Fatalf("rotate: %q", secret)
	}
	if _, err := e.svc.TestWebhook(ctx, alice, w.ID); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("test without sender: %v", err)
	}
	if err := e.svc.DeleteWebhook(ctx, alice, w.ID); err != nil {
		t.Fatal(err)
	}
}

func TestWebhookEnqueueOnPublish(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	alice, bob := e.user("alice", domain.RoleUser), e.user("bob", domain.RoleUser)
	ci := must(e.svc.CreateTopic(ctx, alice, CreateTopicInput{Name: "ci"}))
	e.share(alice, ci.ID, "bob")
	must(e.svc.CreateTopic(ctx, alice, CreateTopicInput{Name: "home"}))

	all := must(e.svc.CreateWebhook(ctx, alice, WebhookInput{Name: ptr("all"), URL: ptr("https://a.example")})).Webhook
	ciOnly := must(e.svc.CreateWebhook(ctx, alice, WebhookInput{
		Name: ptr("ci"), URL: ptr("https://c.example"),
		Topics: ptr([]string{"ci"}), MinPriority: ptr(4),
	})).Webhook
	off := must(e.svc.CreateWebhook(ctx, alice, WebhookInput{
		Name: ptr("off"), URL: ptr("https://o.example"),
		Enabled: ptr(false),
	})).Webhook
	bobs := must(e.svc.CreateWebhook(ctx, bob, WebhookInput{Name: ptr("bob"), URL: ptr("https://b.example")})).Webhook

	queued := func(w *domain.Webhook) []domain.WebhookDelivery {
		t.Helper()
		return must(e.svc.store.ListWebhookDeliveries(ctx, w.ID, 100))
	}
	pub := e.token(alice, []string{"notifications:publish"})
	must(e.svc.Publish(ctx, pub, domain.Message{Topic: "ci", Title: "build", Body: "ok", Priority: 4, DedupKey: "b"}))
	must(e.svc.Publish(ctx, pub, domain.Message{Topic: "ci", Title: "build", Body: "again", Priority: 4, DedupKey: "b"}))
	must(e.svc.Publish(ctx, pub, domain.Message{Topic: "home", Body: "door", Priority: 5}))
	must(e.svc.Publish(ctx, pub, domain.Message{Topic: "ci", Body: "low", Priority: 2}))

	if got := queued(all); len(got) != 4 {
		t.Fatalf("all: %d deliveries", len(got))
	}
	ci4 := queued(ciOnly)
	if len(ci4) != 2 || ci4[0].Event != domain.WebhookUpdated || ci4[1].Event != domain.WebhookCreated {
		t.Fatalf("ci P4+: %+v", ci4)
	}
	var n domain.Notification
	if err := json.Unmarshal([]byte(ci4[0].Payload), &n); err != nil || n.Body != "again" || n.Topic.Name != "ci" {
		t.Fatalf("payload snapshot: %+v %v", n, err)
	}
	if got := queued(off); len(got) != 0 {
		t.Fatalf("disabled webhook queued %d", len(got))
	}
	// Bob is a member of ci, so his own webhook fires for ci messages only.
	if got := queued(bobs); len(got) != 3 {
		t.Fatalf("bob: %d deliveries", len(got))
	}
	// Everything is due now.
	if due := must(e.svc.DueWebhookDeliveries(ctx, 100)); len(due) != 4+2+3 {
		t.Fatalf("due: %d", len(due))
	}
}

func TestWebhookRetrySchedule(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	alice := e.user("alice", domain.RoleUser)
	w := must(e.svc.CreateWebhook(ctx, alice, WebhookInput{Name: ptr("x"), URL: ptr("https://x.example")})).Webhook
	must(e.svc.Publish(ctx, alice, domain.Message{Body: "hi"}))
	d := must(e.svc.DueWebhookDeliveries(ctx, 1))[0]

	claim := func() {
		t.Helper()
		ok, err := e.svc.ClaimWebhookDelivery(ctx, d.ID, 2*time.Minute)
		if err != nil || !ok {
			t.Fatalf("claim: %v %v", ok, err)
		}
		d.Attempts++
	}
	claim()
	if ok, _ := e.svc.ClaimWebhookDelivery(ctx, d.ID, 2*time.Minute); ok {
		t.Fatal("claimed twice")
	}
	if err := e.svc.WebhookAttemptFailed(ctx, &d, 503, "HTTP 503", false, 0); err != nil {
		t.Fatal(err)
	}
	if !d.NextAttempt.Equal(e.clock.Add(30 * time.Second)) {
		t.Fatalf("first retry at %v", d.NextAttempt)
	}
	// Retry-After longer than the schedule wins, capped at an hour.
	e.advance(30 * time.Second)
	claim()
	_ = e.svc.WebhookAttemptFailed(ctx, &d, 429, "HTTP 429", false, 5*time.Hour)
	if !d.NextAttempt.Equal(e.clock.Add(time.Hour)) {
		t.Fatalf("retry-after: %v", d.NextAttempt)
	}
	hook := must(e.svc.GetWebhook(ctx, alice, w.ID))
	if hook.LastFailure == nil || hook.LastError != "HTTP 429" {
		t.Fatalf("webhook not marked: %+v", hook)
	}
	// After the last attempt the delivery fails for good.
	d.Attempts = domain.MaxWebhookAttempts
	_ = e.svc.WebhookAttemptFailed(ctx, &d, 500, "HTTP 500", false, 0)
	if d.NextAttempt != nil || d.FailedAt == nil {
		t.Fatalf("not given up: %+v", d)
	}
	e.advance(8 * 24 * time.Hour)
	if n := must(e.svc.PruneWebhookDeliveries(ctx, 7*24*time.Hour)); n != 1 {
		t.Fatalf("pruned %d", n)
	}
}
