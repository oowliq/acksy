package domain

import (
	"errors"
	"testing"
)

func TestOIDCSettingsValidate(t *testing.T) {
	valid := func() OIDCSettings {
		s := DefaultOIDCSettings()
		s.Enabled, s.Issuer, s.ClientID = true, "https://idp.example", "acksy"
		return s
	}
	if err := valid().Validate(); err != nil {
		t.Fatalf("valid settings rejected: %v", err)
	}
	if err := DefaultOIDCSettings().Validate(); err != nil {
		t.Fatalf("defaults (disabled) rejected: %v", err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*OIDCSettings)
	}{
		{"no client id", func(s *OIDCSettings) { s.ClientID = "" }},
		{"relative issuer", func(s *OIDCSettings) { s.Issuer = "idp.example" }},
		{"no issuer", func(s *OIDCSettings) { s.Issuer = "" }},
		{"issuer with query", func(s *OIDCSettings) { s.Issuer = "https://internal.example/x?" }},
		{"issuer with fragment", func(s *OIDCSettings) { s.Issuer = "https://idp.example/#a" }},
		{"no openid scope", func(s *OIDCSettings) { s.Scopes = []string{"profile"} }},
		{"admin values without claim", func(s *OIDCSettings) { s.AdminValues, s.RoleClaim = []string{"x"}, "" }},
		{"password login off without oidc", func(s *OIDCSettings) { s.Enabled, s.DisablePasswordLogin = false, true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := valid()
			tc.mutate(&s)
			if err := s.Validate(); !errors.Is(err, ErrInvalid) {
				t.Fatalf("got %v, want invalid", err)
			}
		})
	}
	if s := valid(); !s.Active() {
		t.Fatal("valid enabled settings should be active")
	}
	if s := valid(); func() bool { s.Enabled = false; return s.Active() }() {
		t.Fatal("disabled settings must not be active")
	}
}
