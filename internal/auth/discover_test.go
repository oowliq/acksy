package auth

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type fixedResponse struct {
	status int
	ctype  string
	body   string
}

func (f fixedResponse) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: f.status, Status: http.StatusText(f.status),
		Header: http.Header{"Content-Type": {f.ctype}}, Body: io.NopCloser(strings.NewReader(f.body)),
	}, nil
}

// Test connection must not turn into a way to read internal HTTP services.
func TestDiscoverHidesResponseBodies(t *testing.T) {
	for _, tc := range []struct {
		name string
		resp fixedResponse
		want string
	}{
		{"not found", fixedResponse{404, "text/plain", "INTERNAL-SECRET"}, "HTTP 404"},
		{"not json", fixedResponse{200, "text/html", "<html>INTERNAL-SECRET</html>"}, "OpenID"},
		{"bad json", fixedResponse{200, "application/json", `{"INTERNAL-SECRET": true}`}, "metadata"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := NewOIDCProvider(OIDCConfig{Issuer: "https://idp.test", ClientID: "x", HTTPClient: &http.Client{Transport: tc.resp}})
			err := p.Discover(context.Background())
			if err == nil {
				t.Fatal("expected an error")
			}
			if strings.Contains(err.Error(), "INTERNAL-SECRET") || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q", err)
			}
		})
	}
}
