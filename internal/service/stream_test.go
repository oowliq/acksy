package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/oowliq/acksy/internal/auth"
	"github.com/oowliq/acksy/internal/domain"
)

func ptr[T any](v T) *T { return &v }

func next(t *testing.T, ch <-chan StreamMessage) StreamMessage {
	t.Helper()
	select {
	case m, ok := <-ch:
		if !ok {
			t.Fatal("stream closed")
		}
		return m
	case <-time.After(2 * time.Second):
		t.Fatal("no stream message")
	}
	return StreamMessage{}
}

func expectEvent(t *testing.T, ch <-chan StreamMessage, seq int64, typ domain.EventType) *domain.Event {
	t.Helper()
	m := next(t, ch)
	if m.Kind != StreamEvent || m.Seq != seq || m.Event.Type != typ {
		t.Fatalf("got %+v (event %+v), want seq %d %s", m, m.Event, seq, typ)
	}
	return m.Event
}

func expectClosed(t *testing.T, ch <-chan StreamMessage) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("stream not closed")
		}
	}
}

func openStream(t *testing.T, e *env, p *auth.Principal, since *int64) <-chan StreamMessage {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ch, err := e.svc.Stream(ctx, p, since)
	if err != nil {
		t.Fatal(err)
	}
	return ch
}

func TestStreamLiveWithoutSince(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	alice := e.user("alice", domain.RoleUser)
	must(e.svc.Publish(ctx, alice, domain.Message{Body: "old"})) // seq 1

	ch := openStream(t, e, alice, nil)
	if m := next(t, ch); m.Kind != StreamReady || m.Seq != 1 {
		t.Fatalf("ready: %+v", m)
	}
	must(e.svc.Publish(ctx, alice, domain.Message{Body: "new"}))
	ev := expectEvent(t, ch, 2, domain.EventNotificationCreated)
	var n domain.Notification
	if err := json.Unmarshal(ev.Data, &n); err != nil || n.Body != "new" {
		t.Fatalf("payload: %s", ev.Data)
	}
}

func TestStreamReplaysBacklogThenLive(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	alice := e.user("alice", domain.RoleUser)
	for range 3 {
		must(e.svc.Publish(ctx, alice, domain.Message{Body: "x"}))
	}

	ch := openStream(t, e, alice, ptr[int64](1))
	expectEvent(t, ch, 2, domain.EventNotificationCreated)
	expectEvent(t, ch, 3, domain.EventNotificationCreated)
	if m := next(t, ch); m.Kind != StreamReady || m.Seq != 3 {
		t.Fatalf("ready: %+v", m)
	}
	must(e.svc.Bulk(ctx, alice, domain.BulkRead, BulkSelector{All: true}))
	expectEvent(t, ch, 4, domain.EventNotificationsBulk)
}

func TestStreamFillsGapsFromDatabase(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	alice := e.user("alice", domain.RoleUser)
	ch := openStream(t, e, alice, nil)
	next(t, ch) // ready 0

	// An event committed without reaching this subscriber (e.g. written by
	// another instance without a shared broker, or dropped as slow consumer).
	err := e.svc.store.InTx(ctx, func(q storeQuerier) error {
		em := &emitter{q: q, now: e.clock}
		return em.emit(ctx, alice.User.ID, domain.EventTopicChanged, domain.IDEventData{ID: "silent"})
	})
	if err != nil {
		t.Fatal(err)
	}
	// The next live event (seq 2) exposes the gap; seq 1 is replayed first.
	must(e.svc.Publish(ctx, alice, domain.Message{Body: "x"}))
	expectEvent(t, ch, 1, domain.EventTopicChanged)
	expectEvent(t, ch, 2, domain.EventNotificationCreated)
}

func TestStreamPeriodicCatchUp(t *testing.T) {
	e := newEnv(t)
	e.svc.opts.StreamCheckInterval = 20 * time.Millisecond
	ctx := context.Background()
	alice := e.user("alice", domain.RoleUser)
	sess, _ := must2(t)(e.svc.Login(ctx, "alice", "password123", ""))
	p := must(e.svc.AuthenticateSession(ctx, sess))
	ch := openStream(t, e, p, nil)
	next(t, ch)

	err := e.svc.store.InTx(ctx, func(q storeQuerier) error {
		em := &emitter{q: q, now: e.clock}
		return em.emit(ctx, alice.User.ID, domain.EventTopicChanged, domain.IDEventData{ID: "silent"})
	})
	if err != nil {
		t.Fatal(err)
	}
	expectEvent(t, ch, 1, domain.EventTopicChanged) // no live event needed
}

func TestStreamResets(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	alice := e.user("alice", domain.RoleUser)
	for range 3 {
		must(e.svc.Publish(ctx, alice, domain.Message{Body: "x"}))
	}

	// Position from the future (e.g. restored database).
	if m := next(t, openStream(t, e, alice, ptr[int64](99))); m.Kind != StreamReset || m.Seq != 3 {
		t.Fatalf("future: %+v", m)
	}
	// Pruned history.
	e.advance(48 * time.Hour)
	if _, err := e.svc.PruneEvents(ctx, 24*time.Hour); err != nil {
		t.Fatal(err)
	}
	ch := openStream(t, e, alice, ptr[int64](1))
	if m := next(t, ch); m.Kind != StreamReset || m.Seq != 3 {
		t.Fatalf("pruned: %+v", m)
	}
	// Streaming continues after a reset.
	must(e.svc.Publish(ctx, alice, domain.Message{Body: "y"}))
	expectEvent(t, ch, 4, domain.EventNotificationCreated)

	if _, err := e.svc.Stream(ctx, alice, ptr[int64](-1)); err == nil {
		t.Fatal("negative since accepted")
	}
}

func TestStreamHonoursTopicRestriction(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	alice := e.user("alice", domain.RoleUser)
	must(e.svc.CreateTopic(ctx, alice, CreateTopicInput{Name: "ci"}))   // seq 1
	must(e.svc.CreateTopic(ctx, alice, CreateTopicInput{Name: "home"})) // seq 2
	ci := e.token(alice, []string{"notifications:read"}, "ci")

	ch := openStream(t, e, ci, nil)
	next(t, ch)
	must(e.svc.Publish(ctx, alice, domain.Message{Topic: "home", Body: "private"})) // seq 3, hidden
	must(e.svc.Publish(ctx, alice, domain.Message{Body: "no topic"}))               // seq 4, hidden
	must(e.svc.Publish(ctx, alice, domain.Message{Topic: "ci", Body: "build"}))     // seq 5
	expectEvent(t, ch, 5, domain.EventNotificationCreated)

	publishOnly := e.token(alice, []string{"notifications:publish"})
	if _, err := e.svc.Stream(ctx, publishOnly, nil); err == nil {
		t.Fatal("stream without read scope")
	}
}

func TestStreamEndsWhenCredentialRevoked(t *testing.T) {
	e := newEnv(t)
	e.svc.opts.StreamCheckInterval = 20 * time.Millisecond
	ctx := context.Background()
	alice := e.user("alice", domain.RoleUser)

	secret, _ := must2(t)(e.svc.Login(ctx, "alice", "password123", ""))
	sessP := must(e.svc.AuthenticateSession(ctx, secret))
	sessCh := openStream(t, e, sessP, nil)
	next(t, sessCh)

	tokP := e.token(UserPrincipal(alice.User), []string{"notifications:read"})
	tokCh := openStream(t, e, tokP, nil)
	next(t, tokCh)

	if err := e.svc.Logout(ctx, sessP); err != nil {
		t.Fatal(err)
	}
	expectClosed(t, sessCh)

	if err := e.svc.DeleteToken(ctx, alice, tokP.Token.ID); err != nil {
		t.Fatal(err)
	}
	expectClosed(t, tokCh)
}

func TestStreamEndsWithBroker(t *testing.T) {
	e := newEnv(t)
	alice := e.user("alice", domain.RoleUser)
	ch := openStream(t, e, alice, nil)
	next(t, ch)
	_ = e.b.Close()
	expectClosed(t, ch)
}
