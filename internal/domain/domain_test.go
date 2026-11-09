package domain

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestErrorIs(t *testing.T) {
	err := NotFound("topic")
	if !errors.Is(err, ErrNotFound) || errors.Is(err, ErrConflict) {
		t.Fatal("kind matching broken")
	}
	if err.Error() != "topic not found" {
		t.Fatalf("msg %q", err)
	}
}

func TestValidateUsername(t *testing.T) {
	for _, ok := range []string{"al", "alice", "bob.smith", "a_1-2"} {
		if err := ValidateUsername(ok); err != nil {
			t.Errorf("%q rejected: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "a", "-alice", "Alice", "al ice", "ali/ce", strings.Repeat("a", 33)} {
		if ValidateUsername(bad) == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestParseTopicRef(t *testing.T) {
	tests := []struct {
		in      string
		want    TopicRef
		wantErr bool
	}{
		{"ci", TopicRef{Name: "ci"}, false},
		{" CI ", TopicRef{Name: "ci"}, false},
		{"alice/backups", TopicRef{Owner: "alice", Name: "backups"}, false},
		{"alice/", TopicRef{}, true},
		{"/ci", TopicRef{}, true},
		{"a b", TopicRef{}, true},
	}
	for _, tt := range tests {
		got, err := ParseTopicRef(tt.in)
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Errorf("ParseTopicRef(%q) = %+v, %v", tt.in, got, err)
		}
	}
}

func TestParseScopes(t *testing.T) {
	s, err := ParseScopes([]string{"notifications:read", "notifications:publish", "notifications:read"})
	if err != nil {
		t.Fatal(err)
	}
	if s.String() != "notifications:publish notifications:read" {
		t.Fatalf("got %q", s)
	}
	if _, err := ParseScopes([]string{"root"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown scope: %v", err)
	}
	if _, err := ParseScopes(nil); err == nil {
		t.Fatal("empty scopes accepted")
	}
	if got := ScopesFromString(s.String()); got.String() != s.String() {
		t.Fatalf("roundtrip %q", got)
	}
}

func TestSessionScopes(t *testing.T) {
	if SessionScopes(&User{Role: RoleUser}).Has(ScopeAdmin) {
		t.Fatal("user got admin scope")
	}
	if !SessionScopes(&User{Role: RoleAdmin}).Has(ScopeAdmin) {
		t.Fatal("admin lacks admin scope")
	}
}

func TestMessageNormalize(t *testing.T) {
	now := time.Now()
	past := now.Add(-time.Minute)
	valid := func() Message {
		return Message{Title: " Deploy ", Body: "**ok**\n", Tags: []string{"CI", "ci", " prod "}}
	}

	m := valid()
	if err := m.Normalize(now); err != nil {
		t.Fatal(err)
	}
	if m.Title != "Deploy" || m.Body != "**ok**" || len(m.Tags) != 2 || m.Tags[0] != "ci" || m.Source != "acksy" {
		t.Fatalf("normalized: %+v", m)
	}

	tests := []struct {
		name   string
		mutate func(*Message)
	}{
		{"empty", func(m *Message) { m.Title, m.Body = "", "" }},
		{"priority", func(m *Message) { m.Priority = 6 }},
		{"long title", func(m *Message) { m.Title = strings.Repeat("x", MaxTitleLen+1) }},
		{"big body", func(m *Message) { m.Body = strings.Repeat("x", MaxBodyLen+1) }},
		{"bad click url", func(m *Message) { m.ClickURL = "javascript:alert(1)" }},
		{"relative url", func(m *Message) { m.ClickURL = "/x" }},
		{"expired", func(m *Message) { m.ExpiresAt = &past }},
		{"too many tags", func(m *Message) {
			m.Tags = nil
			for i := range MaxTags + 1 {
				m.Tags = append(m.Tags, strings.Repeat("t", i+1))
			}
		}},
		{"action type", func(m *Message) { m.Actions = []Action{{Type: "broadcast", Label: "x", URL: "https://a"}} }},
		{"http method", func(m *Message) { m.Actions = []Action{{Type: "http", Label: "x", URL: "https://a", Method: "TRACE"}} }},
		{"http header name", func(m *Message) {
			m.Actions = []Action{{Type: "http", Label: "x", URL: "https://a", Headers: map[string]string{"Bad Name": "v"}}}
		}},
		{"http header injection", func(m *Message) {
			m.Actions = []Action{{Type: "http", Label: "x", URL: "https://a", Headers: map[string]string{"X-A": "v\r\nHost: evil"}}}
		}},
		{"http reserved header", func(m *Message) {
			m.Actions = []Action{{Type: "http", Label: "x", URL: "https://a", Headers: map[string]string{"Host": "evil"}}}
		}},
		{"view with body", func(m *Message) { m.Actions = []Action{{Label: "x", URL: "https://a", Body: "b"}} }},
		{"action label", func(m *Message) { m.Actions = []Action{{URL: "https://a"}} }},
		{"action url", func(m *Message) { m.Actions = []Action{{Label: "x", URL: "ftp://a"}} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := valid()
			tt.mutate(&m)
			if err := m.Normalize(now); !errors.Is(err, ErrInvalid) {
				t.Fatalf("want invalid, got %v", err)
			}
		})
	}
}

func TestUserJSON(t *testing.T) {
	for _, tc := range []struct {
		hash string
		want string
	}{{"x", `"has_password":true`}, {"", `"has_password":false`}} {
		b, err := json.Marshal(User{ID: "u1", Username: "alice", PasswordHash: tc.hash})
		if err != nil {
			t.Fatal(err)
		}
		if s := string(b); !strings.Contains(s, tc.want) || strings.Contains(s, "password_hash") || (tc.hash != "" && strings.Contains(s, tc.hash+`"`)) {
			t.Fatalf("hash %q: %s", tc.hash, s)
		}
	}
}
