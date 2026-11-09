package service

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/oowliq/acksy/internal/auth"
	"github.com/oowliq/acksy/internal/domain"
)

const day = 24 * time.Hour

type retentionEnv struct {
	*env
	t     *testing.T
	alice *auth.Principal
}

func newRetentionEnv(t *testing.T, pol domain.RetentionPolicy) *retentionEnv {
	t.Helper()
	e := newEnv(t)
	e.svc.opts.Retention = pol
	return &retentionEnv{env: e, t: t, alice: e.user("alice", domain.RoleUser)}
}

// publish creates a notification and optionally reads it; returns its ID.
func (r *retentionEnv) publish(topic string, prio int, read bool) string {
	r.t.Helper()
	res, err := r.svc.Publish(context.Background(), r.alice, domain.Message{Topic: topic, Body: "x", Priority: prio})
	if err != nil {
		r.t.Fatal(err)
	}
	if read {
		yes := true
		if _, err := r.svc.UpdateNotificationState(context.Background(), r.alice, res.NotificationID, StatePatch{Read: &yes}); err != nil {
			r.t.Fatal(err)
		}
	}
	return res.NotificationID
}

func (r *retentionEnv) remaining() []string {
	r.t.Helper()
	page, err := r.svc.ListNotifications(context.Background(), r.alice, ListQuery{View: domain.ViewAll, Limit: MaxPageSize})
	if err != nil {
		r.t.Fatal(err)
	}
	ids := make([]string, len(page.Items))
	for i, n := range page.Items {
		ids[i] = n.ID
	}
	return ids
}

func (r *retentionEnv) run() int {
	r.t.Helper()
	res, err := r.svc.ApplyRetention(context.Background())
	if err != nil {
		r.t.Fatal(err)
	}
	return res.Deleted
}

func (r *retentionEnv) topic(name string, days, maxN *int) {
	r.t.Helper()
	if _, err := r.svc.CreateTopic(context.Background(), r.alice, CreateTopicInput{Name: name, RetentionDays: days, MaxNotifications: maxN}); err != nil {
		r.t.Fatal(err)
	}
}

func assertKept(t *testing.T, got []string, kept []string, gone []string) {
	t.Helper()
	for _, id := range kept {
		if !slices.Contains(got, id) {
			t.Errorf("%s was deleted", id)
		}
	}
	for _, id := range gone {
		if slices.Contains(got, id) {
			t.Errorf("%s was kept", id)
		}
	}
}

func TestRetentionTTLs(t *testing.T) {
	r := newRetentionEnv(t, domain.RetentionPolicy{
		ReadTTL: 30 * day, ReadTTLByPriority: map[int]time.Duration{1: 7 * day},
	})
	r.topic("short", ptr(3), nil)
	r.topic("long", ptr(60), nil)

	oldRead := r.publish("", 3, true)
	oldUnread := r.publish("", 3, false)
	longRead := r.publish("long", 3, true)
	r.advance(25 * day)
	recentRead := r.publish("", 3, true)
	lowRead := r.publish("", 1, true)
	shortRead := r.publish("short", 3, true)
	shortUnread := r.publish("short", 3, false)
	r.advance(10 * day) // oldRead & longRead are 35 days old, the rest 10

	if n := r.run(); n != 3 {
		t.Fatalf("deleted %d", n)
	}
	assertKept(t, r.remaining(),
		[]string{oldUnread, longRead, recentRead, shortUnread}, // unread never by default; topic 60d beats global 30d
		[]string{oldRead, lowRead, shortRead})                  // global 30d, P1 7d, topic 3d

	// The deletion reached open clients as a bulk delete event.
	events := must(r.svc.store.ListEvents(context.Background(), r.alice.User.ID, 0, 1000))
	last := events[len(events)-1]
	var data domain.BulkEventData
	_ = json.Unmarshal(last.Data, &data)
	if last.Type != domain.EventNotificationsBulk || data.Action != domain.BulkDelete || len(data.IDs) == 0 {
		t.Fatalf("event: %+v %+v", last, data)
	}

	// A second run finds nothing.
	if n := r.run(); n != 0 {
		t.Fatalf("second run deleted %d", n)
	}
}

func TestRetentionUnreadAndExpired(t *testing.T) {
	r := newRetentionEnv(t, domain.RetentionPolicy{UnreadTTL: 100 * day})
	unread := r.publish("", 3, false)
	expiring, err := r.svc.Publish(context.Background(), r.alice, domain.Message{Body: "temp", ExpiresAt: ptr(r.clock.Add(time.Hour))})
	if err != nil {
		t.Fatal(err)
	}
	r.advance(2 * time.Hour) // expired, but still within the grace period
	if n := r.run(); n != 0 {
		t.Fatalf("deleted within grace: %d", n)
	}
	r.advance(day)
	if n := r.run(); n != 1 {
		t.Fatalf("expired: %d", n)
	}
	r.advance(100 * day)
	if n := r.run(); n != 1 {
		t.Fatalf("unread ttl: %d", n)
	}
	assertKept(t, r.remaining(), nil, []string{unread, expiring.NotificationID})
}

func TestRetentionCountLimits(t *testing.T) {
	r := newRetentionEnv(t, domain.RetentionPolicy{MaxPerUser: 14})
	r.topic("ci", nil, ptr(10))
	var ci []string
	for i := range 12 {
		ci = append(ci, r.publish("ci", 3, i%4 == 3)) // read: #3, #7, #11
		r.advance(time.Minute)
	}
	if n := r.run(); n != 2 {
		t.Fatalf("topic limit deleted %d", n)
	}
	// The two oldest read ones go before any unread one.
	assertKept(t, r.remaining(), []string{ci[0], ci[1], ci[11]}, []string{ci[3], ci[7]})

	// User limit across topics: 10 in ci + 6 elsewhere = 16 > 14.
	var other []string
	for range 6 {
		other = append(other, r.publish("", 3, false))
		r.advance(time.Minute)
	}
	if n := r.run(); n != 2 {
		t.Fatalf("user limit deleted %d", n)
	}
	// One read left (ci[11]) goes first, then the oldest unread (ci[0]).
	assertKept(t, r.remaining(), append([]string{ci[1]}, other...), []string{ci[11], ci[0]})
}

func TestTopicRetentionValidation(t *testing.T) {
	ctx := context.Background()
	r := newRetentionEnv(t, domain.RetentionPolicy{})
	if _, err := r.svc.CreateTopic(ctx, r.alice, CreateTopicInput{Name: "x", RetentionDays: ptr(0)}); err == nil {
		t.Fatal("0 days accepted")
	}
	tp := must(r.svc.CreateTopic(ctx, r.alice, CreateTopicInput{Name: "y", MaxNotifications: ptr(50)}))
	upd := must(r.svc.UpdateTopic(ctx, r.alice, tp.ID, UpdateTopicInput{RetentionDays: ptr(14), SetRetentionDays: true, SetMaxNotifications: true}))
	if *upd.RetentionDays != 14 || upd.MaxNotifications != nil {
		t.Fatalf("update: %+v", upd)
	}
	if _, err := r.svc.UpdateTopic(ctx, r.alice, tp.ID, UpdateTopicInput{MaxNotifications: ptr(5), SetMaxNotifications: true}); err == nil {
		t.Fatal("max 5 accepted")
	}
}
