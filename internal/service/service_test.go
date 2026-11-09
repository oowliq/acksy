package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oowliq/acksy/internal/auth"
	"github.com/oowliq/acksy/internal/broker"
	"github.com/oowliq/acksy/internal/broker/memory"
	"github.com/oowliq/acksy/internal/domain"
	"github.com/oowliq/acksy/internal/store"
)

type env struct {
	t     *testing.T
	svc   *Service
	b     *memory.Broker
	clock time.Time
}

func newEnv(t *testing.T) *env {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, "sqlite://:memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	b := memory.New()
	t.Cleanup(func() { _ = b.Close() })

	e := &env{t: t, b: b, clock: time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)}
	e.svc = New(store.New(db), b, slog.New(slog.NewTextHandler(io.Discard, nil)), Options{})
	e.svc.now = func() time.Time { return e.clock }
	var n atomic.Int64
	e.svc.newID = func() string { return fmt.Sprintf("01TEST%020d", n.Add(1)) }
	return e
}

func (e *env) advance(d time.Duration) { e.clock = e.clock.Add(d) }

func (e *env) user(name string, role domain.Role) *auth.Principal {
	e.t.Helper()
	u, err := e.svc.CreateUser(context.Background(), SystemPrincipal(), CreateUserInput{Username: name, Password: "password123", Role: role})
	if err != nil {
		e.t.Fatal(err)
	}
	return UserPrincipal(u)
}

func (e *env) token(p *auth.Principal, scopes []string, topics ...string) *auth.Principal {
	e.t.Helper()
	ct, err := e.svc.CreateToken(context.Background(), p, CreateTokenInput{Name: "t", Scopes: scopes, Topics: topics})
	if err != nil {
		e.t.Fatal(err)
	}
	tp, err := e.svc.AuthenticateToken(context.Background(), ct.Secret)
	if err != nil {
		e.t.Fatal(err)
	}
	return tp
}

// share invites username to the owner's topic and accepts as them.
func (e *env) share(owner *auth.Principal, topicID, username string) {
	e.t.Helper()
	ctx := context.Background()
	if _, err := e.svc.InviteToTopic(ctx, owner, topicID, username); err != nil {
		e.t.Fatal(err)
	}
	u, err := e.svc.store.GetUserByUsername(ctx, username)
	if err != nil {
		e.t.Fatal(err)
	}
	if _, err := e.svc.AcceptTopicInvite(ctx, UserPrincipal(u), topicID); err != nil {
		e.t.Fatal(err)
	}
}

// must unwraps (v, err); a panic fails the test with a stack trace.
func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func wantKind(t *testing.T, err error, target error) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Fatalf("want %v, got %v", target, err)
	}
}

func TestPublishToSelfAndList(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	alice := e.user("alice", domain.RoleUser)

	sub := must(e.b.Subscribe(ctx, broker.UserChannel(alice.User.ID)))
	defer sub.Close()

	res := must(e.svc.Publish(ctx, alice, domain.Message{Title: "Hello", Body: "**hi**"}))
	if res.Recipients != 1 || res.NotificationID == "" {
		t.Fatalf("result: %+v", res)
	}

	select {
	case raw := <-sub.C():
		var ev domain.Event
		if err := json.Unmarshal(raw, &ev); err != nil {
			t.Fatal(err)
		}
		if ev.Seq != 1 || ev.Type != domain.EventNotificationCreated {
			t.Fatalf("event: %+v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("no realtime event")
	}

	page := must(e.svc.ListNotifications(ctx, alice, ListQuery{}))
	if len(page.Items) != 1 || page.Items[0].Priority != domain.PriorityDefault || page.Items[0].Topic != nil {
		t.Fatalf("list: %+v", page.Items)
	}
}

func TestSharedTopicFanOutAndIndependentReadState(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	alice, bob := e.user("alice", domain.RoleUser), e.user("bob", domain.RoleUser)

	prio := 4
	topic := must(e.svc.CreateTopic(ctx, alice, CreateTopicInput{Name: "Backups", DefaultPriority: &prio}))
	if topic.Name != "backups" || topic.Role != domain.MemberOwner {
		t.Fatalf("topic: %+v", topic)
	}
	e.share(alice, topic.ID, "bob")

	// Bob publishes into the shared topic by its unqualified name.
	res := must(e.svc.Publish(ctx, bob, domain.Message{Topic: "backups", Body: "done"}))
	if res.Recipients != 2 {
		t.Fatalf("recipients: %+v", res)
	}

	aliceN := must(e.svc.ListNotifications(ctx, alice, ListQuery{Topic: "backups"})).Items
	bobN := must(e.svc.ListNotifications(ctx, bob, ListQuery{})).Items
	if len(aliceN) != 1 || len(bobN) != 1 || aliceN[0].ID == bobN[0].ID || aliceN[0].MessageID != bobN[0].MessageID {
		t.Fatalf("fan-out: %+v / %+v", aliceN, bobN)
	}
	if aliceN[0].Priority != 4 {
		t.Fatalf("topic default priority not applied: %d", aliceN[0].Priority)
	}

	read := true
	must(e.svc.UpdateNotificationState(ctx, alice, aliceN[0].ID, StatePatch{Read: &read}))
	if c := must(e.svc.Counts(ctx, alice)); c.Unread != 0 {
		t.Fatalf("alice unread: %+v", c)
	}
	if c := must(e.svc.Counts(ctx, bob)); c.Unread != 1 || c.ByTopic[topic.ID] != 1 {
		t.Fatalf("bob's read state must be independent: %+v", c)
	}

	// Users cannot touch each other's notifications.
	_, err := e.svc.UpdateNotificationState(ctx, bob, aliceN[0].ID, StatePatch{Read: &read})
	wantKind(t, err, domain.ErrNotFound)

	// Bob leaves; further messages reach only alice.
	wantKind(t, e.svc.RemoveTopicMember(ctx, alice, topic.ID, alice.User.ID), domain.ErrConflict)
	if err := e.svc.RemoveTopicMember(ctx, bob, topic.ID, bob.User.ID); err != nil {
		t.Fatal(err)
	}
	if res := must(e.svc.Publish(ctx, alice, domain.Message{Topic: "backups", Body: "again"})); res.Recipients != 1 {
		t.Fatalf("after leave: %+v", res)
	}
	_, err = e.svc.Publish(ctx, bob, domain.Message{Topic: "alice/backups", Body: "x"})
	wantKind(t, err, domain.ErrNotFound)
}

func TestPublishAutoCreatesOwnTopic(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	alice := e.user("alice", domain.RoleUser)

	must(e.svc.Publish(ctx, alice, domain.Message{Topic: "cron", Body: "x"}))
	topics := must(e.svc.ListTopics(ctx, alice))
	if len(topics) != 1 || topics[0].Name != "cron" {
		t.Fatalf("auto-create: %+v", topics)
	}
}

func TestDedupResurfacesExisting(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	alice := e.user("alice", domain.RoleUser)

	first := must(e.svc.Publish(ctx, alice, domain.Message{Title: "Disk 90%", DedupKey: "disk"}))
	archived := true
	must(e.svc.UpdateNotificationState(ctx, alice, first.NotificationID, StatePatch{Archived: &archived}))

	e.advance(time.Minute)
	second := must(e.svc.Publish(ctx, alice, domain.Message{Title: "Disk 95%", DedupKey: "disk"}))
	if second.Deduplicated != 1 || second.NotificationID != first.NotificationID {
		t.Fatalf("dedup: %+v vs %+v", second, first)
	}
	items := must(e.svc.ListNotifications(ctx, alice, ListQuery{})).Items
	if len(items) != 1 || items[0].Title != "Disk 95%" || items[0].RepeatCount != 2 || items[0].ReadAt != nil {
		t.Fatalf("resurfaced: %+v", items)
	}
}

func TestStateChangesAndBulk(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	alice := e.user("alice", domain.RoleUser)

	var ids []string
	for i := range 3 {
		r := must(e.svc.Publish(ctx, alice, domain.Message{Body: fmt.Sprint(i), GroupKey: "g"}))
		ids = append(ids, r.NotificationID)
	}

	// Snooze hides from inbox until the time passes.
	until := e.clock.Add(time.Hour)
	must(e.svc.UpdateNotificationState(ctx, alice, ids[0], StatePatch{SetSnooze: true, SnoozedUntil: &until}))
	if n := len(must(e.svc.ListNotifications(ctx, alice, ListQuery{})).Items); n != 2 {
		t.Fatalf("inbox while snoozed: %d", n)
	}
	past := e.clock.Add(-time.Minute)
	_, err := e.svc.UpdateNotificationState(ctx, alice, ids[0], StatePatch{SetSnooze: true, SnoozedUntil: &past})
	wantKind(t, err, domain.ErrInvalid)
	e.advance(2 * time.Hour)
	if n := len(must(e.svc.ListNotifications(ctx, alice, ListQuery{})).Items); n != 3 {
		t.Fatalf("inbox after snooze: %d", n)
	}

	_, err = e.svc.Bulk(ctx, alice, domain.BulkRead, BulkSelector{})
	wantKind(t, err, domain.ErrInvalid)

	res := must(e.svc.Bulk(ctx, alice, domain.BulkRead, BulkSelector{GroupKey: "g"}))
	if res.Affected != 3 {
		t.Fatalf("bulk read: %+v", res)
	}
	// Already read: nothing to do.
	if res := must(e.svc.Bulk(ctx, alice, domain.BulkRead, BulkSelector{All: true})); res.Affected != 0 {
		t.Fatalf("idempotent read: %+v", res)
	}
	if res := must(e.svc.Bulk(ctx, alice, domain.BulkArchive, BulkSelector{IDs: ids[:2]})); res.Affected != 2 {
		t.Fatalf("archive: %+v", res)
	}
	if n := len(must(e.svc.ListNotifications(ctx, alice, ListQuery{View: domain.ViewArchived})).Items); n != 2 {
		t.Fatalf("archived view: %d", n)
	}
	if err := e.svc.DeleteNotification(ctx, alice, ids[2]); err != nil {
		t.Fatal(err)
	}
	if n := len(must(e.svc.ListNotifications(ctx, alice, ListQuery{View: domain.ViewAll})).Items); n != 2 {
		t.Fatalf("after delete: %d", n)
	}
}

func TestPagination(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	alice := e.user("alice", domain.RoleUser)
	for i := range 5 {
		must(e.svc.Publish(ctx, alice, domain.Message{Body: fmt.Sprint(i)}))
	}
	p1 := must(e.svc.ListNotifications(ctx, alice, ListQuery{Limit: 2}))
	p2 := must(e.svc.ListNotifications(ctx, alice, ListQuery{Limit: 2, Before: p1.NextCursor}))
	p3 := must(e.svc.ListNotifications(ctx, alice, ListQuery{Limit: 2, Before: p2.NextCursor}))
	if len(p1.Items) != 2 || len(p2.Items) != 2 || len(p3.Items) != 1 || p3.NextCursor != "" {
		t.Fatalf("pages: %d %d %d next=%q", len(p1.Items), len(p2.Items), len(p3.Items), p3.NextCursor)
	}
	if p1.Items[0].Body != "4" || p3.Items[0].Body != "0" {
		t.Fatal("order must be newest first")
	}
}

func TestTokenScopesAndTopicRestriction(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	alice := e.user("alice", domain.RoleUser)
	must(e.svc.CreateTopic(ctx, alice, CreateTopicInput{Name: "ci"}))
	must(e.svc.CreateTopic(ctx, alice, CreateTopicInput{Name: "home"}))
	must(e.svc.Publish(ctx, alice, domain.Message{Body: "no topic"}))
	must(e.svc.Publish(ctx, alice, domain.Message{Topic: "home", Body: "home"}))

	ci := e.token(alice, []string{"notifications:publish", "notifications:read"}, "ci")

	must(e.svc.Publish(ctx, ci, domain.Message{Topic: "ci", Body: "build ok"}))
	_, err := e.svc.Publish(ctx, ci, domain.Message{Topic: "home", Body: "x"})
	wantKind(t, err, domain.ErrForbidden)
	_, err = e.svc.Publish(ctx, ci, domain.Message{Body: "x"})
	wantKind(t, err, domain.ErrForbidden)
	_, err = e.svc.Publish(ctx, ci, domain.Message{Topic: "newtopic", Body: "x"})
	wantKind(t, err, domain.ErrNotFound) // restricted tokens never auto-create

	items := must(e.svc.ListNotifications(ctx, ci, ListQuery{})).Items
	if len(items) != 1 || items[0].Body != "build ok" {
		t.Fatalf("restricted list leaked: %+v", items)
	}
	if c := must(e.svc.Counts(ctx, ci)); c.Unread != 1 {
		t.Fatalf("restricted counts: %+v", c)
	}
	if topics := must(e.svc.ListTopics(ctx, ci)); len(topics) != 1 {
		t.Fatalf("restricted topics: %+v", topics)
	}

	// Missing scope.
	read := true
	_, err = e.svc.UpdateNotificationState(ctx, ci, items[0].ID, StatePatch{Read: &read})
	wantKind(t, err, domain.ErrForbidden)
	_, err = e.svc.CreateTopic(ctx, ci, CreateTopicInput{Name: "x"})
	wantKind(t, err, domain.ErrForbidden)

	// Tokens cannot manage tokens, and non-admins cannot mint admin tokens.
	_, err = e.svc.CreateToken(ctx, ci, CreateTokenInput{Name: "x", Scopes: []string{"notifications:read"}})
	wantKind(t, err, domain.ErrForbidden)
	_, err = e.svc.CreateToken(ctx, alice, CreateTokenInput{Name: "x", Scopes: []string{"admin"}})
	wantKind(t, err, domain.ErrForbidden)
}

func TestTokenLifecycle(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	alice := e.user("alice", domain.RoleUser)

	exp := e.clock.Add(time.Hour)
	ct := must(e.svc.CreateToken(ctx, alice, CreateTokenInput{Name: "cron", Scopes: []string{"notifications:publish"}, ExpiresAt: &exp}))
	must(e.svc.AuthenticateToken(ctx, ct.Secret))

	e.advance(2 * time.Hour)
	_, err := e.svc.AuthenticateToken(ctx, ct.Secret)
	wantKind(t, err, domain.ErrUnauthorized)

	if err := e.svc.DeleteToken(ctx, alice, ct.Token.ID); err != nil {
		t.Fatal(err)
	}
	if list := must(e.svc.ListTokens(ctx, alice)); len(list) != 0 {
		t.Fatalf("tokens: %+v", list)
	}
	_, err = e.svc.AuthenticateToken(ctx, "ack_garbage")
	wantKind(t, err, domain.ErrUnauthorized)
}

func TestLoginSessionsAndDisable(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	admin := e.user("root", domain.RoleAdmin)
	bob := e.user("bob", domain.RoleUser)

	_, _, err := e.svc.Login(ctx, "bob", "wrong-password", "")
	wantKind(t, err, domain.ErrUnauthorized)
	_, _, err = e.svc.Login(ctx, "nobody", "password123", "")
	wantKind(t, err, domain.ErrUnauthorized)

	secret, u, err := e.svc.Login(ctx, " BOB ", "password123", "test")
	if err != nil || u.ID != bob.User.ID {
		t.Fatalf("login: %v", err)
	}
	p := must(e.svc.AuthenticateSession(ctx, secret))
	if !p.IsSession() || p.Scopes.Has(domain.ScopeAdmin) {
		t.Fatalf("principal: %+v", p)
	}

	// Disabling ends sessions and blocks login.
	disabled := true
	must(e.svc.UpdateUser(ctx, admin, bob.User.ID, UpdateUserInput{Disabled: &disabled}))
	_, err = e.svc.AuthenticateSession(ctx, secret)
	wantKind(t, err, domain.ErrUnauthorized)
	_, _, err = e.svc.Login(ctx, "bob", "password123", "")
	wantKind(t, err, domain.ErrUnauthorized)

	// Session expiry.
	secret, _ = must2(t)(e.svc.Login(ctx, "root", "password123", ""))
	e.advance(31 * 24 * time.Hour)
	_, err = e.svc.AuthenticateSession(ctx, secret)
	wantKind(t, err, domain.ErrUnauthorized)
}

func must2(t *testing.T) func(string, *domain.User, error) (string, *domain.User) {
	return func(s string, u *domain.User, err error) (string, *domain.User) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return s, u
	}
}

func TestChangePasswordKeepsCurrentSession(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.user("alice", domain.RoleUser)

	s1, _ := must2(t)(e.svc.Login(ctx, "alice", "password123", ""))
	s2, _ := must2(t)(e.svc.Login(ctx, "alice", "password123", ""))
	p1 := must(e.svc.AuthenticateSession(ctx, s1))

	wantKind(t, e.svc.ChangePassword(ctx, p1, "nope", "newpassword1"), domain.ErrInvalid)
	if err := e.svc.ChangePassword(ctx, p1, "password123", "newpassword1"); err != nil {
		t.Fatal(err)
	}
	must(e.svc.AuthenticateSession(ctx, s1))
	_, err := e.svc.AuthenticateSession(ctx, s2)
	wantKind(t, err, domain.ErrUnauthorized)
	must2(t)(e.svc.Login(ctx, "alice", "newpassword1", ""))
}

func TestAdminRules(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	root := e.user("root", domain.RoleAdmin)
	bob := e.user("bob", domain.RoleUser)

	_, err := e.svc.CreateUser(ctx, bob, CreateUserInput{Username: "eve", Password: "password123"})
	wantKind(t, err, domain.ErrForbidden)
	_, err = e.svc.ListUsers(ctx, bob)
	wantKind(t, err, domain.ErrForbidden)
	_, err = e.svc.CreateUser(ctx, root, CreateUserInput{Username: "bob", Password: "password123"})
	wantKind(t, err, domain.ErrConflict)
	_, err = e.svc.CreateUser(ctx, root, CreateUserInput{Username: "x", Password: "password123"})
	wantKind(t, err, domain.ErrInvalid)

	// The last admin cannot be demoted, disabled or deleted.
	user := domain.RoleUser
	_, err = e.svc.UpdateUser(ctx, root, root.User.ID, UpdateUserInput{Role: &user})
	wantKind(t, err, domain.ErrConflict)
	wantKind(t, e.svc.DeleteUser(ctx, root, root.User.ID), domain.ErrConflict)

	admin := domain.RoleAdmin
	must(e.svc.UpdateUser(ctx, root, bob.User.ID, UpdateUserInput{Role: &admin}))
	must(e.svc.UpdateUser(ctx, root, root.User.ID, UpdateUserInput{Role: &user}))
}

func TestBootstrapAdminOnlyOnEmptyInstance(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if created := must(e.svc.BootstrapAdmin(ctx, "admin", "password123")); !created {
		t.Fatal("first bootstrap must create")
	}
	if created := must(e.svc.BootstrapAdmin(ctx, "other", "password123")); created {
		t.Fatal("second bootstrap must be a no-op")
	}
}

func TestEventsAreSequencedPerUser(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	alice, bob := e.user("alice", domain.RoleUser), e.user("bob", domain.RoleUser)
	topic := must(e.svc.CreateTopic(ctx, alice, CreateTopicInput{Name: "x"}))
	e.share(alice, topic.ID, "bob")
	must(e.svc.Publish(ctx, alice, domain.Message{Topic: "x", Body: "hi"}))

	inv, changed, created := domain.EventInvitationsChanged, domain.EventTopicChanged, domain.EventNotificationCreated
	for _, c := range []struct {
		p    *auth.Principal
		want []domain.EventType
	}{
		// create topic; invite; accept (invitation, then membership); publish
		{alice, []domain.EventType{changed, inv, inv, changed, created}},
		// invitation notice and invitation; accept (notice deleted, invitation, membership); publish
		{bob, []domain.EventType{created, inv, domain.EventNotificationsBulk, inv, changed, created}},
	} {
		evs := must(e.svc.store.ListEvents(ctx, c.p.User.ID, 0, 100))
		if len(evs) != len(c.want) {
			t.Fatalf("%s events: %+v", c.p.User.Username, evs)
		}
		for i, ev := range evs {
			if ev.Seq != int64(i+1) || ev.Type != c.want[i] {
				t.Fatalf("%s event %d: %+v", c.p.User.Username, i, ev)
			}
		}
	}
}

type storeQuerier = store.Querier
