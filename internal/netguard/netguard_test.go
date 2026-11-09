package netguard

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"testing"
)

func TestPublic(t *testing.T) {
	for addr, want := range map[string]bool{
		"8.8.8.8":              true,
		"142.250.74.110":       true,
		"2606:4700:4700::1111": true,
		"127.0.0.1":            false,
		"10.1.2.3":             false,
		"172.16.0.1":           false,
		"192.168.1.10":         false,
		"169.254.169.254":      false, // cloud metadata
		"100.64.0.1":           false, // CGNAT
		"0.0.0.0":              false,
		"255.255.255.255":      false,
		"224.0.0.1":            false,
		"::1":                  false,
		"fd00::1":              false, // ULA
		"fe80::1":              false,
		"::ffff:127.0.0.1":     false, // v4-mapped loopback
		"::ffff:10.0.0.1":      false,
		"64:ff9b::a00:1":       false, // NAT64 of 10.0.0.1
		"64:ff9b:1::a00:1":     false, // local-use NAT64
		"::a00:1":              false, // IPv4-compatible 10.0.0.1
		"2002:7f00:1::1":       false, // 6to4 of 127.0.0.1
		"2001:0:4136:e378::1":  false, // Teredo
	} {
		if got := Public(netip.MustParseAddr(addr)); got != want {
			t.Errorf("Public(%s) = %v, want %v", addr, got, want)
		}
	}
}

func TestClientBlocksLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp, err := NewClient(Options{}).Do(req); !Blocked(err) {
		if resp != nil {
			_ = resp.Body.Close()
		}
		t.Fatalf("loopback request not blocked: %v", err)
	}

	resp, err := NewClient(Options{AllowPrivate: true}).Do(req)
	if err != nil {
		t.Fatalf("AllowPrivate: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestDescribeHidesURL(t *testing.T) {
	err := &url.Error{Op: "Post", URL: "https://discord.com/api/webhooks/123/SECRET-TOKEN", Err: errors.New("connection refused")}
	if got := Describe(err); got != "connection refused" {
		t.Fatalf("Describe = %q", got)
	}
	timeout := &url.Error{Op: "Post", URL: "https://x/SECRET-TOKEN", Err: context.DeadlineExceeded}
	if got := Describe(timeout); got != "timeout" {
		t.Fatalf("timeout = %q", got)
	}
}
