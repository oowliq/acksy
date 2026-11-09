// Package action runs a notification's http action when a recipient
// presses its button: the server sends the request, with the headers and
// body the publisher set (which recipients never see).
package action

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/oowliq/acksy/internal/buildinfo"
	"github.com/oowliq/acksy/internal/domain"
	"github.com/oowliq/acksy/internal/netguard"
	"github.com/oowliq/acksy/internal/service"
)

// Config tunes the runner.
type Config struct {
	// AllowPrivate lets actions reach private and loopback addresses (LAN
	// services such as Home Assistant). Off by default: the URL comes from
	// whoever published, and the request is made by the server.
	AllowPrivate bool
	// Client overrides the HTTP client (tests).
	Client *http.Client
}

// Runner sends action requests.
type Runner struct {
	client *http.Client
}

// New builds a runner.
func New(cfg Config) *Runner {
	c := cfg.Client
	if c == nil {
		c = netguard.NewClient(netguard.Options{AllowPrivate: cfg.AllowPrivate, Timeout: 15 * time.Second})
	}
	// Redirects are not followed: Go would resend the secret headers (all but
	// Authorization and Cookie) and, for 307/308, the body to wherever the
	// target points, possibly another host. A redirect is reported as its
	// status instead.
	nc := *c
	nc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Runner{client: &nc}
}

// Run implements service.ActionRunner.
func (r *Runner) Run(ctx context.Context, a domain.Action) service.ActionResult {
	var body io.Reader
	if a.Body != "" {
		body = strings.NewReader(a.Body)
	}
	req, err := http.NewRequestWithContext(ctx, a.Method, a.URL, body)
	if err != nil {
		return service.ActionResult{Error: "invalid request"}
	}
	for k, v := range a.Headers {
		req.Header.Set(k, v)
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "Acksy-Action/"+buildinfo.Version)
	}
	if a.Body != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := r.client.Do(req)
	if err != nil {
		if netguard.Blocked(err) {
			return service.ActionResult{Error: "destination is a private address; set ACKSY_ACTIONS_ALLOW_PRIVATE=true to allow LAN targets"}
		}
		return service.ActionResult{Error: netguard.Describe(err)}
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64*1024))
	res := service.ActionResult{Status: resp.StatusCode, OK: resp.StatusCode >= 200 && resp.StatusCode < 300}
	if !res.OK {
		res.Error = "HTTP " + strconv.Itoa(resp.StatusCode)
	}
	return res
}
