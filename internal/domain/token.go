package domain

import "time"

// APIToken is a scoped credential for scripts and integrations.
// The secret itself is never stored, only its hash.
type APIToken struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	SecretHash string     `json:"-"`
	Scopes     Scopes     `json:"scopes"`
	TopicIDs   []string   `json:"topic_ids"` // empty = all topics
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// Expired reports whether the token is expired at now.
func (t *APIToken) Expired(now time.Time) bool {
	return t.ExpiresAt != nil && !now.Before(*t.ExpiresAt)
}

// Session is an interactive login session. ID is the hash of the cookie value.
type Session struct {
	ID         string
	UserID     string
	UserAgent  string
	CreatedAt  time.Time
	ExpiresAt  time.Time
	LastSeenAt time.Time
}
