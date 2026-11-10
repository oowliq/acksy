package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ServerSettingsKey is the instance_settings key holding ServerSettings as JSON.
const ServerSettingsKey = "server"

// Server settings bounds.
const (
	MaxPublishLimit   = 100_000
	MinSessionDays    = 1
	MaxSessionDays    = 365
	MinRetentionDays  = 1
	MaxRetentionDays  = 3650
	MinMaxPerUser     = 100
	MaxEventRetention = 365
)

// ServerSettings are the instance-wide runtime settings admins edit in the
// web UI (ADR 0008). Only what is needed to start the server stays in the
// environment.
type ServerSettings struct {
	Adapters     AdapterSettings      `json:"adapters"`
	PublishLimit PublishLimitSettings `json:"publish_limit"`
	Push         PushSettings         `json:"push"`
	Webhooks     WebhookSettings      `json:"webhooks"`
	Actions      ActionSettings       `json:"actions"`
	Retention    RetentionSettings    `json:"retention"`
	// SessionDays is how long a login session lives (extended on activity).
	SessionDays int `json:"session_days"`
	// EventRetentionDays is how long realtime events are kept for resuming
	// streams with ?since=<seq>.
	EventRetentionDays int `json:"event_retention_days"`
}

// AdapterSettings switch the ingest adapters.
type AdapterSettings struct {
	NTFY   bool `json:"ntfy"`
	Gotify bool `json:"gotify"`
	SMTP   bool `json:"smtp"`
}

// Enabled reports whether the adapter with this name is on.
func (a AdapterSettings) Enabled(name string) bool {
	switch name {
	case "ntfy":
		return a.NTFY
	case "gotify":
		return a.Gotify
	case "smtp":
		return a.SMTP
	}
	return false
}

// PublishLimitSettings cap publishes per minute per API token and per user
// (across all of the user's tokens); bursts of the same size are allowed.
// Zero disables a limit.
type PublishLimitSettings struct {
	PerToken int `json:"per_token"`
	PerUser  int `json:"per_user"`
}

// PushSettings configure Web Push.
type PushSettings struct {
	Enabled bool `json:"enabled"`
	// Subject identifies the operator to push services: an https URL or an
	// e-mail address. Empty falls back to an https base URL.
	Subject string `json:"subject"`
}

// WebhookSettings configure outgoing webhooks.
type WebhookSettings struct {
	Enabled bool `json:"enabled"`
	// AllowPrivate lets webhooks reach private and loopback addresses (LAN
	// services such as Home Assistant). Off by default to prevent SSRF.
	AllowPrivate bool `json:"allow_private"`
	// FailedRetentionDays is how long failed deliveries are kept.
	FailedRetentionDays int `json:"failed_retention_days"`
}

// ActionSettings configure http action buttons run by the server.
type ActionSettings struct {
	Enabled bool `json:"enabled"`
	// AllowPrivate lets actions reach private and loopback addresses. The URL
	// is set by the publisher, so enable it only if all publishers are trusted.
	AllowPrivate bool `json:"allow_private"`
}

// RetentionSettings are the instance retention rules in days; zero disables
// a rule. See RetentionPolicy.
type RetentionSettings struct {
	ReadDays           int         `json:"read_days"`
	ReadDaysByPriority map[int]int `json:"read_days_by_priority"`
	UnreadDays         int         `json:"unread_days"`
	MaxPerUser         int         `json:"max_per_user"`
}

// DefaultServerSettings is what an instance starts with.
func DefaultServerSettings() ServerSettings {
	return ServerSettings{
		Adapters:           AdapterSettings{NTFY: true, Gotify: true},
		PublishLimit:       PublishLimitSettings{PerToken: 60, PerUser: 120},
		Push:               PushSettings{Enabled: true},
		Webhooks:           WebhookSettings{Enabled: true, FailedRetentionDays: 7},
		Actions:            ActionSettings{Enabled: true},
		Retention:          RetentionSettings{ReadDays: 90, ReadDaysByPriority: map[int]int{}, MaxPerUser: 10_000},
		SessionDays:        30,
		EventRetentionDays: 7,
	}
}

const day = 24 * time.Hour

// SessionTTL is the login session lifetime.
func (s ServerSettings) SessionTTL() time.Duration { return time.Duration(s.SessionDays) * day }

// EventRetention is how long realtime events are kept.
func (s ServerSettings) EventRetention() time.Duration {
	return time.Duration(s.EventRetentionDays) * day
}

// WebhookFailedRetention is how long failed webhook deliveries are kept.
func (s ServerSettings) WebhookFailedRetention() time.Duration {
	return time.Duration(s.Webhooks.FailedRetentionDays) * day
}

// RetentionPolicy converts the retention rules for ApplyRetention.
func (s ServerSettings) RetentionPolicy() RetentionPolicy {
	r := s.Retention
	p := RetentionPolicy{
		ReadTTL: time.Duration(r.ReadDays) * day, UnreadTTL: time.Duration(r.UnreadDays) * day, MaxPerUser: r.MaxPerUser,
		ReadTTLByPriority: make(map[int]time.Duration, len(r.ReadDaysByPriority)),
	}
	for prio, days := range r.ReadDaysByPriority {
		p.ReadTTLByPriority[prio] = time.Duration(days) * day
	}
	return p
}

// PushSubject resolves the VAPID subject: the configured one, else baseURL
// when it is https, else "".
func (s ServerSettings) PushSubject(baseURL string) string {
	if s.Push.Subject != "" {
		return strings.TrimPrefix(s.Push.Subject, "mailto:")
	}
	if strings.HasPrefix(strings.ToLower(baseURL), "https://") {
		return strings.TrimRight(baseURL, "/")
	}
	return ""
}

// Normalize fills nil collections so stored and returned values look alike.
func (s *ServerSettings) Normalize() {
	if s.Retention.ReadDaysByPriority == nil {
		s.Retention.ReadDaysByPriority = map[int]int{}
	}
	s.Push.Subject = strings.TrimSpace(s.Push.Subject)
}

// Validate checks the settings are usable.
func (s ServerSettings) Validate() error {
	var errs []error
	inRange := func(name string, v, lo, hi int) {
		if v < lo || v > hi {
			errs = append(errs, fmt.Errorf("%s must be between %d and %d", name, lo, hi))
		}
	}
	inRange("publish limit per token", s.PublishLimit.PerToken, 0, MaxPublishLimit)
	inRange("publish limit per user", s.PublishLimit.PerUser, 0, MaxPublishLimit)
	if sub := s.Push.Subject; sub != "" && !strings.HasPrefix(sub, "https://") && !strings.Contains(sub, "@") {
		errs = append(errs, errors.New("push subject must be an https URL or an e-mail address"))
	}
	inRange("failed webhook retention days", s.Webhooks.FailedRetentionDays, MinRetentionDays, MaxEventRetention)
	r := s.Retention
	inRange("read retention days", r.ReadDays, 0, MaxRetentionDays)
	inRange("unread retention days", r.UnreadDays, 0, MaxRetentionDays)
	for p, d := range r.ReadDaysByPriority {
		if p < PriorityMin || p > PriorityMax {
			errs = append(errs, fmt.Errorf("retention by priority: priority %d must be %d-%d", p, PriorityMin, PriorityMax))
		}
		inRange(fmt.Sprintf("read retention days for priority %d", p), d, 0, MaxRetentionDays)
	}
	if r.MaxPerUser != 0 {
		inRange("max notifications per user", r.MaxPerUser, MinMaxPerUser, MaxTopicMaxCount)
	}
	inRange("session days", s.SessionDays, MinSessionDays, MaxSessionDays)
	inRange("event retention days", s.EventRetentionDays, MinRetentionDays, MaxEventRetention)
	if err := errors.Join(errs...); err != nil {
		return Invalid("%s", err.Error())
	}
	return nil
}
