package service

import (
	"context"
	"errors"
	"strings"

	"github.com/oowliq/acksy/internal/auth"
	"github.com/oowliq/acksy/internal/domain"
	"github.com/oowliq/acksy/internal/store"
)

// OIDC login failures. They are distinct values (compare with ==) so the
// HTTP layer can show a specific message.
var (
	ErrOIDCNoAccount   = &domain.Error{Kind: domain.KindForbidden, Msg: "no Acksy account is linked to this identity"}
	ErrAccountDisabled = &domain.Error{Kind: domain.KindForbidden, Msg: "account disabled"}
)

// maxUsernameSuffix bounds the search for a free username on provisioning.
const maxUsernameSuffix = 100

// LoginOIDC signs in a verified external identity and opens a session.
//
// A linked identity logs into its user. An unknown identity gets a new user
// when auto-creation is on; it is never linked to an existing user by
// username or email (see docs/adr/0002-oidc.md). With AdminValues set, the
// role is re-derived from the claim on every login.
func (s *Service) LoginOIDC(ctx context.Context, c auth.OIDCClaims, userAgent string) (string, *domain.User, error) {
	if c.Issuer == "" || c.Subject == "" {
		return "", nil, domain.Invalid("identity has no issuer or subject")
	}
	// Read the settings before the transaction: a reload inside it would wait
	// for a connection the transaction holds (SQLite has only one).
	settings := s.SignIn(ctx).Settings
	var (
		secret string
		user   *domain.User
	)
	// A concurrent first login of the same identity (or a username race)
	// surfaces as a conflict; the retry then finds the winner's rows.
	for attempt := 0; ; attempt++ {
		err := s.store.InTx(ctx, func(q store.Querier) error {
			u, err := s.resolveOIDCUser(ctx, q, c, settings)
			if err != nil {
				return err
			}
			secret, err = s.newSession(ctx, q, u.ID, userAgent)
			user = u
			return err
		})
		if err == nil {
			return secret, user, nil
		}
		if !errors.Is(err, domain.ErrConflict) || attempt == 1 {
			return "", nil, err
		}
	}
}

func (s *Service) resolveOIDCUser(ctx context.Context, q store.Querier, c auth.OIDCClaims, settings domain.OIDCSettings) (*domain.User, error) {
	now := s.now()
	ident, err := q.GetIdentity(ctx, c.Issuer, c.Subject)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		if !settings.AutoCreate {
			return nil, ErrOIDCNoAccount
		}
		return s.provisionOIDCUser(ctx, q, c, settings.AdminValues)
	case err != nil:
		return nil, err
	}
	u, err := q.GetUser(ctx, ident.UserID)
	if err != nil {
		return nil, err
	}
	if u.Disabled() {
		return nil, ErrAccountDisabled
	}
	if err := s.syncOIDCRole(ctx, q, u, c.RoleValues, settings.AdminValues); err != nil {
		return nil, err
	}
	if err := q.TouchIdentity(ctx, c.Issuer, c.Subject, now); err != nil {
		return nil, err
	}
	return u, nil
}

func (s *Service) provisionOIDCUser(ctx context.Context, q store.Querier, c auth.OIDCClaims, admins []string) (*domain.User, error) {
	emailLocal, _, _ := strings.Cut(c.Email, "@")
	base := domain.DeriveUsername(c.Username, emailLocal, c.Name)
	username, err := freeUsername(ctx, q, base)
	if err != nil {
		return nil, err
	}
	role := domain.RoleUser
	if len(admins) > 0 {
		role = domain.RoleFromClaim(c.RoleValues, admins)
	}
	name := strings.TrimSpace(c.Name)
	if len(name) > 128 {
		name = name[:128]
	}
	email := strings.TrimSpace(c.Email)
	if validateProfile("", email) != nil {
		email = ""
	}
	u, err := s.createUser(ctx, q, CreateUserInput{Username: username, DisplayName: name, Email: email, Role: role})
	if err != nil {
		return nil, err
	}
	now := s.now()
	if err := q.CreateIdentity(ctx, &domain.Identity{
		Issuer: c.Issuer, Subject: c.Subject, UserID: u.ID, CreatedAt: now, LastLoginAt: &now,
	}); err != nil {
		return nil, err
	}
	s.log.InfoContext(ctx, "oidc user provisioned", "user_id", u.ID, "username", u.Username, "role", u.Role)
	return u, nil
}

// freeUsername returns base, or base-2, base-3, … whichever is not taken.
// It checks before inserting because a failed INSERT aborts a Postgres
// transaction.
func freeUsername(ctx context.Context, q store.Querier, base string) (string, error) {
	for n := 1; n <= maxUsernameSuffix; n++ {
		name := domain.UsernameWithSuffix(base, n)
		_, err := q.GetUserByUsername(ctx, name)
		if errors.Is(err, domain.ErrNotFound) {
			return name, nil
		}
		if err != nil {
			return "", err
		}
	}
	return "", domain.Conflict("no free username for %q", base)
}

// syncOIDCRole applies the claim-derived role. Demoting the last active
// admin is refused: the login proceeds with the current role.
func (s *Service) syncOIDCRole(ctx context.Context, q store.Querier, u *domain.User, values, admins []string) error {
	if len(admins) == 0 {
		return nil
	}
	want := domain.RoleFromClaim(values, admins)
	if want == u.Role {
		return nil
	}
	if u.IsAdmin() {
		err := ensureAnotherAdmin(ctx, q)
		if errors.Is(err, domain.ErrConflict) {
			s.log.WarnContext(ctx, "oidc role sync skipped: last active admin", "user_id", u.ID)
			return nil
		}
		if err != nil {
			return err
		}
	}
	s.log.InfoContext(ctx, "oidc role synced", "user_id", u.ID, "from", u.Role, "to", want)
	u.Role, u.UpdatedAt = want, s.now()
	return q.UpdateUser(ctx, u)
}

// LinkIdentity links an external identity to a user (admin, login session only). An empty
// issuer means the configured provider.
func (s *Service) LinkIdentity(ctx context.Context, p *auth.Principal, userID, issuer, subject string) (*domain.Identity, error) {
	if err := requireAccountAdmin(p); err != nil {
		return nil, err
	}
	if issuer == "" {
		issuer = s.SignIn(ctx).Settings.Issuer
	}
	if issuer == "" || strings.TrimSpace(subject) == "" {
		return nil, domain.Invalid("issuer and subject are required")
	}
	if _, err := s.store.GetUser(ctx, userID); err != nil {
		return nil, err
	}
	ident := &domain.Identity{Issuer: issuer, Subject: subject, UserID: userID, CreatedAt: s.now()}
	if err := s.store.CreateIdentity(ctx, ident); err != nil {
		return nil, err
	}
	return ident, nil
}

// UnlinkIdentity removes a user's external identity (admin, login session only).
func (s *Service) UnlinkIdentity(ctx context.Context, p *auth.Principal, userID, issuer, subject string) error {
	if err := requireAccountAdmin(p); err != nil {
		return err
	}
	if issuer == "" {
		issuer = s.SignIn(ctx).Settings.Issuer
	}
	// Don't let someone remove their own last way in: when they can't use a
	// password (none set, or password sign-in is off), keep at least one
	// identity at the current provider.
	if p.User.ID == userID {
		settings := s.SignIn(ctx).Settings
		canPassword := p.User.PasswordHash != "" && !settings.DisablePasswordLogin
		// With SSO on, only identities at the current provider let you in;
		// with it off, any identity may matter once it's turned back on.
		counts := func(iss string) bool { return !settings.Active() || iss == settings.Issuer }
		if !canPassword && counts(issuer) {
			idents, err := s.store.ListUserIdentities(ctx, userID)
			if err != nil {
				return err
			}
			current := 0
			for _, i := range idents {
				if counts(i.Issuer) {
					current++
				}
			}
			if current <= 1 {
				return domain.Conflict("this is your only way to sign in; link another identity or have another admin set a password first")
			}
		}
	}
	return s.store.DeleteIdentity(ctx, userID, issuer, subject)
}

// ListIdentities lists a user's external identities (admin only).
func (s *Service) ListIdentities(ctx context.Context, p *auth.Principal, userID string) ([]domain.Identity, error) {
	if err := p.Require(domain.ScopeAdmin); err != nil {
		return nil, err
	}
	if _, err := s.store.GetUser(ctx, userID); err != nil {
		return nil, err
	}
	return s.store.ListUserIdentities(ctx, userID)
}
