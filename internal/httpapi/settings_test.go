package httpapi

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/oowliq/acksy/internal/auth/oidctest"
	"github.com/oowliq/acksy/internal/service"
)

func TestOIDCSettingsAPI(t *testing.T) {
	idp := oidctest.New(t)
	h, svc := newAPIWithService(t, Options{SessionTTL: time.Hour}, service.Options{
		NewOIDCProvider: oidcFactory(idp), OIDCRedirectURL: "https://acksy.test/api/v1/auth/oidc/callback",
	})
	seedUsers(t, svc)
	root := login(t, h, "root", "password123")
	bob := login(t, h, "bob", "password123")

	rec := root.do(http.MethodGet, "/api/v1/settings/oidc", nil)
	expect(t, rec, http.StatusOK)
	if body := rec.Body.String(); !strings.Contains(body, `"client_secret_set":false`) || !strings.Contains(body, `"redirect_url":"https://acksy.test/api/v1/auth/oidc/callback"`) {
		t.Fatalf("defaults: %s", body)
	}
	expect(t, bob.do(http.MethodGet, "/api/v1/settings/oidc", nil), http.StatusForbidden)

	// No SSO yet.
	if body := root.do(http.MethodGet, "/api/v1/auth/methods", nil).Body.String(); !strings.Contains(body, `"oidc":null`) {
		t.Fatalf("methods before: %s", body)
	}

	settings := map[string]any{
		"enabled": true, "issuer": oidctest.Issuer, "client_id": idp.ClientID, "client_secret": "hunter2-secret",
		"name": "Authentik", "scopes": []string{"openid"}, "username_claim": "preferred_username",
		"role_claim": "groups", "admin_values": []string{}, "auto_create": true, "disable_password_login": false,
	}
	put := root.do(http.MethodPut, "/api/v1/settings/oidc", settings)
	expect(t, put, http.StatusOK)
	get := root.do(http.MethodGet, "/api/v1/settings/oidc", nil)
	for _, body := range []string{put.Body.String(), get.Body.String()} {
		if strings.Contains(body, `client_secret"`) || strings.Contains(body, "hunter2-secret") {
			t.Fatalf("secret leaked: %s", body)
		}
		if !strings.Contains(body, `"client_secret_set":true`) {
			t.Fatalf("secret flag: %s", body)
		}
	}
	// Effective at once.
	if body := root.do(http.MethodGet, "/api/v1/auth/methods", nil).Body.String(); !strings.Contains(body, `"name":"Authentik"`) {
		t.Fatalf("methods after: %s", body)
	}

	ok := decode[struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}](t, root.do(http.MethodPost, "/api/v1/settings/oidc/test", settings))
	if !ok.OK {
		t.Fatalf("test against fake IdP: %+v", ok)
	}
	bad := map[string]any{"enabled": true, "issuer": "https://unknown-idp.test", "client_id": "x", "scopes": []string{"openid"}}
	res := decode[struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}](t, root.do(http.MethodPost, "/api/v1/settings/oidc/test", bad))
	if res.OK || res.Error == "" {
		t.Fatalf("test against unknown IdP: %+v", res)
	}

	// Turning password sign-in off without an identity at the provider is refused.
	settings["disable_password_login"] = true
	expect(t, root.do(http.MethodPut, "/api/v1/settings/oidc", settings), http.StatusConflict)
}
