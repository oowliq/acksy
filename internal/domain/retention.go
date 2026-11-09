package domain

import "time"

// RetentionPolicy is the instance-wide notification retention. Zero values
// disable a rule. Topics may override ReadTTL and add a count limit.
type RetentionPolicy struct {
	// ReadTTL deletes read (and archived) notifications this long after their
	// last change.
	ReadTTL time.Duration
	// ReadTTLByPriority overrides ReadTTL per priority (e.g. P1-2 shorter).
	ReadTTLByPriority map[int]time.Duration
	// UnreadTTL deletes unread notifications this long after their last
	// change. Off by default: unread notifications never vanish silently.
	UnreadTTL time.Duration
	// MaxPerUser caps each user's notifications; the oldest read ones go
	// first, unread ones only when there are not enough read ones.
	MaxPerUser int
}

// ReadTTLFor returns the read TTL for a priority.
func (p RetentionPolicy) ReadTTLFor(priority int) time.Duration {
	if ttl, ok := p.ReadTTLByPriority[priority]; ok {
		return ttl
	}
	return p.ReadTTL
}

// ExpiredGrace is how long a notification past its expires_at (already
// hidden) is kept before retention deletes it.
const ExpiredGrace = 24 * time.Hour

// Topic retention bounds.
const (
	MinTopicRetentionDays = 1
	MaxTopicRetentionDays = 3650
	MinTopicMaxCount      = 10
	MaxTopicMaxCount      = 1_000_000
)

// ValidateTopicRetention checks topic overrides; nil means inherit.
func ValidateTopicRetention(days, maxCount *int) error {
	if days != nil && (*days < MinTopicRetentionDays || *days > MaxTopicRetentionDays) {
		return Invalid("retention_days must be between %d and %d", MinTopicRetentionDays, MaxTopicRetentionDays)
	}
	if maxCount != nil && (*maxCount < MinTopicMaxCount || *maxCount > MaxTopicMaxCount) {
		return Invalid("max_notifications must be between %d and %d", MinTopicMaxCount, MaxTopicMaxCount)
	}
	return nil
}

// ReadState selects notifications by read state.
type ReadState int

const (
	AnyState ReadState = iota
	ReadOnly
	UnreadOnly
)

// ExpiryQuery selects notifications for retention deletion. All set
// conditions combine with AND.
type ExpiryQuery struct {
	State ReadState
	// UpdatedBefore matches updated_at < UpdatedBefore (zero: no condition).
	UpdatedBefore time.Time
	// ExpiresBefore matches expires_at < ExpiresBefore (zero: no condition).
	ExpiresBefore time.Time
	Priority      int    // 0: any
	TopicID       string // "": any, including none
	// SkipTopicOverrides leaves out topics with their own retention_days.
	SkipTopicOverrides bool
}
