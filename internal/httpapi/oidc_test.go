package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/oowliq/acksy/internal/auth"
	"github.com/oowliq/acksy/internal/auth/oidctest"
	"github.com/oowliq/acksy/internal/domain"
	"github.com/oowliq/acksy/internal/service"
)

// oidcPolicy is the part of the OIDC settings these tests vary.
type oidcPolicy struct {
	AutoCreate  bool
	AdminValues []string
}

// oidcFactory builds providers that talk to the fake IdP.
func oidcFactory(idp *oidctest.IdP) func(domain.OIDCSettings) service.OIDCProvider {
	return func(s domain.OIDCSettings) service.OIDCProvider {
		return auth.NewOIDCProvider(auth.OIDCConfig{
			Issuer: s.Issuer, ClientID: s.ClientID, ClientSecret: s.ClientSecret,
			RedirectURL: "https://acksy.test/api/v1/auth/oidc/callback",
			Scopes:      s.Scopes, UsernameClaim: s.UsernameClaim, RoleClaim: s.RoleClaim,
			HTTPClient: idp.Client(),
		})
	}
}

func newOIDCAPI(t *testing.T, policy oidcPolicy, passwordOff bool) (http.Handler, *service.Service, *oidctest.IdP) {
	t.Helper()
	idp := oidctest.New(t)
	h, svc := newAPIWithService(t, Options{}, service.Options{
		NewOIDCProvider: oidcFactory(idp), OIDCRedirectURL: "https://acksy.test/api/v1/auth/oidc/callback",
	})
	st := domain.DefaultOIDCSettings()
	st.Enabled, st.Issuer, st.ClientID, st.ClientSecret, st.Name = true, oidctest.Issuer, idp.ClientID, idp.ClientSecret, "Authentik"
	st.Scopes = []string{"openid"}
	st.AutoCreate, st.AdminValues, st.DisablePasswordLogin = policy.AutoCreate, policy.AdminValues, passwordOff
	if _, err := svc.ImportOIDCSettings(context.Background(), st); err != nil {
		t.Fatal(err)
	}
	return h, svc, idp
}

// browser keeps cookies like a user agent would for one origin.
type browser struct {
	t       *testing.T
	h       http.Handler
	cookies map[string]*http.Cookie
}

func (b *browser) get(target string) *httptest.ResponseRecorder {
	b.t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	for _, c := range b.cookies {
		if strings.HasPrefix(req.URL.Path, c.Path) {
			req.AddCookie(c)
		}
	}
	rec := httptest.NewRecorder()
	b.h.ServeHTTP(rec, req)
	for _, c := range rec.Result().Cookies() {
		if c.MaxAge < 0 {
			delete(b.cookies, c.Name)
			continue
		}
		if c.Path == "" {
			c.Path = "/"
		}
		b.cookies[c.Name] = c
	}
	return rec
}

// signIn starts the flow, lets the IdP authorize, and follows the callback.
func (b *browser) signIn(idp *oidctest.IdP, start string) *httptest.ResponseRecorder {
	b.t.Helper()
	rec := b.get(start)
	expect(b.t, rec, http.StatusFound)
	oc := b.cookies[oidcCookie]
	if oc == nil || !oc.HttpOnly || oc.SameSite != http.SameSiteLaxMode || oc.Path != oidcCookiePath {
		b.t.Fatalf("bad oidc cookie: %+v", oc)
	}
	cb, err := idp.Authorize(rec.Header().Get("Location"))
	if err != nil {
		b.t.Fatal(err)
	}
	u, _ := url.Parse(cb)
	rec = b.get(u.RequestURI())
	if b.cookies[oidcCookie] != nil {
		b.t.Fatal("oidc cookie not cleared after callback")
	}
	return rec
}

func TestOIDCSignInFlow(t *testing.T) {
	h, _, idp := newOIDCAPI(t, oidcPolicy{AutoCreate: true, AdminValues: []string{"admins"}}, false)
	idp.SetUser("sub-1", map[string]any{"preferred_username": "alice", "groups": []string{"admins"}})

	b := &browser{t: t, h: h, cookies: map[string]*http.Cookie{}}
	rec := b.signIn(idp, "/api/v1/auth/oidc/login?redirect=/topics/ci")
	expect(t, rec, http.StatusFound)
	if loc := rec.Header().Get("Location"); loc != "/topics/ci" {
		t.Fatalf("redirect %q", loc)
	}
	sess := b.cookies[SessionCookie]
	if sess == nil {
		t.Fatal("no session cookie")
	}
	c := &client{t: t, h: h, cookie: sess}
	me := decode[meResponse](t, c.do(http.MethodGet, "/api/v1/me", nil))
	if me.User.Username != "alice" || !me.User.IsAdmin() || me.Auth.Type != "session" {
		t.Fatalf("me: %+v", me)
	}
	if me.Auth.HasPassword {
		t.Fatal("OIDC-provisioned user has no password")
	}
}

func TestOIDCCallbackFailures(t *testing.T) {
	loginError := func(t *testing.T, rec *httptest.ResponseRecorder, code string) {
		t.Helper()
		expect(t, rec, http.StatusFound)
		if loc := rec.Header().Get("Location"); loc != "/login?error="+code {
			t.Fatalf("location %q, want error %s", loc, code)
		}
	}

	t.Run("open redirect is neutralised", func(t *testing.T) {
		h, _, idp := newOIDCAPI(t, oidcPolicy{AutoCreate: true}, false)
		idp.SetUser("s", nil)
		b := &browser{t: t, h: h, cookies: map[string]*http.Cookie{}}
		rec := b.signIn(idp, "/api/v1/auth/oidc/login?redirect=//evil.example/x")
		if loc := rec.Header().Get("Location"); loc != "/" {
			t.Fatalf("redirect %q", loc)
		}
	})
	t.Run("state without cookie", func(t *testing.T) {
		h, _, _ := newOIDCAPI(t, oidcPolicy{AutoCreate: true}, false)
		b := &browser{t: t, h: h, cookies: map[string]*http.Cookie{}}
		loginError(t, b.get("/api/v1/auth/oidc/callback?code=x&state=y"), "oidc_expired")
	})
	t.Run("state mismatch", func(t *testing.T) {
		h, _, idp := newOIDCAPI(t, oidcPolicy{AutoCreate: true}, false)
		idp.SetUser("s", nil)
		b := &browser{t: t, h: h, cookies: map[string]*http.Cookie{}}
		cb, err := idp.Authorize(b.get("/api/v1/auth/oidc/login").Header().Get("Location"))
		if err != nil {
			t.Fatal(err)
		}
		u, _ := url.Parse(cb)
		q := u.Query()
		q.Set("state", "forged")
		loginError(t, b.get("/api/v1/auth/oidc/callback?"+q.Encode()), "oidc_expired")
	})
	t.Run("provider error", func(t *testing.T) {
		h, _, _ := newOIDCAPI(t, oidcPolicy{AutoCreate: true}, false)
		b := &browser{t: t, h: h, cookies: map[string]*http.Cookie{}}
		b.get("/api/v1/auth/oidc/login")
		loginError(t, b.get("/api/v1/auth/oidc/callback?error=access_denied"), "oidc_denied")
	})
	t.Run("nonce replay", func(t *testing.T) {
		h, _, idp := newOIDCAPI(t, oidcPolicy{AutoCreate: true}, false)
		idp.SetUser("s", nil)
		idp.SetNonceOverride("other")
		b := &browser{t: t, h: h, cookies: map[string]*http.Cookie{}}
		loginError(t, b.signIn(idp, "/api/v1/auth/oidc/login"), "oidc_failed")
	})
	t.Run("no linked account", func(t *testing.T) {
		h, _, idp := newOIDCAPI(t, oidcPolicy{AutoCreate: false}, false)
		idp.SetUser("s", nil)
		b := &browser{t: t, h: h, cookies: map[string]*http.Cookie{}}
		loginError(t, b.signIn(idp, "/api/v1/auth/oidc/login"), "oidc_no_account")
		if b.cookies[SessionCookie] != nil {
			t.Fatal("session issued without account")
		}
	})
	t.Run("disabled account", func(t *testing.T) {
		h, svc, idp := newOIDCAPI(t, oidcPolicy{AutoCreate: true}, false)
		idp.SetUser("s", map[string]any{"preferred_username": "carol"})
		b := &browser{t: t, h: h, cookies: map[string]*http.Cookie{}}
		b.signIn(idp, "/api/v1/auth/oidc/login")
		u, err := svc.GetUserByUsername(context.Background(), service.SystemPrincipal(), "carol")
		if err != nil {
			t.Fatal(err)
		}
		off := true
		if _, err := svc.UpdateUser(context.Background(), service.SystemPrincipal(), u.ID, service.UpdateUserInput{Disabled: &off}); err != nil {
			t.Fatal(err)
		}
		b2 := &browser{t: t, h: h, cookies: map[string]*http.Cookie{}}
		loginError(t, b2.signIn(idp, "/api/v1/auth/oidc/login"), "account_disabled")
	})
}

func TestAuthMethods(t *testing.T) {
	type methods struct {
		Password bool `json:"password"`
		OIDC     *struct {
			Name     string `json:"name"`
			LoginURL string `json:"login_url"`
		} `json:"oidc"`
	}
	h, _ := newAPI(t)
	anon := &client{t: t, h: h}
	m := decode[methods](t, anon.do(http.MethodGet, "/api/v1/auth/methods", nil))
	if !m.Password || m.OIDC != nil {
		t.Fatalf("password-only: %+v", m)
	}

	h, svc, _ := newOIDCAPI(t, oidcPolicy{}, true)
	seedUsers(t, svc)
	anon = &client{t: t, h: h}
	m = decode[methods](t, anon.do(http.MethodGet, "/api/v1/auth/methods", nil))
	if m.Password || m.OIDC == nil || m.OIDC.Name != "Authentik" || m.OIDC.LoginURL != "/api/v1/auth/oidc/login" {
		t.Fatalf("oidc-only: %+v", m)
	}
	rec := anon.do(http.MethodPost, "/api/v1/auth/login", map[string]string{"username": "root", "password": "password123"})
	expect(t, rec, http.StatusForbidden)

	// A stale session cookie must not break the public endpoint.
	stale := &client{t: t, h: h, cookie: &http.Cookie{Name: SessionCookie, Value: "stale"}}
	expect(t, stale.do(http.MethodGet, "/api/v1/auth/methods", nil), http.StatusOK)
}
