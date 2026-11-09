// Package oidctest is an in-process OpenID Connect provider for tests. It
// serves discovery, JWKS, token and userinfo endpoints through an
// http.Client transport, so no ports are opened.
package oidctest

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
)

// Issuer is the fake provider's issuer URL.
const Issuer = "https://idp.test"

// IdP is a fake OpenID Connect provider.
type IdP struct {
	ClientID     string
	ClientSecret string

	key *rsa.PrivateKey

	mu    sync.Mutex
	codes map[string]pending
	// user is who "signs in" at the next Authorize call.
	sub      string
	claims   map[string]any
	userinfo map[string]any
	// nonceOverride replaces the nonce in issued ID tokens when non-empty.
	nonceOverride string
}

type pending struct {
	nonce, challenge, redirect, sub string
	claims                          map[string]any
}

// New creates a provider with a fresh signing key.
func New(t testing.TB) *IdP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return &IdP{ClientID: "acksy", ClientSecret: "secret", key: key, codes: map[string]pending{}}
}

// SetUser sets the identity that signs in next and its ID token claims.
func (p *IdP) SetUser(sub string, claims map[string]any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sub, p.claims = sub, claims
}

// SetUserInfo sets extra claims served only by the userinfo endpoint.
func (p *IdP) SetUserInfo(claims map[string]any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.userinfo = claims
}

// SetNonceOverride makes issued ID tokens carry nonce instead of the
// requested one (to test replay protection).
func (p *IdP) SetNonceOverride(nonce string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.nonceOverride = nonce
}

// Client returns an HTTP client whose requests are served by the provider.
func (p *IdP) Client() *http.Client {
	return &http.Client{Transport: roundTripper{p}}
}

type roundTripper struct{ p *IdP }

func (rt roundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Scheme+"://"+r.URL.Host != Issuer {
		return nil, fmt.Errorf("oidctest: unexpected host %s", r.URL.Host)
	}
	rec := httptest.NewRecorder()
	rt.p.ServeHTTP(rec, r)
	return rec.Result(), nil
}

// Authorize plays the user agent at the authorization endpoint: it accepts
// the auth URL the relying party redirected to and returns the callback URL
// (redirect_uri with code and state) the provider would redirect back to.
func (p *IdP) Authorize(authURL string) (string, error) {
	u, err := url.Parse(authURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	switch {
	case u.Scheme+"://"+u.Host+u.Path != Issuer+"/authorize":
		return "", fmt.Errorf("oidctest: not an authorize URL: %s", authURL)
	case q.Get("client_id") != p.ClientID:
		return "", fmt.Errorf("oidctest: client_id %q", q.Get("client_id"))
	case q.Get("response_type") != "code":
		return "", fmt.Errorf("oidctest: response_type %q", q.Get("response_type"))
	case q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "":
		return "", fmt.Errorf("oidctest: PKCE S256 required")
	case q.Get("nonce") == "" || q.Get("state") == "":
		return "", fmt.Errorf("oidctest: nonce and state required")
	}
	p.mu.Lock()
	code := randomID()
	p.codes[code] = pending{
		nonce: q.Get("nonce"), challenge: q.Get("code_challenge"), redirect: q.Get("redirect_uri"),
		sub: p.sub, claims: p.claims,
	}
	p.mu.Unlock()
	cb, err := url.Parse(q.Get("redirect_uri"))
	if err != nil {
		return "", err
	}
	cq := cb.Query()
	cq.Set("code", code)
	cq.Set("state", q.Get("state"))
	cb.RawQuery = cq.Encode()
	return cb.String(), nil
}

func (p *IdP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/.well-known/openid-configuration":
		writeJSON(w, map[string]any{
			"issuer":                                Issuer,
			"authorization_endpoint":                Issuer + "/authorize",
			"token_endpoint":                        Issuer + "/token",
			"jwks_uri":                              Issuer + "/jwks",
			"userinfo_endpoint":                     Issuer + "/userinfo",
			"id_token_signing_alg_values_supported": []string{"RS256"},
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
		})
	case "/jwks":
		writeJSON(w, jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &p.key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"}}})
	case "/token":
		p.token(w, r)
	case "/userinfo":
		p.userinfoHandler(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (p *IdP) token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	id, secret, ok := r.BasicAuth()
	if !ok {
		id, secret = r.PostForm.Get("client_id"), r.PostForm.Get("client_secret")
	}
	if id != p.ClientID || secret != p.ClientSecret {
		tokenError(w, "invalid_client")
		return
	}
	p.mu.Lock()
	pc, found := p.codes[r.PostForm.Get("code")]
	delete(p.codes, r.PostForm.Get("code"))
	nonce := pc.nonce
	if p.nonceOverride != "" {
		nonce = p.nonceOverride
	}
	p.mu.Unlock()
	if !found || r.PostForm.Get("redirect_uri") != pc.redirect {
		tokenError(w, "invalid_grant")
		return
	}
	sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
	if base64.RawURLEncoding.EncodeToString(sum[:]) != pc.challenge {
		tokenError(w, "invalid_grant")
		return
	}
	now := time.Now()
	claims := map[string]any{
		"iss": Issuer, "sub": pc.sub, "aud": p.ClientID, "nonce": nonce,
		"iat": now.Unix(), "exp": now.Add(5 * time.Minute).Unix(),
	}
	for k, v := range pc.claims {
		claims[k] = v
	}
	idToken, err := p.sign(claims)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"access_token": "at-" + pc.sub, "token_type": "Bearer", "expires_in": 300, "id_token": idToken,
	})
}

func (p *IdP) userinfoHandler(w http.ResponseWriter, r *http.Request) {
	const prefix = "Bearer at-"
	h := r.Header.Get("Authorization")
	if len(h) <= len(prefix) || h[:len(prefix)] != prefix {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	p.mu.Lock()
	out := map[string]any{"sub": h[len(prefix):]}
	for k, v := range p.userinfo {
		out[k] = v
	}
	p.mu.Unlock()
	writeJSON(w, out)
}

func (p *IdP) sign(claims map[string]any) (string, error) {
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: p.key},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "k1"))
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	jws, err := signer.Sign(payload)
	if err != nil {
		return "", err
	}
	return jws.CompactSerialize()
}

func tokenError(w http.ResponseWriter, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func randomID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
