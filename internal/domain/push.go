package domain

import (
	"net/url"
	"strings"
	"time"
)

// PushSubscription is a browser's Web Push subscription for one user. The
// endpoint and keys are secrets: together they allow sending to the device,
// so the API exposes only the push service host.
type PushSubscription struct {
	ID            string     `json:"id"`
	UserID        string     `json:"-"`
	Endpoint      string     `json:"-"`
	P256dh        string     `json:"-"`
	Auth          string     `json:"-"`
	Name          string     `json:"name"`
	MinPriority   int        `json:"min_priority"`
	CreatedAt     time.Time  `json:"created_at"`
	LastSuccessAt *time.Time `json:"last_success_at"`
	Failures      int        `json:"failures"`
	// Service is the push service host (from Endpoint), for display.
	Service string `json:"push_service"`
}

// PushServiceHost is the host of the push service an endpoint belongs to,
// e.g. fcm.googleapis.com.
func PushServiceHost(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// Limits for subscriptions.
const (
	MaxPushNameLen     = 64
	MaxPushEndpointLen = 2048
	MaxPushKeyLen      = 256
	// MaxPushSubscriptions per user bounds fan-out per notification.
	MaxPushSubscriptions = 20
)

// ValidatePushSubscription checks client-supplied subscription fields.
func ValidatePushSubscription(s *PushSubscription) error {
	s.Name = strings.TrimSpace(s.Name)
	u, err := url.Parse(s.Endpoint)
	switch {
	case err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil:
		return Invalid("endpoint must be an https URL")
	case len(s.Endpoint) > MaxPushEndpointLen:
		return Invalid("endpoint is too long")
	case s.P256dh == "" || s.Auth == "" || len(s.P256dh) > MaxPushKeyLen || len(s.Auth) > MaxPushKeyLen:
		return Invalid("keys.p256dh and keys.auth are required")
	case len([]rune(s.Name)) > MaxPushNameLen:
		return Invalid("name must be at most %d characters", MaxPushNameLen)
	case s.MinPriority < PriorityMin || s.MinPriority > PriorityMax:
		return Invalid("min_priority must be between %d and %d", PriorityMin, PriorityMax)
	}
	return nil
}
