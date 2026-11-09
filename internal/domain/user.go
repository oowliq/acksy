package domain

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"
)

// Role is a user's instance-wide role.
type Role string

const (
	RoleAdmin Role = "admin"
	RoleUser  Role = "user"
)

// Valid reports whether r is a known role.
func (r Role) Valid() bool { return r == RoleAdmin || r == RoleUser }

// User owns notifications.
type User struct {
	ID           string     `json:"id"`
	Username     string     `json:"username"`
	DisplayName  string     `json:"display_name"`
	Email        string     `json:"email,omitempty"`
	Role         Role       `json:"role"`
	PasswordHash string     `json:"-"`
	DisabledAt   *time.Time `json:"disabled_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// MarshalJSON adds has_password, so clients can tell single-sign-on-only
// accounts apart. The hash itself is never serialized.
func (u User) MarshalJSON() ([]byte, error) {
	type plain User // no methods: avoids recursion
	return json.Marshal(struct {
		plain
		HasPassword bool `json:"has_password"`
	}{plain(u), u.PasswordHash != ""})
}

// IsAdmin reports whether the user has the admin role.
func (u *User) IsAdmin() bool { return u.Role == RoleAdmin }

// Disabled reports whether the account is disabled.
func (u *User) Disabled() bool { return u.DisabledAt != nil }

var usernameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,31}$`)

// NormalizeUsername lowercases and trims a username.
func NormalizeUsername(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// ValidateUsername checks a normalized username.
func ValidateUsername(s string) error {
	if !usernameRe.MatchString(s) {
		return Invalid("username must be 2-32 characters: a-z, 0-9, '.', '_', '-' and start with a letter or digit")
	}
	return nil
}

// MinPasswordLen is the minimum accepted password length.
const MinPasswordLen = 8

// ValidatePassword checks password strength rules.
func ValidatePassword(p string) error {
	if len(p) < MinPasswordLen {
		return Invalid("password must be at least %d characters", MinPasswordLen)
	}
	if len(p) > 256 {
		return Invalid("password must be at most 256 characters")
	}
	return nil
}
