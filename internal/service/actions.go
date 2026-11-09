package service

import (
	"context"
	"strconv"
	"sync"

	"github.com/oowliq/acksy/internal/auth"
	"github.com/oowliq/acksy/internal/domain"
)

// ActionResult is the outcome of running an http action.
type ActionResult struct {
	OK     bool   `json:"ok"`
	Status int    `json:"status"` // HTTP status, 0 when there was no response
	Error  string `json:"error,omitempty"`
	// Cleared is true when the action marked the notification read.
	Cleared bool `json:"cleared"`
}

// ActionRunner sends an http action's request.
type ActionRunner interface {
	Run(ctx context.Context, a domain.Action) ActionResult
}

// SetActionRunner enables http actions.
func (s *Service) SetActionRunner(r ActionRunner) { s.actionRunner = r }

// actionLimitPerMinute bounds how often one user can trigger actions.
const actionLimitPerMinute = 30

// actionsInFlight keeps one run per notification action at a time, so a
// double click or two devices do not fire it twice concurrently. It is
// per process; across instances a near-simultaneous press may run twice.
type actionsInFlight struct {
	mu   sync.Mutex
	keys map[string]bool
}

func (f *actionsInFlight) start(key string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.keys == nil {
		f.keys = map[string]bool{}
	}
	if f.keys[key] {
		return false
	}
	f.keys[key] = true
	return true
}

func (f *actionsInFlight) done(key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.keys, key)
}

// RunAction runs the http action at index of one of the caller's
// notifications. Only recipients can run it (the notification must be
// visible to the caller); it needs notifications:write. With Clear set,
// success also marks the notification read.
func (s *Service) RunAction(ctx context.Context, p *auth.Principal, notificationID string, index int) (*ActionResult, error) {
	if err := p.Require(domain.ScopeWrite); err != nil {
		return nil, err
	}
	if s.actionRunner == nil {
		return nil, domain.Conflict("http actions are disabled on this server")
	}
	n, err := s.visibleNotification(ctx, s.store, p, notificationID)
	if err != nil {
		return nil, err
	}
	if index < 0 || index >= len(n.Actions) {
		return nil, domain.NotFound("action")
	}
	a := n.Actions[index]
	if a.Type != domain.ActionHTTP {
		return nil, domain.Invalid("only http actions run on the server; open view actions in the client")
	}
	if ok, wait := s.actionLimit.Allow(p.User.ID, s.now()); !ok {
		return nil, &domain.RateLimitError{Msg: "too many actions, try again shortly", RetryAfter: wait}
	}
	key := n.ID + "#" + strconv.Itoa(index)
	if !s.actionsRunning.start(key) {
		return nil, domain.Conflict("this action is already running")
	}
	defer s.actionsRunning.done(key)

	res := s.actionRunner.Run(context.WithoutCancel(ctx), a)
	s.log.InfoContext(ctx, "action run", "notification_id", n.ID, "index", index, "status", res.Status, "ok", res.OK)
	if res.OK && a.Clear && n.ReadAt == nil {
		read := true
		if _, err := s.UpdateNotificationState(ctx, p, n.ID, StatePatch{Read: &read}); err != nil {
			return &res, err
		}
		res.Cleared = true
	}
	return &res, nil
}
