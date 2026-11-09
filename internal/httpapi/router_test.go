package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

type pingFunc func(context.Context) error

func (f pingFunc) Ping(ctx context.Context) error { return f(ctx) }

func newTestRouter(web fstest.MapFS, ready map[string]Pinger) http.Handler {
	d := Deps{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Ready: ready}
	if web != nil {
		d.Web = web
	}
	return NewRouter(d)
}

func do(t *testing.T, h http.Handler, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec
}

func TestHealthz(t *testing.T) {
	rec := do(t, newTestRouter(nil, nil), http.MethodGet, "/healthz")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestReadyz(t *testing.T) {
	ok := pingFunc(func(context.Context) error { return nil })
	bad := pingFunc(func(context.Context) error { return errors.New("down") })

	rec := do(t, newTestRouter(nil, map[string]Pinger{"db": ok}), http.MethodGet, "/readyz")
	if rec.Code != http.StatusOK {
		t.Fatalf("all ok: status %d", rec.Code)
	}

	rec = do(t, newTestRouter(nil, map[string]Pinger{"db": ok, "redis": bad}), http.MethodGet, "/readyz")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("one down: status %d", rec.Code)
	}
	var body struct{ Checks map[string]string }
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Checks["redis"] != "unavailable" || body.Checks["db"] != "ok" {
		t.Fatalf("checks = %v", body.Checks)
	}
	if strings.Contains(rec.Body.String(), "down") {
		t.Fatal("internal error text must not leak")
	}
}

func TestAPIVersionAndNotFound(t *testing.T) {
	h := newTestRouter(fstest.MapFS{"index.html": {Data: []byte("spa")}}, nil)

	if rec := do(t, h, http.MethodGet, "/api/v1/version"); rec.Code != http.StatusOK ||
		!strings.Contains(rec.Body.String(), `"version"`) {
		t.Fatalf("version: %d %s", rec.Code, rec.Body)
	}
	// Unknown API routes must return JSON 404, not the SPA.
	rec := do(t, h, http.MethodGet, "/api/v1/nope")
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `"not_found"`) {
		t.Fatalf("api 404: %d %s", rec.Code, rec.Body)
	}
}

func TestSPA(t *testing.T) {
	web := fstest.MapFS{
		"index.html":           {Data: []byte("<html>spa</html>")},
		"assets/app-1.js":      {Data: []byte("js")},
		"manifest.webmanifest": {Data: []byte("{}")},
		"sw.js":                {Data: []byte("self")},
	}
	h := newTestRouter(web, nil)

	tests := []struct {
		target    string
		status    int
		body      string
		immutable bool
	}{
		{"/", 200, "spa", false},
		{"/inbox/123", 200, "spa", false},
		{"/assets/app-1.js", 200, "js", true},
		{"/manifest.webmanifest", 200, "{}", false},
		{"/missing.js", 404, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			rec := do(t, h, http.MethodGet, tt.target)
			if rec.Code != tt.status {
				t.Fatalf("status %d, want %d", rec.Code, tt.status)
			}
			if !strings.Contains(rec.Body.String(), tt.body) {
				t.Fatalf("body %q missing %q", rec.Body, tt.body)
			}
			cc := rec.Header().Get("Cache-Control")
			if tt.immutable != strings.Contains(cc, "immutable") {
				t.Fatalf("cache-control %q", cc)
			}
		})
	}

	// The service worker and manifest are unhashed: always revalidated.
	for _, f := range []string{"/sw.js", "/manifest.webmanifest"} {
		if cc := do(t, h, http.MethodGet, f).Header().Get("Cache-Control"); cc != "no-cache" {
			t.Fatalf("%s cache-control %q", f, cc)
		}
	}
	if ct := do(t, h, http.MethodGet, "/manifest.webmanifest").Header().Get("Content-Type"); ct != "application/manifest+json" {
		t.Fatalf("manifest content-type %q", ct)
	}

	if rec := do(t, h, http.MethodPost, "/inbox"); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST to SPA: %d", rec.Code)
	}
}

func TestSPANotBuilt(t *testing.T) {
	rec := do(t, newTestRouter(fstest.MapFS{".gitkeep": {}}, nil), http.MethodGet, "/")
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "not built") {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
}

func TestSecurityHeaders(t *testing.T) {
	inline := `try { document.documentElement.dataset.theme = "latte" } catch {}`
	web := fstest.MapFS{"index.html": {Data: []byte(
		`<html><head><script>` + inline + `</script><script type="module" src="/assets/a.js"></script></head></html>`)}}
	h := newTestRouter(web, nil)

	for _, target := range []string{"/", "/api/v1/version", "/healthz"} {
		rec := do(t, h, http.MethodGet, target)
		if rec.Header().Get("X-Content-Type-Options") != "nosniff" || rec.Header().Get("X-Frame-Options") != "DENY" ||
			rec.Header().Get("Referrer-Policy") != "same-origin" ||
			!strings.Contains(rec.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
			t.Fatalf("%s headers: %v", target, rec.Header())
		}
	}
	if csp := do(t, h, http.MethodGet, "/api/v1/version").Header().Get("Content-Security-Policy"); csp != apiCSP {
		t.Fatalf("api csp %q", csp)
	}

	sum := sha256.Sum256([]byte(inline))
	hash := "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
	csp := do(t, h, http.MethodGet, "/").Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "script-src 'self' "+hash+";") || strings.Count(csp, "sha256-") != 1 ||
		strings.Contains(csp, "unsafe-eval") {
		t.Fatalf("spa csp %q", csp)
	}
}
