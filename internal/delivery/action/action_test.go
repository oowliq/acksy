package action

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oowliq/acksy/internal/domain"
)

func TestRun(t *testing.T) {
	var got *http.Request
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got, body = r.Clone(context.Background()), string(b)
		if strings.HasSuffix(r.URL.Path, "/redirect") {
			http.Redirect(w, r, "/elsewhere", http.StatusTemporaryRedirect)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/fail") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	a := domain.Action{
		Type: "http", Label: "Open", URL: srv.URL + "/api/services/cover/open?x=1", Method: "PUT",
		Headers: map[string]string{"Authorization": "Bearer S3CRET"}, Body: `{"entity_id":"cover.gate"}`,
	}
	res := New(Config{Client: http.DefaultClient}).Run(context.Background(), a)
	if !res.OK || res.Status != 200 || res.Error != "" {
		t.Fatalf("result: %+v", res)
	}
	if got.Method != http.MethodPut || got.URL.RawQuery != "x=1" || got.Header.Get("Authorization") != "Bearer S3CRET" ||
		got.Header.Get("Content-Type") != "application/json" || !strings.HasPrefix(got.Header.Get("User-Agent"), "Acksy-Action/") ||
		body != `{"entity_id":"cover.gate"}` {
		t.Fatalf("request: %s %s %v %q", got.Method, got.URL, got.Header, body)
	}

	a.URL = srv.URL + "/fail"
	if res := New(Config{Client: http.DefaultClient}).Run(context.Background(), a); res.OK || res.Error != "HTTP 500" {
		t.Fatalf("failure: %+v", res)
	}

	// Redirects are not followed, so secret headers and body never reach
	// the redirect target.
	a.URL = srv.URL + "/redirect"
	if res := New(Config{Client: http.DefaultClient}).Run(context.Background(), a); res.OK || res.Error != "HTTP 307" ||
		got.URL.Path != "/redirect" {
		t.Fatalf("redirect: %+v, last request %s", res, got.URL)
	}

	// LAN targets are refused unless allowed.
	a.URL = srv.URL + "/ok"
	if res := New(Config{}).Run(context.Background(), a); res.OK || !strings.Contains(res.Error, "ACKSY_ACTIONS_ALLOW_PRIVATE") {
		t.Fatalf("private: %+v", res)
	}
	if res := New(Config{AllowPrivate: true}).Run(context.Background(), a); !res.OK {
		t.Fatalf("allow private: %+v", res)
	}
}
