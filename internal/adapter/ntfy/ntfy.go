// Package ntfy accepts ntfy's publish API (https://docs.ntfy.sh/publish/)
// under /ntfy, so existing ntfy scripts can publish to Acksy by changing the
// base URL and the token. Subscribing in ntfy format is not supported.
package ntfy

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/oowliq/acksy/internal/adapter"
	"github.com/oowliq/acksy/internal/domain"
)

// Name is the adapter name used in config and Message.Source.
const Name = "ntfy"

// Adapter implements adapter.Adapter for ntfy.
type Adapter struct {
	// NewID generates message IDs for publishes without a sequence ID.
	NewID func() string
}

var _ adapter.Adapter = (*Adapter)(nil)

// New returns the ntfy adapter.
func New() *Adapter { return &Adapter{NewID: randomID} }

// Name implements adapter.Adapter.
func (*Adapter) Name() string { return Name }

// Prefix implements adapter.Adapter.
func (*Adapter) Prefix() string { return "/ntfy" }

// echo is what WriteResult needs beyond the message.
type echo struct {
	// ID is the ntfy message ID returned to the client; it doubles as the
	// sequence ID (dedup key) so the client can update the message later.
	ID         string
	SequenceID string // as given by the client, "" when generated
}

// sequenceIDRe mirrors ntfy's sequence ID rules.
var sequenceIDRe = regexp.MustCompile(`^[-_A-Za-z0-9]{1,64}$`)

// Parse implements adapter.Adapter.
func (a *Adapter) Parse(r *http.Request) (*adapter.IncomingMessage, error) {
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, a.Prefix()), "/")
	var segs []string
	if rest != "" {
		segs = strings.Split(rest, "/")
	}
	in := &adapter.IncomingMessage{Token: adapter.BearerOrBasic(r)}
	write := r.Method == http.MethodPost || r.Method == http.MethodPut

	var err error
	switch {
	case len(segs) == 0 && write:
		err = a.parseJSON(r, in)
	case len(segs) == 1 && write:
		err = a.parsePublish(r, in, segs[0], "", true)
	case len(segs) == 2 && r.Method == http.MethodGet && isPublishVerb(segs[1]):
		err = a.parsePublish(r, in, segs[0], "", false)
	case len(segs) == 2 && write:
		err = a.parsePublish(r, in, segs[0], segs[1], true)
	case len(segs) == 2 && r.Method == http.MethodDelete,
		len(segs) == 3 && segs[2] == "delete" && (r.Method == http.MethodGet || write):
		err = keyed(in, adapter.OpDelete, segs[0], segs[1])
	case len(segs) == 3 && segs[2] == "clear" && (r.Method == http.MethodGet || write):
		err = keyed(in, adapter.OpClear, segs[0], segs[1])
	case r.Method == http.MethodGet:
		return nil, &domain.Error{Kind: domain.KindNotFound, Msg: "ntfy subscriptions are not supported; read notifications through the Acksy API"}
	default:
		return nil, domain.NotFound("ntfy route")
	}
	if err != nil {
		return nil, err
	}
	return in, nil
}

func isPublishVerb(s string) bool { return s == "publish" || s == "send" || s == "trigger" }

func keyed(in *adapter.IncomingMessage, op adapter.Op, topic, sid string) error {
	if !sequenceIDRe.MatchString(sid) {
		return domain.Invalid("invalid sequence ID")
	}
	in.Op = op
	in.Message = domain.Message{Topic: topic, DedupKey: sid, Source: Name}
	in.Echo = echo{ID: sid, SequenceID: sid}
	return nil
}

// param reads a publish parameter from headers or the query string, trying
// each alias in order, as ntfy does.
func param(r *http.Request, names ...string) string {
	for _, n := range names {
		if v := r.Header.Get(n); v != "" {
			return strings.TrimSpace(v)
		}
	}
	q := r.URL.Query()
	for _, n := range names {
		if v := q.Get(strings.ToLower(n)); v != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func (a *Adapter) parsePublish(r *http.Request, in *adapter.IncomingMessage, topic, pathSID string, readBody bool) error {
	if v := param(r, "X-Delay", "Delay", "X-At", "At", "X-In", "In"); v != "" {
		return errDelay
	}
	attach := param(r, "X-Attach", "Attach", "A")
	// A filename without an attachment URL names an uploaded file body.
	if attach == "" && param(r, "X-Filename", "Filename", "File", "F") != "" {
		return errAttachment
	}
	var body string
	if readBody {
		var err error
		if body, err = readText(r.Body); err != nil {
			return err
		}
	}
	message := param(r, "X-Message", "Message", "M")
	if message != "" && body != "" {
		// ntfy treats the body as a file attachment in this case.
		return errAttachment
	}
	if message == "" {
		message = body
	}
	prio, err := parsePriority(param(r, "X-Priority", "Priority", "Prio", "P"))
	if err != nil {
		return err
	}
	actions, err := parseActions(param(r, "X-Actions", "Actions", "Action"))
	if err != nil {
		return err
	}
	sid := pathSID
	if sid == "" {
		sid = param(r, "X-Sequence-ID", "Sequence-ID", "SID")
	}
	m := domain.Message{
		Topic:    topic,
		Title:    param(r, "X-Title", "Title", "T"),
		Body:     message,
		Priority: prio,
		Tags:     splitTags(param(r, "X-Tags", "Tags", "Tag", "Ta")),
		ClickURL: param(r, "X-Click", "Click"),
		Actions:  withAttachment(actions, attach),
	}
	return a.finish(in, m, sid)
}

// finish fills defaults shared by header and JSON publishing.
func (a *Adapter) finish(in *adapter.IncomingMessage, m domain.Message, sid string) error {
	e := echo{ID: sid, SequenceID: sid}
	if sid != "" && !sequenceIDRe.MatchString(sid) {
		return domain.Invalid("invalid sequence ID")
	}
	if sid == "" {
		// Every ntfy message can be updated through its returned ID.
		e.ID = a.NewID()
	}
	if m.Title == "" && m.Body == "" {
		m.Body = "triggered" // ntfy's default for empty messages
	}
	m.DedupKey, m.Source = e.ID, Name
	in.Op, in.Message, in.Echo = adapter.OpPublish, m, e
	return nil
}

// jsonPublish is ntfy's JSON publish body (POST / with a topic field).
type jsonPublish struct {
	Topic      string       `json:"topic"`
	Message    string       `json:"message"`
	Title      string       `json:"title"`
	Tags       []string     `json:"tags"`
	Priority   int          `json:"priority"`
	Click      string       `json:"click"`
	Attach     string       `json:"attach"`
	Delay      string       `json:"delay"`
	Actions    []jsonAction `json:"actions"`
	SequenceID string       `json:"sequence_id"`
	// Filename (names the attach URL), Markdown, Email, Call, Icon, Cache and
	// Firebase are accepted and ignored. JSON cannot upload files.
}

type jsonAction struct {
	Action  string            `json:"action"`
	Label   string            `json:"label"`
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
	Clear   bool              `json:"clear"`
}

// toAction converts an ntfy action. view and http are supported; http
// keeps its method, headers, body and clear flag (the server runs it when
// a recipient presses the button). broadcast and copy are dropped.
func (ja jsonAction) toAction() (domain.Action, bool) {
	switch strings.ToLower(strings.TrimSpace(ja.Action)) {
	case domain.ActionView:
		return domain.Action{Type: domain.ActionView, Label: ja.Label, URL: ja.URL}, true
	case domain.ActionHTTP:
		return domain.Action{
			Type: domain.ActionHTTP, Label: ja.Label, URL: ja.URL, Method: ja.Method,
			Headers: ja.Headers, Body: ja.Body, Clear: ja.Clear,
		}, true
	}
	return domain.Action{}, false
}

func (a *Adapter) parseJSON(r *http.Request, in *adapter.IncomingMessage) error {
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxBody+1))
	if err != nil {
		return domain.Invalid("read request body: %v", err)
	}
	if len(raw) > maxBody {
		return domain.Invalid("request body is too large")
	}
	var jp jsonPublish
	if err := json.Unmarshal(raw, &jp); err != nil {
		return domain.Invalid("invalid JSON: %v", err)
	}
	switch {
	case jp.Topic == "":
		return domain.Invalid("topic is required")
	case jp.Delay != "":
		return errDelay
	case jp.Priority < 0 || jp.Priority > domain.PriorityMax:
		return domain.Invalid("priority must be between 1 and 5")
	}
	var actions []domain.Action
	for _, ja := range jp.Actions {
		if a, ok := ja.toAction(); ok {
			actions = append(actions, a)
		}
	}
	m := domain.Message{
		Topic: jp.Topic, Title: jp.Title, Body: jp.Message, Priority: jp.Priority,
		Tags: jp.Tags, ClickURL: jp.Click, Actions: withAttachment(actions, jp.Attach),
	}
	return a.finish(in, m, jp.SequenceID)
}

var (
	errDelay      = domain.Invalid("delayed delivery (X-Delay) is not supported by Acksy")
	errAttachment = domain.Invalid("file attachments are not supported by Acksy; send text, or link a file with X-Attach")
)

// maxBody bounds request bodies; larger ones would be attachments in ntfy.
const maxBody = domain.MaxBodyLen

func readText(body io.Reader) (string, error) {
	if body == nil {
		return "", nil
	}
	raw, err := io.ReadAll(io.LimitReader(body, maxBody+1))
	if err != nil {
		return "", domain.Invalid("read request body: %v", err)
	}
	if len(raw) > maxBody || !utf8.Valid(raw) {
		return "", errAttachment
	}
	return string(raw), nil
}

var priorities = map[string]int{
	"1": 1, "min": 1,
	"2": 2, "low": 2,
	"3": 3, "default": 3,
	"4": 4, "high": 4,
	"5": 5, "max": 5, "urgent": 5,
}

func parsePriority(v string) (int, error) {
	if v == "" {
		return 0, nil
	}
	p, ok := priorities[strings.ToLower(v)]
	if !ok {
		return 0, domain.Invalid("invalid priority %q: use 1-5 or min, low, default, high, max, urgent", v)
	}
	return p, nil
}

func splitTags(v string) []string {
	var out []string
	for t := range strings.SplitSeq(v, ",") {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// withAttachment adds an attachment URL as a link button, when there is room.
func withAttachment(actions []domain.Action, url string) []domain.Action {
	if url != "" && len(actions) < domain.MaxActions {
		actions = append(actions, domain.Action{Type: "view", Label: "Attachment", URL: url})
	}
	return actions
}

// parseActions reads ntfy actions in JSON or the short format
// ("view, Open, https://…; http, Restart, https://…, method=PUT,
// headers.Authorization=Bearer x, body={…}, clear=true"). view and http
// actions are kept; broadcast and copy are dropped.
func parseActions(v string) ([]domain.Action, error) {
	if v == "" {
		return nil, nil
	}
	var out []domain.Action
	if strings.HasPrefix(v, "[") {
		var list []jsonAction
		if err := json.Unmarshal([]byte(v), &list); err != nil {
			return nil, domain.Invalid("invalid actions JSON: %v", err)
		}
		for _, ja := range list {
			if a, ok := ja.toAction(); ok {
				out = append(out, a)
			}
		}
		return out, nil
	}
	defs, err := splitQuoted(v, ';', true)
	if err != nil {
		return nil, err
	}
	for _, def := range defs {
		fields, err := splitQuoted(def, ',', false)
		if err != nil {
			return nil, err
		}
		var ja jsonAction
		pos := 0
		for _, f := range fields {
			if k, val, ok := strings.Cut(f, "="); ok && isActionKey(k) {
				switch k = strings.TrimSpace(k); {
				case k == "action":
					ja.Action = val
				case k == "label":
					ja.Label = val
				case k == "url":
					ja.URL = val
				case k == "method":
					ja.Method = val
				case k == "body":
					ja.Body = val
				case k == "clear":
					ja.Clear = strings.EqualFold(strings.TrimSpace(val), "true")
				case strings.HasPrefix(k, "headers."):
					if ja.Headers == nil {
						ja.Headers = map[string]string{}
					}
					ja.Headers[strings.TrimPrefix(k, "headers.")] = val
				}
				continue
			}
			switch pos {
			case 0:
				ja.Action = f
			case 1:
				ja.Label = f
			case 2:
				ja.URL = f
			}
			pos++
		}
		if a, ok := ja.toAction(); ok {
			out = append(out, a)
		}
	}
	return out, nil
}

func isActionKey(k string) bool {
	k = strings.TrimSpace(k)
	switch k {
	case "action", "label", "url", "clear", "method", "body", "intent", "value":
		return true
	}
	return strings.HasPrefix(k, "headers.") || strings.HasPrefix(k, "extras.")
}

// splitQuoted splits on sep outside single or double quotes, trimming
// whitespace. Quotes are kept when keepQuotes is set (for a later split).
func splitQuoted(s string, sep rune, keepQuotes bool) ([]string, error) {
	var (
		out   []string
		cur   strings.Builder
		quote rune
	)
	for _, r := range s {
		switch {
		case quote != 0 && r == quote:
			quote = 0
			if keepQuotes {
				cur.WriteRune(r)
			}
		case quote != 0:
			cur.WriteRune(r)
		case r == '"' || r == '\'':
			quote = r
			if keepQuotes {
				cur.WriteRune(r)
			}
		case r == sep:
			out = append(out, strings.TrimSpace(cur.String()))
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	if quote != 0 {
		return nil, domain.Invalid("invalid actions: unterminated quote")
	}
	if last := strings.TrimSpace(cur.String()); last != "" || len(out) > 0 {
		out = append(out, last)
	}
	return out, nil
}

// response is ntfy's publish response (a subset of its message JSON).
type response struct {
	ID         string   `json:"id"`
	Time       int64    `json:"time"`
	Event      string   `json:"event"`
	Topic      string   `json:"topic"`
	Message    string   `json:"message,omitempty"`
	Title      string   `json:"title,omitempty"`
	Priority   int      `json:"priority,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	Click      string   `json:"click,omitempty"`
	SequenceID string   `json:"sequence_id,omitempty"`
}

// WriteResult implements adapter.Adapter.
func (a *Adapter) WriteResult(w http.ResponseWriter, _ *http.Request, in *adapter.IncomingMessage, res adapter.Result) {
	e, _ := in.Echo.(echo)
	m := in.Message
	resp := response{ID: e.ID, Time: res.At.Unix(), Topic: m.Topic, SequenceID: e.SequenceID}
	switch in.Op {
	case adapter.OpPublish:
		resp.Event, resp.Message, resp.Title, resp.Priority, resp.Tags, resp.Click =
			"message", m.Body, m.Title, m.Priority, m.Tags, m.ClickURL
	case adapter.OpDelete:
		resp.ID, resp.Event = a.NewID(), "message_delete"
	case adapter.OpClear:
		resp.ID, resp.Event = a.NewID(), "message_clear"
	}
	writeJSON(w, http.StatusOK, resp)
}

// WriteError implements adapter.Adapter in ntfy's error format.
func (*Adapter) WriteError(w http.ResponseWriter, _ *http.Request, status int, msg string) {
	writeJSON(w, status, map[string]any{"code": status * 100, "http": status, "error": msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

const idAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// randomID returns a 12-character ID in ntfy's style.
func randomID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		panic(errors.Join(errors.New("ntfy: crypto/rand failed"), err))
	}
	for i := range b {
		b[i] = idAlphabet[int(b[i])%len(idAlphabet)]
	}
	return string(b)
}
