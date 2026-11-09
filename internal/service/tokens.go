package service

import (
	"context"
	"strings"
	"time"

	"github.com/oowliq/acksy/internal/auth"
	"github.com/oowliq/acksy/internal/domain"
	"github.com/oowliq/acksy/internal/store"
)

// CreateTokenInput describes a new API token.
type CreateTokenInput struct {
	Name      string
	Scopes    []string
	Topics    []string // topic references; empty = all topics
	ExpiresAt *time.Time
}

// CreatedToken carries the one-time visible secret.
type CreatedToken struct {
	Token  *domain.APIToken
	Secret string
}

// CreateToken issues an API token for the caller. Tokens can only be managed
// from a login session, so a leaked token cannot mint new ones.
func (s *Service) CreateToken(ctx context.Context, p *auth.Principal, in CreateTokenInput) (*CreatedToken, error) {
	if !p.IsSession() {
		return nil, domain.Forbidden("tokens can only be managed from a login session")
	}
	name := strings.TrimSpace(in.Name)
	if name == "" || len(name) > 64 {
		return nil, domain.Invalid("name must be 1-64 characters")
	}
	scopes, err := domain.ParseScopes(in.Scopes)
	if err != nil {
		return nil, err
	}
	if scopes.Has(domain.ScopeAdmin) && !p.User.IsAdmin() {
		return nil, domain.Forbidden("only admins can create tokens with the admin scope")
	}
	now := s.now()
	if in.ExpiresAt != nil && !in.ExpiresAt.After(now) {
		return nil, domain.Invalid("expires_at must be in the future")
	}

	var out *CreatedToken
	err = s.store.InTx(ctx, func(q store.Querier) error {
		topicIDs := []string{}
		for _, ref := range in.Topics {
			t, err := s.resolveTopic(ctx, q, p, ref, false, nil)
			if err != nil {
				return err
			}
			topicIDs = append(topicIDs, t.ID)
		}
		secret, prefix, hash := auth.NewAPIToken()
		tok := &domain.APIToken{
			ID: s.newID(), UserID: p.User.ID, Name: name, Prefix: prefix, SecretHash: hash,
			Scopes: scopes, TopicIDs: topicIDs, ExpiresAt: in.ExpiresAt, CreatedAt: now,
		}
		if err := q.CreateToken(ctx, tok); err != nil {
			return err
		}
		out = &CreatedToken{Token: tok, Secret: secret}
		return nil
	})
	return out, err
}

// ListTokens lists the caller's tokens.
func (s *Service) ListTokens(ctx context.Context, p *auth.Principal) ([]domain.APIToken, error) {
	if !p.IsSession() {
		return nil, domain.Forbidden("tokens can only be managed from a login session")
	}
	return s.store.ListTokens(ctx, p.User.ID)
}

// DeleteToken revokes one of the caller's tokens.
func (s *Service) DeleteToken(ctx context.Context, p *auth.Principal, id string) error {
	if !p.IsSession() {
		return domain.Forbidden("tokens can only be managed from a login session")
	}
	return s.store.DeleteToken(ctx, p.User.ID, id)
}
