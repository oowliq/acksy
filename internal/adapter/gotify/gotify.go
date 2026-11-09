// Package gotify accepts gotify's message API (POST /message) under /gotify,
// so existing gotify scripts and integrations can publish to Acksy by
// changing the base URL and the token.
//
// A gotify application token maps to an Acksy API token with the
// notifications:publish scope. When that token is restricted to exactly one
// topic, messages go to that topic (like a gotify application); otherwise
// they go to the token owner's inbox without a topic.
package gotify

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/oowliq/acksy/internal/adapter"
	"github.com/oowliq/acksy/internal/domain"
)

// Name is the adapter name used in config and Message.Source.
const Name = "gotify"

// Adapter implements adapter.Adapter for gotify.
type Adapter struct{}

var _ adapter.Adapter = (*Adapter)(nil)

// New returns the gotify adapter.
func New() *Adapter { return &Adapter{} }

// Name implements adapter.Adapter.
func (*Adapter) Name() string { return Name }

// Prefix implements adapter.Adapter.
func (*Adapter) Prefix() string { return "/gotify" }

// createMessage is gotify's CreateMessage request.
type createMessage struct {
	Title    string         `json:"title"`
	Message  string         `json:"message"`
	Priority *int           `json:"priority"`
	Extras   map[string]any `json:"extras"`
}

// maxBody bounds the request: the message plus title and JSON overhead.
const maxBody = domain.MaxBodyLen + 16*1024

// Parse implements adapter.Adapter.
func (a *Adapter) Parse(r *http.Request) (*adapter.IncomingMessage, error) {
	path := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, a.Prefix()), "/")
	if path != "/message" {
		return nil, domain.NotFound("gotify route")
	}
	if r.Method != http.MethodPost {
		return nil, &domain.Error{Kind: domain.KindNotFound, Msg: "only POST /message is supported"}
	}
	cm, err := decode(r)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(cm.Message) == "" {
		return nil, domain.Invalid("message is required")
	}
	m := domain.Message{
		Title:          cm.Title,
		Body:           cm.Message,
		Priority:       mapPriority(cm.Priority),
		ClickURL:       clickURL(cm.Extras),
		Source:         Name,
		TopicFromToken: true,
	}
	return &adapter.IncomingMessage{Op: adapter.OpPublish, Token: token(r), Message: m, Echo: cm}, nil
}

// token reads the application token the ways gotify accepts it. The query
// form is allowed only here (AGENTS.md); request logs never include queries.
func token(r *http.Request) string {
	if t := r.Header.Get("X-Gotify-Key"); t != "" {
		return strings.TrimSpace(t)
	}
	if t := r.URL.Query().Get("token"); t != "" {
		return t
	}
	return adapter.BearerOrBasic(r)
}

func decode(r *http.Request) (createMessage, error) {
	var cm createMessage
	mt, params, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mt != "application/json" && mt != "application/x-www-form-urlencoded" && mt != "multipart/form-data" {
		return cm, domain.Invalid("Content-Type must be application/json or a form")
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxBody+1))
	if err != nil {
		return cm, domain.Invalid("read request body: %v", err)
	}
	if len(raw) > maxBody {
		return cm, domain.Invalid("request body is too large")
	}

	var form url.Values
	switch mt {
	case "application/json":
		if err := json.Unmarshal(raw, &cm); err != nil {
			return cm, domain.Invalid("invalid JSON: %v", err)
		}
		return cm, nil
	case "application/x-www-form-urlencoded":
		if form, err = url.ParseQuery(string(raw)); err != nil {
			return cm, domain.Invalid("invalid form: %v", err)
		}
	default: // multipart/form-data, parsed from the bounded body in memory
		if form, err = multipartFields(raw, params["boundary"]); err != nil {
			return cm, err
		}
	}
	cm.Title, cm.Message = form.Get("title"), form.Get("message")
	if v := form.Get("priority"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil {
			return cm, domain.Invalid("priority must be an integer")
		}
		cm.Priority = &p
	}
	return cm, nil
}

// multipartFields reads the text fields gotify understands; files are ignored.
func multipartFields(raw []byte, boundary string) (url.Values, error) {
	if boundary == "" {
		return nil, domain.Invalid("invalid form: missing multipart boundary")
	}
	form := url.Values{}
	mr := multipart.NewReader(bytes.NewReader(raw), boundary)
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			return form, nil
		}
		if err != nil {
			return nil, domain.Invalid("invalid form: %v", err)
		}
		name := part.FormName()
		if part.FileName() != "" || (name != "title" && name != "message" && name != "priority") {
			continue
		}
		v, err := io.ReadAll(part)
		if err != nil {
			return nil, domain.Invalid("invalid form: %v", err)
		}
		form.Set(name, string(v))
	}
}

// mapPriority maps gotify's 0-10 scale (as its Android client groups it) to
// Acksy's 1-5. A missing priority uses the topic default.
func mapPriority(p *int) int {
	switch {
	case p == nil:
		return 0
	case *p <= 0:
		return 1
	case *p <= 3:
		return 2
	case *p <= 7:
		return 3
	case *p <= 9:
		return 4
	default:
		return 5
	}
}

// clickURL reads extras["client::notification"]["click"]["url"].
func clickURL(extras map[string]any) string {
	n, _ := extras["client::notification"].(map[string]any)
	click, _ := n["click"].(map[string]any)
	u, _ := click["url"].(string)
	return u
}

// message is gotify's Message response.
type message struct {
	ID       int64          `json:"id"`
	AppID    int64          `json:"appid"`
	Message  string         `json:"message"`
	Title    string         `json:"title"`
	Priority int            `json:"priority"`
	Extras   map[string]any `json:"extras,omitempty"`
	Date     string         `json:"date"`
}

// WriteResult implements adapter.Adapter. Gotify IDs are integers; Acksy's
// are ULIDs, so id is a millisecond timestamp and not an Acksy ID.
func (*Adapter) WriteResult(w http.ResponseWriter, _ *http.Request, in *adapter.IncomingMessage, res adapter.Result) {
	cm, _ := in.Echo.(createMessage)
	prio := 0
	if cm.Priority != nil {
		prio = *cm.Priority
	}
	writeJSON(w, http.StatusOK, message{
		ID: res.At.UnixMilli(), Message: cm.Message, Title: cm.Title, Priority: prio,
		Extras: cm.Extras, Date: res.At.UTC().Format(time.RFC3339Nano),
	})
}

// WriteError implements adapter.Adapter in gotify's error format.
func (*Adapter) WriteError(w http.ResponseWriter, _ *http.Request, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": http.StatusText(status), "errorCode": status, "errorDescription": msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
