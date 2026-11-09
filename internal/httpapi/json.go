package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/oowliq/acksy/internal/domain"
)

// maxBodyBytes bounds JSON request bodies (the largest valid payload is a
// 64 KiB notification body plus metadata).
const maxBodyBytes = 256 << 10

// decodeJSON strictly decodes a JSON request body into dst.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	if ct := r.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, "application/json") {
		return domain.Invalid("Content-Type must be application/json")
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		switch {
		case errors.As(err, &maxErr):
			return &domain.Error{Kind: domain.KindInvalid, Msg: "request body too large"}
		case errors.Is(err, io.EOF):
			return domain.Invalid("request body is empty")
		default:
			return domain.Invalid("invalid JSON: %s", strings.TrimPrefix(err.Error(), "json: "))
		}
	}
	if dec.More() {
		return domain.Invalid("request body must contain a single JSON object")
	}
	return nil
}

// opt distinguishes an absent JSON field from an explicit null.
type opt[T any] struct {
	Set   bool
	Value *T
}

func (o *opt[T]) UnmarshalJSON(b []byte) error {
	o.Set = true
	if string(b) == "null" {
		o.Value = nil
		return nil
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	o.Value = &v
	return nil
}

// errorBody is the uniform API error shape.
type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	var b errorBody
	b.Error.Code, b.Error.Message = code, msg
	writeJSON(w, status, b)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

var kindStatus = map[domain.Kind]struct {
	status int
	code   string
}{
	domain.KindInvalid:      {http.StatusBadRequest, "invalid"},
	domain.KindNotFound:     {http.StatusNotFound, "not_found"},
	domain.KindConflict:     {http.StatusConflict, "conflict"},
	domain.KindUnauthorized: {http.StatusUnauthorized, "unauthorized"},
	domain.KindForbidden:    {http.StatusForbidden, "forbidden"},
}

// httpError is an error translated for HTTP.
type httpError struct {
	status     int
	code, msg  string
	retryAfter time.Duration // sets Retry-After when positive
	expected   bool          // false for unexpected errors (500), which must be logged
}

// statusFor maps an error to HTTP. This is the single place where domain
// errors become status codes, for the native API and the adapters alike.
func statusFor(err error) httpError {
	var rl *domain.RateLimitError
	if errors.As(err, &rl) {
		return httpError{status: http.StatusTooManyRequests, code: "rate_limited", msg: rl.Msg, retryAfter: rl.RetryAfter, expected: true}
	}
	var de *domain.Error
	if errors.As(err, &de) {
		if m, found := kindStatus[de.Kind]; found {
			return httpError{status: m.status, code: m.code, msg: de.Msg, expected: true}
		}
	}
	return httpError{status: http.StatusInternalServerError, code: "internal", msg: "internal server error"}
}

// setHeaders adds the headers that go with an error status.
func (e httpError) setHeaders(w http.ResponseWriter) {
	switch {
	case e.status == http.StatusUnauthorized:
		w.Header().Set("WWW-Authenticate", `Bearer realm="acksy"`)
	case e.retryAfter > 0:
		w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(e.retryAfter.Seconds()))))
	}
}

// respondErr maps domain errors to HTTP; anything else is a logged 500.
func respondErr(w http.ResponseWriter, r *http.Request, log *slog.Logger, err error) {
	he := statusFor(err)
	if !he.expected {
		log.ErrorContext(r.Context(), "request failed", "method", r.Method, "path", r.URL.Path, "err", err)
	}
	he.setHeaders(w)
	writeError(w, he.status, he.code, he.msg)
}

// Query helpers.

func queryBool(r *http.Request, key string) (bool, error) {
	v := r.URL.Query().Get(key)
	if v == "" {
		return false, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, domain.Invalid("query parameter %s must be true or false", key)
	}
	return b, nil
}

func queryInt(r *http.Request, key string) (int, error) {
	v := r.URL.Query().Get(key)
	if v == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, domain.Invalid("query parameter %s must be an integer", key)
	}
	return n, nil
}
