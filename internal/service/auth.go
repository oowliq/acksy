package service

import (
	"context"
	"errors"
	"time"

	"github.com/oowliq/acksy/internal/auth"
	"github.com/oowliq/acksy/internal/domain"
	"github.com/oowliq/acksy/internal/store"
)

var errBadCredentials = &domain.Error{Kind: domain.KindUnauthorized, Msg: "invalid username or password"}

// Login checks credentials and opens a session. It returns the cookie secret.
func (s *Service) Login(ctx context.Context, username, password, userAgent string) (string, *domain.User, error) {
	if s.SignIn(ctx).Settings.DisablePasswordLogin {
		return "", nil, ErrPasswordLoginDisabled
	}
	u, err := s.store.GetUserByUsername(ctx, domain.NormalizeUsername(username))
	if errors.Is(err, domain.ErrNotFound) {
		auth.BurnPasswordCheck(password)
		return "", nil, errBadCredentials
	}
	if err != nil {
		return "", nil, err
	}
	if u.PasswordHash == "" {
		auth.BurnPasswordCheck(password)
		return "", nil, errBadCredentials
	}
	ok, err := auth.CheckPassword(u.PasswordHash, password)
	if err != nil {
		return "", nil, err
	}
	if !ok || u.Disabled() {
		return "", nil, errBadCredentials
	}

	secret, err := s.newSession(ctx, s.store, u.ID, userAgent)
	if err != nil {
		return "", nil, err
	}
	return secret, u, nil
}

// ErrPasswordLoginDisabled is returned by Login when only OIDC sign-in is allowed.
var ErrPasswordLoginDisabled = &domain.Error{Kind: domain.KindForbidden, Msg: "password login is disabled; sign in with SSO"}

// newSession stores a login session for userID and returns the cookie secret.
func (s *Service) newSession(ctx context.Context, q store.Querier, userID, userAgent string) (string, error) {
	secret, hash := auth.NewSessionSecret()
	now := s.now()
	if len(userAgent) > 256 {
		userAgent = userAgent[:256]
	}
	sess := &domain.Session{ID: hash, UserID: userID, UserAgent: userAgent, CreatedAt: now, ExpiresAt: now.Add(s.opts.SessionTTL), LastSeenAt: now}
	if err := q.CreateSession(ctx, sess); err != nil {
		return "", err
	}
	return secret, nil
}

// Logout ends the principal's session.
func (s *Service) Logout(ctx context.Context, p *auth.Principal) error {
	if p.SessionID == "" {
		return nil
	}
	return s.store.DeleteSession(ctx, p.SessionID)
}

// sessionTouchInterval limits how often a sliding session is written.
const sessionTouchInterval = time.Hour

// AuthenticateSession resolves a session cookie secret.
func (s *Service) AuthenticateSession(ctx context.Context, secret string) (*auth.Principal, error) {
	sess, err := s.store.GetSession(ctx, auth.HashSecret(secret))
	if errors.Is(err, domain.ErrNotFound) {
		return nil, domain.ErrUnauthorized
	}
	if err != nil {
		return nil, err
	}
	now := s.now()
	if !now.Before(sess.ExpiresAt) {
		_ = s.store.DeleteSession(ctx, sess.ID)
		return nil, domain.ErrUnauthorized
	}
	u, err := s.activeUser(ctx, sess.UserID)
	if err != nil {
		return nil, err
	}
	if now.Sub(sess.LastSeenAt) >= sessionTouchInterval {
		if err := s.store.TouchSession(ctx, sess.ID, now, now.Add(s.opts.SessionTTL)); err != nil {
			s.log.Warn("touch session", "err", err)
		}
	}
	return &auth.Principal{User: u, SessionID: sess.ID, Scopes: domain.SessionScopes(u)}, nil
}

// tokenTouchInterval limits last_used_at writes.
const tokenTouchInterval = time.Minute

// AuthenticateToken resolves an API token secret.
func (s *Service) AuthenticateToken(ctx context.Context, secret string) (*auth.Principal, error) {
	if !auth.LooksLikeAPIToken(secret) {
		return nil, domain.ErrUnauthorized
	}
	tok, err := s.store.GetTokenByHash(ctx, auth.HashSecret(secret))
	if errors.Is(err, domain.ErrNotFound) {
		return nil, domain.ErrUnauthorized
	}
	if err != nil {
		return nil, err
	}
	now := s.now()
	if tok.Expired(now) {
		return nil, &domain.Error{Kind: domain.KindUnauthorized, Msg: "token expired"}
	}
	u, err := s.activeUser(ctx, tok.UserID)
	if err != nil {
		return nil, err
	}
	if tok.LastUsedAt == nil || now.Sub(*tok.LastUsedAt) >= tokenTouchInterval {
		if err := s.store.TouchToken(ctx, tok.ID, now); err != nil {
			s.log.Warn("touch token", "err", err)
		}
	}
	return &auth.Principal{User: u, Token: tok, Scopes: tok.Scopes}, nil
}

func (s *Service) activeUser(ctx context.Context, id string) (*domain.User, error) {
	u, err := s.store.GetUser(ctx, id)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, domain.ErrUnauthorized
	}
	if err != nil {
		return nil, err
	}
	if u.Disabled() {
		return nil, &domain.Error{Kind: domain.KindUnauthorized, Msg: "account disabled"}
	}
	return u, nil
}

// PruneSessions deletes expired sessions.
func (s *Service) PruneSessions(ctx context.Context) (int64, error) {
	return s.store.DeleteExpiredSessions(ctx, s.now())
}
