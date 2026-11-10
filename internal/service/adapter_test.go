package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/oowliq/acksy/internal/auth"
	"github.com/oowliq/acksy/internal/domain"
)

func inbox(t *testing.T, e *env, p *auth.Principal) []domain.Notification {
	t.Helper()
	page, err := e.svc.ListNotifications(context.Background(), p, ListQuery{View: domain.ViewAll})
	if err != nil {
		t.Fatal(err)
	}
	return page.Items
}

func TestPublishTopicFromToken(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	alice, bob := e.user("alice", domain.RoleUser), e.user("bob", domain.RoleUser)
	backups := must(e.svc.CreateTopic(ctx, alice, CreateTopicInput{Name: "backups"}))
	must(e.svc.CreateTopic(ctx, alice, CreateTopicInput{Name: "ci"}))
	e.share(alice, backups.ID, "bob")

	// Token bound to one topic: the message goes to that topic's members.
	single := e.token(alice, []string{"notifications:publish"}, "backups")
	res, err := e.svc.Publish(ctx, single, domain.Message{Body: "done", TopicFromToken: true})
	if err != nil || res.Recipients != 2 {
		t.Fatalf("single-topic token: %+v %v", res, err)
	}
	if got := inbox(t, e, bob); len(got) != 1 || got[0].Topic == nil || got[0].Topic.Name != "backups" {
		t.Fatalf("bob's copy: %+v", got)
	}

	// Unrestricted token: the owner's own inbox, no topic.
	free := e.token(alice, []string{"notifications:publish", "notifications:read"})
	res, err = e.svc.Publish(ctx, free, domain.Message{Body: "self", TopicFromToken: true})
	if err != nil || res.Recipients != 1 {
		t.Fatalf("unrestricted token: %+v %v", res, err)
	}
	if own := must(e.svc.GetNotification(ctx, free, res.NotificationID)); own.Topic != nil {
		t.Fatalf("expected no topic, got %+v", own.Topic)
	}

	// Token bound to two topics is ambiguous: still forbidden without a topic.
	multi := e.token(alice, []string{"notifications:publish"}, "backups", "ci")
	if _, err := e.svc.Publish(ctx, multi, domain.Message{Body: "x", TopicFromToken: true}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("multi-topic token: %v", err)
	}
	// Without the flag the old rule holds for single-topic tokens.
	if _, err := e.svc.Publish(ctx, single, domain.Message{Body: "x"}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("no flag: %v", err)
	}
}

func TestApplyToKey(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	alice, bob, carol := e.user("alice", domain.RoleUser), e.user("bob", domain.RoleUser), e.user("carol", domain.RoleUser)
	ci := must(e.svc.CreateTopic(ctx, alice, CreateTopicInput{Name: "ci"}))
	e.share(alice, ci.ID, "bob")

	pub := e.token(alice, []string{"notifications:publish"})
	publish := func(body string) {
		t.Helper()
		if _, err := e.svc.Publish(ctx, pub, domain.Message{Topic: "ci", Body: body, DedupKey: "build-42"}); err != nil {
			t.Fatal(err)
		}
	}
	publish("building")
	publish("passed") // replaces, does not add
	if got := inbox(t, e, bob); len(got) != 1 || got[0].Body != "passed" {
		t.Fatalf("update by key: %+v", got)
	}

	// Clear marks every recipient's copy read and emits a bulk event.
	res, err := e.svc.ApplyToKey(ctx, pub, "ci", "build-42", domain.BulkRead)
	if err != nil || res.Affected != 2 {
		t.Fatalf("clear: %+v %v", res, err)
	}
	for _, p := range []*auth.Principal{alice, bob} {
		if got := inbox(t, e, p); got[0].ReadAt == nil {
			t.Fatalf("%s copy not read", p.User.Username)
		}
	}

	// Outsiders and read-only tokens cannot act on the topic.
	if _, err := e.svc.ApplyToKey(ctx, carol, "alice/ci", "build-42", domain.BulkDelete); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("non-member: %v", err)
	}
	reader := e.token(bob, []string{"notifications:read"})
	if _, err := e.svc.ApplyToKey(ctx, reader, "ci", "build-42", domain.BulkDelete); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("read-only token: %v", err)
	}
	if _, err := e.svc.ApplyToKey(ctx, pub, "ci", "build-42", domain.BulkArchive); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("archive by key: %v", err)
	}

	// Bob, a member, may delete it for everyone (he could already overwrite it).
	res, err = e.svc.ApplyToKey(ctx, bob, "ci", "build-42", domain.BulkDelete)
	if err != nil || res.Affected != 2 {
		t.Fatalf("delete: %+v %v", res, err)
	}
	if len(inbox(t, e, alice)) != 0 || len(inbox(t, e, bob)) != 0 {
		t.Fatal("copies survived delete")
	}
	// Unknown key: nothing to do, not an error (ntfy is idempotent here).
	if res, err := e.svc.ApplyToKey(ctx, pub, "ci", "nope", domain.BulkDelete); err != nil || res.Affected != 0 {
		t.Fatalf("unknown key: %+v %v", res, err)
	}
}

func TestPublishRateLimit(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	e.svc.publishLimit.Store(&publishLimiters{
		limit: domain.DefaultServerSettings().PublishLimit, // kept while the settings don't change
		token: auth.NewRateLimiter(60, 2),                  // 1/s, bursts of 2 per token
		user:  auth.NewRateLimiter(60, 3),                  // 1/s, bursts of 3 per user
	})
	alice := e.user("alice", domain.RoleUser)
	t1 := e.token(alice, []string{"notifications:publish"})
	t2 := e.token(alice, []string{"notifications:publish"})
	pub := func(p *auth.Principal) error {
		_, err := e.svc.Publish(ctx, p, domain.Message{Body: "x"})
		return err
	}
	rateLimited := func(err error) *domain.RateLimitError {
		t.Helper()
		var rl *domain.RateLimitError
		if !errors.As(err, &rl) || rl.RetryAfter <= 0 {
			t.Fatalf("want rate limit error, got %v", err)
		}
		return rl
	}

	// Token limit: two, then 429.
	for i := range 2 {
		if err := pub(t1); err != nil {
			t.Fatalf("burst request %d: %v", i+1, err)
		}
	}
	if rl := rateLimited(pub(t1)); !strings.Contains(rl.Msg, "token") {
		t.Fatalf("message: %q", rl.Msg)
	}
	// User limit spans tokens: t2 gets one more (3 per user), then the user is out.
	if err := pub(t2); err != nil {
		t.Fatal(err)
	}
	if rl := rateLimited(pub(t2)); !strings.Contains(rl.Msg, "user") {
		t.Fatalf("message: %q", rl.Msg)
	}
	// ApplyToKey counts too.
	_, err := e.svc.ApplyToKey(ctx, t2, "x", "k", domain.BulkRead)
	_ = rateLimited(err) // only the kind of error matters here

	// Time refills the buckets (refunds are covered in auth's limiter tests).
	e.advance(time.Second)
	if err := pub(t2); err != nil {
		t.Fatalf("after refill: %v", err)
	}
	// The system principal (CLI) is never limited.
	for range 10 {
		if _, err := e.svc.Publish(ctx, SystemPrincipal(), domain.Message{Body: "x"}); err != nil &&
			errors.As(err, new(*domain.RateLimitError)) {
			t.Fatal("system principal rate limited")
		}
	}
}
