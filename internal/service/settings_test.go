package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oowliq/acksy/internal/auth"
	"github.com/oowliq/acksy/internal/domain"
)

// sessionOf marks p as a login session (no token), as UserPrincipal is.
func sessionOf(p *auth.Principal) *auth.Principal {
	s := *p
	s.SessionID = "session"
	return &s
}

func TestServerSettingsAccess(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	root := e.user("root", domain.RoleAdmin)
	bob := e.user("bob", domain.RoleUser)

	if _, _, err := e.svc.ServerSettings(ctx, e.token(root, []string{"admin"})); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("admin token: %v", err)
	}
	if _, _, err := e.svc.ServerSettings(ctx, sessionOf(bob)); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("user: %v", err)
	}
	s, at, err := e.svc.ServerSettings(ctx, sessionOf(root))
	if err != nil || !at.IsZero() || s.SessionDays != 30 {
		t.Fatalf("defaults: %+v %v %v", s, at, err)
	}
	s.SessionDays = 0
	if _, _, err := e.svc.SaveServerSettings(ctx, sessionOf(root), s); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("invalid saved: %v", err)
	}
	s.SessionDays = 3
	if _, _, err := e.svc.SaveServerSettings(ctx, sessionOf(bob), s); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("user saved: %v", err)
	}
	if _, at, err := e.svc.SaveServerSettings(ctx, sessionOf(root), s); err != nil || at.IsZero() {
		t.Fatalf("save: %v", err)
	}
	if got := e.svc.SessionTTL(ctx); got != 3*24*time.Hour {
		t.Fatalf("not effective at once: %v", got)
	}
}

// Another instance picks up a change within SignInTTL.
func TestServerSettingsReload(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	other := New(e.svc.store, e.b, e.svc.log, Options{})
	other.now = e.svc.now
	if !other.Settings(ctx).Adapters.NTFY {
		t.Fatal("default ntfy off")
	}
	root := sessionOf(e.user("root", domain.RoleAdmin))
	s, _, _ := e.svc.ServerSettings(ctx, root)
	s.Adapters.NTFY = false
	if _, _, err := e.svc.SaveServerSettings(ctx, root, s); err != nil {
		t.Fatal(err)
	}
	if !other.Settings(ctx).Adapters.NTFY {
		t.Fatal("cached snapshot should still be used")
	}
	e.advance(DefaultSignInTTL)
	if other.Settings(ctx).Adapters.NTFY {
		t.Fatal("change not picked up after the TTL")
	}
}

func TestSwitchedOffDelivery(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	alice := e.user("alice", domain.RoleUser)
	push, always := &recordingNotifier{}, &recordingNotifier{}
	e.svc.AddNotifier(push, func(s domain.ServerSettings) bool { return s.Push.Enabled })
	e.svc.AddNotifier(always, nil)
	e.svc.SetActionRunner(&fakeRunner{result: ActionResult{OK: true}})

	s := domain.DefaultServerSettings()
	s.Push.Enabled, s.Actions.Enabled = false, false
	e.putSettings(s)

	res := must(e.svc.Publish(ctx, alice, domain.Message{Body: "x", Actions: []domain.Action{
		{Type: "http", Label: "Go", URL: "https://ha.example/api"},
	}}))
	if len(push.take()) != 0 || len(always.take()) != 1 {
		t.Fatal("push notifier not gated by the settings")
	}
	if _, err := e.svc.RunAction(ctx, alice, res.NotificationID, 0); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("action ran while switched off: %v", err)
	}
}

func TestPublishLimitFromSettings(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	alice := e.user("alice", domain.RoleUser)
	tok := e.token(alice, []string{"notifications:publish"})
	s := domain.DefaultServerSettings()
	s.PublishLimit.PerToken = 1
	e.putSettings(s)
	must(e.svc.Publish(ctx, tok, domain.Message{Body: "1"}))
	var rl *domain.RateLimitError
	if _, err := e.svc.Publish(ctx, tok, domain.Message{Body: "2"}); !errors.As(err, &rl) {
		t.Fatalf("limit from settings not applied: %v", err)
	}
	s.PublishLimit.PerToken = 0
	e.putSettings(s)
	if _, err := e.svc.Publish(ctx, tok, domain.Message{Body: "3"}); err != nil {
		t.Fatalf("limit not lifted: %v", err)
	}
}
