package domain

import (
	"slices"
	"strconv"
	"strings"
	"time"
)

// Identity links an external (OIDC) account, identified by issuer and
// subject, to a local user.
type Identity struct {
	Issuer      string     `json:"issuer"`
	Subject     string     `json:"subject"`
	UserID      string     `json:"user_id"`
	CreatedAt   time.Time  `json:"created_at"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
}

// maxUsernameLen mirrors usernameRe.
const maxUsernameLen = 32

// DeriveUsername turns external claims into a valid username. Candidates are
// tried in order (e.g. preferred_username, then the email local part); the
// first that can be sanitized into a valid username wins, otherwise "user".
func DeriveUsername(candidates ...string) string {
	for _, c := range candidates {
		if name := sanitizeUsername(c); ValidateUsername(name) == nil {
			return name
		}
	}
	return "user"
}

func sanitizeUsername(s string) string {
	s = NormalizeUsername(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	out := strings.TrimLeft(b.String(), "._-")
	if len(out) > maxUsernameLen {
		out = out[:maxUsernameLen]
	}
	return out
}

// UsernameWithSuffix returns base with "-n" appended, truncating base so the
// result stays a valid length. n < 2 returns base unchanged.
func UsernameWithSuffix(base string, n int) string {
	if n < 2 {
		return base
	}
	suffix := "-" + strconv.Itoa(n)
	if len(base)+len(suffix) > maxUsernameLen {
		base = base[:maxUsernameLen-len(suffix)]
	}
	return base + suffix
}

// RoleFromClaim maps the values of a role claim (e.g. groups) to a role:
// admin when any value is in adminValues, user otherwise.
func RoleFromClaim(values, adminValues []string) Role {
	for _, v := range values {
		if slices.Contains(adminValues, v) {
			return RoleAdmin
		}
	}
	return RoleUser
}
