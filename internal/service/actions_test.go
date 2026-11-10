package service

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"

	"github.com/oowliq/acksy/internal/domain"
)

type fakeRunner struct {
	mu      sync.Mutex
	got     []domain.Action
	result  ActionResult
	block   chan struct{} // when set, Run waits for it
	started chan struct{}
}

func (f *fakeRunner) Run(_ context.Context, a domain.Action, _ bool) ActionResult {
	f.mu.Lock()
	f.got = append(f.got, a)
	f.mu.Unlock()
	if f.block != nil {
		f.started <- struct{}{}
		<-f.block
	}
	return f.result
}

func TestRunAction(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	runner := &fakeRunner{result: ActionResult{OK: true, Status: 200}}
	alice, bob := e.user("alice", domain.RoleUser), e.user("bob", domain.RoleUser)

	if _, err := e.svc.RunAction(ctx, alice, "x", 0); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("no runner: %v", err)
	}
	e.svc.SetActionRunner(runner)

	res := must(e.svc.Publish(ctx, alice, domain.Message{Body: "gate", Actions: []domain.Action{
		{Type: "view", Label: "Camera", URL: "https://cam.lan/"},
		{
			Type: "http", Label: "Open", URL: "https://ha.lan/api/open", Headers: map[string]string{"Authorization": "Bearer S3CRET"},
			Body: `{"e":"gate"}`, Clear: true,
		},
	}}))
	id := res.NotificationID

	// The API never shows the secret, but the runner gets it from the store.
	n := must(e.svc.GetNotification(ctx, alice, id))
	if n.Actions[1].Headers["Authorization"] != "Bearer S3CRET" {
		t.Fatalf("stored action lost its headers: %+v", n.Actions[1])
	}

	out, err := e.svc.RunAction(ctx, alice, id, 1)
	if err != nil || !out.OK || !out.Cleared {
		t.Fatalf("run: %+v %v", out, err)
	}
	if got := runner.got[0]; got.Method != http.MethodPost || got.Headers["Authorization"] != "Bearer S3CRET" || got.Body != `{"e":"gate"}` {
		t.Fatalf("runner saw: %+v", got)
	}
	if n := must(e.svc.GetNotification(ctx, alice, id)); n.ReadAt == nil {
		t.Fatal("clear did not mark read")
	}

	// Pressing again is allowed; a failure does not clear anything.
	runner.result = ActionResult{Status: 500, Error: "HTTP 500"}
	if out, err := e.svc.RunAction(ctx, alice, id, 1); err != nil || out.OK || out.Cleared {
		t.Fatalf("second run: %+v %v", out, err)
	}

	if _, err := e.svc.RunAction(ctx, alice, id, 0); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("view action: %v", err)
	}
	if _, err := e.svc.RunAction(ctx, alice, id, 7); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("bad index: %v", err)
	}
	if _, err := e.svc.RunAction(ctx, bob, id, 1); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("other user: %v", err)
	}
	ro := e.token(alice, []string{"notifications:read"})
	if _, err := e.svc.RunAction(ctx, ro, id, 1); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("read-only token: %v", err)
	}
}

func TestRunActionNotConcurrently(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	runner := &fakeRunner{result: ActionResult{OK: true, Status: 204}, block: make(chan struct{}), started: make(chan struct{}, 1)}
	e.svc.SetActionRunner(runner)
	alice := e.user("alice", domain.RoleUser)
	id := must(e.svc.Publish(ctx, alice, domain.Message{Body: "x", Actions: []domain.Action{
		{Type: "http", Label: "Go", URL: "https://ha.lan/go"},
	}})).NotificationID

	done := make(chan error, 1)
	go func() { _, err := e.svc.RunAction(ctx, alice, id, 0); done <- err }()
	<-runner.started
	if _, err := e.svc.RunAction(ctx, alice, id, 0); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("concurrent press: %v", err)
	}
	close(runner.block)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	runner.block = nil
	if _, err := e.svc.RunAction(ctx, alice, id, 0); err != nil {
		t.Fatalf("press after finish: %v", err)
	}
}
