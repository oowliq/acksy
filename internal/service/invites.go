package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/oowliq/acksy/internal/auth"
	"github.com/oowliq/acksy/internal/domain"
	"github.com/oowliq/acksy/internal/store"
)

// Sharing a topic needs the invitee's consent: the owner invites, the
// invitee accepts (and only then receives the topic's notifications) or
// declines. The invitee learns about it from a notification in their inbox.

// InviteToTopic invites a user to a topic the caller owns. It fails for
// unknown users, members and users already invited.
func (s *Service) InviteToTopic(ctx context.Context, p *auth.Principal, topicID, username string) (*domain.TopicInvite, error) {
	if err := p.Require(domain.ScopeTopicsManage); err != nil {
		return nil, err
	}
	// The invitation sends a notification: it shares the publish budget, so
	// re-inviting after declines cannot flood someone's inbox.
	if err := s.checkPublishRate(p); err != nil {
		return nil, err
	}
	var (
		out       *domain.TopicInvite
		delivered []domain.Notification
	)
	err := s.tx(ctx, func(q store.Querier, em *emitter) error {
		delivered = delivered[:0]
		t, err := s.ownedTopic(ctx, q, p, topicID)
		if err != nil {
			return err
		}
		u, err := q.GetUserByUsername(ctx, domain.NormalizeUsername(username))
		if errors.Is(err, domain.ErrNotFound) {
			return domain.NotFound("user")
		}
		if err != nil {
			return err
		}
		switch _, err := q.GetMemberRole(ctx, t.ID, u.ID); {
		case err == nil:
			return domain.Conflict("%s is already a member of this topic", u.Username)
		case !errors.Is(err, domain.ErrNotFound):
			return err
		}
		now := s.now()
		if err := q.CreateTopicInvite(ctx, t.ID, u.ID, now); err != nil {
			return err
		}
		if out, err = q.GetTopicInvite(ctx, t.ID, u.ID); err != nil {
			return err
		}

		name := t.Name
		if t.Title != "" {
			name = t.Title
		}
		n := &domain.Notification{
			ID: s.newID(), UserID: u.ID, MessageID: s.newID(),
			Title:    fmt.Sprintf("%s invited you to the topic “%s”", p.User.Username, name),
			Body:     "You will receive its notifications once you accept. Accept or decline in **Settings → Topics**.",
			Priority: domain.PriorityDefault, Tags: []string{"invitation"}, Actions: []domain.Action{},
			DedupKey: inviteDedupKey(t.ID), Source: "acksy", CreatedAt: now, UpdatedAt: now,
		}
		wq := &webhookQueue{byUser: map[string][]domain.Webhook{}}
		stored, _, err := s.storeNotification(ctx, q, em, wq, n)
		if err != nil {
			return err
		}
		delivered = append(delivered, *stored)
		return emitInvitationChange(ctx, em, t, u.ID)
	})
	if err != nil {
		return nil, err
	}
	s.notify(delivered)
	return out, nil
}

// inviteDedupKey makes a repeated invitation to the same topic raise the
// existing inbox notification instead of adding another.
func inviteDedupKey(topicID string) string { return "acksy:topic-invite:" + topicID }

// dropInviteNotice deletes the inbox notification about an invitation once
// it is answered or revoked: it no longer asks anything of the user.
func (s *Service) dropInviteNotice(ctx context.Context, q store.Querier, em *emitter, userID, topicID string) error {
	now := s.now()
	ids, err := q.ListNotificationIDs(ctx, domain.NotificationFilter{
		UserID: userID, View: domain.ViewAll, NoTopic: true, DedupKey: inviteDedupKey(topicID), Now: now,
	})
	if err != nil || len(ids) == 0 {
		return err
	}
	if err := q.BulkUpdateNotifications(ctx, userID, ids, domain.BulkDelete, now); err != nil {
		return err
	}
	return em.emit(ctx, userID, domain.EventNotificationsBulk, domain.BulkEventData{Action: domain.BulkDelete, IDs: ids, At: now})
}

// emitInvitationChange tells the invitee and the topic owner that an
// invitation changed, so their open clients refresh.
func emitInvitationChange(ctx context.Context, em *emitter, t *domain.Topic, inviteeID string) error {
	data := domain.IDEventData{ID: t.ID}
	if err := em.emit(ctx, inviteeID, domain.EventInvitationsChanged, data); err != nil {
		return err
	}
	return em.emit(ctx, t.OwnerID, domain.EventInvitationsChanged, data)
}

// ListTopicInvites lists the pending invitations of a topic (owner only).
func (s *Service) ListTopicInvites(ctx context.Context, p *auth.Principal, topicID string) ([]domain.TopicInvite, error) {
	if _, err := s.ownedTopic(ctx, s.store, p, topicID); err != nil {
		return nil, err
	}
	return s.store.ListTopicInvites(ctx, topicID)
}

// RevokeTopicInvite withdraws a pending invitation (owner only).
func (s *Service) RevokeTopicInvite(ctx context.Context, p *auth.Principal, topicID, userID string) error {
	return s.tx(ctx, func(q store.Querier, em *emitter) error {
		t, err := s.ownedTopic(ctx, q, p, topicID)
		if err != nil {
			return err
		}
		if err := q.DeleteTopicInvite(ctx, t.ID, userID); err != nil {
			return err
		}
		if err := s.dropInviteNotice(ctx, q, em, userID, t.ID); err != nil {
			return err
		}
		return emitInvitationChange(ctx, em, t, userID)
	})
}

// requireJoin checks the caller may answer invitations. Topic-restricted
// tokens cannot: like creating topics, joining widens what they reach.
func requireJoin(p *auth.Principal) error {
	if err := p.Require(domain.ScopeTopicsManage); err != nil {
		return err
	}
	if p.TopicRestriction() != nil {
		return domain.Forbidden("topic-restricted tokens cannot answer invitations")
	}
	return nil
}

// ListMyInvites lists the caller's pending invitations.
func (s *Service) ListMyInvites(ctx context.Context, p *auth.Principal) ([]domain.TopicInvite, error) {
	if err := p.Require(domain.ScopeRead); err != nil {
		return nil, err
	}
	if p.TopicRestriction() != nil {
		return []domain.TopicInvite{}, nil
	}
	return s.store.ListUserInvites(ctx, p.User.ID)
}

// AcceptTopicInvite makes the caller a member of the topic they were
// invited to and returns the topic.
func (s *Service) AcceptTopicInvite(ctx context.Context, p *auth.Principal, topicID string) (*domain.Topic, error) {
	if err := requireJoin(p); err != nil {
		return nil, err
	}
	var out *domain.Topic
	err := s.tx(ctx, func(q store.Querier, em *emitter) error {
		if _, err := q.GetTopicInvite(ctx, topicID, p.User.ID); err != nil {
			return err
		}
		if err := q.DeleteTopicInvite(ctx, topicID, p.User.ID); err != nil {
			return err
		}
		if err := q.AddTopicMember(ctx, topicID, p.User.ID, domain.MemberMember, s.now()); err != nil {
			return err
		}
		if err := s.dropInviteNotice(ctx, q, em, p.User.ID, topicID); err != nil {
			return err
		}
		t, err := q.GetTopic(ctx, topicID)
		if err != nil {
			return err
		}
		if err := emitInvitationChange(ctx, em, t, p.User.ID); err != nil {
			return err
		}
		// Members get the topic without a role: it differs per member.
		if err := s.emitToMembers(ctx, q, em, t.ID, domain.EventTopicChanged, t); err != nil {
			return err
		}
		out = t
		out.Role = domain.MemberMember
		return nil
	})
	return out, err
}

// DeclineTopicInvite drops the caller's invitation to a topic.
func (s *Service) DeclineTopicInvite(ctx context.Context, p *auth.Principal, topicID string) error {
	if err := requireJoin(p); err != nil {
		return err
	}
	return s.tx(ctx, func(q store.Querier, em *emitter) error {
		inv, err := q.GetTopicInvite(ctx, topicID, p.User.ID)
		if err != nil {
			return err
		}
		if err := q.DeleteTopicInvite(ctx, topicID, p.User.ID); err != nil {
			return err
		}
		if err := s.dropInviteNotice(ctx, q, em, p.User.ID, inv.TopicID); err != nil {
			return err
		}
		t, err := q.GetTopic(ctx, inv.TopicID)
		if err != nil {
			return err
		}
		return emitInvitationChange(ctx, em, t, p.User.ID)
	})
}
