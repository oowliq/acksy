package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oowliq/acksy/internal/domain"
)

var (
	idSeq atomic.Int64
	// idRun keeps IDs unique across test runs that share a Postgres
	// database (the compose container outlives a single `go test`).
	idRun = time.Now().UnixMilli()
)

// id returns increasing, ULID-like sortable IDs for tests.
func id(prefix string) string {
	return fmt.Sprintf("%s%013d%07d", prefix, idRun, idSeq.Add(1))
}

func openTestStore(t *testing.T, url string) *SQLStore {
	t.Helper()
	ctx := context.Background()
	db, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	return New(db)
}

func TestContractSQLite(t *testing.T) {
	runContract(t, func(t *testing.T) *SQLStore { return openTestStore(t, "sqlite://:memory:") })
}

func TestContractPostgres(t *testing.T) {
	url := os.Getenv("ACKSY_TEST_POSTGRES_URL")
	if url == "" {
		t.Skip("ACKSY_TEST_POSTGRES_URL not set")
	}
	// Tests share one database; every fixture uses unique IDs and usernames.
	runContract(t, func(t *testing.T) *SQLStore { return openTestStore(t, url) })
}

var now = time.UnixMilli(1_800_000_000_000).UTC()

func mkUser(t *testing.T, s Store, role domain.Role) *domain.User {
	t.Helper()
	u := &domain.User{ID: id("u"), Username: id("user"), Role: role, PasswordHash: "h", CreatedAt: now, UpdatedAt: now}
	if err := s.CreateUser(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	return u
}

func mkTopic(t *testing.T, s Store, owner *domain.User, name string) *domain.Topic {
	t.Helper()
	tp := &domain.Topic{ID: id("t"), OwnerID: owner.ID, Name: name, CreatedAt: now, UpdatedAt: now}
	if err := s.CreateTopic(context.Background(), tp); err != nil {
		t.Fatal(err)
	}
	return tp
}

// sameSet reports whether got holds exactly want, in any order.
func sameSet(got []string, want ...string) bool {
	if len(got) != len(want) {
		return false
	}
	seen := map[string]bool{}
	for _, g := range got {
		seen[g] = true
	}
	for _, w := range want {
		if !seen[w] {
			return false
		}
	}
	return true
}

func mkNotif(userID string, topic *domain.Topic, mut func(*domain.Notification)) *domain.Notification {
	n := &domain.Notification{
		ID: id("n"), UserID: userID, MessageID: id("m"), Title: "t", Body: "b", Priority: 3,
		Tags: []string{"a"}, Source: "acksy", CreatedAt: now, UpdatedAt: now,
	}
	if topic != nil {
		n.Topic = &domain.TopicSummary{ID: topic.ID, Name: topic.Name}
	}
	if mut != nil {
		mut(n)
	}
	return n
}

func runContract(t *testing.T, newStore func(*testing.T) *SQLStore) {
	ctx := context.Background()

	t.Run("users", func(t *testing.T) {
		s := newStore(t)
		u := mkUser(t, s, domain.RoleAdmin)

		dup := *u
		dup.ID = id("u")
		if err := s.CreateUser(ctx, &dup); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("duplicate username: %v", err)
		}
		got, err := s.GetUserByUsername(ctx, u.Username)
		if err != nil || got.ID != u.ID || !got.CreatedAt.Equal(now) || got.Role != domain.RoleAdmin {
			t.Fatalf("get: %+v %v", got, err)
		}
		if _, err := s.GetUser(ctx, "missing"); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("missing: %v", err)
		}

		got.DisplayName, got.Email = "Alice", "a@example.com"
		disabled := now.Add(time.Hour)
		got.DisabledAt = &disabled
		if err := s.UpdateUser(ctx, got); err != nil {
			t.Fatal(err)
		}
		again, _ := s.GetUser(ctx, u.ID)
		if again.DisplayName != "Alice" || again.Email != "a@example.com" || again.DisabledAt == nil {
			t.Fatalf("update not persisted: %+v", again)
		}

		s1, err := s.NextEventSeq(ctx, u.ID)
		if err != nil {
			t.Fatal(err)
		}
		s2, _ := s.NextEventSeq(ctx, u.ID)
		if s2 != s1+1 {
			t.Fatalf("seq not monotonic: %d %d", s1, s2)
		}
		if err := s.DeleteUser(ctx, u.ID); err != nil {
			t.Fatal(err)
		}
		if err := s.DeleteUser(ctx, u.ID); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("double delete: %v", err)
		}
	})

	t.Run("sessions", func(t *testing.T) {
		s := newStore(t)
		u := mkUser(t, s, domain.RoleUser)
		sess := &domain.Session{ID: id("s"), UserID: u.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour), LastSeenAt: now}
		if err := s.CreateSession(ctx, sess); err != nil {
			t.Fatal(err)
		}
		got, err := s.GetSession(ctx, sess.ID)
		if err != nil || got.UserID != u.ID || !got.ExpiresAt.Equal(sess.ExpiresAt) {
			t.Fatalf("get: %+v %v", got, err)
		}
		n, err := s.DeleteExpiredSessions(ctx, now.Add(2*time.Hour))
		if err != nil || n < 1 {
			t.Fatalf("expire: %d %v", n, err)
		}
		if _, err := s.GetSession(ctx, sess.ID); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expired session still there: %v", err)
		}
	})

	t.Run("identities", func(t *testing.T) {
		s := newStore(t)
		u := mkUser(t, s, domain.RoleUser)
		other := mkUser(t, s, domain.RoleUser)
		iss, sub := "https://idp.example/"+id("i"), id("sub")
		ident := &domain.Identity{Issuer: iss, Subject: sub, UserID: u.ID, CreatedAt: now}
		if err := s.CreateIdentity(ctx, ident); err != nil {
			t.Fatal(err)
		}
		dup := &domain.Identity{Issuer: iss, Subject: sub, UserID: other.ID, CreatedAt: now}
		if err := s.CreateIdentity(ctx, dup); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("duplicate identity: %v", err)
		}
		got, err := s.GetIdentity(ctx, iss, sub)
		if err != nil || got.UserID != u.ID || got.LastLoginAt != nil {
			t.Fatalf("get: %+v %v", got, err)
		}
		if _, err := s.GetIdentity(ctx, iss, "nope"); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("missing identity: %v", err)
		}
		if err := s.TouchIdentity(ctx, iss, sub, now.Add(time.Minute)); err != nil {
			t.Fatal(err)
		}
		list, err := s.ListUserIdentities(ctx, u.ID)
		if err != nil || len(list) != 1 || list[0].LastLoginAt == nil || !list[0].LastLoginAt.Equal(now.Add(time.Minute)) {
			t.Fatalf("list: %+v %v", list, err)
		}
		if err := s.DeleteIdentity(ctx, other.ID, iss, sub); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("delete by non-owner: %v", err)
		}
		if err := s.DeleteIdentity(ctx, u.ID, iss, sub); err != nil {
			t.Fatal(err)
		}
		// Deleting the user cascades to identities.
		if err := s.CreateIdentity(ctx, ident); err != nil {
			t.Fatal(err)
		}
		if err := s.DeleteUser(ctx, u.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := s.GetIdentity(ctx, iss, sub); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("identity survived user delete: %v", err)
		}
	})

	t.Run("push subscriptions", func(t *testing.T) {
		s := newStore(t)
		u, other := mkUser(t, s, domain.RoleUser), mkUser(t, s, domain.RoleUser)
		endpoint := "https://push.example/" + id("ep")
		sub := &domain.PushSubscription{
			ID: id("ps"), UserID: u.ID, Endpoint: endpoint, P256dh: "k", Auth: "a",
			Name: "phone", MinPriority: 4, CreatedAt: now,
		}
		if err := s.UpsertPushSubscription(ctx, sub); err != nil {
			t.Fatal(err)
		}
		firstID := sub.ID
		hi, _ := s.ListPushTargets(ctx, u.ID, 5)
		lo, _ := s.ListPushTargets(ctx, u.ID, 3)
		if len(hi) != 1 || len(lo) != 0 || hi[0].P256dh != "k" {
			t.Fatalf("targets: hi=%+v lo=%+v", hi, lo)
		}
		if err := s.RecordPushResult(ctx, sub.ID, false, now); err != nil {
			t.Fatal(err)
		}
		if err := s.RecordPushResult(ctx, sub.ID, false, now); err != nil {
			t.Fatal(err)
		}
		got, err := s.GetPushSubscription(ctx, u.ID, sub.ID)
		if err != nil || got.Failures != 2 || got.LastSuccessAt != nil {
			t.Fatalf("failures: %+v %v", got, err)
		}
		_ = s.RecordPushResult(ctx, sub.ID, true, now.Add(time.Minute))
		got, _ = s.GetPushSubscription(ctx, u.ID, sub.ID)
		if got.Failures != 0 || got.LastSuccessAt == nil {
			t.Fatalf("success: %+v", got)
		}

		// The same browser subscribing for another user moves the row.
		again := &domain.PushSubscription{
			ID: id("ps"), UserID: other.ID, Endpoint: endpoint, P256dh: "k2", Auth: "a2",
			MinPriority: 3, CreatedAt: now,
		}
		if err := s.UpsertPushSubscription(ctx, again); err != nil || again.ID != firstID {
			t.Fatalf("takeover: %v id=%s want %s", err, again.ID, firstID)
		}
		if mine, _ := s.ListPushSubscriptions(ctx, u.ID); len(mine) != 0 {
			t.Fatalf("old owner still has it: %+v", mine)
		}
		if _, err := s.GetPushSubscription(ctx, u.ID, firstID); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("old owner get: %v", err)
		}
		if n, _ := s.CountPushSubscriptions(ctx, other.ID); n != 1 {
			t.Fatalf("count %d", n)
		}
		again.MinPriority, again.Name = 1, "desk"
		if err := s.UpdatePushSubscription(ctx, again); err != nil {
			t.Fatal(err)
		}
		if err := s.DeletePushSubscription(ctx, u.ID, firstID); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("delete by non-owner: %v", err)
		}
		if err := s.DeletePushSubscriptionByID(ctx, firstID); err != nil {
			t.Fatal(err)
		}
		if err := s.DeletePushSubscriptionByID(ctx, firstID); err != nil {
			t.Fatalf("idempotent delete: %v", err)
		}
	})

	t.Run("webhooks", func(t *testing.T) {
		s := newStore(t)
		u, other := mkUser(t, s, domain.RoleUser), mkUser(t, s, domain.RoleUser)
		tp := mkTopic(t, s, u, "ci")
		w := &domain.Webhook{
			ID: id("wh"), UserID: u.ID, Name: "ha", URL: "https://ha.example/api/webhook/x",
			Format: domain.WebhookAcksy, Secret: "whsec", TopicIDs: []string{tp.ID}, MinPriority: 3, Enabled: true,
			CreatedAt: now, UpdatedAt: now,
		}
		if err := s.CreateWebhook(ctx, w); err != nil {
			t.Fatal(err)
		}
		got, err := s.GetWebhook(ctx, u.ID, w.ID)
		if err != nil || !got.Enabled || len(got.TopicIDs) != 1 || got.TopicIDs[0] != tp.ID || got.Secret != "whsec" {
			t.Fatalf("get: %+v %v", got, err)
		}
		if _, err := s.GetWebhook(ctx, other.ID, w.ID); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("foreign get: %v", err)
		}
		w.Enabled, w.TopicIDs, w.UpdatedAt = false, nil, now.Add(time.Minute)
		if err := s.UpdateWebhook(ctx, w); err != nil {
			t.Fatal(err)
		}
		if en, _ := s.ListEnabledWebhooks(ctx, u.ID); len(en) != 0 {
			t.Fatalf("disabled listed as enabled: %+v", en)
		}
		all, _ := s.ListWebhooks(ctx, u.ID)
		if len(all) != 1 || all[0].Enabled || len(all[0].TopicIDs) != 0 {
			t.Fatalf("after update: %+v", all)
		}
		if err := s.RecordWebhookResult(ctx, w.ID, false, now, "HTTP 500"); err != nil {
			t.Fatal(err)
		}
		got, _ = s.GetWebhookByID(ctx, w.ID)
		if got.LastFailure == nil || got.LastError != "HTTP 500" {
			t.Fatalf("failure: %+v", got)
		}

		// Queue: enqueue, claim once, reschedule, give up, prune.
		due := now
		d := &domain.WebhookDelivery{
			ID: id("wd"), WebhookID: w.ID, Event: domain.WebhookCreated, Payload: "{}",
			NextAttempt: &due, CreatedAt: now,
		}
		if err := s.EnqueueWebhookDelivery(ctx, d); err != nil {
			t.Fatal(err)
		}
		if list, _ := s.ListDueWebhookDeliveries(ctx, now.Add(-time.Second), 10); len(list) != 0 {
			t.Fatalf("not yet due: %+v", list)
		}
		list, err := s.ListDueWebhookDeliveries(ctx, now, 10)
		if err != nil || len(list) != 1 {
			t.Fatalf("due: %+v %v", list, err)
		}
		lease := now.Add(2 * time.Minute)
		if ok, err := s.ClaimWebhookDelivery(ctx, d.ID, now, lease); !ok || err != nil {
			t.Fatalf("first claim: %v %v", ok, err)
		}
		if ok, _ := s.ClaimWebhookDelivery(ctx, d.ID, now, lease); ok {
			t.Fatal("second concurrent claim succeeded")
		}
		retry := now.Add(time.Minute)
		d.NextAttempt, d.LastStatus, d.LastError = &retry, 503, "HTTP 503"
		if err := s.RescheduleWebhookDelivery(ctx, d); err != nil {
			t.Fatal(err)
		}
		hist, _ := s.ListWebhookDeliveries(ctx, w.ID, 10)
		if len(hist) != 1 || hist[0].Attempts != 1 || hist[0].LastStatus != 503 || !hist[0].NextAttempt.Equal(retry) {
			t.Fatalf("history: %+v", hist)
		}
		d.NextAttempt, d.FailedAt = nil, &retry
		_ = s.RescheduleWebhookDelivery(ctx, d)
		if list, _ := s.ListDueWebhookDeliveries(ctx, now.Add(time.Hour), 10); len(list) != 0 {
			t.Fatalf("failed delivery still due: %+v", list)
		}
		if n, err := s.DeleteFailedWebhookDeliveries(ctx, retry.Add(time.Second)); err != nil || n != 1 {
			t.Fatalf("prune: %d %v", n, err)
		}

		// Deleting the webhook removes its pending deliveries.
		_ = s.EnqueueWebhookDelivery(ctx, &domain.WebhookDelivery{
			ID: id("wd"), WebhookID: w.ID, Event: domain.WebhookCreated,
			Payload: "{}", NextAttempt: &due, CreatedAt: now,
		})
		if err := s.DeleteWebhook(ctx, other.ID, w.ID); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("foreign delete: %v", err)
		}
		if err := s.DeleteWebhook(ctx, u.ID, w.ID); err != nil {
			t.Fatal(err)
		}
		if hist, _ := s.ListWebhookDeliveries(ctx, w.ID, 10); len(hist) != 0 {
			t.Fatalf("deliveries survived webhook delete: %+v", hist)
		}
	})

	t.Run("retention queries", func(t *testing.T) {
		s := newStore(t)
		u, other := mkUser(t, s, domain.RoleUser), mkUser(t, s, domain.RoleUser)
		short := mkTopic(t, s, u, "short")
		plain := mkTopic(t, s, u, "plain")
		days, maxN := 7, 10
		short.RetentionDays, short.MaxNotifications, short.UpdatedAt = &days, &maxN, now
		if err := s.UpdateTopic(ctx, short); err != nil {
			t.Fatal(err)
		}
		old, recent := now.Add(-30*24*time.Hour), now.Add(-time.Hour)
		add := func(user *domain.User, topic *domain.Topic, prio int, read bool, updated time.Time, expires *time.Time) string {
			t.Helper()
			n := mkNotif(user.ID, topic, func(n *domain.Notification) { n.Priority, n.ExpiresAt = prio, expires })
			if _, err := s.UpsertNotification(ctx, n); err != nil {
				t.Fatal(err)
			}
			n.UpdatedAt = updated
			if read {
				n.ReadAt = &updated
			}
			if err := s.SetNotificationState(ctx, n); err != nil {
				t.Fatal(err)
			}
			return n.ID
		}
		oldReadP1 := add(u, nil, 1, true, old, nil)
		oldReadP3 := add(u, plain, 3, true, old, nil)
		oldReadShort := add(u, short, 3, true, old, nil)
		oldUnread := add(u, nil, 3, false, old, nil)
		_ = add(u, nil, 3, true, recent, nil)
		expiredAt := now.Add(-48 * time.Hour)
		expired := add(other, nil, 3, false, recent, &expiredAt)

		// Retention queries span the whole database; Postgres tests share one
		// across runs, so keep only this test's users.
		mine := map[string]bool{u.ID: true, other.ID: true}
		ids := func(e domain.ExpiryQuery) []string {
			t.Helper()
			refs, err := s.ListExpiredNotificationRefs(ctx, e, 10_000)
			if err != nil {
				t.Fatal(err)
			}
			var out []string
			for _, r := range refs {
				if mine[r.UserID] {
					out = append(out, r.ID)
				}
			}
			return out
		}
		cutoff := now.Add(-7 * 24 * time.Hour)
		if got := ids(domain.ExpiryQuery{State: domain.ReadOnly, UpdatedBefore: cutoff, SkipTopicOverrides: true}); !sameSet(got, oldReadP1, oldReadP3) {
			t.Fatalf("read, no overrides: %v", got)
		}
		if got := ids(domain.ExpiryQuery{State: domain.ReadOnly, UpdatedBefore: cutoff, Priority: 1}); !sameSet(got, oldReadP1) {
			t.Fatalf("read P1: %v", got)
		}
		if got := ids(domain.ExpiryQuery{State: domain.ReadOnly, UpdatedBefore: cutoff, TopicID: short.ID}); !sameSet(got, oldReadShort) {
			t.Fatalf("topic: %v", got)
		}
		if got := ids(domain.ExpiryQuery{State: domain.UnreadOnly, UpdatedBefore: cutoff}); !sameSet(got, oldUnread) {
			t.Fatalf("unread: %v", got)
		}
		if got := ids(domain.ExpiryQuery{ExpiresBefore: now.Add(-24 * time.Hour)}); !sameSet(got, expired) {
			t.Fatalf("expired: %v", got)
		}
		if _, err := s.ListExpiredNotificationRefs(ctx, domain.ExpiryQuery{}, 10); err == nil {
			t.Fatal("unconditional query accepted")
		}

		topics, err := s.ListRetentionTopics(ctx)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, tp := range topics {
			if tp.ID == plain.ID {
				t.Fatal("topic without overrides listed")
			}
			if tp.ID == short.ID {
				found = *tp.RetentionDays == 7 && *tp.MaxNotifications == 10
			}
		}
		if !found {
			t.Fatalf("retention topics: %+v", topics)
		}

		// Overflow: u has 5 notifications; keep 2 → the 3 oldest read go,
		// before the unread one.
		users, _ := s.ListUsersOverNotificationLimit(ctx, "", 2)
		if !slices.Contains(users, u.ID) || slices.Contains(users, other.ID) {
			t.Fatalf("over limit: %v", users)
		}
		refs, err := s.ListOverflowNotificationRefs(ctx, u.ID, "", 2, 100)
		if err != nil || len(refs) != 3 {
			t.Fatalf("overflow: %+v %v", refs, err)
		}
		for _, r := range refs {
			if r.ID == oldUnread {
				t.Fatal("unread evicted while read ones remain")
			}
		}
		if refs, _ := s.ListOverflowNotificationRefs(ctx, u.ID, short.ID, 1, 100); len(refs) != 0 {
			t.Fatalf("topic overflow under limit: %+v", refs)
		}
		if refs, _ := s.ListOverflowNotificationRefs(ctx, u.ID, "", 2, 1); len(refs) != 1 {
			t.Fatalf("limit not applied: %+v", refs)
		}
	})

	t.Run("instance settings", func(t *testing.T) {
		s := newStore(t)
		key := id("setting")
		if _, err := s.GetSetting(ctx, key); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("missing: %v", err)
		}
		if err := s.InsertSettingIfAbsent(ctx, key, "first", now); err != nil {
			t.Fatal(err)
		}
		if err := s.InsertSettingIfAbsent(ctx, key, "second", now); err != nil {
			t.Fatal(err)
		}
		if v, err := s.GetSetting(ctx, key); err != nil || v != "first" {
			t.Fatalf("first writer must win: %q %v", v, err)
		}
	})

	t.Run("tokens", func(t *testing.T) {
		s := newStore(t)
		u := mkUser(t, s, domain.RoleUser)
		tp := mkTopic(t, s, u, "ci")
		tok := &domain.APIToken{
			ID: id("k"), UserID: u.ID, Name: "ci", Prefix: "abcd", SecretHash: id("hash"),
			Scopes: domain.Scopes{domain.ScopePublish}, TopicIDs: []string{tp.ID}, CreatedAt: now,
		}
		if err := s.CreateToken(ctx, tok); err != nil {
			t.Fatal(err)
		}
		got, err := s.GetTokenByHash(ctx, tok.SecretHash)
		if err != nil || got.ID != tok.ID || !got.Scopes.Has(domain.ScopePublish) || len(got.TopicIDs) != 1 {
			t.Fatalf("get: %+v %v", got, err)
		}
		if err := s.TouchToken(ctx, tok.ID, now); err != nil {
			t.Fatal(err)
		}
		list, _ := s.ListTokens(ctx, u.ID)
		if len(list) != 1 || list[0].LastUsedAt == nil {
			t.Fatalf("list: %+v", list)
		}
		other := mkUser(t, s, domain.RoleUser)
		if err := s.DeleteToken(ctx, other.ID, tok.ID); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("delete by non-owner: %v", err)
		}
		if err := s.DeleteToken(ctx, u.ID, tok.ID); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("topic invites", func(t *testing.T) {
		s := newStore(t)
		alice, bob, carol := mkUser(t, s, domain.RoleUser), mkUser(t, s, domain.RoleUser), mkUser(t, s, domain.RoleUser)
		tp := mkTopic(t, s, alice, "backups")
		other := mkTopic(t, s, carol, "ci")

		if err := s.CreateTopicInvite(ctx, tp.ID, bob.ID, now); err != nil {
			t.Fatal(err)
		}
		if err := s.CreateTopicInvite(ctx, tp.ID, bob.ID, now); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("duplicate invite: %v", err)
		}
		if err := s.CreateTopicInvite(ctx, other.ID, bob.ID, now.Add(time.Second)); err != nil {
			t.Fatal(err)
		}
		if err := s.CreateTopicInvite(ctx, tp.ID, carol.ID, now); err != nil {
			t.Fatal(err)
		}

		inv, err := s.GetTopicInvite(ctx, tp.ID, bob.ID)
		if err != nil || inv.TopicName != "backups" || inv.OwnerUsername != alice.Username || inv.Username != bob.Username ||
			!inv.CreatedAt.Equal(now) {
			t.Fatalf("get invite: %+v %v", inv, err)
		}
		if _, err := s.GetTopicInvite(ctx, tp.ID, alice.ID); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("missing invite: %v", err)
		}
		mine, _ := s.ListUserInvites(ctx, bob.ID)
		if len(mine) != 2 || mine[0].TopicID != tp.ID || mine[1].TopicID != other.ID {
			t.Fatalf("bob's invites: %+v", mine)
		}
		forTopic, _ := s.ListTopicInvites(ctx, tp.ID)
		if len(forTopic) != 2 {
			t.Fatalf("topic invites: %+v", forTopic)
		}
		// An invitation is not a membership.
		if _, err := s.GetMemberRole(ctx, tp.ID, bob.ID); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("invitee became a member: %v", err)
		}
		if recips, _ := s.ListRecipientIDs(ctx, tp.ID); len(recips) != 1 {
			t.Fatalf("invitee receives: %v", recips)
		}

		if err := s.DeleteTopicInvite(ctx, tp.ID, bob.ID); err != nil {
			t.Fatal(err)
		}
		if err := s.DeleteTopicInvite(ctx, tp.ID, bob.ID); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("delete twice: %v", err)
		}
		// Deleting the topic drops its invitations.
		if err := s.DeleteTopic(ctx, tp.ID); err != nil {
			t.Fatal(err)
		}
		if left, _ := s.ListUserInvites(ctx, carol.ID); len(left) != 0 {
			t.Fatalf("invites of a deleted topic: %+v", left)
		}
	})

	t.Run("topics and members", func(t *testing.T) {
		s := newStore(t)
		alice, bob := mkUser(t, s, domain.RoleUser), mkUser(t, s, domain.RoleUser)
		tp := mkTopic(t, s, alice, "backups")

		dup := *tp
		dup.ID = id("t")
		if err := s.CreateTopic(ctx, &dup); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("duplicate topic: %v", err)
		}
		if role, err := s.GetMemberRole(ctx, tp.ID, alice.ID); err != nil || role != domain.MemberOwner {
			t.Fatalf("owner membership: %q %v", role, err)
		}
		if err := s.AddTopicMember(ctx, tp.ID, bob.ID, domain.MemberMember, now); err != nil {
			t.Fatal(err)
		}
		if err := s.AddTopicMember(ctx, tp.ID, bob.ID, domain.MemberMember, now); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("duplicate member: %v", err)
		}

		bobTopics, _ := s.ListTopicsForUser(ctx, bob.ID)
		if len(bobTopics) != 1 || bobTopics[0].Role != domain.MemberMember || bobTopics[0].MemberCount != 2 ||
			bobTopics[0].OwnerUsername != alice.Username {
			t.Fatalf("bob topics: %+v", bobTopics)
		}
		found, _ := s.FindMemberTopicsByName(ctx, bob.ID, "backups")
		if len(found) != 1 {
			t.Fatalf("find by name: %+v", found)
		}
		recips, _ := s.ListRecipientIDs(ctx, tp.ID)
		if len(recips) != 2 {
			t.Fatalf("recipients: %v", recips)
		}

		// Disabled users stop receiving.
		bob.DisabledAt = &now
		_ = s.UpdateUser(ctx, bob)
		if recips, _ := s.ListRecipientIDs(ctx, tp.ID); len(recips) != 1 {
			t.Fatalf("disabled recipient included: %v", recips)
		}

		members, _ := s.ListTopicMembers(ctx, tp.ID)
		if len(members) != 2 || members[0].Role != domain.MemberOwner {
			t.Fatalf("members: %+v", members)
		}
		if err := s.RemoveTopicMember(ctx, tp.ID, bob.ID); err != nil {
			t.Fatal(err)
		}
		p := 5
		tp.Title, tp.DefaultPriority, tp.UpdatedAt = "Backups", &p, now.Add(time.Minute)
		if err := s.UpdateTopic(ctx, tp); err != nil {
			t.Fatal(err)
		}
		got, _ := s.GetTopicByOwnerAndName(ctx, alice.ID, "backups")
		if got.Title != "Backups" || got.DefaultPriority == nil || *got.DefaultPriority != 5 {
			t.Fatalf("update: %+v", got)
		}
		if err := s.DeleteTopic(ctx, tp.ID); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("notifications", func(t *testing.T) {
		s := newStore(t)
		u := mkUser(t, s, domain.RoleUser)
		tp := mkTopic(t, s, u, "ci")

		n1 := mkNotif(u.ID, tp, func(n *domain.Notification) {
			n.Priority = 5
			n.Actions = []domain.Action{{Type: "view", Label: "Open", URL: "https://x"}}
		})
		n2 := mkNotif(u.ID, nil, func(n *domain.Notification) { n.GroupKey = "g" })
		n3 := mkNotif(u.ID, tp, nil)
		for _, n := range []*domain.Notification{n1, n2, n3} {
			if deduped, err := s.UpsertNotification(ctx, n); err != nil || deduped {
				t.Fatalf("insert: %v %v", deduped, err)
			}
		}

		got, err := s.GetNotification(ctx, u.ID, n1.ID)
		if err != nil || got.Topic == nil || got.Topic.Name != "ci" || len(got.Actions) != 1 || got.Tags[0] != "a" {
			t.Fatalf("get: %+v %v", got, err)
		}
		if _, err := s.GetNotification(ctx, "other", n1.ID); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("cross-user get: %v", err)
		}

		list := func(f domain.NotificationFilter) []string {
			t.Helper()
			f.UserID, f.Now = u.ID, now
			ns, err := s.ListNotifications(ctx, f)
			if err != nil {
				t.Fatal(err)
			}
			ids := make([]string, len(ns))
			for i, n := range ns {
				ids[i] = n.ID
			}
			return ids
		}
		if ids := list(domain.NotificationFilter{}); len(ids) != 3 || ids[0] != n3.ID {
			t.Fatalf("inbox order: %v", ids)
		}
		if ids := list(domain.NotificationFilter{Limit: 2}); len(ids) != 2 {
			t.Fatalf("limit: %v", ids)
		}
		if ids := list(domain.NotificationFilter{Before: n3.ID}); len(ids) != 2 || ids[0] != n2.ID {
			t.Fatalf("cursor: %v", ids)
		}
		if ids := list(domain.NotificationFilter{TopicIDs: []string{tp.ID}}); len(ids) != 2 {
			t.Fatalf("topic filter: %v", ids)
		}
		if ids := list(domain.NotificationFilter{TopicIDs: []string{}}); len(ids) != 0 {
			t.Fatalf("empty topic restriction must match nothing: %v", ids)
		}
		if ids := list(domain.NotificationFilter{NoTopic: true}); len(ids) != 1 || ids[0] != n2.ID {
			t.Fatalf("no topic: %v", ids)
		}
		if ids := list(domain.NotificationFilter{MinPriority: 4}); len(ids) != 1 || ids[0] != n1.ID {
			t.Fatalf("priority: %v", ids)
		}
		if ids := list(domain.NotificationFilter{GroupKey: "g"}); len(ids) != 1 {
			t.Fatalf("group: %v", ids)
		}

		// State changes and views.
		later := now.Add(time.Hour)
		got.ReadAt, got.SnoozedUntil, got.UpdatedAt = &now, &later, now
		if err := s.SetNotificationState(ctx, got); err != nil {
			t.Fatal(err)
		}
		if ids := list(domain.NotificationFilter{}); len(ids) != 2 {
			t.Fatalf("snoozed still in inbox: %v", ids)
		}
		if ids := list(domain.NotificationFilter{View: domain.ViewSnoozed}); len(ids) != 1 {
			t.Fatalf("snoozed view: %v", ids)
		}

		c, err := s.Counts(ctx, u.ID, nil, now)
		if err != nil || c.Unread != 2 || c.ByTopic[tp.ID] != 1 || c.ByTopic[""] != 1 {
			t.Fatalf("counts: %+v %v", c, err)
		}

		// Bulk.
		if err := s.BulkUpdateNotifications(ctx, u.ID, []string{n2.ID, n3.ID}, domain.BulkArchive, now); err != nil {
			t.Fatal(err)
		}
		if ids := list(domain.NotificationFilter{View: domain.ViewArchived}); len(ids) != 2 {
			t.Fatalf("archived: %v", ids)
		}
		n2r, _ := s.GetNotification(ctx, u.ID, n2.ID)
		if n2r.ReadAt == nil {
			t.Fatal("archive must also mark read")
		}
		if err := s.BulkUpdateNotifications(ctx, u.ID, []string{n2.ID}, domain.BulkUnread, now); err != nil {
			t.Fatal(err)
		}
		ids, _ := s.ListNotificationIDs(ctx, domain.NotificationFilter{UserID: u.ID, View: domain.ViewAll, UnreadOnly: true, Now: now})
		if len(ids) != 1 || ids[0] != n2.ID {
			t.Fatalf("unread ids: %v", ids)
		}
		if err := s.BulkUpdateNotifications(ctx, u.ID, []string{n3.ID}, domain.BulkDelete, now); err != nil {
			t.Fatal(err)
		}
		if _, err := s.GetNotification(ctx, u.ID, n3.ID); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("bulk delete: %v", err)
		}

		// Expired notifications are hidden.
		past := now.Add(-time.Second)
		exp := mkNotif(u.ID, nil, func(n *domain.Notification) { n.ExpiresAt = &past })
		_, _ = s.UpsertNotification(ctx, exp)
		if ids := list(domain.NotificationFilter{View: domain.ViewAll}); len(ids) != 2 {
			t.Fatalf("expired visible: %v", ids)
		}
	})

	t.Run("refs by topic and key", func(t *testing.T) {
		s := newStore(t)
		a, b := mkUser(t, s, domain.RoleUser), mkUser(t, s, domain.RoleUser)
		tp := mkTopic(t, s, a, "ci")
		other := mkTopic(t, s, a, "other")
		for _, n := range []*domain.Notification{
			mkNotif(a.ID, tp, func(n *domain.Notification) { n.DedupKey = "deploy" }),
			mkNotif(b.ID, tp, func(n *domain.Notification) { n.DedupKey = "deploy" }),
			mkNotif(a.ID, other, func(n *domain.Notification) { n.DedupKey = "deploy-2" }),
			mkNotif(b.ID, tp, func(n *domain.Notification) { n.DedupKey = "else" }),
			mkNotif(b.ID, tp, nil),
		} {
			if _, err := s.UpsertNotification(ctx, n); err != nil {
				t.Fatal(err)
			}
		}
		refs, err := s.ListNotificationRefsByKey(ctx, tp.ID, "deploy")
		if err != nil || len(refs) != 2 {
			t.Fatalf("refs: %+v %v", refs, err)
		}
		users := map[string]bool{refs[0].UserID: true, refs[1].UserID: true}
		if !users[a.ID] || !users[b.ID] {
			t.Fatalf("wrong recipients: %+v", refs)
		}
		if refs, err := s.ListNotificationRefsByKey(ctx, other.ID, "deploy"); err != nil || len(refs) != 0 {
			t.Fatalf("other topic: %+v %v", refs, err)
		}
	})

	t.Run("dedup upsert", func(t *testing.T) {
		s := newStore(t)
		u := mkUser(t, s, domain.RoleUser)

		first := mkNotif(u.ID, nil, func(n *domain.Notification) { n.DedupKey = "disk-full" })
		if deduped, err := s.UpsertNotification(ctx, first); err != nil || deduped {
			t.Fatalf("first: %v %v", deduped, err)
		}
		// Archive and read it, then the same alert fires again.
		first.ReadAt, first.ArchivedAt, first.UpdatedAt = &now, &now, now
		_ = s.SetNotificationState(ctx, first)

		again := mkNotif(u.ID, nil, func(n *domain.Notification) {
			n.DedupKey, n.Title = "disk-full", "Disk 99%"
			n.UpdatedAt = now.Add(time.Minute)
		})
		deduped, err := s.UpsertNotification(ctx, again)
		if err != nil || !deduped || again.ID != first.ID || again.RepeatCount != 2 {
			t.Fatalf("dedup: deduped=%v id=%s want %s repeat=%d err=%v", deduped, again.ID, first.ID, again.RepeatCount, err)
		}
		got, _ := s.GetNotification(ctx, u.ID, first.ID)
		if got.Title != "Disk 99%" || got.ReadAt != nil || got.ArchivedAt != nil || !got.CreatedAt.Equal(now) {
			t.Fatalf("dedup must resurface and keep created_at: %+v", got)
		}

		// Other users with the same key are independent.
		other := mkUser(t, s, domain.RoleUser)
		o := mkNotif(other.ID, nil, func(n *domain.Notification) { n.DedupKey = "disk-full" })
		if deduped, _ := s.UpsertNotification(ctx, o); deduped {
			t.Fatal("dedup crossed users")
		}

		// Keys are scoped to the topic: the same key elsewhere is a new notification.
		tp := mkTopic(t, s, u, "ci")
		inTopic := mkNotif(u.ID, tp, func(n *domain.Notification) { n.DedupKey = "disk-full" })
		if deduped, err := s.UpsertNotification(ctx, inTopic); err != nil || deduped || inTopic.ID == first.ID {
			t.Fatalf("dedup crossed topics: deduped=%v err=%v", deduped, err)
		}
		repeat := mkNotif(u.ID, tp, func(n *domain.Notification) { n.DedupKey = "disk-full" })
		if deduped, err := s.UpsertNotification(ctx, repeat); err != nil || !deduped || repeat.ID != inTopic.ID {
			t.Fatalf("dedup within topic: deduped=%v err=%v", deduped, err)
		}

		// Deleting the topic drops its keys, so the topic-less key stays unique.
		if err := s.DeleteTopic(ctx, tp.ID); err != nil {
			t.Fatalf("delete topic: %v", err)
		}
		orphan, err := s.GetNotification(ctx, u.ID, inTopic.ID)
		if err != nil || orphan.Topic != nil || orphan.DedupKey != "" {
			t.Fatalf("orphaned notification: %+v %v", orphan, err)
		}
		later := mkNotif(u.ID, nil, func(n *domain.Notification) { n.DedupKey = "disk-full" })
		if deduped, err := s.UpsertNotification(ctx, later); err != nil || !deduped || later.ID != first.ID {
			t.Fatalf("topic-less dedup after topic deletion: deduped=%v err=%v", deduped, err)
		}
	})

	t.Run("search", func(t *testing.T) {
		s := newStore(t)
		u := mkUser(t, s, domain.RoleUser)
		titled := func(title string, mut ...func(*domain.Notification)) string {
			n := mkNotif(u.ID, nil, func(n *domain.Notification) {
				n.Title, n.Body, n.Tags = title, "", nil
				for _, m := range mut {
					m(n)
				}
			})
			if _, err := s.UpsertNotification(ctx, n); err != nil {
				t.Fatal(err)
			}
			return n.ID
		}
		backup := titled("Бэкап завершён")
		disk := titled("Disk full on /srv")
		pct := titled("100% done")
		titled("1000 done")
		under := titled("a_b")
		titled("aXb")
		find := func(terms ...string) []string {
			ids, err := s.ListNotificationIDs(ctx, domain.NotificationFilter{UserID: u.ID, View: domain.ViewAll, Search: terms, Now: now})
			if err != nil {
				t.Fatalf("search %q: %v", terms, err)
			}
			return ids
		}
		for _, tc := range []struct {
			terms []string
			want  []string
		}{
			{[]string{"бэкап"}, []string{backup}},
			{[]string{"disk", "full"}, []string{disk}},
			{[]string{"disk", "бэкап"}, nil},
			{[]string{"100%"}, []string{pct}},
			{[]string{"a_b"}, []string{under}},
			{[]string{`\`}, nil},
		} {
			if got := find(tc.terms...); !sameSet(got, tc.want...) {
				t.Errorf("search %q = %v, want %v", tc.terms, got, tc.want)
			}
		}

		// A repeat with a new title is found by the new words only.
		first := titled("old alert", func(n *domain.Notification) { n.DedupKey = "d1" })
		titled("new alert", func(n *domain.Notification) { n.DedupKey = "d1"; n.UpdatedAt = now.Add(time.Minute) })
		if got := find("new"); !sameSet(got, first) {
			t.Errorf("after dedup, new = %v", got)
		}
		if got := find("old"); len(got) != 0 {
			t.Errorf("after dedup, old = %v", got)
		}
	})

	t.Run("snoozed count", func(t *testing.T) {
		s := newStore(t)
		u := mkUser(t, s, domain.RoleUser)
		topic := mkTopic(t, s, u, "ci")
		later := now.Add(time.Hour)
		for _, mut := range []func(*domain.Notification){
			nil, // unread in the inbox
			func(n *domain.Notification) { n.SnoozedUntil = &later },
			func(n *domain.Notification) { n.SnoozedUntil = &later },
			func(n *domain.Notification) { n.SnoozedUntil, n.ArchivedAt = &later, &now },
		} {
			n := mkNotif(u.ID, nil, mut)
			if _, err := s.UpsertNotification(ctx, n); err != nil {
				t.Fatal(err)
			}
			if mut != nil {
				if err := s.SetNotificationState(ctx, n); err != nil {
					t.Fatal(err)
				}
			}
		}
		c, err := s.Counts(ctx, u.ID, nil, now)
		if err != nil || c.Unread != 1 || c.Snoozed != 2 {
			t.Fatalf("counts = %+v, %v; want unread 1, snoozed 2", c, err)
		}
		// A token limited to a topic sees none of these topic-less ones.
		c, err = s.Counts(ctx, u.ID, []string{topic.ID}, now)
		if err != nil || c.Unread != 0 || c.Snoozed != 0 {
			t.Fatalf("restricted counts = %+v, %v", c, err)
		}
	})

	t.Run("settings upsert", func(t *testing.T) {
		s := newStore(t)
		if _, _, err := s.GetSettingWithTime(ctx, "k"); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("missing: %v", err)
		}
		if err := s.PutSetting(ctx, "k", "v1", now); err != nil {
			t.Fatal(err)
		}
		later := now.Add(time.Minute)
		if err := s.PutSetting(ctx, "k", "v2", later); err != nil {
			t.Fatal(err)
		}
		v, at, err := s.GetSettingWithTime(ctx, "k")
		if err != nil || v != "v2" || !at.Equal(later) {
			t.Fatalf("got %q %v %v", v, at, err)
		}
	})

	t.Run("events and tx", func(t *testing.T) {
		s := newStore(t)
		u := mkUser(t, s, domain.RoleUser)

		err := s.InTx(ctx, func(q Querier) error {
			for i := range 3 {
				seq, err := q.NextEventSeq(ctx, u.ID)
				if err != nil {
					return err
				}
				e := &domain.Event{
					UserID: u.ID, Seq: seq, Type: domain.EventNotificationCreated,
					Data: []byte(fmt.Sprintf(`{"i":%d}`, i)), CreatedAt: now,
				}
				if err := q.InsertEvent(ctx, e); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		evs, err := s.ListEvents(ctx, u.ID, 1, 10)
		if err != nil || len(evs) != 2 || evs[0].Seq != 2 || string(evs[1].Data) != `{"i":2}` {
			t.Fatalf("events: %+v %v", evs, err)
		}

		// Rollback discards both the seq bump and the event.
		boom := errors.New("boom")
		err = s.InTx(ctx, func(q Querier) error {
			seq, _ := q.NextEventSeq(ctx, u.ID)
			_ = q.InsertEvent(ctx, &domain.Event{UserID: u.ID, Seq: seq, Type: "x", Data: []byte(`{}`), CreatedAt: now})
			return boom
		})
		if !errors.Is(err, boom) {
			t.Fatalf("tx err: %v", err)
		}
		if seq, _ := s.NextEventSeq(ctx, u.ID); seq != 4 {
			t.Fatalf("rollback leaked seq: %d", seq)
		}

		cur, oldest, err := s.EventSeqRange(ctx, u.ID)
		if err != nil || cur != 4 || oldest != 1 {
			t.Fatalf("range: cur=%d oldest=%d err=%v", cur, oldest, err)
		}
		if _, err := s.DeleteEventsBefore(ctx, now.Add(time.Second)); err != nil {
			t.Fatal(err)
		}
		if cur, oldest, _ := s.EventSeqRange(ctx, u.ID); cur != 4 || oldest != 0 {
			t.Fatalf("after prune: cur=%d oldest=%d", cur, oldest)
		}
		if _, _, err := s.EventSeqRange(ctx, "missing"); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("missing user: %v", err)
		}
	})
}
