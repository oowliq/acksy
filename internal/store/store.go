package store

import (
	"context"
	"time"

	"github.com/oowliq/acksy/internal/domain"
)

// Querier is every persistence operation. Implementations return domain
// errors (domain.ErrNotFound, domain.ErrConflict) for expected failures.
type Querier interface {
	// Users.
	CreateUser(ctx context.Context, u *domain.User) error
	GetUser(ctx context.Context, id string) (*domain.User, error)
	GetUserByUsername(ctx context.Context, username string) (*domain.User, error)
	ListUsers(ctx context.Context) ([]domain.User, error)
	CountUsers(ctx context.Context) (int, error)
	CountActiveAdmins(ctx context.Context) (int, error)
	UpdateUser(ctx context.Context, u *domain.User) error
	DeleteUser(ctx context.Context, id string) error
	// NextEventSeq atomically increments and returns the user's event sequence.
	NextEventSeq(ctx context.Context, userID string) (int64, error)

	// External identities (OIDC).
	CreateIdentity(ctx context.Context, i *domain.Identity) error
	GetIdentity(ctx context.Context, issuer, subject string) (*domain.Identity, error)
	ListUserIdentities(ctx context.Context, userID string) ([]domain.Identity, error)
	TouchIdentity(ctx context.Context, issuer, subject string, at time.Time) error
	DeleteIdentity(ctx context.Context, userID, issuer, subject string) error

	// Sessions.
	CreateSession(ctx context.Context, s *domain.Session) error
	GetSession(ctx context.Context, id string) (*domain.Session, error)
	TouchSession(ctx context.Context, id string, lastSeen, expiresAt time.Time) error
	DeleteSession(ctx context.Context, id string) error
	DeleteUserSessions(ctx context.Context, userID string) error
	DeleteExpiredSessions(ctx context.Context, now time.Time) (int64, error)

	// API tokens.
	CreateToken(ctx context.Context, t *domain.APIToken) error
	GetTokenByHash(ctx context.Context, hash string) (*domain.APIToken, error)
	ListTokens(ctx context.Context, userID string) ([]domain.APIToken, error)
	DeleteToken(ctx context.Context, userID, id string) error
	TouchToken(ctx context.Context, id string, at time.Time) error

	// Topics.
	CreateTopic(ctx context.Context, t *domain.Topic) error
	GetTopic(ctx context.Context, id string) (*domain.Topic, error)
	GetTopicByOwnerAndName(ctx context.Context, ownerID, name string) (*domain.Topic, error)
	// FindMemberTopicsByName returns topics named name the user is a member of.
	FindMemberTopicsByName(ctx context.Context, userID, name string) ([]domain.Topic, error)
	ListTopicsForUser(ctx context.Context, userID string) ([]domain.Topic, error)
	UpdateTopic(ctx context.Context, t *domain.Topic) error
	DeleteTopic(ctx context.Context, id string) error
	GetMemberRole(ctx context.Context, topicID, userID string) (domain.MemberRole, error)
	AddTopicMember(ctx context.Context, topicID, userID string, role domain.MemberRole, at time.Time) error
	RemoveTopicMember(ctx context.Context, topicID, userID string) error
	ListTopicMembers(ctx context.Context, topicID string) ([]domain.TopicMember, error)

	// Topic invitations (pending until the invitee accepts).
	CreateTopicInvite(ctx context.Context, topicID, userID string, at time.Time) error
	GetTopicInvite(ctx context.Context, topicID, userID string) (*domain.TopicInvite, error)
	ListTopicInvites(ctx context.Context, topicID string) ([]domain.TopicInvite, error)
	ListUserInvites(ctx context.Context, userID string) ([]domain.TopicInvite, error)
	DeleteTopicInvite(ctx context.Context, topicID, userID string) error

	// ListRecipientIDs returns IDs of enabled users that receive the topic.
	ListRecipientIDs(ctx context.Context, topicID string) ([]string, error)

	// Notifications.
	// UpsertNotification inserts n, or — when n.DedupKey matches an existing
	// notification of the same user — updates that one in place (bumping
	// repeat_count and resetting read state). n.ID is set to the stored ID.
	UpsertNotification(ctx context.Context, n *domain.Notification) (deduped bool, err error)
	GetNotification(ctx context.Context, userID, id string) (*domain.Notification, error)
	ListNotifications(ctx context.Context, f domain.NotificationFilter) ([]domain.Notification, error)
	ListNotificationIDs(ctx context.Context, f domain.NotificationFilter) ([]string, error)
	SetNotificationState(ctx context.Context, n *domain.Notification) error
	BulkUpdateNotifications(ctx context.Context, userID string, ids []string, action domain.BulkAction, at time.Time) error
	Counts(ctx context.Context, userID string, topicIDs []string, now time.Time) (domain.Counts, error)
	// ListNotificationRefsByKey returns every recipient's notification in the
	// topic carrying dedupKey.
	ListNotificationRefsByKey(ctx context.Context, topicID, dedupKey string) ([]domain.NotificationRef, error)

	// Web Push subscriptions.
	UpsertPushSubscription(ctx context.Context, s *domain.PushSubscription) error
	GetPushSubscription(ctx context.Context, userID, id string) (*domain.PushSubscription, error)
	ListPushSubscriptions(ctx context.Context, userID string) ([]domain.PushSubscription, error)
	CountPushSubscriptions(ctx context.Context, userID string) (int, error)
	// ListPushTargets returns the user's subscriptions whose min_priority
	// is at most priority.
	ListPushTargets(ctx context.Context, userID string, priority int) ([]domain.PushSubscription, error)
	UpdatePushSubscription(ctx context.Context, s *domain.PushSubscription) error
	DeletePushSubscription(ctx context.Context, userID, id string) error
	// DeletePushSubscriptionByID drops a subscription the push service
	// reported as gone; deleting a missing one is not an error.
	DeletePushSubscriptionByID(ctx context.Context, id string) error
	// RecordPushResult stamps last_success_at (ok) or counts a failure.
	RecordPushResult(ctx context.Context, id string, ok bool, at time.Time) error

	// Webhooks.
	CreateWebhook(ctx context.Context, w *domain.Webhook) error
	GetWebhook(ctx context.Context, userID, id string) (*domain.Webhook, error)
	GetWebhookByID(ctx context.Context, id string) (*domain.Webhook, error)
	ListWebhooks(ctx context.Context, userID string) ([]domain.Webhook, error)
	ListEnabledWebhooks(ctx context.Context, userID string) ([]domain.Webhook, error)
	CountWebhooks(ctx context.Context, userID string) (int, error)
	// UpdateWebhook saves all fields and replaces the topic filter.
	UpdateWebhook(ctx context.Context, w *domain.Webhook) error
	DeleteWebhook(ctx context.Context, userID, id string) error
	// RecordWebhookResult stamps last_success_at, or last_failure_at with errMsg.
	RecordWebhookResult(ctx context.Context, id string, ok bool, at time.Time, errMsg string) error

	// Webhook delivery queue.
	EnqueueWebhookDelivery(ctx context.Context, d *domain.WebhookDelivery) error
	// ListDueWebhookDeliveries returns pending deliveries whose time has come.
	ListDueWebhookDeliveries(ctx context.Context, now time.Time, limit int) ([]domain.WebhookDelivery, error)
	// ClaimWebhookDelivery takes a due delivery for this worker: it moves
	// next_attempt_at to leaseUntil and counts the attempt. Only one of
	// several concurrent callers gets true.
	ClaimWebhookDelivery(ctx context.Context, id string, now, leaseUntil time.Time) (bool, error)
	DeleteWebhookDelivery(ctx context.Context, id string) error
	// RescheduleWebhookDelivery stores the outcome of a failed attempt:
	// NextAttempt for a retry, or FailedAt (with NextAttempt nil) to give up.
	RescheduleWebhookDelivery(ctx context.Context, d *domain.WebhookDelivery) error
	// ListWebhookDeliveries returns the newest pending or failed deliveries.
	ListWebhookDeliveries(ctx context.Context, webhookID string, limit int) ([]domain.WebhookDelivery, error)
	DeleteFailedWebhookDeliveries(ctx context.Context, before time.Time) (int64, error)

	// Retention.
	// ListRetentionTopics returns topics with their own retention_days or
	// max_notifications (only ID and those fields are set).
	ListRetentionTopics(ctx context.Context) ([]domain.Topic, error)
	// ListExpiredNotificationRefs returns up to limit notifications matching e.
	ListExpiredNotificationRefs(ctx context.Context, e domain.ExpiryQuery, limit int) ([]domain.NotificationRef, error)
	// ListUsersOverNotificationLimit returns users with more than maxCount
	// notifications (in topicID, or in total when topicID is "").
	ListUsersOverNotificationLimit(ctx context.Context, topicID string, maxCount int) ([]string, error)
	// ListOverflowNotificationRefs returns up to limit of the user's
	// notifications beyond the keep newest (in topicID, or in total), read
	// ones first, oldest first.
	ListOverflowNotificationRefs(ctx context.Context, userID, topicID string, keep, limit int) ([]domain.NotificationRef, error)

	// Instance settings (generated keys and the like).
	GetSetting(ctx context.Context, key string) (string, error)
	GetSettingWithTime(ctx context.Context, key string) (string, time.Time, error)
	PutSetting(ctx context.Context, key, value string, at time.Time) error
	// InsertSettingIfAbsent stores value unless key exists (first writer wins).
	InsertSettingIfAbsent(ctx context.Context, key, value string, at time.Time) error

	// Events.
	InsertEvent(ctx context.Context, e *domain.Event) error
	ListEvents(ctx context.Context, userID string, since int64, limit int) ([]domain.Event, error)
	// EventSeqRange returns the user's current sequence and the oldest
	// retained event sequence (0 when no events are retained).
	EventSeqRange(ctx context.Context, userID string) (current, oldest int64, err error)
	// DeleteEventsBefore prunes events created before cutoff.
	DeleteEventsBefore(ctx context.Context, cutoff time.Time) (int64, error)
}

// Store is a Querier that can also run a function inside a transaction.
type Store interface {
	Querier
	// InTx runs fn in a transaction, committing if fn returns nil. fn must
	// use only the Querier it is given.
	InTx(ctx context.Context, fn func(q Querier) error) error
}
