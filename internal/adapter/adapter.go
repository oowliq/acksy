// Package adapter defines ingest adapters: HTTP front-ends that accept other
// services' publish APIs (ntfy, gotify) and turn requests into Acksy
// messages. Adapters only parse requests and format responses; they never
// touch the store or the broker. Authentication goes through the same token
// check as the native API, and authorization, dedup and persistence happen in
// service (see AGENTS.md).
package adapter

import (
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	"github.com/oowliq/acksy/internal/domain"
)

// Op is what an incoming request asks Acksy to do.
type Op int

const (
	// OpPublish publishes Message.
	OpPublish Op = iota
	// OpDelete deletes every recipient's notification with Message.DedupKey
	// in Message.Topic.
	OpDelete
	// OpClear marks those notifications read.
	OpClear
)

// IncomingMessage is a parsed adapter request.
type IncomingMessage struct {
	Op Op
	// Token is the API token secret found in the request ("" when none).
	// Adapters accept API tokens only, never passwords or session cookies.
	Token   string
	Message domain.Message
	// Echo carries adapter-private data from Parse to WriteResult, such as
	// fields the original API returns but Acksy does not store.
	Echo any
}

// Result is what the service did, for the adapter's response.
type Result struct {
	At         time.Time
	MessageID  string
	Recipients int
	// Affected counts notifications changed by OpDelete / OpClear.
	Affected int
}

// Adapter is an ingest front-end mounted under Prefix.
type Adapter interface {
	// Name identifies the adapter in config, logs and Message.Source.
	Name() string
	// Prefix is the mount path, e.g. "/ntfy". Every request under it is
	// passed to Parse.
	Prefix() string
	// Parse turns a request into an IncomingMessage. Errors are domain
	// errors (domain.Invalid, domain.NotFound, ...).
	Parse(r *http.Request) (*IncomingMessage, error)
	// WriteResult writes the success response in the adapter's format.
	WriteResult(w http.ResponseWriter, r *http.Request, in *IncomingMessage, res Result)
	// WriteError writes an error response in the adapter's format.
	WriteError(w http.ResponseWriter, r *http.Request, status int, msg string)
}

// Registry holds the ingest adapters.
type Registry struct {
	adapters []Adapter
}

// NewRegistry returns a registry of the given adapters. All of them are
// mounted; the server settings switch each on and off at runtime.
func NewRegistry(all ...Adapter) *Registry {
	return &Registry{adapters: all}
}

// All returns the adapters.
func (r *Registry) All() []Adapter {
	if r == nil {
		return nil
	}
	return r.adapters
}

// BearerOrBasic extracts a token from "Authorization: Bearer <token>" or from
// the password of Basic auth (any username, as in `curl -u :token`).
func BearerOrBasic(r *http.Request) string {
	h := r.Header.Get("Authorization")
	scheme, value, ok := strings.Cut(h, " ")
	if !ok {
		return ""
	}
	value = strings.TrimSpace(value)
	switch {
	case strings.EqualFold(scheme, "Bearer"):
		return value
	case strings.EqualFold(scheme, "Basic"):
		raw, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			return ""
		}
		_, password, _ := strings.Cut(string(raw), ":")
		return password
	}
	return ""
}
