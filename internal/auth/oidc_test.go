package auth_test

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"testing"

	"github.com/oowliq/acksy/internal/auth"
	"github.com/oowliq/acksy/internal/auth/oidctest"
)

func newProvider(t *testing.T) (*auth.OIDCProvider, *oidctest.IdP) {
	t.Helper()
	idp := oidctest.New(t)
	p := auth.NewOIDCProvider(auth.OIDCConfig{
		Issuer: oidctest.Issuer, ClientID: idp.ClientID, ClientSecret: idp.ClientSecret,
		RedirectURL: "https://acksy.test/api/v1/auth/oidc/callback",
		Scopes:      []string{"openid", "profile", "email"}, UsernameClaim: "preferred_username", RoleClaim: "groups",
		HTTPClient: idp.Client(),
	})
	return p, idp
}

// login runs the full code flow and returns the claims (or the error).
func login(t *testing.T, p *auth.OIDCProvider, idp *oidctest.IdP, mutate func(*auth.OIDCLoginState, url.Values)) (auth.OIDCClaims, error) {
	t.Helper()
	ctx := context.Background()
	st := auth.NewOIDCLoginState()
	authURL, err := p.AuthCodeURL(ctx, st)
	if err != nil {
		t.Fatal(err)
	}
	cb, err := idp.Authorize(authURL)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(cb)
	q := u.Query()
	if q.Get("state") != st.State {
		t.Fatalf("state not echoed: %q", q.Get("state"))
	}
	if mutate != nil {
		mutate(&st, q)
	}
	return p.Exchange(ctx, q.Get("code"), st)
}

func TestOIDCExchange(t *testing.T) {
	p, idp := newProvider(t)
	idp.SetUser("sub-1", map[string]any{
		"preferred_username": "alice", "email": "alice@example.com", "name": "Alice",
		"groups": []string{"users", "acksy-admins"},
	})
	c, err := login(t, p, idp, nil)
	if err != nil {
		t.Fatal(err)
	}
	if c.Issuer != oidctest.Issuer || c.Subject != "sub-1" || c.Username != "alice" || c.Email != "alice@example.com" ||
		c.Name != "Alice" || !slices.Equal(c.RoleValues, []string{"users", "acksy-admins"}) {
		t.Fatalf("claims: %+v", c)
	}
}

func TestOIDCExchangeStringRoleAndUserInfo(t *testing.T) {
	p, idp := newProvider(t)
	idp.SetUser("sub-2", map[string]any{"groups": "ops"})
	idp.SetUserInfo(map[string]any{"preferred_username": "bob", "email": "bob@example.com", "groups": []string{"ignored"}})
	c, err := login(t, p, idp, nil)
	if err != nil {
		t.Fatal(err)
	}
	// ID token claims win; missing ones come from userinfo.
	if c.Username != "bob" || c.Email != "bob@example.com" || !slices.Equal(c.RoleValues, []string{"ops"}) {
		t.Fatalf("claims: %+v", c)
	}
}

func TestOIDCExchangeRejects(t *testing.T) {
	t.Run("nonce mismatch", func(t *testing.T) {
		p, idp := newProvider(t)
		idp.SetUser("sub", nil)
		idp.SetNonceOverride("replayed")
		if _, err := login(t, p, idp, nil); !errors.Is(err, auth.ErrOIDCNonce) {
			t.Fatalf("want nonce error, got %v", err)
		}
	})
	t.Run("wrong PKCE verifier", func(t *testing.T) {
		p, idp := newProvider(t)
		idp.SetUser("sub", nil)
		_, err := login(t, p, idp, func(st *auth.OIDCLoginState, _ url.Values) { st.Verifier = auth.NewOIDCLoginState().Verifier })
		if err == nil {
			t.Fatal("exchange with wrong verifier succeeded")
		}
	})
	t.Run("unknown code", func(t *testing.T) {
		p, idp := newProvider(t)
		idp.SetUser("sub", nil)
		_, err := login(t, p, idp, func(_ *auth.OIDCLoginState, q url.Values) { q.Set("code", "forged") })
		if err == nil {
			t.Fatal("exchange with forged code succeeded")
		}
	})
}
