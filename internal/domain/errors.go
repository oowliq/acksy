// Package domain holds Acksy's core entities and pure business rules.
// It performs no I/O and imports nothing from internal/.
package domain

import (
	"fmt"
	"time"
)

// Kind classifies domain errors so transports can map them to status codes.
type Kind int

const (
	KindInvalid Kind = iota + 1
	KindNotFound
	KindConflict
	KindUnauthorized
	KindForbidden
)

// Error is a classified domain error with a client-safe message.
type Error struct {
	Kind Kind
	Msg  string
}

func (e *Error) Error() string { return e.Msg }

// Is matches any *Error of the same Kind, so errors.Is(err, ErrNotFound) works
// for every not-found error regardless of message.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && t.Kind == e.Kind
}

// Sentinels for errors.Is checks.
var (
	ErrInvalid      = &Error{KindInvalid, "invalid request"}
	ErrNotFound     = &Error{KindNotFound, "not found"}
	ErrConflict     = &Error{KindConflict, "conflict"}
	ErrUnauthorized = &Error{KindUnauthorized, "authentication required"}
	ErrForbidden    = &Error{KindForbidden, "forbidden"}
)

// Invalid reports a validation failure.
func Invalid(format string, args ...any) error {
	return &Error{KindInvalid, fmt.Sprintf(format, args...)}
}

// NotFound reports a missing entity, e.g. NotFound("topic").
func NotFound(what string) error { return &Error{KindNotFound, what + " not found"} }

// Conflict reports a uniqueness or state conflict.
func Conflict(format string, args ...any) error {
	return &Error{KindConflict, fmt.Sprintf(format, args...)}
}

// Forbidden reports a permission failure.
func Forbidden(format string, args ...any) error {
	return &Error{KindForbidden, fmt.Sprintf(format, args...)}
}

// RateLimitError reports that a caller exceeded a rate limit.
type RateLimitError struct {
	Msg string
	// RetryAfter is how long until the next request may succeed.
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string { return e.Msg }
