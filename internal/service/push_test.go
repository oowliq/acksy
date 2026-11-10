package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/oowliq/acksy/internal/domain"
)

type recordingNotifier struct {
	mu  sync.Mutex
	got []domain.Notification
}

func (r *recordingNotifier) Notify(ns []domain.Notification) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.got = append(r.got, ns...)
}

func (r *recordingNotifier) take() []domain.Notification {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.got
	r.got = nil
	return out
}

func sub(endpoint string, minPrio int) SubscribePushInput {
	return SubscribePushInput{Endpoint: endpoint, P256dh: "BKey", Auth: "auth", Name: "phone", MinPriority: minPrio}
}

func TestPushSubscriptions(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	alice, bob := e.user("alice", domain.RoleUser), e.user("bob", domain.RoleUser)

	s, err := e.svc.SubscribePush(ctx, alice, sub("https://fcm.googleapis.com/fcm/send/abc", 0))
	if err != nil || s.MinPriority != 3 || s.Service != "fcm.googleapis.com" {
		t.Fatalf("subscribe: %+v %v", s, err)
	}
	// Re-subscribing the same browser updates in place.
	again, err := e.svc.SubscribePush(ctx, alice, sub("https://fcm.googleapis.com/fcm/send/abc", 5))
	if err != nil || again.ID != s.ID || again.MinPriority != 5 {
		t.Fatalf("resubscribe: %+v %v", again, err)
	}

	// Tokens cannot manage subscriptions, and input is validated.
	tok := e.token(alice, []string{"notifications:read"})
	if _, err := e.svc.SubscribePush(ctx, tok, sub("https://push.example/x", 3)); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("token: %v", err)
	}
	for _, bad := range []SubscribePushInput{
		sub("http://push.example/x", 3),
		sub("https://push.example/x", 9),
		{Endpoint: "https://push.example/x", MinPriority: 3},
	} {
		if _, err := e.svc.SubscribePush(ctx, alice, bad); !errors.Is(err, domain.ErrInvalid) {
			t.Fatalf("invalid %+v accepted: %v", bad, err)
		}
	}

	// Other users see someone else's subscription as not found.
	prio := 4
	if _, err := e.svc.UpdatePushSubscription(ctx, bob, s.ID, UpdatePushInput{MinPriority: &prio}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("foreign update: %v", err)
	}
	if err := e.svc.DeletePushSubscription(ctx, bob, s.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("foreign delete: %v", err)
	}
	up, err := e.svc.UpdatePushSubscription(ctx, alice, s.ID, UpdatePushInput{MinPriority: &prio})
	if err != nil || up.MinPriority != 4 {
		t.Fatalf("update: %+v %v", up, err)
	}

	// Bob signing in on the same browser takes the subscription over.
	if _, err := e.svc.SubscribePush(ctx, bob, sub("https://fcm.googleapis.com/fcm/send/abc", 3)); err != nil {
		t.Fatal(err)
	}
	if mine := must(e.svc.ListPushSubscriptions(ctx, alice)); len(mine) != 0 {
		t.Fatalf("alice keeps bob's browser: %+v", mine)
	}

	// Per-user cap.
	for i := range domain.MaxPushSubscriptions {
		if _, err := e.svc.SubscribePush(ctx, alice, sub("https://push.example/"+string(rune('a'+i)), 3)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.svc.SubscribePush(ctx, alice, sub("https://push.example/overflow", 3)); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("cap: %v", err)
	}

	// Test push needs a sender.
	if err := e.svc.TestPushSubscription(ctx, bob, s.ID); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("no sender: %v", err)
	}
}

func TestPublishNotifiesAfterCommit(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	rec := &recordingNotifier{}
	e.svc.AddNotifier(rec, nil)
	alice, bob := e.user("alice", domain.RoleUser), e.user("bob", domain.RoleUser)
	tp := must(e.svc.CreateTopic(ctx, alice, CreateTopicInput{Name: "ci"}))
	e.share(alice, tp.ID, "bob")
	if got := rec.take(); len(got) != 1 || got[0].UserID != bob.User.ID || got[0].Topic != nil {
		t.Fatalf("invitation notice: %+v", got)
	}
	pub := e.token(alice, []string{"notifications:publish"})

	must(e.svc.Publish(ctx, pub, domain.Message{Topic: "ci", Body: "built", DedupKey: "b1"}))
	got := rec.take()
	if len(got) != 2 || got[0].Body != "built" || got[0].Topic == nil {
		t.Fatalf("fan-out: %+v", got)
	}

	// A dedup repeat wakes every copy (M3: unread, unsnoozed) and notifies again.
	for _, n := range got {
		if n.UserID == bob.User.ID {
			until := e.clock.Add(time.Hour)
			must(e.svc.UpdateNotificationState(ctx, bob, n.ID, StatePatch{SnoozedUntil: &until, SetSnooze: true}))
		}
	}
	must(e.svc.Publish(ctx, pub, domain.Message{Topic: "ci", Body: "built again", DedupKey: "b1"}))
	got = rec.take()
	if len(got) != 2 || got[0].Body != "built again" || got[0].RepeatCount < 2 {
		t.Fatalf("repeat: %+v", got)
	}

	// Failed publishes notify nobody.
	if _, err := e.svc.Publish(ctx, pub, domain.Message{}); err == nil {
		t.Fatal("empty message accepted")
	}
	if got := rec.take(); len(got) != 0 {
		t.Fatalf("notified on failure: %+v", got)
	}
}

func TestNotifyFiltersQuietNotifications(t *testing.T) {
	e := newEnv(t)
	rec := &recordingNotifier{}
	e.svc.AddNotifier(rec, nil)
	now := e.clock
	past, future := now.Add(-time.Minute), now.Add(time.Hour)
	e.svc.notify(context.Background(), []domain.Notification{
		{ID: "fresh"},
		{ID: "read", ReadAt: &past},
		{ID: "archived", ArchivedAt: &past},
		{ID: "snoozed", SnoozedUntil: &future},
		{ID: "snooze-over", SnoozedUntil: &past},
		{ID: "expired", ExpiresAt: &past},
	})
	got := rec.take()
	if len(got) != 2 || got[0].ID != "fresh" || got[1].ID != "snooze-over" {
		t.Fatalf("filtered: %+v", got)
	}
}

func TestEnsureVAPIDKeys(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	first, err := e.svc.EnsureVAPIDKeys(ctx)
	if err != nil || first.Public == "" || first.Private == "" {
		t.Fatalf("generate: %+v %v", first, err)
	}
	second, err := e.svc.EnsureVAPIDKeys(ctx)
	if err != nil || second != first {
		t.Fatalf("keys must be stable: %+v %v", second, err)
	}
}
