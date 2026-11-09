// Package markdown renders notification bodies on the server, for channels
// that leave Acksy (Web Push, webhooks). Output is always sanitized
// (invariant 8): raw HTML in the source is never passed through.
package markdown

import (
	"bytes"
	"html"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

var (
	// goldmark escapes raw HTML by default (no html.WithUnsafe).
	md     = goldmark.New(goldmark.WithExtensions(extension.GFM))
	ugc    = bluemonday.UGCPolicy()
	strict = bluemonday.StrictPolicy()

	// Images would make the receiver fetch remote content (and leak its IP
	// and read time), as on the web client.
	imgRe   = regexp.MustCompile(`(?is)<img\b[^>]*>`)
	blockRe = regexp.MustCompile(`(?i)</(p|div|h[1-6]|li|tr|blockquote|pre|table)>|<br\s*/?>`)
	spaceRe = regexp.MustCompile(`[ \t]+`)
	nlRe    = regexp.MustCompile(`\s*\n\s*`)
)

func render(src string) string {
	var buf bytes.Buffer
	if err := md.Convert([]byte(src), &buf); err != nil {
		// goldmark only fails on writer errors, which bytes.Buffer never returns.
		return html.EscapeString(src)
	}
	return imgRe.ReplaceAllString(buf.String(), "")
}

// HTML renders markdown to sanitized HTML. Remote images are dropped.
func HTML(src string) string {
	return ugc.Sanitize(render(src))
}

// Plain renders markdown to plain text for previews: no markup, block
// elements become line breaks, runs of whitespace collapse. Text inside raw
// HTML tags is kept as text (the tags are dropped).
func Plain(src string) string {
	// goldmark's own newlines between tags are layout, not content: lines
	// come only from block elements. Code blocks flatten; fine for previews.
	h := strings.ReplaceAll(render(src), "\n", " ")
	h = blockRe.ReplaceAllString(h, "\n")
	text := html.UnescapeString(strict.Sanitize(h))
	text = spaceRe.ReplaceAllString(text, " ")
	text = nlRe.ReplaceAllString(text, "\n")
	return strings.TrimSpace(text)
}

// Truncate shortens s to at most n runes, ending with "…" when cut.
func Truncate(s string, n int) string {
	if n <= 0 || utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return strings.TrimRight(string(r[:n-1]), " \n") + "…"
}
