package domain

import (
	"slices"
	"strings"
)

// Scope is a permission carried by an API token.
type Scope string

const (
	ScopePublish      Scope = "notifications:publish"
	ScopeRead         Scope = "notifications:read"
	ScopeWrite        Scope = "notifications:write" // read-state changes, delete
	ScopeTopicsManage Scope = "topics:manage"
	ScopeAdmin        Scope = "admin"
)

// AllScopes lists every scope in canonical order.
var AllScopes = []Scope{ScopePublish, ScopeRead, ScopeWrite, ScopeTopicsManage, ScopeAdmin}

// Scopes is a set of scopes.
type Scopes []Scope

// ParseScopes parses and validates scope names, deduplicating them.
func ParseScopes(names []string) (Scopes, error) {
	if len(names) == 0 {
		return nil, Invalid("at least one scope is required")
	}
	out := make(Scopes, 0, len(names))
	for _, n := range names {
		s := Scope(strings.TrimSpace(n))
		if !slices.Contains(AllScopes, s) {
			return nil, Invalid("unknown scope %q", n)
		}
		if !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	slices.SortFunc(out, func(a, b Scope) int {
		return slices.Index(AllScopes, a) - slices.Index(AllScopes, b)
	})
	return out, nil
}

// Has reports whether s contains scope.
func (s Scopes) Has(scope Scope) bool { return slices.Contains(s, scope) }

// String encodes scopes space-separated (storage format).
func (s Scopes) String() string {
	parts := make([]string, len(s))
	for i, sc := range s {
		parts[i] = string(sc)
	}
	return strings.Join(parts, " ")
}

// ScopesFromString decodes the storage format without validation.
func ScopesFromString(v string) Scopes {
	var out Scopes
	for _, p := range strings.Fields(v) {
		out = append(out, Scope(p))
	}
	return out
}

// SessionScopes are the scopes of an interactive (cookie) session.
func SessionScopes(u *User) Scopes {
	s := Scopes{ScopePublish, ScopeRead, ScopeWrite, ScopeTopicsManage}
	if u.IsAdmin() {
		s = append(s, ScopeAdmin)
	}
	return s
}
