// Package adaptertest runs golden tests for ingest adapters: every
// testdata/*.http request is parsed and the result compared with the
// matching .golden JSON file. Regenerate with `go test ./internal/adapter/ntfy ./internal/adapter/gotify -update`.
//
// A .http file is a request line ("POST /ntfy/alerts?x=1"), header lines, a
// blank line and the body (the rest of the file, without its final newline).
package adaptertest

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oowliq/acksy/internal/adapter"
	"github.com/oowliq/acksy/internal/domain"
)

var update = flag.Bool("update", false, "rewrite adapter golden files")

// outcome is what a golden file records.
type outcome struct {
	Op      string          `json:"op,omitempty"`
	Token   string          `json:"token,omitempty"`
	Message *domain.Message `json:"message,omitempty"`
	// Actions in full (http headers and body included), which the public
	// JSON of Message hides.
	Actions json.RawMessage          `json:"actions,omitempty"`
	Echo    any                      `json:"echo,omitempty"`
	Error   *struct{ Kind, Msg any } `json:"error,omitempty"`
}

var opNames = map[adapter.Op]string{adapter.OpPublish: "publish", adapter.OpDelete: "delete", adapter.OpClear: "clear"}

var kindNames = map[domain.Kind]string{
	domain.KindInvalid: "invalid", domain.KindNotFound: "not_found", domain.KindConflict: "conflict",
	domain.KindUnauthorized: "unauthorized", domain.KindForbidden: "forbidden",
}

// Run checks every testdata/*.http file against its .golden file.
func Run(t *testing.T, a adapter.Adapter) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("testdata", "*.http"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no testdata/*.http files")
	}
	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), ".http")
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(file) //nolint:gosec // test fixture path
			if err != nil {
				t.Fatal(err)
			}
			got := encode(t, parse(t, a, raw))
			golden := strings.TrimSuffix(file, ".http") + ".golden"
			if *update {
				if err := os.WriteFile(golden, got, 0o600); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden) //nolint:gosec // test fixture path
			if err != nil {
				t.Fatalf("%v (run with -update to create it)", err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("mismatch for %s\n--- got\n%s\n--- want\n%s", file, got, want)
			}
		})
	}
}

func parse(t *testing.T, a adapter.Adapter, raw []byte) outcome {
	t.Helper()
	// Fixtures with multipart bodies use CRLF line endings throughout.
	sep, eol := "\n\n", "\n"
	if bytes.Contains(raw, []byte("\r\n\r\n")) {
		sep, eol = "\r\n\r\n", "\r\n"
	}
	head, body, _ := strings.Cut(string(raw), sep)
	body = strings.TrimSuffix(body, eol)
	lines := strings.Split(strings.TrimRight(head, "\r\n"), eol)
	method, target, ok := strings.Cut(lines[0], " ")
	if !ok {
		t.Fatalf("bad request line %q", lines[0])
	}
	req := httptest.NewRequestWithContext(t.Context(), method, target, strings.NewReader(body))
	for _, l := range lines[1:] {
		k, v, ok := strings.Cut(l, ":")
		if !ok {
			t.Fatalf("bad header line %q", l)
		}
		req.Header.Add(strings.TrimSpace(k), strings.TrimSpace(v))
	}

	in, err := a.Parse(req)
	if err != nil {
		var de *domain.Error
		if !errors.As(err, &de) {
			t.Fatalf("Parse returned a non-domain error: %v", err)
		}
		return outcome{Error: &struct{ Kind, Msg any }{kindNames[de.Kind], de.Msg}}
	}
	o := outcome{Op: opNames[in.Op], Token: in.Token, Message: &in.Message, Echo: in.Echo}
	if len(in.Message.Actions) > 0 {
		raw, err := domain.ActionsForStorage(in.Message.Actions)
		if err != nil {
			t.Fatal(err)
		}
		o.Actions = raw
		m := in.Message
		m.Actions = nil
		o.Message = &m
	}
	return o
}

func encode(t *testing.T, o outcome) []byte {
	t.Helper()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(o); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
