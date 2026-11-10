// Package httpapi wires HTTP routes: REST v1, health checks and the embedded SPA.
package httpapi

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/oowliq/acksy/internal/adapter"
	"github.com/oowliq/acksy/internal/auth"
	"github.com/oowliq/acksy/internal/buildinfo"
	"github.com/oowliq/acksy/internal/service"
)

// Pinger is anything whose availability affects readiness (DB, Redis).
type Pinger interface {
	Ping(ctx context.Context) error
}

// Options tune HTTP behaviour.
type Options struct {
	// SecureCookies forces the Secure cookie flag (set when served over HTTPS
	// behind a proxy). Requests arriving over TLS always get Secure cookies.
	SecureCookies bool
	// KeepAlive is the SSE comment / WebSocket ping interval (default 25s).
	KeepAlive time.Duration
	// PushPublicKey is the VAPID public key; empty means Web Push is not
	// wired. The server settings switch it on and off.
	PushPublicKey string
}

// Deps are the dependencies the HTTP layer needs.
type Deps struct {
	Logger *slog.Logger
	// Service is the application core; nil serves only health and static routes.
	Service *service.Service
	Options Options
	// Ready lists named dependencies checked by /readyz.
	Ready map[string]Pinger
	// Web is the built SPA; nil disables UI serving.
	Web fs.FS
	// OpenAPI is the API specification served at /api/v1/openapi.yaml.
	OpenAPI []byte
	// Adapters are the ingest adapters (ntfy, gotify), mounted outside
	// /api/v1 under their own prefixes. The server settings switch each on
	// and off.
	Adapters []adapter.Adapter
}

type api struct {
	svc     *service.Service
	authn   Authenticator
	log     *slog.Logger
	opts    Options
	limiter *auth.LoginLimiter
}

// NewRouter builds the root handler.
func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(requestLogger(d.Logger))
	r.Use(middleware.Recoverer)
	r.Use(securityHeaders)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Get("/readyz", readyHandler(d.Ready))

	r.Route("/api/v1", func(r chi.Router) {
		// Rejects cross-origin browser requests with unsafe methods (CSRF
		// protection for cookie sessions); non-browser clients are unaffected.
		r.Use(http.NewCrossOriginProtection().Handler)

		r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
			writeError(w, http.StatusNotFound, "not_found", "route not found")
		})
		r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		})
		r.Get("/version", func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusOK, buildinfo.Get())
		})
		if d.OpenAPI != nil {
			r.Get("/openapi.yaml", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/yaml")
				_, _ = w.Write(d.OpenAPI)
			})
		}
		if d.Service != nil {
			a := &api{
				svc: d.Service, authn: d.Service, log: d.Logger, opts: d.Options,
				limiter: auth.NewLoginLimiter(10, 15*time.Minute),
			}
			a.routes(r)
		}
	})

	if d.Service != nil && len(d.Adapters) > 0 {
		a := &api{svc: d.Service, authn: d.Service, log: d.Logger, opts: d.Options}
		a.mountAdapters(r, d.Adapters)
	}

	if d.Web != nil {
		r.Handle("/*", spaHandler(d.Web))
	}
	return r
}

func (a *api) routes(root chi.Router) {
	// Public sign-in routes. They skip authenticate so that a stale session
	// cookie cannot break the login flow.
	root.Get("/auth/methods", a.authMethods)
	// Always routed: whether SSO is on is read per request (ADR 0006).
	root.Get("/auth/oidc/login", a.oidcLogin)
	root.Get("/auth/oidc/callback", a.oidcCallback)

	r := root.With(a.authenticate)
	r.Post("/auth/login", a.login)

	r.Group(func(r chi.Router) {
		r.Use(requireAuth)

		r.Post("/auth/logout", a.logout)
		r.Get("/me", a.me)
		r.Patch("/me", a.updateMe)
		r.Post("/me/password", a.changePassword)

		r.Get("/settings/server", a.getServerSettings)
		r.Put("/settings/server", a.putServerSettings)

		r.Route("/settings/oidc", func(r chi.Router) {
			r.Get("/", a.getOIDCSettings)
			r.Put("/", a.putOIDCSettings)
			r.Post("/test", a.testOIDCSettings)
		})

		r.Route("/users", func(r chi.Router) {
			r.Get("/", a.listUsers)
			r.Post("/", a.createUser)
			r.Get("/{id}", a.getUser)
			r.Patch("/{id}", a.updateUser)
			r.Delete("/{id}", a.deleteUser)
			r.Get("/{id}/identities", a.listIdentities)
			r.Post("/{id}/identities", a.linkIdentity)
			r.Delete("/{id}/identities", a.unlinkIdentity)
		})

		r.Route("/topics", func(r chi.Router) {
			r.Get("/", a.listTopics)
			r.Post("/", a.createTopic)
			r.Get("/{id}", a.getTopic)
			r.Patch("/{id}", a.updateTopic)
			r.Delete("/{id}", a.deleteTopic)
			r.Get("/{id}/members", a.listTopicMembers)
			r.Delete("/{id}/members/{userID}", a.removeTopicMember)
			r.Get("/{id}/invites", a.listTopicInvites)
			r.Post("/{id}/invites", a.inviteToTopic)
			r.Delete("/{id}/invites/{userID}", a.revokeTopicInvite)
		})

		r.Route("/invitations", func(r chi.Router) {
			r.Get("/", a.listInvitations)
			r.Post("/{topicID}/accept", a.acceptInvitation)
			r.Delete("/{topicID}", a.declineInvitation)
		})

		r.Route("/push", func(r chi.Router) {
			r.Get("/config", a.pushConfig)
			r.Get("/subscriptions", a.listPushSubscriptions)
			r.Post("/subscriptions", a.createPushSubscription)
			r.Patch("/subscriptions/{id}", a.updatePushSubscription)
			r.Delete("/subscriptions/{id}", a.deletePushSubscription)
			r.Post("/subscriptions/{id}/test", a.testPushSubscription)
		})

		r.Route("/webhooks", func(r chi.Router) {
			r.Get("/", a.listWebhooks)
			r.Post("/", a.createWebhook)
			r.Get("/{id}", a.getWebhook)
			r.Patch("/{id}", a.updateWebhook)
			r.Delete("/{id}", a.deleteWebhook)
			r.Post("/{id}/rotate-secret", a.rotateWebhookSecret)
			r.Post("/{id}/test", a.testWebhook)
			r.Get("/{id}/deliveries", a.listWebhookDeliveries)
		})

		r.Route("/tokens", func(r chi.Router) {
			r.Get("/", a.listTokens)
			r.Post("/", a.createToken)
			r.Delete("/{id}", a.deleteToken)
		})

		r.Get("/events", a.events)
		r.Get("/ws", a.websocketEvents)

		r.Route("/notifications", func(r chi.Router) {
			r.Get("/", a.listNotifications)
			r.Post("/", a.publish)
			r.Get("/counts", a.counts)
			r.Post("/bulk", a.bulkNotifications)
			r.Get("/{id}", a.getNotification)
			r.Patch("/{id}", a.updateNotification)
			r.Delete("/{id}", a.deleteNotification)
			r.Post("/{id}/actions/{index}", a.runAction)
		})
	})
}

func readyHandler(deps map[string]Pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		checks := make(map[string]string, len(deps))
		status := http.StatusOK
		for name, p := range deps {
			if err := p.Ping(ctx); err != nil {
				checks[name] = "unavailable"
				status = http.StatusServiceUnavailable
				continue
			}
			checks[name] = "ok"
		}
		writeJSON(w, status, map[string]any{"checks": checks})
	}
}

func requestLogger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			// Path only: query strings may carry tokens (gotify ?token=).
			log.LogAttrs(r.Context(), slog.LevelInfo, "http request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", ww.Status()),
				slog.Int("bytes", ww.BytesWritten()),
				slog.Duration("duration", time.Since(start)),
				slog.String("request_id", middleware.GetReqID(r.Context())),
			)
		})
	}
}
