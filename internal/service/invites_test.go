package service

import (
	"context"
	"strings"
	"testing"

	"github.com/oowliq/acksy/internal/auth"
	"github.com/oowliq/acksy/internal/domain"
)

func TestTopicInvitations(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	alice, bob := e.user("alice", domain.RoleUser), e.user("bob", domain.RoleUser)
	carol := e.user("carol", domain.RoleUser)
	tp := must(e.svc.CreateTopic(ctx, alice, CreateTopicInput{Name: "backups", Title: "Nightly backups"}))

	// Unknown users and existing members are reported to the owner.
	_, err := e.svc.InviteToTopic(ctx, alice, tp.ID, "nobody")
	wantKind(t, err, domain.ErrNotFound)
	_, err = e.svc.InviteToTopic(ctx, alice, tp.ID, "alice")
	wantKind(t, err, domain.ErrConflict)
	// Only the owner invites.
	_, err = e.svc.InviteToTopic(ctx, bob, tp.ID, "carol")
	wantKind(t, err, domain.ErrNotFound)

	inv := must(e.svc.InviteToTopic(ctx, alice, tp.ID, "Bob"))
	if inv.Username != "bob" || inv.TopicName != "backups" || inv.OwnerUsername != "alice" {
		t.Fatalf("invite: %+v", inv)
	}
	_, err = e.svc.InviteToTopic(ctx, alice, tp.ID, "bob")
	wantKind(t, err, domain.ErrConflict)

	// Bob is told in his inbox, but receives nothing from the topic yet.
	items := must(e.svc.ListNotifications(ctx, bob, ListQuery{})).Items
	if len(items) != 1 || !strings.Contains(items[0].Title, "alice invited you") ||
		!strings.Contains(items[0].Title, "Nightly backups") || items[0].Topic != nil {
		t.Fatalf("invitation notice: %+v", items)
	}
	if res := must(e.svc.Publish(ctx, alice, domain.Message{Topic: "backups", Body: "x"})); res.Recipients != 1 {
		t.Fatalf("invitee received before accepting: %+v", res)
	}
	// The topic stays hidden from him, and he cannot publish to it.
	_, err = e.svc.GetTopic(ctx, bob, tp.ID)
	wantKind(t, err, domain.ErrNotFound)
	_, err = e.svc.Publish(ctx, bob, domain.Message{Topic: "alice/backups", Body: "x"})
	wantKind(t, err, domain.ErrNotFound)

	if got := must(e.svc.ListTopicInvites(ctx, alice, tp.ID)); len(got) != 1 || got[0].UserID != bob.User.ID {
		t.Fatalf("pending: %+v", got)
	}
	_, err = e.svc.ListTopicInvites(ctx, bob, tp.ID)
	wantKind(t, err, domain.ErrNotFound)
	if mine := must(e.svc.ListMyInvites(ctx, bob)); len(mine) != 1 || mine[0].TopicID != tp.ID {
		t.Fatalf("bob's invitations: %+v", mine)
	}

	// Others cannot answer bob's invitation.
	_, err = e.svc.AcceptTopicInvite(ctx, carol, tp.ID)
	wantKind(t, err, domain.ErrNotFound)

	// A topic-restricted token cannot join anything.
	other := must(e.svc.CreateTopic(ctx, bob, CreateTopicInput{Name: "own"}))
	tok := e.token(bob, []string{"topics:manage", "notifications:read"}, other.Name)
	_, err = e.svc.AcceptTopicInvite(ctx, tok, tp.ID)
	wantKind(t, err, domain.ErrForbidden)
	if mine := must(e.svc.ListMyInvites(ctx, tok)); len(mine) != 0 {
		t.Fatalf("restricted token sees invitations: %+v", mine)
	}

	joined := must(e.svc.AcceptTopicInvite(ctx, bob, tp.ID))
	if joined.Role != domain.MemberMember || joined.MemberCount != 2 {
		t.Fatalf("joined: %+v", joined)
	}
	if res := must(e.svc.Publish(ctx, alice, domain.Message{Topic: "backups", Body: "y"})); res.Recipients != 2 {
		t.Fatalf("after accepting: %+v", res)
	}
	if mine := must(e.svc.ListMyInvites(ctx, bob)); len(mine) != 0 {
		t.Fatalf("invitation left after accepting: %+v", mine)
	}
	_, err = e.svc.AcceptTopicInvite(ctx, bob, tp.ID)
	wantKind(t, err, domain.ErrNotFound)

	// Declining and revoking.
	must(e.svc.InviteToTopic(ctx, alice, tp.ID, "carol"))
	if err := e.svc.DeclineTopicInvite(ctx, carol, tp.ID); err != nil {
		t.Fatal(err)
	}
	wantKind(t, e.svc.DeclineTopicInvite(ctx, carol, tp.ID), domain.ErrNotFound)
	must(e.svc.InviteToTopic(ctx, alice, tp.ID, "carol")) // can be invited again
	wantKind(t, e.svc.RevokeTopicInvite(ctx, bob, tp.ID, carol.User.ID), domain.ErrForbidden)
	if err := e.svc.RevokeTopicInvite(ctx, alice, tp.ID, carol.User.ID); err != nil {
		t.Fatal(err)
	}
	if mine := must(e.svc.ListMyInvites(ctx, carol)); len(mine) != 0 {
		t.Fatalf("revoked invitation: %+v", mine)
	}
	// Answered and revoked invitations leave no notice behind.
	for _, p := range []*auth.Principal{bob, carol} {
		for _, n := range must(e.svc.ListNotifications(ctx, p, ListQuery{View: domain.ViewAll})).Items {
			if strings.Contains(n.Title, "invited you") {
				t.Fatalf("%s kept the invitation notice: %+v", p.User.Username, n)
			}
		}
	}
}
