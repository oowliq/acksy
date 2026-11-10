package service

import (
	"context"
	"errors"
	"math/rand/v2"
	"regexp"
	"strings"

	"github.com/oowliq/acksy/internal/auth"
	"github.com/oowliq/acksy/internal/domain"
	"github.com/oowliq/acksy/internal/store"
)

// resolveTopic finds the topic a caller means by ref ("name" or
// "owner/name"). Unqualified names prefer the caller's own topic, then a
// unique shared one. When autoCreate is set, a missing own topic is created.
func (s *Service) resolveTopic(ctx context.Context, q store.Querier, p *auth.Principal, ref string, autoCreate bool, em *emitter) (*domain.Topic, error) {
	r, err := domain.ParseTopicRef(ref)
	if err != nil {
		return nil, err
	}

	var t *domain.Topic
	switch {
	case r.Owner != "":
		owner, err := q.GetUserByUsername(ctx, r.Owner)
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.NotFound("topic")
		}
		if err != nil {
			return nil, err
		}
		if t, err = q.GetTopicByOwnerAndName(ctx, owner.ID, r.Name); err != nil {
			return nil, err
		}
	default:
		t, err = q.GetTopicByOwnerAndName(ctx, p.User.ID, r.Name)
		if errors.Is(err, domain.ErrNotFound) {
			shared, ferr := q.FindMemberTopicsByName(ctx, p.User.ID, r.Name)
			if ferr != nil {
				return nil, ferr
			}
			switch {
			case len(shared) == 1:
				t, err = &shared[0], nil
			case len(shared) > 1:
				return nil, domain.Invalid("topic %q is ambiguous, use owner/%s", r.Name, r.Name)
			case autoCreate && p.TopicRestriction() == nil:
				return s.createTopic(ctx, q, p, CreateTopicInput{Name: r.Name}, em)
			}
		}
		if err != nil {
			return nil, err
		}
	}

	if _, err := q.GetMemberRole(ctx, t.ID, p.User.ID); err != nil {
		return nil, notMember(err)
	}
	if !p.CanUseTopic(t.ID) {
		return nil, domain.Forbidden("token is not allowed to use topic %q", t.Name)
	}
	return t, nil
}

// notMember hides topics the caller cannot see.
func notMember(err error) error {
	if errors.Is(err, domain.ErrNotFound) {
		return domain.NotFound("topic")
	}
	return err
}

// CreateTopicInput describes a new topic.
type CreateTopicInput struct {
	Color           *string
	Name            string
	Title           string
	Description     string
	DefaultPriority *int
	// RetentionDays and MaxNotifications override instance retention (nil: inherit).
	RetentionDays    *int
	MaxNotifications *int
}

// CreateTopic creates a topic owned by the caller.
func (s *Service) CreateTopic(ctx context.Context, p *auth.Principal, in CreateTopicInput) (*domain.Topic, error) {
	if err := p.Require(domain.ScopeTopicsManage); err != nil {
		return nil, err
	}
	if p.TopicRestriction() != nil {
		return nil, domain.Forbidden("topic-restricted tokens cannot create topics")
	}
	var out *domain.Topic
	err := s.tx(ctx, func(q store.Querier, em *emitter) error {
		var err error
		out, err = s.createTopic(ctx, q, p, in, em)
		return err
	})
	return out, err
}

func (s *Service) createTopic(ctx context.Context, q store.Querier, p *auth.Principal, in CreateTopicInput, em *emitter) (*domain.Topic, error) {
	name := domain.NormalizeTopicName(in.Name)
	if err := domain.ValidateTopicName(name); err != nil {
		return nil, err
	}
	t := &domain.Topic{ID: s.newID(), OwnerID: p.User.ID, Name: name}
	if in.Color == nil {
		t.Color = topicPalette[rand.IntN(len(topicPalette))] //nolint:gosec // Decorative color selection.
	}
	if err := applyTopicFields(t, in.Color, &in.Title, &in.Description, in.DefaultPriority, in.DefaultPriority != nil); err != nil {
		return nil, err
	}
	t.RetentionDays, t.MaxNotifications = in.RetentionDays, in.MaxNotifications
	if err := domain.ValidateTopicRetention(t.RetentionDays, t.MaxNotifications); err != nil {
		return nil, err
	}
	t.CreatedAt, t.UpdatedAt = s.now(), s.now()
	if err := q.CreateTopic(ctx, t); err != nil {
		return nil, err
	}
	created, err := q.GetTopic(ctx, t.ID)
	if err != nil {
		return nil, err
	}
	created.Role = domain.MemberOwner
	return created, em.emit(ctx, p.User.ID, domain.EventTopicChanged, created)
}

var topicPalette = [...]string{"#cba6f7", "#89dceb", "#94e2d5", "#f9e2af", "#74c7ec", "#89b4fa"}

var topicHex = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3,4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`)

func applyTopicFields(t *domain.Topic, color, title, description *string, prio *int, setPrio bool) error {
	if color != nil {
		if !topicHex.MatchString(*color) {
			return domain.Invalid("color must be a hex color such as #89b4fa")
		}
		t.Color = strings.ToLower(*color)
	}
	if title != nil {
		t.Title = strings.TrimSpace(*title)
	}
	if description != nil {
		t.Description = strings.TrimSpace(*description)
	}
	if len(t.Title) > 128 || len(t.Description) > 1024 {
		return domain.Invalid("title must be at most 128 and description at most 1024 characters")
	}
	if setPrio {
		if prio != nil && (*prio < domain.PriorityMin || *prio > domain.PriorityMax) {
			return domain.Invalid("default_priority must be between 1 and 5")
		}
		t.DefaultPriority = prio
	}
	return nil
}

// ListTopics lists topics the caller owns or is a member of.
func (s *Service) ListTopics(ctx context.Context, p *auth.Principal) ([]domain.Topic, error) {
	if err := p.Require(domain.ScopeRead); err != nil {
		return nil, err
	}
	all, err := s.store.ListTopicsForUser(ctx, p.User.ID)
	if err != nil {
		return nil, err
	}
	out := all[:0]
	for _, t := range all {
		if p.CanUseTopic(t.ID) {
			out = append(out, t)
		}
	}
	return out, nil
}

// visibleTopic loads a topic the caller is a member of, with their role.
func (s *Service) visibleTopic(ctx context.Context, q store.Querier, p *auth.Principal, id string) (*domain.Topic, error) {
	role, err := q.GetMemberRole(ctx, id, p.User.ID)
	if err != nil {
		return nil, notMember(err)
	}
	if !p.CanUseTopic(id) {
		return nil, domain.NotFound("topic")
	}
	t, err := q.GetTopic(ctx, id)
	if err != nil {
		return nil, err
	}
	t.Role = role
	return t, nil
}

func (s *Service) ownedTopic(ctx context.Context, q store.Querier, p *auth.Principal, id string) (*domain.Topic, error) {
	if err := p.Require(domain.ScopeTopicsManage); err != nil {
		return nil, err
	}
	t, err := s.visibleTopic(ctx, q, p, id)
	if err != nil {
		return nil, err
	}
	if t.Role != domain.MemberOwner {
		return nil, domain.Forbidden("only the topic owner can do this")
	}
	return t, nil
}

// GetTopic returns a topic the caller is a member of.
func (s *Service) GetTopic(ctx context.Context, p *auth.Principal, id string) (*domain.Topic, error) {
	if err := p.Require(domain.ScopeRead); err != nil {
		return nil, err
	}
	return s.visibleTopic(ctx, s.store, p, id)
}

// UpdateTopicInput is a partial topic update.
type UpdateTopicInput struct {
	Color              *string
	Title              *string
	Description        *string
	DefaultPriority    *int
	SetDefaultPriority bool // true when DefaultPriority was sent (possibly null)
	// Retention overrides; the Set flags mark fields that were sent (null
	// clears the override).
	RetentionDays       *int
	SetRetentionDays    bool
	MaxNotifications    *int
	SetMaxNotifications bool
}

// UpdateTopic changes a topic (owner only).
func (s *Service) UpdateTopic(ctx context.Context, p *auth.Principal, id string, in UpdateTopicInput) (*domain.Topic, error) {
	var out *domain.Topic
	err := s.tx(ctx, func(q store.Querier, em *emitter) error {
		t, err := s.ownedTopic(ctx, q, p, id)
		if err != nil {
			return err
		}
		if err := applyTopicFields(t, in.Color, in.Title, in.Description, in.DefaultPriority, in.SetDefaultPriority); err != nil {
			return err
		}
		if in.SetRetentionDays {
			t.RetentionDays = in.RetentionDays
		}
		if in.SetMaxNotifications {
			t.MaxNotifications = in.MaxNotifications
		}
		if err := domain.ValidateTopicRetention(t.RetentionDays, t.MaxNotifications); err != nil {
			return err
		}
		t.UpdatedAt = s.now()
		if err := q.UpdateTopic(ctx, t); err != nil {
			return err
		}
		out = t
		return s.emitToMembers(ctx, q, em, t.ID, domain.EventTopicChanged, t)
	})
	return out, err
}

// DeleteTopic deletes a topic (owner only). Its notifications stay with their
// recipients without a topic.
func (s *Service) DeleteTopic(ctx context.Context, p *auth.Principal, id string) error {
	return s.tx(ctx, func(q store.Querier, em *emitter) error {
		t, err := s.ownedTopic(ctx, q, p, id)
		if err != nil {
			return err
		}
		if err := s.emitToMembers(ctx, q, em, t.ID, domain.EventTopicRemoved, domain.IDEventData{ID: t.ID}); err != nil {
			return err
		}
		return q.DeleteTopic(ctx, t.ID)
	})
}

func (s *Service) emitToMembers(ctx context.Context, q store.Querier, em *emitter, topicID string, typ domain.EventType, data any) error {
	members, err := q.ListTopicMembers(ctx, topicID)
	if err != nil {
		return err
	}
	for _, m := range members {
		if err := em.emit(ctx, m.UserID, typ, data); err != nil {
			return err
		}
	}
	return nil
}

// ListTopicMembers lists a topic's members (members only).
func (s *Service) ListTopicMembers(ctx context.Context, p *auth.Principal, topicID string) ([]domain.TopicMember, error) {
	if err := p.Require(domain.ScopeRead); err != nil {
		return nil, err
	}
	if _, err := s.visibleTopic(ctx, s.store, p, topicID); err != nil {
		return nil, err
	}
	return s.store.ListTopicMembers(ctx, topicID)
}

// RemoveTopicMember removes a member. Owners remove others; members leave.
func (s *Service) RemoveTopicMember(ctx context.Context, p *auth.Principal, topicID, userID string) error {
	return s.tx(ctx, func(q store.Querier, em *emitter) error {
		t, err := s.visibleTopic(ctx, q, p, topicID)
		if err != nil {
			return err
		}
		self := userID == p.User.ID
		switch {
		case self && t.Role == domain.MemberOwner:
			return domain.Conflict("the owner cannot leave; delete the topic instead")
		case !self:
			if _, err := s.ownedTopic(ctx, q, p, topicID); err != nil {
				return err
			}
		}
		if err := q.RemoveTopicMember(ctx, t.ID, userID); err != nil {
			return err
		}
		if err := em.emit(ctx, userID, domain.EventTopicRemoved, domain.IDEventData{ID: t.ID}); err != nil {
			return err
		}
		t.MemberCount--
		return s.emitToMembers(ctx, q, em, t.ID, domain.EventTopicChanged, t)
	})
}
