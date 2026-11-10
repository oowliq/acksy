// Package app wires dependencies together and owns the server lifecycle.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/oowliq/acksy/api"
	"github.com/oowliq/acksy/internal/adapter"
	"github.com/oowliq/acksy/internal/adapter/gotify"
	"github.com/oowliq/acksy/internal/adapter/ntfy"
	"github.com/oowliq/acksy/internal/auth"
	"github.com/oowliq/acksy/internal/broker"
	"github.com/oowliq/acksy/internal/broker/memory"
	"github.com/oowliq/acksy/internal/broker/redis"
	"github.com/oowliq/acksy/internal/buildinfo"
	"github.com/oowliq/acksy/internal/config"
	"github.com/oowliq/acksy/internal/delivery/action"
	"github.com/oowliq/acksy/internal/delivery/webhook"
	"github.com/oowliq/acksy/internal/delivery/webpush"
	"github.com/oowliq/acksy/internal/domain"
	"github.com/oowliq/acksy/internal/httpapi"
	"github.com/oowliq/acksy/internal/service"
	"github.com/oowliq/acksy/internal/store"
	"github.com/oowliq/acksy/web"
)

// App is a fully wired Acksy server.
type App struct {
	cfg    config.Config
	log    *slog.Logger
	db     *store.DB
	svc    *service.Service
	broker broker.Broker
	server *http.Server
	push   *webpush.Dispatcher
	hooks  *webhook.Worker
	vapid  string // public VAPID key served to browsers
}

// New opens the database, applies migrations, connects the broker and builds
// the HTTP server. Call Close when done, even if Run is never called.
func New(ctx context.Context, cfg config.Config, log *slog.Logger) (_ *App, err error) {
	a := &App{cfg: cfg, log: log}
	defer func() {
		if err != nil {
			a.Close()
		}
	}()

	a.db, err = store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	version, err := a.db.Migrate(ctx)
	if err != nil {
		return nil, err
	}
	log.Info("database ready", "dialect", a.db.Dialect, "schema_version", version)

	ready := map[string]httpapi.Pinger{"database": a.db}
	if cfg.RedisURL != "" {
		rb, err := redis.New(ctx, cfg.RedisURL)
		if err != nil {
			return nil, err
		}
		a.broker = rb
		ready["redis"] = rb
		log.Info("broker ready", "kind", "redis")
	} else {
		a.broker = memory.New()
		log.Info("broker ready", "kind", "memory", "note", "single instance only; set ACKSY_REDIS_URL to scale out")
	}

	redirectURL := ""
	if cfg.BaseURL != "" {
		redirectURL = cfg.OIDCRedirectURL()
	}
	a.svc = service.New(store.New(a.db), a.broker, log, service.Options{
		NewOIDCProvider: oidcProviderFactory(redirectURL),
		OIDCRedirectURL: redirectURL,
	})
	if len(cfg.Obsolete) > 0 {
		log.Warn("ignoring environment variables of settings that moved to Settings → Server and Settings → Sign-in",
			"vars", cfg.Obsolete)
	}
	if snap := a.svc.SignIn(ctx); snap.Settings.Active() {
		log.Info("oidc enabled", "issuer", snap.Settings.Issuer, "redirect_url", redirectURL,
			"auto_create", snap.Settings.AutoCreate, "role_sync", len(snap.Settings.AdminValues) > 0,
			"password_login", !snap.Settings.DisablePasswordLogin)
	}
	if cfg.Admin.Username != "" {
		created, err := a.svc.BootstrapAdmin(ctx, cfg.Admin.Username, cfg.Admin.Password)
		if err != nil {
			return nil, fmt.Errorf("bootstrap admin: %w", err)
		}
		if created {
			log.Info("bootstrap admin created", "username", cfg.Admin.Username)
		}
	}

	// Delivery is always wired; the server settings switch each part on and
	// off at runtime (ADR 0008).
	if err := a.setupPush(ctx); err != nil {
		return nil, err
	}
	a.svc.SetActionRunner(action.New(action.Config{}))
	a.hooks = webhook.New(a.svc, webhook.Config{BaseURL: cfg.BaseURL}, log)
	a.svc.AddNotifier(a.hooks, func(s domain.ServerSettings) bool { return s.Webhooks.Enabled })
	a.svc.SetWebhookSender(a.hooks)

	adapters := adapter.NewRegistry(ntfy.New(), gotify.New()).All()
	s := a.svc.Settings(ctx)
	log.Info("server settings", "ntfy", s.Adapters.NTFY, "gotify", s.Adapters.Gotify, "smtp", s.Adapters.SMTP, "push", s.Push.Enabled,
		"webhooks", s.Webhooks.Enabled, "webhooks_allow_private", s.Webhooks.AllowPrivate,
		"actions", s.Actions.Enabled, "actions_allow_private", s.Actions.AllowPrivate)

	a.server = &http.Server{
		Addr: cfg.Listen,
		Handler: httpapi.NewRouter(httpapi.Deps{
			Logger:  log,
			Service: a.svc,
			Options: httpapi.Options{
				SecureCookies: cfg.UseSecureCookies(),
				PushPublicKey: a.vapid,
			},
			Ready:    ready,
			Web:      web.Dist(),
			OpenAPI:  api.OpenAPI,
			Adapters: adapters,
		}),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		// No WriteTimeout: SSE and WebSocket connections are long-lived.
	}
	return a, nil
}

// Run serves HTTP until ctx is cancelled, then shuts down gracefully.
func (a *App) Run(ctx context.Context) error {
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", a.cfg.Listen)
	if err != nil {
		return fmt.Errorf("listen %s: %w", a.cfg.Listen, err)
	}
	a.log.Info("acksy listening", "addr", ln.Addr().String(), "version", buildinfo.Version)
	if a.cfg.SMTP.Listen == "" {
		return a.Serve(ctx, ln)
	}
	smtpLn, err := listenSMTP(ctx, a.cfg.SMTP)
	if err != nil {
		_ = ln.Close()
		return err
	}
	a.log.Info("SMTP listening", "addr", smtpLn.Addr().String(), "domain", a.cfg.SMTP.Domain)
	ctx, stop := context.WithCancel(ctx)
	defer stop()
	smtpDone := make(chan error, 1)
	go func() {
		err := a.serveSMTP(ctx, smtpLn)
		stop()
		smtpDone <- err
	}()
	httpErr := a.Serve(ctx, ln)
	stop()
	return errors.Join(httpErr, <-smtpDone)
}

// Serve serves HTTP on ln until ctx is cancelled, then shuts down gracefully.
func (a *App) Serve(ctx context.Context, ln net.Listener) error {
	// Background delivery stops with ctx; Serve returns only after it has.
	var bg sync.WaitGroup
	defer bg.Wait()
	ctx, stop := context.WithCancel(ctx)
	defer stop()
	bg.Go(func() { a.janitor(ctx) })
	if a.push != nil {
		bg.Go(func() { a.push.Run(ctx) })
	}
	if a.hooks != nil {
		bg.Go(func() { a.hooks.Run(ctx) })
	}

	// Request contexts derive from base; cancelling it on shutdown ends
	// long-lived SSE and WebSocket streams, which http.Server.Shutdown alone
	// would wait for until the timeout.
	base, cancelBase := context.WithCancel(context.WithoutCancel(ctx))
	defer cancelBase()
	a.server.BaseContext = func(net.Listener) context.Context { return base }

	errCh := make(chan error, 1)
	go func() { errCh <- a.server.Serve(ln) }()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}

	a.log.Info("shutting down", "timeout", a.cfg.ShutdownTimeout)
	cancelBase()
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), a.cfg.ShutdownTimeout)
	defer cancel()
	if err := a.server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("http shutdown: %w", err)
	}
	return nil
}

// Close releases resources. Safe to call on a partially built App.
func (a *App) Close() {
	if a.broker != nil {
		if err := a.broker.Close(); err != nil {
			a.log.Warn("close broker", "err", err)
		}
	}
	if a.db != nil {
		if err := a.db.Close(); err != nil {
			a.log.Warn("close database", "err", err)
		}
	}
}

// Service exposes the application core (used by tests and the CLI).
func (a *App) Service() *service.Service { return a.svc }

// setupPush resolves the VAPID keys (configured, or generated and stored on
// first start) and wires the Web Push dispatcher into the service.
func (a *App) setupPush(ctx context.Context) error {
	keys := auth.VAPIDKeys{Public: a.cfg.Push.VAPIDPublicKey, Private: a.cfg.Push.VAPIDPrivateKey}
	if keys.Public == "" {
		var err error
		if keys, err = a.svc.EnsureVAPIDKeys(ctx); err != nil {
			return fmt.Errorf("web push keys: %w", err)
		}
	} else if err := keys.Validate(); err != nil {
		return fmt.Errorf("ACKSY_VAPID_*: %w", err)
	}
	subject := func(ctx context.Context) string { return a.svc.Settings(ctx).PushSubject(a.cfg.BaseURL) }
	if s := a.svc.Settings(ctx); s.Push.Enabled && subject(ctx) == "" {
		a.log.Warn("web push: no VAPID subject; set one in Settings → Server (https URL or e-mail) or use an https ACKSY_BASE_URL, Safari rejects pushes without one")
	}
	a.push = webpush.New(a.svc, webpush.Config{Keys: keys, Subject: subject}, a.log)
	a.svc.AddNotifier(a.push, func(s domain.ServerSettings) bool { return s.Push.Enabled })
	a.svc.SetPushSender(a.push)
	a.vapid = keys.Public
	return nil
}

// janitor runs periodic housekeeping until ctx is cancelled.
func (a *App) janitor(ctx context.Context) {
	tick := time.NewTicker(time.Hour)
	defer tick.Stop()
	for {
		if n, err := a.svc.PruneSessions(ctx); err != nil {
			a.log.Warn("prune sessions", "err", err)
		} else if n > 0 {
			a.log.Debug("pruned expired sessions", "count", n)
		}
		if res, err := a.svc.ApplyRetention(ctx); err != nil {
			a.log.Warn("notification retention", "err", err)
		} else if res.Deleted > 0 {
			a.log.Info("notification retention", "deleted", res.Deleted)
		}
		settings := a.svc.Settings(ctx)
		if n, err := a.svc.PruneWebhookDeliveries(ctx, settings.WebhookFailedRetention()); err != nil {
			a.log.Warn("prune webhook deliveries", "err", err)
		} else if n > 0 {
			a.log.Debug("pruned failed webhook deliveries", "count", n)
		}
		if n, err := a.svc.PruneEvents(ctx, settings.EventRetention()); err != nil {
			a.log.Warn("prune events", "err", err)
		} else if n > 0 {
			a.log.Debug("pruned old events", "count", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
