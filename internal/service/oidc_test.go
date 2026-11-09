package service

import (
	"context"
	"errors"
	"testing"

	"github.com/oowliq/acksy/internal/auth"
	"github.com/oowliq/acksy/internal/domain"
)

const testIssuer = "https://idp.test"

func claims(sub, username string, groups ...string) auth.OIDCClaims {
	return auth.OIDCClaims{Issuer: testIssuer, Subject: sub, Username: username, Email: username + "@example.com", Name: "N " + username, RoleValues: groups}
}

// oidcPolicy is the part of the OIDC settings these tests vary.
type oidcPolicy struct {
	AutoCreate  bool
	AdminValues []string
}

func oidcEnv(t *testing.T, policy oidcPolicy) *env {
	t.Helper()
	e := newEnv(t)
	st := activeOIDC()
	st.AutoCreate, st.AdminValues = policy.AutoCreate, policy.AdminValues
	e.setOIDC(t, st)
	return e
}

// setOIDC stores settings and makes them effective on this instance at once.
func (e *env) setOIDC(t *testing.T, st domain.OIDCSettings) {
	t.Helper()
	putOIDC(t, e.svc.store, st, e.clock)
	e.svc.replaceSignIn(st, e.clock)
}

func TestLoginOIDCProvisionsAndReuses(t *testing.T) {
	ctx := context.Background()
	e := oidcEnv(t, oidcPolicy{AutoCreate: true})

	secret, u, err := e.svc.LoginOIDC(ctx, claims("s1", "Alice"), "ua")
	if err != nil {
		t.Fatal(err)
	}
	if u.Username != "alice" || u.Role != domain.RoleUser || u.Email != "Alice@example.com" || u.DisplayName != "N Alice" || u.PasswordHash != "" {
		t.Fatalf("provisioned user: %+v", u)
	}
	p, err := e.svc.AuthenticateSession(ctx, secret)
	if err != nil || p.User.ID != u.ID {
		t.Fatalf("session: %v", err)
	}

	// Second login: same user, even if the username claim changed.
	_, again, err := e.svc.LoginOIDC(ctx, claims("s1", "alice-renamed"), "ua")
	if err != nil || again.ID != u.ID {
		t.Fatalf("relogin: %+v %v", again, err)
	}
	ids, err := e.svc.ListIdentities(ctx, SystemPrincipal(), u.ID)
	if err != nil || len(ids) != 1 || ids[0].LastLoginAt == nil {
		t.Fatalf("identities: %+v %v", ids, err)
	}
	// Password login stays impossible for a passwordless OIDC user.
	if _, _, err := e.svc.Login(ctx, "alice", "", "ua"); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("password login: %v", err)
	}
}

func TestLoginOIDCNeverLinksExistingUser(t *testing.T) {
	ctx := context.Background()
	e := oidcEnv(t, oidcPolicy{AutoCreate: true})
	local := e.user("alice", domain.RoleAdmin)

	_, u, err := e.svc.LoginOIDC(ctx, claims("s1", "alice"), "ua")
	if err != nil {
		t.Fatal(err)
	}
	if u.ID == local.User.ID || u.Username != "alice-2" || u.IsAdmin() {
		t.Fatalf("OIDC login took over local account: %+v", u)
	}
	_, u3, err := e.svc.LoginOIDC(ctx, claims("s2", "alice"), "ua")
	if err != nil || u3.Username != "alice-3" {
		t.Fatalf("third alice: %+v %v", u3, err)
	}
}

func TestLoginOIDCWithoutAutoCreate(t *testing.T) {
	ctx := context.Background()
	e := oidcEnv(t, oidcPolicy{AutoCreate: false})
	alice := e.user("alice", domain.RoleUser)

	if _, _, err := e.svc.LoginOIDC(ctx, claims("s1", "alice"), "ua"); err != ErrOIDCNoAccount { //nolint:errorlint // sentinel identity
		t.Fatalf("want ErrOIDCNoAccount, got %v", err)
	}
	if _, err := e.svc.LinkIdentity(ctx, alice, alice.User.ID, "", "s1"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("non-admin link: %v", err)
	}
	ident, err := e.svc.LinkIdentity(ctx, SystemPrincipal(), alice.User.ID, "", "s1")
	if err != nil || ident.Issuer != testIssuer {
		t.Fatalf("link: %+v %v", ident, err)
	}
	if _, err := e.svc.LinkIdentity(ctx, SystemPrincipal(), alice.User.ID, "", "s1"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("double link: %v", err)
	}
	_, u, err := e.svc.LoginOIDC(ctx, claims("s1", "whatever"), "ua")
	if err != nil || u.ID != alice.User.ID {
		t.Fatalf("linked login: %+v %v", u, err)
	}
	if err := e.svc.UnlinkIdentity(ctx, SystemPrincipal(), alice.User.ID, "", "s1"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.svc.LoginOIDC(ctx, claims("s1", "alice"), "ua"); err != ErrOIDCNoAccount { //nolint:errorlint // sentinel identity
		t.Fatalf("after unlink: %v", err)
	}
}

func TestLoginOIDCDisabledUser(t *testing.T) {
	ctx := context.Background()
	e := oidcEnv(t, oidcPolicy{AutoCreate: true})
	_, u, err := e.svc.LoginOIDC(ctx, claims("s1", "alice"), "ua")
	if err != nil {
		t.Fatal(err)
	}
	disabled := true
	if _, err := e.svc.UpdateUser(ctx, SystemPrincipal(), u.ID, UpdateUserInput{Disabled: &disabled}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.svc.LoginOIDC(ctx, claims("s1", "alice"), "ua"); err != ErrAccountDisabled { //nolint:errorlint // sentinel identity
		t.Fatalf("want ErrAccountDisabled, got %v", err)
	}
}

func TestLoginOIDCRoleSync(t *testing.T) {
	ctx := context.Background()
	e := oidcEnv(t, oidcPolicy{AutoCreate: true, AdminValues: []string{"admins"}})

	_, u, err := e.svc.LoginOIDC(ctx, claims("s1", "alice", "admins"), "ua")
	if err != nil || !u.IsAdmin() {
		t.Fatalf("admin on create: %+v %v", u, err)
	}
	// Alice is the only admin: losing the group does not demote her.
	_, u, err = e.svc.LoginOIDC(ctx, claims("s1", "alice", "users"), "ua")
	if err != nil || !u.IsAdmin() {
		t.Fatalf("last admin demoted: %+v %v", u, err)
	}

	_, bob, err := e.svc.LoginOIDC(ctx, claims("s2", "bob", "users"), "ua")
	if err != nil || bob.IsAdmin() {
		t.Fatalf("bob: %+v %v", bob, err)
	}
	_, bob, err = e.svc.LoginOIDC(ctx, claims("s2", "bob", "admins"), "ua")
	if err != nil || !bob.IsAdmin() {
		t.Fatalf("bob promoted: %+v %v", bob, err)
	}
	// With another admin around, alice is demoted now.
	_, u, err = e.svc.LoginOIDC(ctx, claims("s1", "alice"), "ua")
	if err != nil || u.IsAdmin() {
		t.Fatalf("alice demoted: %+v %v", u, err)
	}
	stored := must(e.svc.GetUser(ctx, SystemPrincipal(), u.ID))
	if stored.IsAdmin() {
		t.Fatal("demotion not persisted")
	}
}

func TestLoginOIDCRoleSyncOffKeepsLocalRole(t *testing.T) {
	ctx := context.Background()
	e := oidcEnv(t, oidcPolicy{AutoCreate: true})
	_, u, err := e.svc.LoginOIDC(ctx, claims("s1", "alice", "admins"), "ua")
	if err != nil || u.IsAdmin() {
		t.Fatalf("role mapped without admin values: %+v %v", u, err)
	}
	role := domain.RoleAdmin
	if _, err := e.svc.UpdateUser(ctx, SystemPrincipal(), u.ID, UpdateUserInput{Role: &role}); err != nil {
		t.Fatal(err)
	}
	_, u, err = e.svc.LoginOIDC(ctx, claims("s1", "alice"), "ua")
	if err != nil || !u.IsAdmin() {
		t.Fatalf("local role overwritten: %+v %v", u, err)
	}
}

func TestPasswordLoginDisabled(t *testing.T) {
	e := newEnv(t)
	e.user("alice", domain.RoleUser)
	st := activeOIDC()
	st.DisablePasswordLogin = true
	e.setOIDC(t, st)
	_, _, err := e.svc.Login(context.Background(), "alice", "password123", "ua")
	if err != ErrPasswordLoginDisabled { //nolint:errorlint // sentinel identity
		t.Fatalf("want ErrPasswordLoginDisabled, got %v", err)
	}
}
