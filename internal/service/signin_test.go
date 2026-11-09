package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oowliq/acksy/internal/auth"
	"github.com/oowliq/acksy/internal/domain"
	"github.com/oowliq/acksy/internal/store"
)

// fakeProvider stands in for *auth.OIDCProvider.
type fakeProvider struct {
	issuer      string
	discoverErr error
}

func (f *fakeProvider) Issuer() string { return f.issuer }
func (f *fakeProvider) AuthCodeURL(context.Context, auth.OIDCLoginState) (string, error) {
	return "https://idp/authorize", nil
}

func (f *fakeProvider) Exchange(context.Context, string, auth.OIDCLoginState) (auth.OIDCClaims, error) {
	return auth.OIDCClaims{}, nil
}
func (f *fakeProvider) Discover(context.Context) error { return f.discoverErr }

func activeOIDC() domain.OIDCSettings {
	s := domain.DefaultOIDCSettings()
	s.Enabled, s.Issuer, s.ClientID, s.ClientSecret = true, testIssuer, "acksy", "s3cret"
	return s
}

// putOIDC writes settings straight to the database, like another instance would.
func putOIDC(t *testing.T, st store.Store, s domain.OIDCSettings, at time.Time) {
	t.Helper()
	b, _ := json.Marshal(s)
	if err := st.PutSetting(context.Background(), domain.OIDCSettingsKey, string(b), at); err != nil {
		t.Fatal(err)
	}
}

// withFactory installs a provider factory that counts builds.
func withFactory(svc *Service) *atomic.Int64 {
	var n atomic.Int64
	svc.opts.NewOIDCProvider = func(s domain.OIDCSettings) OIDCProvider {
		n.Add(1)
		return &fakeProvider{issuer: s.Issuer}
	}
	return &n
}

func TestSignInDefaults(t *testing.T) {
	e := newEnv(t)
	withFactory(e.svc)
	snap := e.svc.SignIn(context.Background())
	if snap.Settings.Enabled || snap.Provider != nil || snap.Settings.Name != "SSO" {
		t.Fatalf("defaults: %+v", snap)
	}
}

func TestSignInReloadsAfterTTL(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	builds := withFactory(e.svc)
	e.svc.opts.SignInTTL = 15 * time.Second
	if _, _, err := e.svc.Login(ctx, "nobody", "x", ""); errors.Is(err, ErrPasswordLoginDisabled) {
		t.Fatal("password login should start enabled")
	}

	// Another instance turns password sign-in off.
	s := activeOIDC()
	s.DisablePasswordLogin = true
	putOIDC(t, e.svc.store, s, e.clock)

	if e.svc.SignIn(ctx).Settings.Enabled {
		t.Fatal("snapshot changed before the TTL")
	}
	e.advance(16 * time.Second)
	snap := e.svc.SignIn(ctx)
	if !snap.Settings.Active() || snap.Provider == nil || snap.Provider.Issuer() != testIssuer {
		t.Fatalf("after TTL: %+v", snap)
	}
	if _, _, err := e.svc.Login(ctx, "nobody", "x", ""); !errors.Is(err, ErrPasswordLoginDisabled) {
		t.Fatalf("login: %v", err)
	}

	// Unchanged settings don't rebuild the provider.
	n := builds.Load()
	e.advance(16 * time.Second)
	e.svc.SignIn(ctx)
	if builds.Load() != n {
		t.Fatal("provider rebuilt without a change")
	}
}

type failingSettings struct{ store.Store }

func (failingSettings) GetSettingWithTime(context.Context, string) (string, time.Time, error) {
	return "", time.Time{}, errors.New("database is down")
}

func TestSignInKeepsSnapshotOnReadError(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	putOIDC(t, e.svc.store, activeOIDC(), e.clock)
	svc := New(failingSettings{e.svc.store}, e.b, slog.New(slog.NewTextHandler(io.Discard, nil)), Options{SignInTTL: time.Second})
	svc.now = func() time.Time { return e.clock }
	withFactory(svc)
	// First load fails: defaults, not a crash.
	if svc.SignIn(ctx).Settings.Enabled {
		t.Fatal("unexpected settings")
	}
	// A good snapshot survives later read errors.
	svc.signIn.Store(&SignInSnapshot{Settings: activeOIDC(), loadedAt: e.clock})
	e.advance(2 * time.Second)
	if !svc.SignIn(ctx).Settings.Enabled {
		t.Fatal("snapshot lost on read error")
	}
}

func saveEnv(t *testing.T) (*env, *auth.Principal) {
	t.Helper()
	e := newEnv(t)
	withFactory(e.svc)
	e.svc.opts.OIDCRedirectURL = "https://acksy.example/api/v1/auth/oidc/callback"
	return e, e.user("root", domain.RoleAdmin)
}

func secretPtr(s string) *string { return &s }

func TestSaveOIDCSettingsAccess(t *testing.T) {
	e, root := saveEnv(t)
	ctx := context.Background()
	in := OIDCSettingsInput{OIDCSettings: activeOIDC()}
	if _, _, err := e.svc.SaveOIDCSettings(ctx, e.user("alice", domain.RoleUser), in); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("non-admin: %v", err)
	}
	if _, _, err := e.svc.SaveOIDCSettings(ctx, e.token(root, []string{"admin"}), in); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("admin token: %v", err)
	}
	bad := in
	bad.Issuer = "idp.example"
	if _, _, err := e.svc.SaveOIDCSettings(ctx, root, bad); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("invalid: %v", err)
	}
	e.svc.opts.OIDCRedirectURL = ""
	if _, _, err := e.svc.SaveOIDCSettings(ctx, root, in); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("no base URL: %v", err)
	}
}

func TestSaveOIDCSettingsSecretAndSnapshot(t *testing.T) {
	e, root := saveEnv(t)
	ctx := context.Background()
	in := OIDCSettingsInput{OIDCSettings: activeOIDC(), ClientSecret: secretPtr("s1")}
	if _, _, err := e.svc.SaveOIDCSettings(ctx, root, in); err != nil {
		t.Fatal(err)
	}
	// Effective here at once, without waiting for the TTL.
	if snap := e.svc.SignIn(ctx); !snap.Settings.Active() || snap.Provider == nil {
		t.Fatalf("snapshot not replaced: %+v", snap)
	}
	in.ClientSecret = nil // keep
	in.Name = "Authentik"
	got, _, err := e.svc.SaveOIDCSettings(ctx, root, in)
	if err != nil || got.ClientSecret != "s1" || got.Name != "Authentik" {
		t.Fatalf("keep secret: %+v %v", got, err)
	}
	in.ClientSecret = secretPtr("") // clear
	if got, _, _ = e.svc.SaveOIDCSettings(ctx, root, in); got.ClientSecret != "" {
		t.Fatal("secret not cleared")
	}
}

func TestSaveOIDCSettingsLockout(t *testing.T) {
	e, root := saveEnv(t)
	ctx := context.Background()
	in := OIDCSettingsInput{OIDCSettings: activeOIDC()}
	in.DisablePasswordLogin = true
	if _, _, err := e.svc.SaveOIDCSettings(ctx, root, in); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("no own identity: %v", err)
	}
	must(e.svc.LinkIdentity(ctx, root, root.User.ID, testIssuer, "root-sub"))
	if _, _, err := e.svc.SaveOIDCSettings(ctx, root, in); err != nil {
		t.Fatalf("with identity: %v", err)
	}
	// Moving to another provider needs an identity there too.
	in.Issuer = "https://other-idp.example"
	if _, _, err := e.svc.SaveOIDCSettings(ctx, root, in); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("other issuer: %v", err)
	}
}

func TestTestOIDCSettings(t *testing.T) {
	e, root := saveEnv(t)
	ctx := context.Background()
	if err := e.svc.TestOIDCSettings(ctx, root, OIDCSettingsInput{OIDCSettings: activeOIDC()}); err != nil {
		t.Fatalf("ok provider: %v", err)
	}
	e.svc.opts.NewOIDCProvider = func(domain.OIDCSettings) OIDCProvider {
		return &fakeProvider{discoverErr: errors.New("no such host")}
	}
	if err := e.svc.TestOIDCSettings(ctx, root, OIDCSettingsInput{OIDCSettings: activeOIDC()}); err == nil {
		t.Fatal("failing provider passed")
	}
	if e.svc.SignIn(ctx).Settings.Enabled {
		t.Fatal("test must not save")
	}
}

func TestAllowPasswordLogin(t *testing.T) {
	e, root := saveEnv(t)
	ctx := context.Background()
	must(e.svc.LinkIdentity(ctx, root, root.User.ID, testIssuer, "root-sub"))
	in := OIDCSettingsInput{OIDCSettings: activeOIDC(), ClientSecret: secretPtr("keep-me")}
	in.DisablePasswordLogin = true
	if _, _, err := e.svc.SaveOIDCSettings(ctx, root, in); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.AllowPasswordLogin(ctx, SystemPrincipal()); err != nil {
		t.Fatal(err)
	}
	got, _, err := e.svc.OIDCSettings(ctx, root)
	if err != nil || got.DisablePasswordLogin || got.ClientSecret != "keep-me" || !got.Enabled {
		t.Fatalf("after allow: %+v %v", got, err)
	}
}

// txGuard fails the test if settings are read outside the open transaction:
// SQLite has a single connection, held by the transaction (deadlock).
type txGuard struct {
	store.Store
	t    *testing.T
	inTx atomic.Bool
}

func (g *txGuard) InTx(ctx context.Context, fn func(store.Querier) error) error {
	g.inTx.Store(true)
	defer g.inTx.Store(false)
	return g.Store.InTx(ctx, fn)
}

func (g *txGuard) GetSettingWithTime(ctx context.Context, key string) (string, time.Time, error) {
	if g.inTx.Load() {
		// Calling through would block forever on SQLite: fail fast instead.
		g.t.Error("settings read while a transaction holds the connection")
		return "", time.Time{}, errors.New("settings read inside a transaction")
	}
	return g.Store.GetSettingWithTime(ctx, key)
}

func TestLoginOIDCReadsSettingsOutsideTx(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	st := activeOIDC()
	st.AdminValues = []string{"admins"}
	putOIDC(t, e.svc.store, st, e.clock)
	g := &txGuard{Store: e.svc.store, t: t}
	svc := New(g, e.b, slog.New(slog.DiscardHandler), Options{SignInTTL: time.Second})
	svc.now = func() time.Time { return e.clock }
	withFactory(svc)
	svc.SignIn(ctx)
	e.advance(2 * time.Second) // TTL expired: the next read goes to the database
	if _, _, err := svc.LoginOIDC(ctx, auth.OIDCClaims{Issuer: testIssuer, Subject: "s1", Username: "neo", RoleValues: []string{"admins"}}, "ua"); err != nil {
		t.Fatal(err)
	}
}

func TestSignInFailsClosedOnFirstReadError(t *testing.T) {
	e := newEnv(t)
	svc := New(failingSettings{e.svc.store}, e.b, slog.New(slog.DiscardHandler), Options{})
	svc.now = func() time.Time { return e.clock }
	if !svc.SignIn(context.Background()).Settings.DisablePasswordLogin {
		t.Fatal("password sign-in must stay off when the settings can't be read")
	}
}

func TestImportOIDCSettingsNeedsBaseURL(t *testing.T) {
	e := newEnv(t)
	if _, err := e.svc.ImportOIDCSettings(context.Background(), activeOIDC()); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("import without base URL: %v", err)
	}
}

func TestUnlinkCurrentIdentityWithPasswordOff(t *testing.T) {
	e, root := saveEnv(t)
	ctx := context.Background()
	must(e.svc.LinkIdentity(ctx, root, root.User.ID, testIssuer, "current"))
	must(e.svc.LinkIdentity(ctx, root, root.User.ID, "https://old-idp.example", "old"))
	in := OIDCSettingsInput{OIDCSettings: activeOIDC()}
	in.DisablePasswordLogin = true
	if _, _, err := e.svc.SaveOIDCSettings(ctx, root, in); err != nil {
		t.Fatal(err)
	}
	// root has a password, but password sign-in is off: the current provider is the only way in.
	if err := e.svc.UnlinkIdentity(ctx, root, root.User.ID, "https://old-idp.example", "old"); err != nil {
		t.Fatalf("old provider: %v", err)
	}
	if err := e.svc.UnlinkIdentity(ctx, root, root.User.ID, testIssuer, "current"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("current provider: %v, want conflict", err)
	}
}
