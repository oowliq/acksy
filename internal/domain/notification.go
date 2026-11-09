package domain

import (
	"encoding/json"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// Priority bounds and default.
const (
	PriorityMin     = 1
	PriorityDefault = 3
	PriorityMax     = 5
)

// Limits for notification content.
const (
	MaxTitleLen    = 256
	MaxBodyLen     = 64 * 1024
	MaxTags        = 16
	MaxTagLen      = 64
	MaxActions     = 4
	MaxKeyLen      = 256
	MaxURLLen      = 2048
	MaxActionLabel = 64
)

// TopicSummary is the topic embedded in a notification.
type TopicSummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Action is a button attached to a notification: "view" opens URL in the
// client, "http" makes the server send a request when a recipient presses
// it (see ActionView/ActionHTTP).
//
// Headers and Body of an http action often hold credentials, so they are
// never part of the public JSON (API, realtime events, webhooks) and the
// URL is shown without query and userinfo; see MarshalJSON. They are
// accepted on input (UnmarshalJSON) and stored via ActionsForStorage.
type Action struct {
	Type    string
	Label   string
	URL     string
	Method  string            // http: GET, POST, PUT, PATCH, DELETE (default POST)
	Headers map[string]string // http only, secret
	Body    string            // http only, secret
	Clear   bool              // http: mark the notification read on success
}

// Action types.
const (
	ActionView = "view"
	ActionHTTP = "http"
)

// actionWire is the full shape of an action, secrets included: what
// publishers send and what the store keeps.
type actionWire struct {
	Type    string            `json:"type"`
	Label   string            `json:"label"`
	URL     string            `json:"url"`
	Method  string            `json:"method,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"`
	Clear   bool              `json:"clear,omitempty"`
}

// UnmarshalJSON accepts the full action, secrets included.
func (a *Action) UnmarshalJSON(b []byte) error {
	var w actionWire
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	*a = Action(w)
	return nil
}

// actionPublic is what recipients and integrations see.
type actionPublic struct {
	Type   string `json:"type"`
	Label  string `json:"label"`
	URL    string `json:"url"`
	Method string `json:"method,omitempty"`
	Clear  bool   `json:"clear,omitempty"`
}

// MarshalJSON hides an http action's headers, body and URL query.
func (a Action) MarshalJSON() ([]byte, error) {
	p := actionPublic{Type: a.Type, Label: a.Label, URL: a.URL}
	if a.Type == ActionHTTP {
		p.Method, p.Clear, p.URL = a.Method, a.Clear, publicURL(a.URL)
	}
	return json.Marshal(p)
}

// publicURL drops userinfo, query and fragment, which may carry secrets.
func publicURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	u.User, u.RawQuery, u.ForceQuery, u.Fragment = nil, "", false, ""
	return u.String()
}

// ActionsForStorage encodes actions with their secrets, for the store only.
func ActionsForStorage(actions []Action) ([]byte, error) {
	w := make([]actionWire, len(actions))
	for i, a := range actions {
		w[i] = actionWire(a)
	}
	return json.Marshal(w)
}

// Notification belongs to exactly one user. A message published to a shared
// topic produces one Notification per recipient, sharing MessageID.
type Notification struct {
	ID           string        `json:"id"`
	UserID       string        `json:"user_id"`
	MessageID    string        `json:"message_id"`
	Topic        *TopicSummary `json:"topic"`
	Title        string        `json:"title"`
	Body         string        `json:"body"`
	Priority     int           `json:"priority"`
	Tags         []string      `json:"tags"`
	ClickURL     string        `json:"click_url,omitempty"`
	Actions      []Action      `json:"actions"`
	GroupKey     string        `json:"group_key,omitempty"`
	DedupKey     string        `json:"dedup_key,omitempty"`
	RepeatCount  int           `json:"repeat_count"`
	Source       string        `json:"source"`
	TokenID      string        `json:"-"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
	ExpiresAt    *time.Time    `json:"expires_at,omitempty"`
	ReadAt       *time.Time    `json:"read_at"`
	ArchivedAt   *time.Time    `json:"archived_at"`
	SnoozedUntil *time.Time    `json:"snoozed_until"`
}

// Message is a normalized incoming message, as produced by ingest adapters.
type Message struct {
	Topic     string // topic reference, optional
	Title     string
	Body      string
	Priority  int // 0 = topic default or PriorityDefault
	Tags      []string
	ClickURL  string
	Actions   []Action
	GroupKey  string
	DedupKey  string
	ExpiresAt *time.Time
	Source    string // adapter name
	// TopicFromToken publishes to the token's topic when Topic is empty and
	// the token is restricted to exactly one topic (gotify applications).
	TopicFromToken bool
}

// Normalize trims fields and validates the message in place.
func (m *Message) Normalize(now time.Time) error {
	m.Title = strings.TrimSpace(m.Title)
	m.Body = strings.TrimRight(m.Body, " \t\r\n")
	m.GroupKey = strings.TrimSpace(m.GroupKey)
	m.DedupKey = strings.TrimSpace(m.DedupKey)
	m.ClickURL = strings.TrimSpace(m.ClickURL)

	switch {
	case m.Title == "" && m.Body == "":
		return Invalid("title or body is required")
	case utf8.RuneCountInString(m.Title) > MaxTitleLen:
		return Invalid("title must be at most %d characters", MaxTitleLen)
	case len(m.Body) > MaxBodyLen:
		return Invalid("body must be at most %d bytes", MaxBodyLen)
	case m.Priority != 0 && (m.Priority < PriorityMin || m.Priority > PriorityMax):
		return Invalid("priority must be between %d and %d", PriorityMin, PriorityMax)
	case len(m.GroupKey) > MaxKeyLen || len(m.DedupKey) > MaxKeyLen:
		return Invalid("group_key and dedup_key must be at most %d bytes", MaxKeyLen)
	case m.ExpiresAt != nil && !m.ExpiresAt.After(now):
		return Invalid("expires_at must be in the future")
	}
	if m.ClickURL != "" {
		if err := validateURL(m.ClickURL); err != nil {
			return Invalid("click_url: %v", err)
		}
	}

	tags, err := normalizeTags(m.Tags)
	if err != nil {
		return err
	}
	m.Tags = tags

	if len(m.Actions) > MaxActions {
		return Invalid("at most %d actions are allowed", MaxActions)
	}
	for i := range m.Actions {
		if err := normalizeAction(&m.Actions[i]); err != nil {
			return err
		}
	}
	if m.Actions == nil {
		m.Actions = []Action{}
	}
	if m.Source == "" {
		m.Source = "acksy"
	}
	return nil
}

// HTTP action limits.
const (
	MaxActionHeaders = 10
	MaxActionBody    = 8 * 1024
)

var (
	actionMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE"}
	headerNameRe  = regexp.MustCompile("^[!#$%&'*+.^_`|~0-9A-Za-z-]+$")
	// Headers the server sets itself, or that could smuggle requests.
	reservedHeaders = []string{"host", "content-length", "transfer-encoding", "connection", "upgrade", "te", "trailer"}
)

func normalizeAction(a *Action) error {
	a.Label = strings.TrimSpace(a.Label)
	if a.Type == "" {
		a.Type = ActionView
	}
	if a.Label == "" || utf8.RuneCountInString(a.Label) > MaxActionLabel {
		return Invalid("action label must be 1-%d characters", MaxActionLabel)
	}
	if err := validateURL(a.URL); err != nil {
		return Invalid("action url: %v", err)
	}
	switch a.Type {
	case ActionView:
		if a.Method != "" || len(a.Headers) > 0 || a.Body != "" || a.Clear {
			return Invalid("view actions take only label and url")
		}
	case ActionHTTP:
		a.Method = strings.ToUpper(strings.TrimSpace(a.Method))
		if a.Method == "" {
			a.Method = "POST"
		}
		if !slices.Contains(actionMethods, a.Method) {
			return Invalid("action method must be one of %s", strings.Join(actionMethods, ", "))
		}
		if len(a.Headers) > MaxActionHeaders {
			return Invalid("at most %d action headers", MaxActionHeaders)
		}
		for k, v := range a.Headers {
			if !headerNameRe.MatchString(k) || slices.Contains(reservedHeaders, strings.ToLower(k)) {
				return Invalid("invalid action header %q", k)
			}
			if strings.ContainsAny(v, "\r\n") || len(v) > 4096 {
				return Invalid("invalid value for action header %q", k)
			}
		}
		if len(a.Body) > MaxActionBody {
			return Invalid("action body must be at most %d bytes", MaxActionBody)
		}
	default:
		return Invalid("action type must be view or http")
	}
	return nil
}

func normalizeTags(in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	seen := make(map[string]bool, len(in))
	for _, t := range in {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" || seen[t] {
			continue
		}
		if utf8.RuneCountInString(t) > MaxTagLen {
			return nil, Invalid("tags must be at most %d characters", MaxTagLen)
		}
		seen[t] = true
		out = append(out, t)
	}
	if len(out) > MaxTags {
		return nil, Invalid("at most %d tags are allowed", MaxTags)
	}
	return out, nil
}

type urlError string

func (e urlError) Error() string { return string(e) }

func validateURL(raw string) error {
	if len(raw) > MaxURLLen {
		return urlError("too long")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return urlError("must be an absolute URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return urlError("scheme must be http or https")
	}
	return nil
}

// View selects which notifications a list returns.
type View string

const (
	ViewInbox    View = "inbox"    // not archived, not snoozed
	ViewArchived View = "archived" // archived only
	ViewSnoozed  View = "snoozed"  // currently snoozed
	ViewAll      View = "all"
)

// Valid reports whether v is a known view.
func (v View) Valid() bool {
	switch v {
	case ViewInbox, ViewArchived, ViewSnoozed, ViewAll:
		return true
	}
	return false
}

// NotificationFilter selects a user's notifications.
type NotificationFilter struct {
	UserID      string
	View        View
	UnreadOnly  bool
	TopicIDs    []string // restrict to these topics (token restriction or filter)
	NoTopic     bool     // only notifications without a topic
	MinPriority int
	GroupKey    string
	DedupKey    string
	Search      []string // lowercased words that must all occur (see SearchTerms)
	IDs         []string
	Before      string // cursor: return IDs strictly lower than this
	Limit       int
	Now         time.Time
}

// BulkAction is a read-state change applied to many notifications.
type BulkAction string

const (
	BulkRead      BulkAction = "read"
	BulkUnread    BulkAction = "unread"
	BulkArchive   BulkAction = "archive"
	BulkUnarchive BulkAction = "unarchive"
	BulkDelete    BulkAction = "delete"
)

// Valid reports whether a is a known bulk action.
func (a BulkAction) Valid() bool {
	switch a {
	case BulkRead, BulkUnread, BulkArchive, BulkUnarchive, BulkDelete:
		return true
	}
	return false
}

// Counts summarizes unread inbox notifications.
type Counts struct {
	Unread  int            `json:"unread"`
	ByTopic map[string]int `json:"by_topic"` // topic ID -> unread; "" = no topic
	Snoozed int            `json:"snoozed"`  // snoozed, not archived
}

// NotificationRef identifies one recipient's copy of a message.
type NotificationRef struct {
	UserID string
	ID     string
}
