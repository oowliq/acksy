package service

import (
	"context"
	"strings"

	"github.com/oowliq/acksy/internal/auth"
	"github.com/oowliq/acksy/internal/domain"
	"github.com/oowliq/acksy/internal/store"
)

// SystemPrincipal acts with full admin rights; used by the CLI and bootstrap.
func SystemPrincipal() *auth.Principal {
	u := &domain.User{Username: "system", Role: domain.RoleAdmin}
	return &auth.Principal{User: u, Scopes: domain.SessionScopes(u)}
}

// UserPrincipal returns a session-equivalent principal for u (CLI use).
func UserPrincipal(u *domain.User) *auth.Principal {
	return &auth.Principal{User: u, Scopes: domain.SessionScopes(u)}
}

// CreateUserInput describes a new user.
type CreateUserInput struct {
	Username    string
	Password    string
	DisplayName string
	Email       string
	Role        domain.Role
}

// requireAccountAdmin allows admins in a login session only. Creating users,
// setting passwords, changing roles and linking identities all grant a way to
// sign in, so an admin API token could otherwise turn itself into a login
// session and mint new tokens (invariant 7).
func requireAccountAdmin(p *auth.Principal) error {
	if err := p.Require(domain.ScopeAdmin); err != nil {
		return err
	}
	return requireSession(p, "changing users")
}

// CreateUser creates a user (admin, login session only).
func (s *Service) CreateUser(ctx context.Context, p *auth.Principal, in CreateUserInput) (*domain.User, error) {
	if err := requireAccountAdmin(p); err != nil {
		return nil, err
	}
	return s.createUser(ctx, s.store, in)
}

func (s *Service) createUser(ctx context.Context, q store.Querier, in CreateUserInput) (*domain.User, error) {
	username := domain.NormalizeUsername(in.Username)
	if err := domain.ValidateUsername(username); err != nil {
		return nil, err
	}
	if in.Role == "" {
		in.Role = domain.RoleUser
	}
	if !in.Role.Valid() {
		return nil, domain.Invalid("role must be admin or user")
	}
	if err := validateProfile(in.DisplayName, in.Email); err != nil {
		return nil, err
	}
	var hash string
	if in.Password != "" {
		if err := domain.ValidatePassword(in.Password); err != nil {
			return nil, err
		}
		var err error
		if hash, err = auth.HashPassword(in.Password); err != nil {
			return nil, err
		}
	}
	now := s.now()
	u := &domain.User{
		ID: s.newID(), Username: username, DisplayName: strings.TrimSpace(in.DisplayName),
		Email: strings.TrimSpace(in.Email), Role: in.Role, PasswordHash: hash, CreatedAt: now, UpdatedAt: now,
	}
	if err := q.CreateUser(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

func validateProfile(displayName, email string) error {
	if len(strings.TrimSpace(displayName)) > 128 {
		return domain.Invalid("display_name must be at most 128 characters")
	}
	email = strings.TrimSpace(email)
	if email != "" && (len(email) > 254 || !strings.Contains(email, "@")) {
		return domain.Invalid("email is invalid")
	}
	return nil
}

// BootstrapAdmin creates the first admin when the instance has no users.
func (s *Service) BootstrapAdmin(ctx context.Context, username, password string) (bool, error) {
	created := false
	err := s.store.InTx(ctx, func(q store.Querier) error {
		n, err := q.CountUsers(ctx)
		if err != nil || n > 0 {
			return err
		}
		if _, err := s.createUser(ctx, q, CreateUserInput{Username: username, Password: password, Role: domain.RoleAdmin}); err != nil {
			return err
		}
		created = true
		return nil
	})
	return created, err
}

// ListUsers lists all users (admin only).
func (s *Service) ListUsers(ctx context.Context, p *auth.Principal) ([]domain.User, error) {
	if err := p.Require(domain.ScopeAdmin); err != nil {
		return nil, err
	}
	return s.store.ListUsers(ctx)
}

// GetUser returns a user (admin only).
func (s *Service) GetUser(ctx context.Context, p *auth.Principal, id string) (*domain.User, error) {
	if err := p.Require(domain.ScopeAdmin); err != nil {
		return nil, err
	}
	return s.store.GetUser(ctx, id)
}

// GetUserByUsername returns a user by name (admin only).
func (s *Service) GetUserByUsername(ctx context.Context, p *auth.Principal, username string) (*domain.User, error) {
	if err := p.Require(domain.ScopeAdmin); err != nil {
		return nil, err
	}
	return s.store.GetUserByUsername(ctx, domain.NormalizeUsername(username))
}

// UpdateUserInput is a partial user update; nil fields are unchanged.
type UpdateUserInput struct {
	DisplayName *string
	Email       *string
	Role        *domain.Role
	Disabled    *bool
	Password    *string
}

// UpdateUser changes a user (admin, login session only). Disabling a user or resetting the
// password ends their sessions. The last active admin cannot be demoted or
// disabled.
func (s *Service) UpdateUser(ctx context.Context, p *auth.Principal, id string, in UpdateUserInput) (*domain.User, error) {
	if err := requireAccountAdmin(p); err != nil {
		return nil, err
	}
	var out *domain.User
	err := s.store.InTx(ctx, func(q store.Querier) error {
		u, err := q.GetUser(ctx, id)
		if err != nil {
			return err
		}
		if err := applyProfile(u, in.DisplayName, in.Email); err != nil {
			return err
		}
		wasActiveAdmin := u.IsAdmin() && !u.Disabled()
		if in.Role != nil {
			if !in.Role.Valid() {
				return domain.Invalid("role must be admin or user")
			}
			u.Role = *in.Role
		}
		endSessions := false
		if in.Disabled != nil {
			switch {
			case *in.Disabled && !u.Disabled():
				now := s.now()
				u.DisabledAt, endSessions = &now, true
			case !*in.Disabled:
				u.DisabledAt = nil
			}
		}
		if in.Password != nil {
			if err := setPassword(u, *in.Password); err != nil {
				return err
			}
			endSessions = true
		}
		if wasActiveAdmin && (!u.IsAdmin() || u.Disabled()) {
			if err := ensureAnotherAdmin(ctx, q); err != nil {
				return err
			}
		}
		u.UpdatedAt = s.now()
		if err := q.UpdateUser(ctx, u); err != nil {
			return err
		}
		if endSessions {
			if err := q.DeleteUserSessions(ctx, u.ID); err != nil {
				return err
			}
		}
		out = u
		return nil
	})
	return out, err
}

func ensureAnotherAdmin(ctx context.Context, q store.Querier) error {
	n, err := q.CountActiveAdmins(ctx)
	if err != nil {
		return err
	}
	if n <= 1 {
		return domain.Conflict("cannot remove the last active admin")
	}
	return nil
}

func applyProfile(u *domain.User, displayName, email *string) error {
	dn, em := u.DisplayName, u.Email
	if displayName != nil {
		dn = strings.TrimSpace(*displayName)
	}
	if email != nil {
		em = strings.TrimSpace(*email)
	}
	if err := validateProfile(dn, em); err != nil {
		return err
	}
	u.DisplayName, u.Email = dn, em
	return nil
}

func setPassword(u *domain.User, password string) error {
	if err := domain.ValidatePassword(password); err != nil {
		return err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	u.PasswordHash = hash
	return nil
}

// DeleteUser removes a user and everything they own (admin, login session only).
func (s *Service) DeleteUser(ctx context.Context, p *auth.Principal, id string) error {
	if err := requireAccountAdmin(p); err != nil {
		return err
	}
	return s.store.InTx(ctx, func(q store.Querier) error {
		u, err := q.GetUser(ctx, id)
		if err != nil {
			return err
		}
		if u.IsAdmin() && !u.Disabled() {
			if err := ensureAnotherAdmin(ctx, q); err != nil {
				return err
			}
		}
		return q.DeleteUser(ctx, id)
	})
}

// UpdateProfile changes the caller's own display name and email.
func (s *Service) UpdateProfile(ctx context.Context, p *auth.Principal, displayName, email *string) (*domain.User, error) {
	if !p.IsSession() {
		return nil, domain.Forbidden("profile changes require a login session")
	}
	u, err := s.store.GetUser(ctx, p.User.ID)
	if err != nil {
		return nil, err
	}
	if err := applyProfile(u, displayName, email); err != nil {
		return nil, err
	}
	u.UpdatedAt = s.now()
	return u, s.store.UpdateUser(ctx, u)
}

// ChangePassword changes the caller's password and ends their other sessions.
func (s *Service) ChangePassword(ctx context.Context, p *auth.Principal, current, next string) error {
	if !p.IsSession() {
		return domain.Forbidden("password changes require a login session")
	}
	ttl := s.SessionTTL(ctx) // read before the transaction
	return s.store.InTx(ctx, func(q store.Querier) error {
		u, err := q.GetUser(ctx, p.User.ID)
		if err != nil {
			return err
		}
		if u.PasswordHash != "" {
			ok, err := auth.CheckPassword(u.PasswordHash, current)
			if err != nil {
				return err
			}
			if !ok {
				return domain.Invalid("current password is incorrect")
			}
		}
		if err := setPassword(u, next); err != nil {
			return err
		}
		u.UpdatedAt = s.now()
		if err := q.UpdateUser(ctx, u); err != nil {
			return err
		}
		if err := q.DeleteUserSessions(ctx, u.ID); err != nil {
			return err
		}
		// Keep the session that made the change.
		now := s.now()
		return q.CreateSession(ctx, &domain.Session{
			ID: p.SessionID, UserID: u.ID, CreatedAt: now, ExpiresAt: now.Add(ttl), LastSeenAt: now,
		})
	})
}
