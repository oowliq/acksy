package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/oowliq/acksy/internal/auth"
	"github.com/oowliq/acksy/internal/domain"
)

// serverSnapshot is the server settings in effect (ADR 0008).
type serverSnapshot struct {
	settings  domain.ServerSettings
	updatedAt time.Time
	loadedAt  time.Time
}

// Settings returns the server settings in effect. Like SignIn, the snapshot
// is cached for SignInTTL, then the stored updated_at is checked and the
// settings re-read only when it changed. Read errors keep the old snapshot,
// or use the defaults when there is none yet.
func (s *Service) Settings(ctx context.Context) domain.ServerSettings {
	now := s.now()
	cur := s.server.Load()
	if cur != nil && now.Sub(cur.loadedAt) < s.opts.SignInTTL {
		return cur.settings
	}
	s.serverMu.Lock()
	defer s.serverMu.Unlock()
	if cur = s.server.Load(); cur != nil && now.Sub(cur.loadedAt) < s.opts.SignInTTL {
		return cur.settings
	}
	next, err := s.loadServerSettings(ctx, cur, now)
	if err != nil {
		s.log.WarnContext(ctx, "reading server settings failed; keeping the previous ones", "err", err)
		if cur == nil {
			return domain.DefaultServerSettings() // not cached: the next call retries
		}
		refreshed := *cur
		refreshed.loadedAt = now
		s.server.Store(&refreshed)
		return refreshed.settings
	}
	s.server.Store(next)
	return next.settings
}

func (s *Service) loadServerSettings(ctx context.Context, cur *serverSnapshot, now time.Time) (*serverSnapshot, error) {
	raw, at, err := s.store.GetSettingWithTime(ctx, domain.ServerSettingsKey)
	if errors.Is(err, domain.ErrNotFound) {
		return &serverSnapshot{settings: domain.DefaultServerSettings(), loadedAt: now}, nil
	}
	if err != nil {
		return nil, err
	}
	if cur != nil && cur.updatedAt.Equal(at) {
		next := *cur
		next.loadedAt = now
		return &next, nil
	}
	settings, err := decodeServerSettings(raw)
	if err != nil {
		return nil, err
	}
	return &serverSnapshot{settings: settings, updatedAt: at, loadedAt: now}, nil
}

// decodeServerSettings reads stored settings over the defaults, so fields
// added later start with their default value.
func decodeServerSettings(raw string) (domain.ServerSettings, error) {
	settings := domain.DefaultServerSettings()
	settings.Retention.ReadDaysByPriority = nil // stored map replaces, not merges
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return settings, err
	}
	settings.Normalize()
	return settings, nil
}

// ServerSettings returns the stored settings and when they last changed
// (zero when never saved). Admins in a login session only, like sign-in
// settings: they include SSRF switches.
func (s *Service) ServerSettings(ctx context.Context, p *auth.Principal) (domain.ServerSettings, time.Time, error) {
	if err := requireSessionAdmin(p); err != nil {
		return domain.ServerSettings{}, time.Time{}, err
	}
	raw, at, err := s.store.GetSettingWithTime(ctx, domain.ServerSettingsKey)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.DefaultServerSettings(), time.Time{}, nil
	}
	if err != nil {
		return domain.ServerSettings{}, time.Time{}, err
	}
	settings, err := decodeServerSettings(raw)
	return settings, at, err
}

// SaveServerSettings validates and stores the settings; they take effect on
// this instance at once and on others within SignInTTL.
func (s *Service) SaveServerSettings(ctx context.Context, p *auth.Principal, next domain.ServerSettings) (domain.ServerSettings, time.Time, error) {
	if err := requireSessionAdmin(p); err != nil {
		return next, time.Time{}, err
	}
	next.Normalize()
	if err := next.Validate(); err != nil {
		return next, time.Time{}, err
	}
	b, err := json.Marshal(next)
	if err != nil {
		return next, time.Time{}, err
	}
	at := s.now()
	if err := s.store.PutSetting(ctx, domain.ServerSettingsKey, string(b), at); err != nil {
		return next, time.Time{}, err
	}
	s.serverMu.Lock()
	s.server.Store(&serverSnapshot{settings: next, updatedAt: at, loadedAt: at})
	s.serverMu.Unlock()
	s.log.InfoContext(ctx, "server settings changed", "user_id", p.User.ID,
		"webhooks_allow_private", next.Webhooks.AllowPrivate, "actions_allow_private", next.Actions.AllowPrivate)
	return next, at, nil
}

// SessionTTL is the login session lifetime in effect.
func (s *Service) SessionTTL(ctx context.Context) time.Duration {
	return s.Settings(ctx).SessionTTL()
}

// PushEnabled reports whether Web Push is switched on in the server settings.
func (s *Service) PushEnabled(ctx context.Context) bool {
	return s.Settings(ctx).Push.Enabled
}

// publishLimiters are rate limiters built for one PublishLimitSettings.
type publishLimiters struct {
	limit       domain.PublishLimitSettings
	token, user *auth.RateLimiter
}

// limiters returns the publish limiters for the settings in effect,
// rebuilding them (and so resetting the buckets) when the limits changed.
func (s *Service) limiters(ctx context.Context) *publishLimiters {
	limit := s.Settings(ctx).PublishLimit
	if cur := s.publishLimit.Load(); cur != nil && cur.limit == limit {
		return cur
	}
	next := &publishLimiters{
		limit: limit,
		token: auth.NewRateLimiter(limit.PerToken, limit.PerToken),
		user:  auth.NewRateLimiter(limit.PerUser, limit.PerUser),
	}
	// A concurrent rebuild for the same limit is harmless: one of the two wins.
	s.publishLimit.Store(next)
	return next
}
