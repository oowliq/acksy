package service

import (
	"context"
	"testing"

	"github.com/oowliq/acksy/internal/domain"
)

// An admin API token must not be able to grant itself a way to sign in
// (password, new admin, linked identity): it could then open a login
// session and mint tokens, which invariant 7 forbids.
func TestAdminTokenCannotChangeAccounts(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	root := e.user("root", domain.RoleAdmin)
	alice := e.user("alice", domain.RoleUser)
	tok := e.token(root, []string{"admin"})

	_, err := e.svc.CreateUser(ctx, tok, CreateUserInput{Username: "mallory", Password: "password123", Role: domain.RoleAdmin})
	wantKind(t, err, domain.ErrForbidden)
	pw := "new-password-123"
	_, err = e.svc.UpdateUser(ctx, tok, root.User.ID, UpdateUserInput{Password: &pw})
	wantKind(t, err, domain.ErrForbidden)
	role := domain.RoleAdmin
	_, err = e.svc.UpdateUser(ctx, tok, alice.User.ID, UpdateUserInput{Role: &role})
	wantKind(t, err, domain.ErrForbidden)
	wantKind(t, e.svc.DeleteUser(ctx, tok, alice.User.ID), domain.ErrForbidden)
	_, err = e.svc.LinkIdentity(ctx, tok, root.User.ID, "https://evil.example", "attacker")
	wantKind(t, err, domain.ErrForbidden)
	wantKind(t, e.svc.UnlinkIdentity(ctx, tok, root.User.ID, "https://idp.example", "x"), domain.ErrForbidden)

	// Reading stays available to admin tokens.
	if users := must(e.svc.ListUsers(ctx, tok)); len(users) != 2 {
		t.Fatalf("list users: %+v", users)
	}
	// The session can still do all of it.
	must(e.svc.UpdateUser(ctx, root, alice.User.ID, UpdateUserInput{Password: &pw}))
}

// A dedup key identifies a message within its topic. Publishing into one
// topic (as a member of a shared topic, or with a token restricted to it)
// must not overwrite the recipient's notifications elsewhere.
func TestDedupStaysWithinTopic(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	alice, bob := e.user("alice", domain.RoleUser), e.user("bob", domain.RoleUser)
	shared := must(e.svc.CreateTopic(ctx, bob, CreateTopicInput{Name: "shared"}))
	e.share(bob, shared.ID, "alice")

	own := must(e.svc.Publish(ctx, alice, domain.Message{Title: "Deploy ok", DedupKey: "deploy"}))
	private := must(e.svc.Publish(ctx, alice, domain.Message{Topic: "private", Title: "Backup ok", DedupKey: "deploy"}))
	if private.Deduplicated != 0 || private.NotificationID == own.NotificationID {
		t.Fatalf("different topic must not dedup: %+v vs %+v", private, own)
	}

	// Bob reuses the key in the shared topic.
	res := must(e.svc.Publish(ctx, bob, domain.Message{Topic: "shared", Title: "pwned", DedupKey: "deploy"}))
	if res.Deduplicated != 0 {
		t.Fatalf("bob overwrote alice's notification: %+v", res)
	}
	items := must(e.svc.ListNotifications(ctx, alice, ListQuery{View: domain.ViewAll})).Items
	titles := map[string]bool{}
	for _, n := range items {
		titles[n.Title] = true
	}
	if len(items) != 3 || !titles["Deploy ok"] || !titles["Backup ok"] || !titles["pwned"] {
		t.Fatalf("alice's notifications: %+v", items)
	}

	// Within the topic, the key still updates the message.
	again := must(e.svc.Publish(ctx, bob, domain.Message{Topic: "shared", Title: "fixed", DedupKey: "deploy"}))
	if again.Deduplicated != 2 {
		t.Fatalf("same topic must dedup for every recipient: %+v", again)
	}

	// A token restricted to "private" cannot touch the topic-less one either.
	tok := e.token(alice, []string{"notifications:publish"}, "private")
	must(e.svc.Publish(ctx, tok, domain.Message{Topic: "private", Title: "Backup again", DedupKey: "deploy"}))
	if n := must(e.svc.GetNotification(ctx, alice, own.NotificationID)); n.Title != "Deploy ok" {
		t.Fatalf("topic-less notification changed: %+v", n)
	}

	// Deleting a topic leaves its notifications topic-less; they keep
	// existing next to the topic-less one with the same key.
	if err := e.svc.DeleteTopic(ctx, alice, private.NotificationID); err == nil {
		t.Fatal("expected not found for a notification id")
	}
	priv := must(e.svc.ListTopics(ctx, alice))
	for _, tp := range priv {
		if tp.Name == "private" {
			if err := e.svc.DeleteTopic(ctx, alice, tp.ID); err != nil {
				t.Fatalf("delete topic: %v", err)
			}
		}
	}
	if n := must(e.svc.GetNotification(ctx, alice, private.NotificationID)); n.Topic != nil {
		t.Fatalf("notification of deleted topic: %+v", n)
	}
	// The topic-less key still dedups onto the original notification.
	if r := must(e.svc.Publish(ctx, alice, domain.Message{Title: "Deploy again", DedupKey: "deploy"})); r.NotificationID != own.NotificationID {
		t.Fatalf("topic-less dedup after topic deletion: %+v", r)
	}
}
