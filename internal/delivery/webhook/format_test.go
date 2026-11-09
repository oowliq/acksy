package webhook

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oowliq/acksy/internal/domain"
)

var update = flag.Bool("update", false, "rewrite golden files")

var (
	fixedNow = time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	created  = time.Date(2026, 11, 9, 11, 59, 0, 0, time.UTC)
)

// tricky carries content each format must neutralize.
func tricky() *domain.Notification {
	return &domain.Notification{
		ID: "01NOTIF", UserID: "01USER", MessageID: "01MSG", Topic: &domain.TopicSummary{ID: "01TOPIC", Name: "ci"},
		Title:    "Deploy <prod> failed",
		Body:     "**api** is down @everyone <!channel>\n\n<script>alert(1)</script> ![px](https://evil.example/p.png)",
		Priority: 5, Tags: []string{"deploy"}, ClickURL: "https://ci.example/run/42",
		Actions:  []domain.Action{{Type: "view", Label: "Logs", URL: "https://ci.example/run/42/logs"}},
		DedupKey: "deploy", RepeatCount: 2, Source: "ntfy", TokenID: "secret-token-id",
		CreatedAt: created, UpdatedAt: created,
	}
}

func TestRenderGolden(t *testing.T) {
	for _, f := range []domain.WebhookFormat{domain.WebhookAcksy, domain.WebhookDiscord, domain.WebhookSlack} {
		t.Run(string(f), func(t *testing.T) {
			w := &domain.Webhook{Format: f, Secret: "whsec_test"}
			req, err := Render(w, domain.WebhookUpdated, "01DELIVERY", tricky(), "https://acksy.example/", fixedNow)
			if err != nil {
				t.Fatal(err)
			}
			var pretty bytes.Buffer
			if err := json.Indent(&pretty, req.Body, "", "  "); err != nil {
				t.Fatal(err)
			}
			pretty.WriteByte('\n')
			golden := filepath.Join("testdata", string(f)+".golden")
			if *update {
				if err := os.WriteFile(golden, pretty.Bytes(), 0o600); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden) //nolint:gosec // test fixture
			if err != nil {
				t.Fatalf("%v (run with -update)", err)
			}
			if !bytes.Equal(pretty.Bytes(), want) {
				t.Fatalf("mismatch\n--- got\n%s--- want\n%s", pretty.Bytes(), want)
			}
			// The token ID and read state never leave Acksy.
			for _, leak := range []string{"secret-token-id", "read_at", "snoozed_until"} {
				if strings.Contains(string(req.Body), leak) {
					t.Errorf("%s body contains %q", f, leak)
				}
			}
			// Rendered text must be safe; the raw markdown body is passed on
			// as-is in the acksy format (and Discord renders it itself).
			var rendered []string
			switch f {
			case domain.WebhookAcksy:
				var p acksyPayload
				_ = json.Unmarshal(req.Body, &p)
				rendered = []string{p.Notification.BodyHTML, p.Notification.BodyText}
			case domain.WebhookSlack:
				var p slackPayload
				_ = json.Unmarshal(req.Body, &p)
				rendered = []string{p.Text}
			}
			for _, r := range rendered {
				// <!channel> is only a control sequence in Slack's text field.
				if strings.Contains(r, "<script") || strings.Contains(r, "evil.example") ||
					(f == domain.WebhookSlack && strings.Contains(r, "<!channel>")) {
					t.Errorf("%s rendered text is unsafe: %q", f, r)
				}
			}
		})
	}
}

func TestRenderHeadersAndSignature(t *testing.T) {
	w := &domain.Webhook{Format: domain.WebhookAcksy, Secret: "whsec_test"}
	req, err := Render(w, domain.WebhookCreated, "01DELIVERY", tricky(), "", fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	h := req.Headers
	if h["X-Acksy-Event"] != "notification.created" || h["X-Acksy-Delivery"] != "01DELIVERY" ||
		h["X-Acksy-Timestamp"] != "1794225600" || h["Content-Type"] != "application/json" {
		t.Fatalf("headers: %v", h)
	}
	if h["X-Acksy-Signature"] != Sign("whsec_test", "1794225600", req.Body) {
		t.Fatal("signature does not match the body")
	}
	// Independent vector (Python: hmac.new(b"key", b"1.{}", sha256)).
	if got := Sign("key", "1", []byte("{}")); got != "sha256=1ba6b8171186efc613e8bcc0cbdab2748f24984d7c5a84faa2637afa0e40d224" {
		t.Fatalf("signature vector: %s", got)
	}
}
