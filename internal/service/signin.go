package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/oowliq/acksy/internal/auth"
	"github.com/oowliq/acksy/internal/domain"
)

// DefaultSignInTTL is how long an instance trusts its sign-in snapshot before
// checking the database for changes made on another instance.
const DefaultSignInTTL = 15 * time.Second

// OIDCProvider runs the sign-in flow against the configured provider
// (implemented by *auth.OIDCProvider; built by Options.NewOIDCProvider).
type OIDCProvider interface {
	Issuer() string
	AuthCodeURL(ctx context.Context, st auth.OIDCLoginState) (string, error)
	Exchange(ctx context.Context, code string, st auth.OIDCLoginState) (auth.OIDCClaims, error)
	// Discover fetches the provider metadata (used to test settings).
	Discover(ctx context.Context) error
}

// SignInSnapshot is the sign-in configuration in effect (ADR 0006).
type SignInSnapshot struct {
	Settings domain.OIDCSettings
	// Provider is nil unless Settings.Active() and a factory is configured.
	Provider  OIDCProvider
	UpdatedAt time.Time
	loadedAt  time.Time
}

// SignIn returns the current sign-in configuration. The snapshot is cached
// for SignInTTL; after that the database's updated_at is checked and the
// snapshot rebuilt only when it changed. Read errors keep the old snapshot.
func (s *Service) SignIn(ctx context.Context) *SignInSnapshot {
	now := s.now()
	cur := s.signIn.Load()
	if cur != nil && now.Sub(cur.loadedAt) < s.opts.SignInTTL {
		return cur
	}
	s.signInMu.Lock()
	defer s.signInMu.Unlock()
	if cur = s.signIn.Load(); cur != nil && now.Sub(cur.loadedAt) < s.opts.SignInTTL {
		return cur
	}
	next, err := s.loadSignIn(ctx, cur, now)
	if err != nil {
		s.log.WarnContext(ctx, "reading sign-in settings failed; keeping the previous ones", "err", err)
		if cur == nil {
			// Never fail open: without the settings, password sign-in might be
			// off by policy. Not cached, so the next request retries.
			closed := domain.DefaultOIDCSettings()
			closed.DisablePasswordLogin = true
			return &SignInSnapshot{Settings: closed}
		}
		refreshed := *cur
		refreshed.loadedAt = now // try again after the TTL, not on every request
		s.signIn.Store(&refreshed)
		return &refreshed
	}
	s.signIn.Store(next)
	return next
}

func (s *Service) loadSignIn(ctx context.Context, cur *SignInSnapshot, now time.Time) (*SignInSnapshot, error) {
	raw, at, err := s.store.GetSettingWithTime(ctx, domain.OIDCSettingsKey)
	if errors.Is(err, domain.ErrNotFound) {
		if cur != nil && cur.UpdatedAt.IsZero() {
			next := *cur
			next.loadedAt = now
			return &next, nil
		}
		return s.buildSignIn(domain.DefaultOIDCSettings(), time.Time{}, now), nil
	}
	if err != nil {
		return nil, err
	}
	if cur != nil && cur.UpdatedAt.Equal(at) {
		next := *cur
		next.loadedAt = now
		return &next, nil
	}
	settings := domain.DefaultOIDCSettings()
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return nil, err
	}
	return s.buildSignIn(settings, at, now), nil
}

func (s *Service) buildSignIn(settings domain.OIDCSettings, updatedAt, now time.Time) *SignInSnapshot {
	snap := &SignInSnapshot{Settings: settings, UpdatedAt: updatedAt, loadedAt: now}
	if settings.Active() && s.opts.NewOIDCProvider != nil {
		snap.Provider = s.opts.NewOIDCProvider(settings)
	}
	return snap
}

// replaceSignIn makes freshly saved settings effective on this instance at once.
func (s *Service) replaceSignIn(settings domain.OIDCSettings, updatedAt time.Time) {
	s.signInMu.Lock()
	defer s.signInMu.Unlock()
	s.signIn.Store(s.buildSignIn(settings, updatedAt, s.now()))
}

// OIDCSettingsInput is a settings update. ClientSecret nil keeps the stored
// secret; a pointer to "" removes it.
type OIDCSettingsInput struct {
	domain.OIDCSettings
	ClientSecret *string
}

// requireSignInAdmin allows admins in a login session only: like token
// management, a leaked token must not be able to change how people sign in.
func requireSignInAdmin(p *auth.Principal) error {
	if err := p.Require(domain.ScopeAdmin); err != nil {
		return err
	}
	if !p.IsSession() {
		return domain.Forbidden("sign-in settings can only be changed from a login session")
	}
	return nil
}

// OIDCRedirectURL is the callback URL to register at the provider; empty
// when ACKSY_BASE_URL is not set.
func (s *Service) OIDCRedirectURL() string { return s.opts.OIDCRedirectURL }

// OIDCSettings returns the stored settings, secret included: callers that
// answer clients must drop it.
func (s *Service) OIDCSettings(ctx context.Context, p *auth.Principal) (domain.OIDCSettings, time.Time, error) {
	if err := requireSignInAdmin(p); err != nil {
		return domain.OIDCSettings{}, time.Time{}, err
	}
	return s.storedOIDCSettings(ctx)
}

func (s *Service) storedOIDCSettings(ctx context.Context) (domain.OIDCSettings, time.Time, error) {
	settings := domain.DefaultOIDCSettings()
	raw, at, err := s.store.GetSettingWithTime(ctx, domain.OIDCSettingsKey)
	if errors.Is(err, domain.ErrNotFound) {
		return settings, time.Time{}, nil
	}
	if err != nil {
		return settings, time.Time{}, err
	}
	return settings, at, json.Unmarshal([]byte(raw), &settings)
}

// merge applies an update to the stored settings: the secret is kept unless
// the input replaces or clears it.
func (s *Service) mergeOIDCInput(ctx context.Context, in OIDCSettingsInput) (domain.OIDCSettings, error) {
	next := in.OIDCSettings
	if in.ClientSecret != nil {
		next.ClientSecret = *in.ClientSecret
		return next, nil
	}
	stored, _, err := s.storedOIDCSettings(ctx)
	if err != nil {
		return next, err
	}
	next.ClientSecret = stored.ClientSecret
	return next, nil
}

// SaveOIDCSettings validates and stores the sign-in settings; they take effect
// on this instance at once and on others within SignInTTL.
func (s *Service) SaveOIDCSettings(ctx context.Context, p *auth.Principal, in OIDCSettingsInput) (domain.OIDCSettings, time.Time, error) {
	if err := requireSignInAdmin(p); err != nil {
		return domain.OIDCSettings{}, time.Time{}, err
	}
	next, err := s.mergeOIDCInput(ctx, in)
	if err != nil {
		return next, time.Time{}, err
	}
	if err := next.Validate(); err != nil {
		return next, time.Time{}, err
	}
	if next.Active() && s.opts.OIDCRedirectURL == "" {
		return next, time.Time{}, domain.Invalid("set ACKSY_BASE_URL first: the provider needs a redirect URL")
	}
	// Don't let an admin save themselves out: without password sign-in they
	// need an identity at this provider.
	if next.DisablePasswordLogin {
		idents, err := s.store.ListUserIdentities(ctx, p.User.ID)
		if err != nil {
			return next, time.Time{}, err
		}
		linked := false
		for _, i := range idents {
			linked = linked || i.Issuer == next.Issuer
		}
		if !linked {
			return next, time.Time{}, domain.Conflict("link your own identity with this provider before turning off password sign-in")
		}
	}
	at, err := s.putOIDCSettings(ctx, next)
	if err != nil {
		return next, time.Time{}, err
	}
	s.log.InfoContext(ctx, "sign-in settings changed", "user_id", p.User.ID, "oidc", next.Active(),
		"password_login", !next.DisablePasswordLogin)
	return next, at, nil
}

func (s *Service) putOIDCSettings(ctx context.Context, settings domain.OIDCSettings) (time.Time, error) {
	b, err := json.Marshal(settings) //nolint:gosec // stored server-side (ADR 0006); API responses use a view without the secret
	if err != nil {
		return time.Time{}, err
	}
	at := s.now()
	if err := s.store.PutSetting(ctx, domain.OIDCSettingsKey, string(b), at); err != nil {
		return time.Time{}, err
	}
	s.replaceSignIn(settings, at)
	return at, nil
}

// TestOIDCSettings fetches the provider metadata for unsaved settings.
func (s *Service) TestOIDCSettings(ctx context.Context, p *auth.Principal, in OIDCSettingsInput) error {
	if err := requireSignInAdmin(p); err != nil {
		return err
	}
	next, err := s.mergeOIDCInput(ctx, in)
	if err != nil {
		return err
	}
	next.Enabled = true
	if err := next.Validate(); err != nil {
		return err
	}
	if s.opts.NewOIDCProvider == nil {
		return domain.Conflict("single sign-on is not available on this server")
	}
	return s.opts.NewOIDCProvider(next).Discover(ctx)
}

// AllowPasswordLogin turns password sign-in back on (recovery from the CLI
// when the provider is gone). Other settings are untouched.
func (s *Service) AllowPasswordLogin(ctx context.Context, p *auth.Principal) error {
	if err := p.Require(domain.ScopeAdmin); err != nil {
		return err
	}
	settings, _, err := s.storedOIDCSettings(ctx)
	if err != nil {
		return err
	}
	if !settings.DisablePasswordLogin {
		return nil
	}
	settings.DisablePasswordLogin = false
	_, err = s.putOIDCSettings(ctx, settings)
	return err
}

// ImportOIDCSettings stores settings from the deprecated ACKSY_OIDC_* config
// unless the database already has some. It reports whether it imported.
func (s *Service) ImportOIDCSettings(ctx context.Context, settings domain.OIDCSettings) (bool, error) {
	if _, _, err := s.store.GetSettingWithTime(ctx, domain.OIDCSettingsKey); err == nil {
		return false, nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return false, err
	}
	if err := settings.Validate(); err != nil {
		return false, err
	}
	if settings.Active() && s.opts.OIDCRedirectURL == "" {
		return false, domain.Invalid("ACKSY_BASE_URL is required for OIDC sign-in (used for the redirect URL)")
	}
	_, err := s.putOIDCSettings(ctx, settings)
	return err == nil, err
}
