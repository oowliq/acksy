package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/oowliq/acksy/internal/auth"
	"github.com/oowliq/acksy/internal/domain"
)

// SessionCookie is the name of the login session cookie.
const SessionCookie = "acksy_session"

// Authenticator resolves credentials into principals.
type Authenticator interface {
	AuthenticateSession(ctx context.Context, secret string) (*auth.Principal, error)
	AuthenticateToken(ctx context.Context, secret string) (*auth.Principal, error)
}

// authenticate attaches the principal (if any credentials are present) to the
// request context. Invalid credentials fail the request; missing credentials
// do not (requireAuth decides).
func (a *api) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, err := a.principalFrom(r)
		if err != nil {
			if errors.Is(err, domain.ErrUnauthorized) {
				a.clearSessionCookie(w, r)
			}
			respondErr(w, r, a.log, err)
			return
		}
		if p != nil {
			r = r.WithContext(auth.WithPrincipal(r.Context(), p))
		}
		next.ServeHTTP(w, r)
	})
}

func (a *api) principalFrom(r *http.Request) (*auth.Principal, error) {
	// An explicit Authorization header wins over the cookie.
	if h := r.Header.Get("Authorization"); h != "" {
		scheme, secret, ok := strings.Cut(h, " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") {
			return nil, &domain.Error{Kind: domain.KindUnauthorized, Msg: "Authorization must be a Bearer token"}
		}
		return a.authn.AuthenticateToken(r.Context(), strings.TrimSpace(secret))
	}
	if c, err := r.Cookie(SessionCookie); err == nil && c.Value != "" {
		return a.authn.AuthenticateSession(r.Context(), c.Value)
	}
	return nil, nil
}

// requireAuth rejects requests without a principal.
func requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth.FromContext(r.Context()) == nil {
			w.Header().Set("WWW-Authenticate", `Bearer realm="acksy"`)
			writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func principal(r *http.Request) *auth.Principal { return auth.FromContext(r.Context()) }

func (a *api) secure(r *http.Request) bool {
	return a.opts.SecureCookies || r.TLS != nil
}

func (a *api) setSessionCookie(w http.ResponseWriter, r *http.Request, secret string) {
	// Secure follows the deployment (HTTPS or not); HttpOnly and SameSite are fixed.
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // see above
		Name: SessionCookie, Value: secret, Path: "/", HttpOnly: true, Secure: a.secure(r),
		SameSite: http.SameSiteLaxMode, MaxAge: int(a.opts.SessionTTL.Seconds()),
	})
}

func (a *api) clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	if _, err := r.Cookie(SessionCookie); err != nil {
		return
	}
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // deletion cookie, same attributes as above
		Name: SessionCookie, Value: "", Path: "/", HttpOnly: true, Secure: a.secure(r),
		SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
}
