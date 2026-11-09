package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/oowliq/acksy/internal/domain"
	"github.com/oowliq/acksy/internal/markdown"
)

// Request is a rendered webhook request.
type Request struct {
	Body    []byte
	Headers map[string]string
}

// Render builds the request for one delivery: the body in the webhook's
// format and the Acksy headers, including the HMAC signature.
func Render(w *domain.Webhook, event domain.WebhookEvent, deliveryID string, n *domain.Notification, baseURL string, now time.Time) (Request, error) {
	var (
		body []byte
		err  error
	)
	switch w.Format {
	case domain.WebhookDiscord:
		body, err = json.Marshal(discordBody(n, baseURL))
	case domain.WebhookSlack:
		body, err = json.Marshal(slackBody(n))
	default:
		body, err = json.Marshal(acksyBody(event, deliveryID, n, baseURL, now))
	}
	if err != nil {
		return Request{}, fmt.Errorf("encode webhook body: %w", err)
	}
	ts := strconv.FormatInt(now.Unix(), 10)
	return Request{Body: body, Headers: map[string]string{
		"Content-Type":      "application/json",
		"X-Acksy-Event":     string(event),
		"X-Acksy-Delivery":  deliveryID,
		"X-Acksy-Timestamp": ts,
		"X-Acksy-Signature": Sign(w.Secret, ts, body),
	}}, nil
}

// Sign returns "sha256=<hex HMAC-SHA256(secret, timestamp + "." + body)>".
// Receivers recompute it and reject old timestamps to stop replays.
func Sign(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func appURL(baseURL, id string) string {
	if baseURL == "" {
		return ""
	}
	return strings.TrimRight(baseURL, "/") + "/?n=" + id
}

func displayTitle(n *domain.Notification) string {
	switch {
	case n.Title != "":
		return n.Title
	case n.Topic != nil:
		return n.Topic.Name
	default:
		return "Acksy"
	}
}

// footer is "topic · P4" (parts omitted when absent or default).
func footer(n *domain.Notification) string {
	var parts []string
	if n.Topic != nil {
		parts = append(parts, n.Topic.Name)
	}
	parts = append(parts, fmt.Sprintf("P%d", n.Priority))
	if n.RepeatCount > 1 {
		parts = append(parts, fmt.Sprintf("×%d", n.RepeatCount))
	}
	return strings.Join(parts, " · ")
}

// Acksy JSON.

type acksyNotification struct {
	*domain.Notification
	BodyHTML string `json:"body_html"`
	BodyText string `json:"body_text"`
	URL      string `json:"url,omitempty"`
	// Hidden from the embedded struct: read state belongs to the user's
	// client, not to integrations.
	ReadAt       *struct{} `json:"read_at,omitempty"`
	ArchivedAt   *struct{} `json:"archived_at,omitempty"`
	SnoozedUntil *struct{} `json:"snoozed_until,omitempty"`
}

type acksyPayload struct {
	Event        domain.WebhookEvent `json:"event"`
	DeliveryID   string              `json:"delivery_id"`
	Timestamp    time.Time           `json:"timestamp"`
	Notification acksyNotification   `json:"notification"`
}

func acksyBody(event domain.WebhookEvent, deliveryID string, n *domain.Notification, baseURL string, now time.Time) acksyPayload {
	return acksyPayload{
		Event: event, DeliveryID: deliveryID, Timestamp: now.UTC(),
		Notification: acksyNotification{
			Notification: n, BodyHTML: markdown.HTML(n.Body), BodyText: markdown.Plain(n.Body), URL: appURL(baseURL, n.ID),
		},
	}
}

// Discord (https://discord.com/developers/docs/resources/webhook).

type discordEmbed struct {
	Title       string         `json:"title"`
	Description string         `json:"description,omitempty"`
	URL         string         `json:"url,omitempty"`
	Color       int            `json:"color"`
	Timestamp   string         `json:"timestamp"`
	Footer      map[string]any `json:"footer"`
}

type discordPayload struct {
	Username string         `json:"username"`
	Embeds   []discordEmbed `json:"embeds"`
	// No pings: notification text must not be able to @everyone a server.
	AllowedMentions map[string][]string `json:"allowed_mentions"`
}

// Priority colours (Catppuccin Mocha): red is kept for urgent, as in the UI.
var discordColors = map[int]int{5: 0xf38ba8, 4: 0xfab387, 3: 0xb4befe, 2: 0x6c7086, 1: 0x6c7086}

func discordBody(n *domain.Notification, baseURL string) discordPayload {
	link := n.ClickURL
	if link == "" {
		link = appURL(baseURL, n.ID)
	}
	return discordPayload{
		Username: "Acksy",
		Embeds: []discordEmbed{{
			Title: markdown.Truncate(displayTitle(n), 256),
			// Discord renders markdown itself; its limit is 4096.
			Description: markdown.Truncate(n.Body, 4000),
			URL:         link, Color: discordColors[n.Priority],
			Timestamp: n.UpdatedAt.UTC().Format(time.RFC3339),
			Footer:    map[string]any{"text": footer(n)},
		}},
		AllowedMentions: map[string][]string{"parse": {}},
	}
}

// Slack (Block Kit). Plain text only: Slack's mrkdwn differs from markdown
// and would turn <!channel> in a notification into a mention.

type slackText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type slackBlock struct {
	Type     string      `json:"type"`
	Text     *slackText  `json:"text,omitempty"`
	Elements []slackText `json:"elements,omitempty"`
}

type slackPayload struct {
	Text   string       `json:"text"` // fallback for notifications
	Blocks []slackBlock `json:"blocks"`
}

// escapeSlack escapes the characters Slack treats as control sequences in
// the top-level (mrkdwn) text field.
func escapeSlack(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

func slackBody(n *domain.Notification) slackPayload {
	title := displayTitle(n)
	text := markdown.Plain(n.Body)
	blocks := []slackBlock{{Type: "header", Text: &slackText{Type: "plain_text", Text: markdown.Truncate(title, 150)}}}
	if text != "" {
		blocks = append(blocks, slackBlock{Type: "section", Text: &slackText{Type: "plain_text", Text: markdown.Truncate(text, 3000)}})
	}
	blocks = append(blocks, slackBlock{Type: "context", Elements: []slackText{{Type: "plain_text", Text: footer(n)}}})
	fallback := title
	if text != "" {
		fallback += ": " + text
	}
	return slackPayload{Text: escapeSlack(markdown.Truncate(fallback, 3000)), Blocks: blocks}
}
