package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/oowliq/acksy/internal/domain"
)

func TestPassword(t *testing.T) {
	h, err := HashPassword("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("unexpected encoding %q", h)
	}
	if ok, err := CheckPassword(h, "correct horse"); !ok || err != nil {
		t.Fatalf("valid password rejected: %v", err)
	}
	if ok, _ := CheckPassword(h, "wrong"); ok {
		t.Fatal("wrong password accepted")
	}
	h2, _ := HashPassword("correct horse")
	if h == h2 {
		t.Fatal("salt not random")
	}
	for _, bad := range []string{"", "$bcrypt$x", "$argon2id$v=19$m=1,t=1,p=1$!!$AAAA", "$argon2id$v=18$m=1,t=1,p=1$AAAA$AAAA"} {
		if _, err := CheckPassword(bad, "x"); err == nil {
			t.Errorf("malformed %q accepted", bad)
		}
	}
}

func TestAPIToken(t *testing.T) {
	secret, hash := NewAPIToken()
	if !LooksLikeAPIToken(secret) {
		t.Fatalf("bad shape %q", secret)
	}
	if hash != HashSecret(secret) || len(hash) != 64 {
		t.Fatal("hash mismatch")
	}
	other, _ := NewAPIToken()
	if other == secret {
		t.Fatal("tokens not random")
	}
	if LooksLikeAPIToken("ack_short") || LooksLikeAPIToken(strings.Repeat("x", 44)) {
		t.Fatal("shape check too loose")
	}
}

func TestPrincipal(t *testing.T) {
	user := &domain.User{Role: domain.RoleUser}
	admin := &domain.User{Role: domain.RoleAdmin}

	sess := &Principal{User: user, Scopes: domain.SessionScopes(user)}
	if err := sess.Require(domain.ScopePublish); err != nil {
		t.Fatal(err)
	}
	if err := sess.Require(domain.ScopeAdmin); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("user got admin: %v", err)
	}
	// A token keeping the admin scope after its owner was demoted is useless.
	demoted := &Principal{User: user, Token: &domain.APIToken{}, Scopes: domain.Scopes{domain.ScopeAdmin}}
	if err := demoted.Require(domain.ScopeAdmin); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("demoted admin token: %v", err)
	}
	if err := (&Principal{User: admin, Scopes: domain.SessionScopes(admin)}).Require(domain.ScopeAdmin); err != nil {
		t.Fatal(err)
	}

	if !sess.CanUseTopic("any") || sess.TopicRestriction() != nil {
		t.Fatal("session must be unrestricted")
	}
	restricted := &Principal{User: user, Token: &domain.APIToken{TopicIDs: []string{"t1"}}}
	if !restricted.CanUseTopic("t1") || restricted.CanUseTopic("t2") {
		t.Fatal("topic restriction broken")
	}

	ctx := WithPrincipal(context.Background(), sess)
	if FromContext(ctx) != sess || FromContext(context.Background()) != nil {
		t.Fatal("context roundtrip")
	}
}

func TestLoginLimiter(t *testing.T) {
	now := time.Unix(0, 0)
	l := NewLoginLimiter(3, time.Minute)
	l.Now = func() time.Time { return now }

	for range 3 {
		if ok, _ := l.Allowed("bob"); !ok {
			t.Fatal("blocked too early")
		}
		l.Failure("bob")
	}
	if ok, wait := l.Allowed("bob"); ok || wait != time.Minute {
		t.Fatalf("not blocked: %v %v", ok, wait)
	}
	if ok, _ := l.Allowed("alice"); !ok {
		t.Fatal("keys must be independent")
	}
	now = now.Add(time.Minute)
	if ok, _ := l.Allowed("bob"); !ok {
		t.Fatal("window did not reset")
	}
	l.Failure("bob")
	l.Success("bob")
	if ok, _ := l.Allowed("bob"); !ok {
		t.Fatal("success must reset")
	}
}
