package domain

import (
	"regexp"
	"strings"
	"time"
)

// MemberRole is a user's role within a topic.
type MemberRole string

const (
	MemberOwner  MemberRole = "owner"
	MemberMember MemberRole = "member"
)

// Topic is an optional category for notifications, owned by a user and
// optionally shared with other users (members).
type Topic struct {
	ID              string `json:"id"`
	OwnerID         string `json:"owner_id"`
	OwnerUsername   string `json:"owner_username"`
	Name            string `json:"name"`
	Title           string `json:"title"`
	Description     string `json:"description"`
	DefaultPriority *int   `json:"default_priority,omitempty"`
	// RetentionDays and MaxNotifications override the instance retention
	// for this topic's notifications (nil: inherit).
	RetentionDays    *int       `json:"retention_days"`
	MaxNotifications *int       `json:"max_notifications"`
	Role             MemberRole `json:"role,omitempty"` // caller's role, when listed for a user
	MemberCount      int        `json:"member_count"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// TopicMember is a user subscribed to a topic.
type TopicMember struct {
	UserID    string     `json:"user_id"`
	Username  string     `json:"username"`
	Role      MemberRole `json:"role"`
	CreatedAt time.Time  `json:"created_at"`
}

var topicNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// NormalizeTopicName lowercases and trims a topic name.
func NormalizeTopicName(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// ValidateTopicName checks a normalized topic name.
func ValidateTopicName(s string) error {
	if !topicNameRe.MatchString(s) {
		return Invalid("topic name must be 1-64 characters: a-z, 0-9, '_', '-' and start with a letter or digit")
	}
	return nil
}

// TopicRef is how callers address a topic: "name" (own or uniquely shared)
// or "owner/name".
type TopicRef struct {
	Owner string // username; empty when unqualified
	Name  string
}

// ParseTopicRef parses "name" or "owner/name".
func ParseTopicRef(s string) (TopicRef, error) {
	s = strings.TrimSpace(s)
	owner, name, qualified := strings.Cut(s, "/")
	if !qualified {
		name, owner = owner, ""
	}
	ref := TopicRef{Owner: NormalizeUsername(owner), Name: NormalizeTopicName(name)}
	if qualified {
		if err := ValidateUsername(ref.Owner); err != nil {
			return TopicRef{}, Invalid("invalid topic reference %q", s)
		}
	}
	if err := ValidateTopicName(ref.Name); err != nil {
		return TopicRef{}, err
	}
	return ref, nil
}

// TopicInvite is a pending invitation of a user to a shared topic. The user
// becomes a member (and starts receiving its notifications) only by
// accepting it.
type TopicInvite struct {
	TopicID       string    `json:"topic_id"`
	TopicName     string    `json:"topic_name"`
	TopicTitle    string    `json:"topic_title"`
	OwnerUsername string    `json:"owner_username"`
	UserID        string    `json:"user_id"`
	Username      string    `json:"username"`
	CreatedAt     time.Time `json:"created_at"`
}
