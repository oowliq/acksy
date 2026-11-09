// Package netguard builds HTTP clients for requests to user-supplied URLs
// (Web Push endpoints, webhooks) that refuse to connect to private,
// loopback and other non-public addresses, preventing SSRF into the
// operator's network.
//
// The check runs on the resolved address at connect time, so DNS rebinding
// and redirects to internal hosts are blocked as well.
package netguard

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"syscall"
	"time"
)

// ErrBlocked is returned when a connection to a non-public address is refused.
var ErrBlocked = errors.New("netguard: destination address is not allowed")

// nonPublic lists ranges that are never reachable from the internet or are
// special-purpose, beyond what netip.Addr's predicates cover.
var nonPublic = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),     // "this network"
	netip.MustParsePrefix("100.64.0.0/10"), // carrier-grade NAT
	netip.MustParsePrefix("192.0.0.0/24"),  // IETF protocol assignments
	netip.MustParsePrefix("192.0.2.0/24"),  // TEST-NET-1
	netip.MustParsePrefix("198.18.0.0/15"), // benchmarking
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),    // reserved, incl. broadcast
	netip.MustParsePrefix("64:ff9b::/96"),   // NAT64 may map to private IPv4
	netip.MustParsePrefix("64:ff9b:1::/48"), // local-use NAT64 (RFC 8215)
	netip.MustParsePrefix("::/96"),          // deprecated IPv4-compatible addresses
	netip.MustParsePrefix("2001::/32"),      // Teredo tunnels
	netip.MustParsePrefix("2002::/16"),      // 6to4 embeds an arbitrary IPv4 address
	netip.MustParsePrefix("2001:db8::/32"),
}

// Public reports whether addr is a globally routable unicast address.
func Public(addr netip.Addr) bool {
	addr = addr.Unmap()
	if !addr.IsValid() || addr.IsLoopback() || addr.IsPrivate() || addr.IsUnspecified() ||
		addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsInterfaceLocalMulticast() ||
		addr.IsMulticast() || !addr.IsGlobalUnicast() {
		return false
	}
	for _, p := range nonPublic {
		if p.Contains(addr) {
			return false
		}
	}
	return true
}

// control rejects connections to non-public addresses.
func control(_, address string, _ syscall.RawConn) error {
	ap, err := netip.ParseAddrPort(address)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrBlocked, address)
	}
	if !Public(ap.Addr()) {
		return fmt.Errorf("%w: %s", ErrBlocked, ap.Addr())
	}
	return nil
}

// Options configure NewClient.
type Options struct {
	// AllowPrivate disables the address check (for trusted LAN targets).
	AllowPrivate bool
	// Timeout bounds a whole request (default 15s).
	Timeout time.Duration
}

// maxRedirects bounds redirect chains; every hop is checked by the dialer.
const maxRedirects = 3

// NewClient returns an HTTP client for user-supplied URLs. Proxies from the
// environment are ignored, since a proxy would hide the destination address
// from the check.
func NewClient(o Options) *http.Client {
	if o.Timeout <= 0 {
		o.Timeout = 15 * time.Second
	}
	d := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	if !o.AllowPrivate {
		d.Control = control
	}
	tr := &http.Transport{
		Proxy:                 nil,
		DialContext:           d.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          50,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
	}
	return &http.Client{
		Transport: tr,
		Timeout:   o.Timeout,
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return errors.New("netguard: too many redirects")
			}
			return nil
		},
	}
}

// Blocked reports whether err (from a request made with a NewClient client)
// was caused by the address check.
func Blocked(err error) bool { return errors.Is(err, ErrBlocked) }

// Describe reports a request error without the URL, which may embed a
// credential (Discord and Slack webhook URLs, tokens in queries).
func Describe(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	return err.Error()
}
