package domain

import (
	"strings"
	"testing"
)

func TestDeriveUsername(t *testing.T) {
	long := strings.Repeat("a", 40)
	tests := []struct {
		name       string
		candidates []string
		want       string
	}{
		{"plain", []string{"alice"}, "alice"},
		{"lowercased and trimmed", []string{"  Alice "}, "alice"},
		{"invalid chars replaced", []string{"Alice Smith"}, "alice-smith"},
		{"leading separators dropped", []string{"_.alice"}, "alice"},
		{"truncated", []string{long}, long[:32]},
		{"falls through to next", []string{"", "x", "bob"}, "bob"},
		{"unicode replaced", []string{"jöhn"}, "j-hn"},
		{"nothing usable", []string{"", "!"}, "user"},
		{"no candidates", nil, "user"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeriveUsername(tt.candidates...)
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
			if err := ValidateUsername(got); err != nil {
				t.Fatalf("derived username invalid: %v", err)
			}
		})
	}
}

func TestUsernameWithSuffix(t *testing.T) {
	long := strings.Repeat("b", 32)
	tests := []struct {
		base string
		n    int
		want string
	}{
		{"alice", 1, "alice"},
		{"alice", 2, "alice-2"},
		{"alice", 12, "alice-12"},
		{long, 3, long[:30] + "-3"},
	}
	for _, tt := range tests {
		got := UsernameWithSuffix(tt.base, tt.n)
		if got != tt.want {
			t.Errorf("UsernameWithSuffix(%q, %d) = %q, want %q", tt.base, tt.n, got, tt.want)
		}
		if err := ValidateUsername(got); err != nil {
			t.Errorf("%q invalid: %v", got, err)
		}
	}
}

func TestRoleFromClaim(t *testing.T) {
	admins := []string{"acksy-admins", "ops"}
	tests := []struct {
		values []string
		want   Role
	}{
		{nil, RoleUser},
		{[]string{"users"}, RoleUser},
		{[]string{"users", "ops"}, RoleAdmin},
		{[]string{"Acksy-Admins"}, RoleUser}, // exact match only
	}
	for _, tt := range tests {
		if got := RoleFromClaim(tt.values, admins); got != tt.want {
			t.Errorf("RoleFromClaim(%v) = %s, want %s", tt.values, got, tt.want)
		}
	}
	if got := RoleFromClaim([]string{"ops"}, nil); got != RoleUser {
		t.Errorf("no admin values: got %s", got)
	}
}
