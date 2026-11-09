package service

import (
	"context"
	"errors"
	"testing"

	"github.com/oowliq/acksy/internal/domain"
)

func TestUnlinkLastIdentityOfPasswordlessSelf(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	other := e.user("root", domain.RoleAdmin)
	sso, err := e.svc.CreateUser(ctx, SystemPrincipal(), CreateUserInput{Username: "sso", Role: domain.RoleAdmin})
	if err != nil {
		t.Fatal(err)
	}
	me := UserPrincipal(sso)
	const iss = "https://idp.example"
	must(e.svc.LinkIdentity(ctx, other, sso.ID, iss, "a"))
	must(e.svc.LinkIdentity(ctx, other, sso.ID, iss, "b"))

	// With two identities, one can go.
	if err := e.svc.UnlinkIdentity(ctx, me, sso.ID, iss, "a"); err != nil {
		t.Fatalf("unlink one of two: %v", err)
	}
	// The last one would lock a password-less user out of their own account.
	if err := e.svc.UnlinkIdentity(ctx, me, sso.ID, iss, "b"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("unlink own last identity: %v, want conflict", err)
	}
	// Another admin may still do it (they can also set a password or disable the user).
	if err := e.svc.UnlinkIdentity(ctx, other, sso.ID, iss, "b"); err != nil {
		t.Fatalf("other admin unlink: %v", err)
	}
}
