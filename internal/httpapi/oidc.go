package httpapi

import (
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/oowliq/acksy/internal/auth"
	"github.com/oowliq/acksy/internal/domain"
	"github.com/oowliq/acksy/internal/service"
)

const (
	oidcCookie     = "acksy_oidc"
	oidcCookiePath = "/api/v1/auth/oidc"
	// oidcCookieTTL bounds how long the user may spend at the provider.
	oidcCookieTTL = 600 // seconds
)

// oidcAttempt is stored in the short-lived acksy_oidc cookie between the
// redirect to the provider and the callback. It is HttpOnly and scoped to
// the OIDC routes; the state value binds the callback to this browser.
type oidcAttempt struct {
	State    string `json:"s"`
	Nonce    string `json:"n"`
	Verifier string `json:"v"`
	Redirect string `json:"r"`
}

func (a *api) authMethods(w http.ResponseWriter, r *http.Request) {
	type oidcMethod struct {
		Name     string `json:"name"`
		LoginURL string `json:"login_url"`
	}
	resp := struct {
		Password bool        `json:"password"`
		OIDC     *oidcMethod `json:"oidc"`
	}{}
	snap := a.svc.SignIn(r.Context())
	resp.Password = !snap.Settings.DisablePasswordLogin
	if snap.Provider != nil {
		resp.OIDC = &oidcMethod{Name: snap.Settings.Name, LoginURL: oidcCookiePath + "/login"}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *api) oidcLogin(w http.ResponseWriter, r *http.Request) {
	provider := a.svc.SignIn(r.Context()).Provider
	if provider == nil {
		redirectLoginError(w, r, "oidc_unavailable")
		return
	}
	st := auth.NewOIDCLoginState()
	target, err := provider.AuthCodeURL(r.Context(), st)
	if err != nil {
		a.log.ErrorContext(r.Context(), "oidc login", "err", err)
		redirectLoginError(w, r, "oidc_unavailable")
		return
	}
	raw, err := json.Marshal(oidcAttempt{
		State: st.State, Nonce: st.Nonce, Verifier: st.Verifier, Redirect: safeRedirect(r.URL.Query().Get("redirect")),
	})
	if err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	a.setOIDCCookie(w, r, base64.RawURLEncoding.EncodeToString(raw), oidcCookieTTL)
	http.Redirect(w, r, target, http.StatusFound)
}

func (a *api) oidcCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	attempt, ok := readOIDCAttempt(r)
	a.setOIDCCookie(w, r, "", -1) // single use

	if e := q.Get("error"); e != "" {
		a.log.InfoContext(ctx, "oidc provider returned error", "error", e)
		redirectLoginError(w, r, "oidc_denied")
		return
	}
	if !ok || attempt.State == "" ||
		subtle.ConstantTimeCompare([]byte(attempt.State), []byte(q.Get("state"))) != 1 {
		a.log.InfoContext(ctx, "oidc callback without matching login state")
		redirectLoginError(w, r, "oidc_expired")
		return
	}
	provider := a.svc.SignIn(ctx).Provider
	if provider == nil {
		redirectLoginError(w, r, "oidc_unavailable")
		return
	}
	claims, err := provider.Exchange(ctx, q.Get("code"), auth.OIDCLoginState{
		State: attempt.State, Nonce: attempt.Nonce, Verifier: attempt.Verifier,
	})
	if err != nil {
		a.log.WarnContext(ctx, "oidc exchange failed", "err", err)
		redirectLoginError(w, r, "oidc_failed")
		return
	}
	secret, u, err := a.svc.LoginOIDC(ctx, claims, r.UserAgent())
	if err != nil {
		var de *domain.Error
		errors.As(err, &de)
		switch de {
		case service.ErrOIDCNoAccount:
			a.log.InfoContext(ctx, "oidc login without linked account", "issuer", claims.Issuer, "subject", claims.Subject)
			redirectLoginError(w, r, "oidc_no_account")
		case service.ErrAccountDisabled:
			redirectLoginError(w, r, "account_disabled")
		default:
			a.log.ErrorContext(ctx, "oidc login", "err", err)
			redirectLoginError(w, r, "oidc_failed")
		}
		return
	}
	a.log.InfoContext(ctx, "oidc login", "user_id", u.ID)
	a.setSessionCookie(w, r, secret)
	http.Redirect(w, r, safeRedirect(attempt.Redirect), http.StatusFound)
}

func readOIDCAttempt(r *http.Request) (oidcAttempt, bool) {
	var at oidcAttempt
	c, err := r.Cookie(oidcCookie)
	if err != nil {
		return at, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(c.Value)
	if err != nil {
		return at, false
	}
	return at, json.Unmarshal(raw, &at) == nil
}

func (a *api) setOIDCCookie(w http.ResponseWriter, r *http.Request, value string, maxAge int) {
	// Lax (not Strict) so the cookie survives the top-level redirect back
	// from the provider.
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // Secure follows the deployment, as for the session cookie
		Name: oidcCookie, Value: value, Path: oidcCookiePath, HttpOnly: true, Secure: a.secure(r),
		SameSite: http.SameSiteLaxMode, MaxAge: maxAge,
	})
}

// safeRedirect keeps post-login redirects on this origin.
func safeRedirect(p string) string {
	if !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") || strings.HasPrefix(p, "/\\") ||
		strings.ContainsAny(p, "\r\n") {
		return "/"
	}
	return p
}

func redirectLoginError(w http.ResponseWriter, r *http.Request, code string) {
	http.Redirect(w, r, "/login?error="+url.QueryEscape(code), http.StatusFound)
}
