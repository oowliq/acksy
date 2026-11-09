package auth

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// OIDCClaims is a verified external identity, reduced to what Acksy uses.
type OIDCClaims struct {
	Issuer  string
	Subject string
	// Username is the configured username claim (preferred_username by default).
	Username string
	Email    string
	Name     string
	// RoleValues are the values of the configured role claim (e.g. groups).
	RoleValues []string
}

// OIDCConfig configures an OIDCProvider.
type OIDCConfig struct {
	Issuer        string
	ClientID      string
	ClientSecret  string
	RedirectURL   string
	Scopes        []string
	UsernameClaim string
	RoleClaim     string
	// HTTPClient talks to the provider; nil uses a client with a 10s timeout.
	HTTPClient *http.Client
}

// OIDCProvider runs the authorization code flow with PKCE against a single
// provider. Discovery happens lazily on first use and is retried after a
// failure, so an unreachable provider does not prevent the server from
// starting.
type OIDCProvider struct {
	cfg OIDCConfig

	mu       sync.Mutex
	oauth    *oauth2.Config
	verifier *oidc.IDTokenVerifier
	provider *oidc.Provider
}

// NewOIDCProvider returns a provider; it does not contact the issuer yet.
func NewOIDCProvider(cfg OIDCConfig) *OIDCProvider {
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &OIDCProvider{cfg: cfg}
}

// Discover checks the provider answers OIDC discovery. Errors describe what
// went wrong without echoing the response: the issuer is an admin-entered URL
// that may point at internal services, whose bodies must not be readable this way.
func (p *OIDCProvider) Discover(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	wellKnown := strings.TrimSuffix(p.cfg.Issuer, "/") + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, wellKnown, nil)
	if err != nil {
		return errors.New("invalid issuer URL")
	}
	resp, err := p.cfg.HTTPClient.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err // drop the URL
		}
		return fmt.Errorf("provider unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("provider answered HTTP %d at /.well-known/openid-configuration", resp.StatusCode)
	}
	if mt, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type")); mt != "application/json" {
		return errors.New("not an OpenID provider: the discovery document is not JSON")
	}
	var meta struct {
		Issuer string `json:"issuer"`
		Auth   string `json:"authorization_endpoint"`
		Token  string `json:"token_endpoint"`
		JWKS   string `json:"jwks_uri"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&meta); err != nil ||
		meta.Auth == "" || meta.Token == "" || meta.JWKS == "" {
		return errors.New("invalid provider metadata: authorization, token or JWKS endpoint missing")
	}
	if meta.Issuer != p.cfg.Issuer {
		return fmt.Errorf("issuer mismatch: the provider calls itself %q", meta.Issuer)
	}
	if _, _, err := p.discover(ctx); err != nil {
		return errors.New("invalid provider metadata")
	}
	return nil
}

// Issuer returns the configured issuer URL.
func (p *OIDCProvider) Issuer() string { return p.cfg.Issuer }

func (p *OIDCProvider) clientCtx(ctx context.Context) context.Context {
	return oidc.ClientContext(ctx, p.cfg.HTTPClient)
}

func (p *OIDCProvider) discover(ctx context.Context) (*oauth2.Config, *oidc.IDTokenVerifier, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.oauth != nil {
		return p.oauth, p.verifier, nil
	}
	// The provider keeps its context for JWKS refreshes, so it must outlive
	// the request that triggered discovery.
	base := p.clientCtx(context.WithoutCancel(ctx))
	dctx, cancel := context.WithTimeout(base, 10*time.Second)
	defer cancel()
	prov, err := oidc.NewProvider(dctx, p.cfg.Issuer)
	if err != nil {
		return nil, nil, fmt.Errorf("oidc discovery: %w", err)
	}
	p.provider = prov
	p.oauth = &oauth2.Config{
		ClientID: p.cfg.ClientID, ClientSecret: p.cfg.ClientSecret, RedirectURL: p.cfg.RedirectURL,
		Endpoint: prov.Endpoint(), Scopes: p.cfg.Scopes,
	}
	p.verifier = prov.VerifierContext(base, &oidc.Config{ClientID: p.cfg.ClientID})
	return p.oauth, p.verifier, nil
}

// OIDCLoginState is the per-attempt secret material bound to the browser.
type OIDCLoginState struct {
	State    string
	Nonce    string
	Verifier string // PKCE code verifier
}

// NewOIDCLoginState generates fresh state, nonce and PKCE verifier.
func NewOIDCLoginState() OIDCLoginState {
	return OIDCLoginState{State: randomString(24), Nonce: randomString(24), Verifier: oauth2.GenerateVerifier()}
}

// AuthCodeURL returns the provider URL to send the browser to.
func (p *OIDCProvider) AuthCodeURL(ctx context.Context, st OIDCLoginState) (string, error) {
	oc, _, err := p.discover(ctx)
	if err != nil {
		return "", err
	}
	return oc.AuthCodeURL(st.State, oidc.Nonce(st.Nonce), oauth2.S256ChallengeOption(st.Verifier)), nil
}

// ErrOIDCNonce means the ID token was not issued for this login attempt.
var ErrOIDCNonce = errors.New("oidc: nonce mismatch")

// Exchange redeems the authorization code, verifies the ID token (signature,
// issuer, audience, expiry and nonce) and extracts the claims. Claims absent
// from the ID token are filled from the UserInfo endpoint when available.
func (p *OIDCProvider) Exchange(ctx context.Context, code string, st OIDCLoginState) (OIDCClaims, error) {
	oc, verifier, err := p.discover(ctx)
	if err != nil {
		return OIDCClaims{}, err
	}
	ctx = p.clientCtx(ctx)
	tok, err := oc.Exchange(ctx, code, oauth2.VerifierOption(st.Verifier))
	if err != nil {
		return OIDCClaims{}, fmt.Errorf("oidc code exchange: %w", err)
	}
	raw, ok := tok.Extra("id_token").(string)
	if !ok || raw == "" {
		return OIDCClaims{}, errors.New("oidc: token response has no id_token")
	}
	idt, err := verifier.Verify(ctx, raw)
	if err != nil {
		return OIDCClaims{}, fmt.Errorf("oidc: verify id token: %w", err)
	}
	// go-oidc leaves nonce validation to the caller.
	if subtle.ConstantTimeCompare([]byte(idt.Nonce), []byte(st.Nonce)) != 1 {
		return OIDCClaims{}, ErrOIDCNonce
	}
	var m map[string]any
	if err := idt.Claims(&m); err != nil {
		return OIDCClaims{}, fmt.Errorf("oidc: decode claims: %w", err)
	}
	if p.needsUserInfo(m) && p.provider.UserInfoEndpoint() != "" {
		p.mergeUserInfo(ctx, tok, idt.Subject, m)
	}
	return OIDCClaims{
		Issuer:     idt.Issuer,
		Subject:    idt.Subject,
		Username:   stringClaim(m, p.cfg.UsernameClaim),
		Email:      stringClaim(m, "email"),
		Name:       stringClaim(m, "name"),
		RoleValues: listClaim(m, p.cfg.RoleClaim),
	}, nil
}

func (p *OIDCProvider) needsUserInfo(m map[string]any) bool {
	for _, k := range []string{p.cfg.UsernameClaim, p.cfg.RoleClaim, "email", "name"} {
		if _, ok := m[k]; k != "" && !ok {
			return true
		}
	}
	return false
}

// mergeUserInfo adds UserInfo claims missing from the ID token. Failures are
// ignored: the ID token alone is enough to sign in.
func (p *OIDCProvider) mergeUserInfo(ctx context.Context, tok *oauth2.Token, subject string, m map[string]any) {
	ui, err := p.provider.UserInfo(ctx, oauth2.StaticTokenSource(tok))
	if err != nil || ui.Subject != subject {
		return
	}
	var um map[string]any
	if err := ui.Claims(&um); err != nil {
		return
	}
	for k, v := range um {
		if _, ok := m[k]; !ok {
			m[k] = v
		}
	}
}

func stringClaim(m map[string]any, key string) string {
	if key == "" {
		return ""
	}
	s, _ := m[key].(string)
	return strings.TrimSpace(s)
}

// listClaim reads a claim that is either a string or a list of strings.
func listClaim(m map[string]any, key string) []string {
	if key == "" {
		return nil
	}
	switch v := m[key].(type) {
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
