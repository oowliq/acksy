package domain

import (
	"errors"
	"testing"
	"time"
)

func TestServerSettingsValidate(t *testing.T) {
	if err := DefaultServerSettings().Validate(); err != nil {
		t.Fatalf("defaults invalid: %v", err)
	}
	tests := []struct {
		name   string
		mutate func(*ServerSettings)
	}{
		{"negative limit", func(s *ServerSettings) { s.PublishLimit.PerToken = -1 }},
		{"huge limit", func(s *ServerSettings) { s.PublishLimit.PerUser = MaxPublishLimit + 1 }},
		{"bad subject", func(s *ServerSettings) { s.Push.Subject = "ops" }},
		{"zero failed retention", func(s *ServerSettings) { s.Webhooks.FailedRetentionDays = 0 }},
		{"negative read", func(s *ServerSettings) { s.Retention.ReadDays = -1 }},
		{"priority 9", func(s *ServerSettings) { s.Retention.ReadDaysByPriority = map[int]int{9: 1} }},
		{"tiny max per user", func(s *ServerSettings) { s.Retention.MaxPerUser = 14 }},
		{"zero session", func(s *ServerSettings) { s.SessionDays = 0 }},
		{"zero event retention", func(s *ServerSettings) { s.EventRetentionDays = 0 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := DefaultServerSettings()
			tt.mutate(&s)
			if err := s.Validate(); !errors.Is(err, ErrInvalid) {
				t.Fatalf("want invalid, got %v", err)
			}
		})
	}
	ok := DefaultServerSettings()
	ok.PublishLimit, ok.Retention.MaxPerUser, ok.Push.Subject = PublishLimitSettings{}, 0, "mailto:ops@example.com"
	if err := ok.Validate(); err != nil {
		t.Fatalf("zeros disable rules: %v", err)
	}
}

func TestServerSettingsDerived(t *testing.T) {
	s := DefaultServerSettings()
	s.Retention.ReadDaysByPriority = map[int]int{1: 7}
	p := s.RetentionPolicy()
	if p.ReadTTLFor(1) != 7*24*time.Hour || p.ReadTTLFor(3) != 90*24*time.Hour || p.MaxPerUser != 10_000 {
		t.Fatalf("policy: %+v", p)
	}
	if s.PushSubject("http://lan") != "" || s.PushSubject("https://acksy.example/") != "https://acksy.example" {
		t.Fatal("subject fallback")
	}
	s.Push.Subject = "mailto:ops@example.com"
	if s.PushSubject("https://acksy.example") != "ops@example.com" {
		t.Fatal("explicit subject")
	}
	if !s.Adapters.Enabled("ntfy") || s.Adapters.Enabled("other") {
		t.Fatal("adapter switch")
	}
}
