package httpapi

import (
	"crypto/sha256"
	"encoding/base64"
	"io/fs"
	"net/http"
	"regexp"
	"strings"
)

// apiCSP applies to everything but the SPA: API responses are data and never
// render as a page.
const apiCSP = "default-src 'none'; frame-ancestors 'none'"

// securityHeaders sets headers that are safe for every response. The SPA
// replaces the Content-Security-Policy with its own (see spaCSP).
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY") // clickjacking; frame-ancestors for modern browsers
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Content-Security-Policy", apiCSP)
		next.ServeHTTP(w, r)
	})
}

var inlineScriptRe = regexp.MustCompile(`(?is)<script\b([^>]*)>(.*?)</script>`)

// spaCSP is the policy for the web UI. Scripts come only from this origin,
// plus the inline scripts of index.html, allowed by hash. Styles allow inline
// attributes (Radix positions popovers with style=""), which cannot run code.
// Remote images are not loaded anyway (privacy, see markdown rendering).
func spaCSP(fsys fs.FS) string {
	scripts := []string{"'self'"}
	if b, err := fs.ReadFile(fsys, "index.html"); err == nil {
		for _, m := range inlineScriptRe.FindAllSubmatch(b, -1) {
			if strings.Contains(strings.ToLower(string(m[1])), "src=") {
				continue
			}
			sum := sha256.Sum256(m[2])
			scripts = append(scripts, "'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'")
		}
	}
	return strings.Join([]string{
		"default-src 'self'",
		"script-src " + strings.Join(scripts, " "),
		"style-src 'self' 'unsafe-inline'",
		"img-src 'self' data: blob:",
		"font-src 'self' data:",
		"connect-src 'self'",
		"worker-src 'self'",
		"manifest-src 'self'",
		"object-src 'none'",
		"base-uri 'none'",
		"form-action 'self'",
		"frame-ancestors 'none'",
	}, "; ")
}
