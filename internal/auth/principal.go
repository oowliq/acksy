package auth

import (
	"context"
	"slices"

	"github.com/oowliq/acksy/internal/domain"
)

// Principal is the authenticated caller of a request.
type Principal struct {
	User  *domain.User
	Token *domain.APIToken // nil for interactive sessions
	// SessionID is set for cookie sessions.
	SessionID string
	Scopes    domain.Scopes
}

// IsSession reports whether the caller authenticated with a login session.
func (p *Principal) IsSession() bool { return p.Token == nil }

// Require returns ErrForbidden unless the principal has scope. Admin scope
// additionally requires the admin role (a demoted user's token loses it).
func (p *Principal) Require(scope domain.Scope) error {
	if !p.Scopes.Has(scope) {
		return domain.Forbidden("missing scope %s", scope)
	}
	if scope == domain.ScopeAdmin && !p.User.IsAdmin() {
		return domain.Forbidden("admin role required")
	}
	return nil
}

// TopicRestriction returns the topics the caller is limited to, or nil when
// unrestricted.
func (p *Principal) TopicRestriction() []string {
	if p.Token == nil || len(p.Token.TopicIDs) == 0 {
		return nil
	}
	return p.Token.TopicIDs
}

// CanUseTopic reports whether a topic restriction allows topicID.
func (p *Principal) CanUseTopic(topicID string) bool {
	r := p.TopicRestriction()
	return r == nil || slices.Contains(r, topicID)
}

type ctxKey struct{}

// WithPrincipal stores p in ctx.
func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, ctxKey{}, p)
}

// FromContext returns the principal stored in ctx, or nil.
func FromContext(ctx context.Context) *Principal {
	p, _ := ctx.Value(ctxKey{}).(*Principal)
	return p
}
