package domain

import (
	"encoding/json"
	"time"
)

// EventType names a change delivered to a user's clients.
type EventType string

const (
	EventNotificationCreated EventType = "notification.created"
	EventNotificationUpdated EventType = "notification.updated" // content or state
	EventNotificationDeleted EventType = "notification.deleted"
	EventNotificationsBulk   EventType = "notifications.bulk"
	EventTopicChanged        EventType = "topic.changed"
	EventTopicRemoved        EventType = "topic.removed"
	// EventInvitationsChanged: a topic invitation of the user was created,
	// accepted, declined or revoked (sent to the invitee and the topic owner).
	EventInvitationsChanged EventType = "invitations.changed"
)

// Event is a per-user, monotonically sequenced change record. It is written in
// the same transaction as the change it describes.
type Event struct {
	UserID    string          `json:"-"`
	Seq       int64           `json:"seq"`
	Type      EventType       `json:"type"`
	Data      json.RawMessage `json:"data"`
	CreatedAt time.Time       `json:"created_at"`
}

// BulkEventData describes a bulk read-state change.
type BulkEventData struct {
	Action BulkAction `json:"action"`
	IDs    []string   `json:"ids"`
	At     time.Time  `json:"at"`
}

// IDEventData carries just an entity ID.
type IDEventData struct {
	ID string `json:"id"`
}
