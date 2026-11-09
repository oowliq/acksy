package markdown

import (
	"strings"
	"testing"
)

func TestPlain(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"emphasis", "**prod** updated, *fast*", "prod updated, fast"},
		{"paragraphs and lists", "Deploy done\n\n- api\n- web", "Deploy done\napi\nweb"},
		{"links keep text", "See [run 42](https://ci.example/42)", "See run 42"},
		{"code", "Run `make test`", "Run make test"},
		{"entities decoded", "a < b & c", "a < b & c"},
		{"raw html tags dropped, text kept", "<b>bold</b> <script>alert(1)</script>", "bold alert(1)"},
		{"images dropped", "![cat](https://evil.example/pixel.png) hi", "hi"},
		{"table", "| a | b |\n|---|---|\n| 1 | 2 |", "a b\n1 2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Plain(tt.in); got != tt.want {
				t.Fatalf("Plain(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestHTMLIsSanitized(t *testing.T) {
	out := HTML("**ok** <script>alert(1)</script> [x](javascript:alert(1)) ![p](https://evil.example/p.png)\n\n<img src=x onerror=alert(1)>")
	for _, bad := range []string{"<script", "javascript:", "onerror", "<img", "evil.example"} {
		if strings.Contains(out, bad) {
			t.Fatalf("unsafe output %q contains %q", out, bad)
		}
	}
	if !strings.Contains(out, "<strong>ok</strong>") {
		t.Fatalf("markdown not rendered: %q", out)
	}
	if !strings.Contains(HTML("| a |\n|---|\n| 1 |"), "<table>") {
		t.Fatal("GFM tables not rendered")
	}
}

func TestTruncate(t *testing.T) {
	if got := Truncate("hello", 10); got != "hello" {
		t.Fatalf("short: %q", got)
	}
	if got := Truncate("hello world", 7); got != "hello…" {
		t.Fatalf("cut: %q", got)
	}
	if got := Truncate("привет мир", 7); got != "привет…" {
		t.Fatalf("runes: %q", got)
	}
}
