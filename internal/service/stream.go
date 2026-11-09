package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/oowliq/acksy/internal/auth"
	"github.com/oowliq/acksy/internal/broker"
	"github.com/oowliq/acksy/internal/domain"
)

// StreamKind classifies stream messages.
type StreamKind string

const (
	// StreamReady: backlog delivered; Seq is the position the client is at.
	StreamReady StreamKind = "ready"
	// StreamReset: the requested position is unavailable (pruned or from the
	// future). The client must refetch its state; streaming continues from Seq.
	StreamReset StreamKind = "reset"
	// StreamEvent carries one event.
	StreamEvent StreamKind = "event"
)

// StreamMessage is one item of a user's realtime stream.
type StreamMessage struct {
	Kind  StreamKind
	Seq   int64
	Event *domain.Event
}

const (
	streamPage         = 500
	defaultStreamCheck = 30 * time.Second
)

// Stream delivers the caller's events in order without gaps. With since set,
// it first replays events after since from the database; then it forwards live
// events from the broker. Gaps (dropped broker messages, other instances
// without a shared broker) are filled from the database, and every check
// interval the stream also re-validates the caller's credentials and catches
// up. The returned channel is closed when ctx ends, the credential is revoked
// or the broker subscription ends; clients reconnect with since=<last seq>.
func (s *Service) Stream(ctx context.Context, p *auth.Principal, since *int64) (<-chan StreamMessage, error) {
	if err := p.Require(domain.ScopeRead); err != nil {
		return nil, err
	}
	if since != nil && *since < 0 {
		return nil, domain.Invalid("since must not be negative")
	}

	// Subscribe before reading the position so nothing falls in between.
	sub, err := s.broker.Subscribe(ctx, broker.UserChannel(p.User.ID))
	if err != nil {
		return nil, err
	}
	current, oldest, err := s.store.EventSeqRange(ctx, p.User.ID)
	if err != nil {
		_ = sub.Close()
		return nil, err
	}

	st := &stream{s: s, p: p, sub: sub, out: make(chan StreamMessage, 16)}
	go st.run(ctx, since, current, oldest)
	return st.out, nil
}

type stream struct {
	s    *Service
	p    *auth.Principal
	sub  broker.Subscription
	out  chan StreamMessage
	last int64
}

func (st *stream) run(ctx context.Context, since *int64, current, oldest int64) {
	defer close(st.out)
	defer st.sub.Close()

	switch {
	case since == nil, *since == current:
		st.last = current
		if !st.send(ctx, StreamMessage{Kind: StreamReady, Seq: current}) {
			return
		}
	case *since > current, oldest == 0 || *since+1 < oldest:
		st.last = current
		if !st.send(ctx, StreamMessage{Kind: StreamReset, Seq: current}) {
			return
		}
	default:
		st.last = *since
		if !st.catchUp(ctx) || !st.send(ctx, StreamMessage{Kind: StreamReady, Seq: st.last}) {
			return
		}
	}

	interval := st.s.opts.StreamCheckInterval
	if interval <= 0 {
		interval = defaultStreamCheck
	}
	tick := time.NewTicker(interval)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case raw, ok := <-st.sub.C():
			if !ok {
				return
			}
			var ev domain.Event
			if err := json.Unmarshal(raw, &ev); err != nil {
				st.s.log.Warn("decode broker event", "err", err)
				continue
			}
			ev.UserID = st.p.User.ID
			switch {
			case ev.Seq <= st.last: // already delivered (backlog overlap)
			case ev.Seq == st.last+1:
				if !st.deliver(ctx, ev) {
					return
				}
			default: // gap: replay from the database
				if !st.catchUp(ctx) {
					return
				}
			}
		case <-tick.C:
			if err := st.s.stillValid(ctx, st.p); err != nil {
				return
			}
			if !st.catchUp(ctx) {
				return
			}
		}
	}
}

// catchUp replays every stored event after st.last.
func (st *stream) catchUp(ctx context.Context) bool {
	for {
		evs, err := st.s.store.ListEvents(ctx, st.p.User.ID, st.last, streamPage)
		if err != nil {
			if ctx.Err() == nil {
				st.s.log.Warn("stream catch-up", "user_id", st.p.User.ID, "err", err)
			}
			return false
		}
		for _, ev := range evs {
			if !st.deliver(ctx, ev) {
				return false
			}
		}
		if len(evs) < streamPage {
			return true
		}
	}
}

func (st *stream) deliver(ctx context.Context, ev domain.Event) bool {
	st.last = ev.Seq
	if !eventVisible(st.p, &ev) {
		return true // position advances, content stays hidden
	}
	return st.send(ctx, StreamMessage{Kind: StreamEvent, Seq: ev.Seq, Event: &ev})
}

func (st *stream) send(ctx context.Context, m StreamMessage) bool {
	select {
	case st.out <- m:
		return true
	case <-ctx.Done():
		return false
	}
}

// eventVisible applies token topic restrictions to events. Notification
// deletions and bulk changes carry only opaque IDs and pass through.
func eventVisible(p *auth.Principal, ev *domain.Event) bool {
	if p.TopicRestriction() == nil {
		return true
	}
	switch ev.Type {
	case domain.EventNotificationCreated, domain.EventNotificationUpdated:
		var n struct {
			Topic *struct {
				ID string `json:"id"`
			} `json:"topic"`
		}
		return json.Unmarshal(ev.Data, &n) == nil && n.Topic != nil && p.CanUseTopic(n.Topic.ID)
	case domain.EventTopicChanged, domain.EventTopicRemoved, domain.EventInvitationsChanged:
		var t domain.IDEventData
		return json.Unmarshal(ev.Data, &t) == nil && p.CanUseTopic(t.ID)
	default:
		return true
	}
}

// stillValid reports whether a long-lived stream's credential is still good.
func (s *Service) stillValid(ctx context.Context, p *auth.Principal) error {
	now := s.now()
	switch {
	case p.Token != nil:
		tok, err := s.store.GetTokenByHash(ctx, p.Token.SecretHash)
		if err != nil {
			return err
		}
		if tok.Expired(now) {
			return domain.ErrUnauthorized
		}
	case p.SessionID != "":
		sess, err := s.store.GetSession(ctx, p.SessionID)
		if err != nil {
			return err
		}
		if !now.Before(sess.ExpiresAt) {
			return domain.ErrUnauthorized
		}
	default:
		return errors.New("stream principal has no credential")
	}
	_, err := s.activeUser(ctx, p.User.ID)
	return err
}

// PruneEvents deletes events older than retention.
func (s *Service) PruneEvents(ctx context.Context, retention time.Duration) (int64, error) {
	return s.store.DeleteEventsBefore(ctx, s.now().Add(-retention))
}
