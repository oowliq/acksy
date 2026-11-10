package domain

import (
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
)

// WebhookFormat is the request body a webhook sends.
type WebhookFormat string

const (
	// WebhookAcksy is Acksy's JSON (for Home Assistant, n8n, own services).
	WebhookAcksy WebhookFormat = "acksy"
	// WebhookDiscord posts a message to a Discord incoming webhook.
	WebhookDiscord WebhookFormat = "discord"
	// WebhookSlack posts a message to a Slack incoming webhook.
	WebhookSlack    WebhookFormat = "slack"
	WebhookTelegram WebhookFormat = "telegram"
)

// Valid reports whether f is a known format.
func (f WebhookFormat) Valid() bool {
	return f == WebhookAcksy || f == WebhookDiscord || f == WebhookSlack || f == WebhookTelegram
}

// Webhook forwards a user's notifications to a URL. Secret signs each
// request (HMAC-SHA256); it is kept in clear because signing needs it, and
// the API shows it only on creation and rotation.
type Webhook struct {
	ID     string        `json:"id"`
	UserID string        `json:"-"`
	Name   string        `json:"name"`
	URL    string        `json:"-"`
	Format WebhookFormat `json:"format"`
	Secret string        `json:"-"`
	// TopicIDs limits the webhook to these topics; empty means every
	// notification, including those without a topic.
	TopicIDs    []string   `json:"topic_ids"`
	MinPriority int        `json:"min_priority"`
	Enabled     bool       `json:"enabled"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	LastSuccess *time.Time `json:"last_success_at"`
	LastFailure *time.Time `json:"last_failure_at"`
	LastError   string     `json:"last_error,omitempty"`
}

// Matches reports whether n should be delivered by this webhook.
func (w *Webhook) Matches(n *Notification) bool {
	if !w.Enabled || n.Priority < w.MinPriority {
		return false
	}
	if len(w.TopicIDs) == 0 {
		return true
	}
	return n.Topic != nil && slices.Contains(w.TopicIDs, n.Topic.ID)
}

// URLDisplay shows where the webhook points without its secret parts:
// Discord and Slack put the credential in the path, others may use the
// query or userinfo.
func (w *Webhook) URLDisplay() string {
	u, err := url.Parse(w.URL)
	if err != nil {
		return ""
	}
	return u.Scheme + "://" + u.Host + "/…"
}

// Limits for webhooks.
const (
	MaxWebhooks       = 20
	MaxWebhookNameLen = 64
	MaxWebhookTopics  = 32
)

// Telegram webhooks store the destination in the existing write-only URL.
var telegramDestination = regexp.MustCompile(`^(https://api\.telegram\.org/bot[0-9]+:[A-Za-z0-9_-]+/sendMessage)\?chat_id=(-?[1-9][0-9]*|@[a-zA-Z0-9_]+)$`)

func TelegramDestination(raw string) (endpoint, chatID string, err error) {
	parts := telegramDestination.FindStringSubmatch(raw)
	if parts == nil {
		return "", "", Invalid("expected a Telegram bot sendMessage URL with a chat_id")
	}
	return parts[1], parts[2], nil
}

// ValidateWebhook checks user-supplied fields (topics are checked by the
// service, which resolves them).
func ValidateWebhook(w *Webhook) error {
	w.Name = strings.TrimSpace(w.Name)
	w.URL = strings.TrimSpace(w.URL)
	u, err := url.Parse(w.URL)
	switch {
	case w.Name == "" || len([]rune(w.Name)) > MaxWebhookNameLen:
		return Invalid("name must be 1-%d characters", MaxWebhookNameLen)
	case err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "":
		return Invalid("url must be an http(s) URL")
	case len(w.URL) > MaxURLLen:
		return Invalid("url is too long")
	case !w.Format.Valid():
		return Invalid("format must be acksy, discord, slack or telegram")
	case w.MinPriority < PriorityMin || w.MinPriority > PriorityMax:
		return Invalid("min_priority must be between %d and %d", PriorityMin, PriorityMax)
	case len(w.TopicIDs) > MaxWebhookTopics:
		return Invalid("at most %d topics per webhook", MaxWebhookTopics)
	}
	if w.Format == WebhookTelegram {
		_, _, err := TelegramDestination(w.URL)
		return err
	}
	return nil
}

// WebhookEvent names what happened to a notification.
type WebhookEvent string

const (
	WebhookCreated WebhookEvent = "notification.created"
	// WebhookUpdated is a dedup repeat that raised an existing notification.
	WebhookUpdated WebhookEvent = "notification.updated"
	// WebhookTest is sent by the test endpoint.
	WebhookTest WebhookEvent = "webhook.test"
)

// WebhookDelivery is one queued request. Payload is the notification as it
// was when published (JSON), so later edits do not change what is sent.
type WebhookDelivery struct {
	ID          string       `json:"id"`
	WebhookID   string       `json:"webhook_id"`
	Event       WebhookEvent `json:"event"`
	Payload     string       `json:"-"`
	Attempts    int          `json:"attempts"`
	NextAttempt *time.Time   `json:"next_attempt_at"` // nil once given up
	LastStatus  int          `json:"last_status,omitempty"`
	LastError   string       `json:"last_error,omitempty"`
	CreatedAt   time.Time    `json:"created_at"`
	FailedAt    *time.Time   `json:"failed_at,omitempty"`
}

// WebhookRetry is the wait before attempt n+1 after n failed attempts;
// MaxWebhookAttempts bounds the schedule (about 11 hours in total).
var WebhookRetry = []time.Duration{
	30 * time.Second, 2 * time.Minute, 10 * time.Minute, 30 * time.Minute,
	time.Hour, 3 * time.Hour, 6 * time.Hour,
}

// MaxWebhookAttempts is the number of attempts before a delivery fails.
var MaxWebhookAttempts = len(WebhookRetry) + 1
