package smtp

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/oowliq/acksy/internal/domain"
)

func TestParseMessage(t *testing.T) {
	mixed := "Content-Type: multipart/mixed; boundary=x\r\n\r\n" +
		"--x\r\nContent-Disposition: attachment; filename=a.txt\r\n\r\nsecret\r\n" +
		"--x\r\nContent-Type: multipart/alternative; boundary=y\r\n\r\n" +
		"--y\r\nContent-Type: text/html\r\n\r\n<b>hidden</b>\r\n" +
		"--y\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: base64\r\n\r\ncGxhaW4=\r\n--y--\r\n--x--\r\n"
	for _, tt := range []struct{ name, raw, title, body string }{
		{"default", "To: ignored@example.com\r\nSubject: test\r\n\r\nfirst\r\nsecond\r\n", "test", "first\nsecond"},
		{"encoded", "Subject: =?UTF-8?B?0KLQtdGB0YI=?=\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\ncaf=C3=A9", "Тест", "café"},
		{"nested", mixed, "", "plain"},
		{"subject only", "Subject: test\n\n", "test", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseMessage(strings.NewReader(tt.raw), "alerts")
			if err != nil || got.Title != tt.title || got.Body != tt.body || got.Topic != "alerts" || got.Source != "smtp" {
				t.Fatalf("parseMessage() = %+v, %v", got, err)
			}
		})
	}
}

func TestParseMessageRejectsInvalidMail(t *testing.T) {
	nested := "\r\nbody"
	for i := range 20 {
		nested = fmt.Sprintf("Content-Type: multipart/mixed; boundary=x%d\r\n\r\n--x%d\r\n%s\r\n--x%d--\r\n", i, i, nested, i)
	}
	for name, raw := range map[string]string{
		"malformed header":       "bad header\n\nbody",
		"empty":                  "\n",
		"html only":              "Subject: ignored\nContent-Type: text/html\n\n<b>hi</b>",
		"attached text":          "Content-Type: text/plain; name=secret.txt\n\nsecret",
		"bad base64":             "Content-Transfer-Encoding: base64\n\n!",
		"unknown encoding":       "Content-Transfer-Encoding: x-uuencode\n\ntext",
		"unknown charset":        "Content-Type: text/plain; charset=invalid\n\nhello",
		"invalid utf8":           "\n\xff",
		"invalid subject":        "Subject: =?UTF-8?B?/w==?=\n\nhello",
		"unterminated multipart": "Content-Type: multipart/mixed; boundary=x\n\n--x\n\ntext",
		"nested too deeply":      nested,
		"large body":             "\n" + strings.Repeat("x", domain.MaxBodyLen+1),
		"long title":             "Subject: " + strings.Repeat("x", domain.MaxTitleLen+1) + "\n\nbody",
		"oversized wire message": "X-Pad: " + strings.Repeat("x", maxMessageBytes) + "\n\nbody",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseMessage(strings.NewReader(raw), "alerts"); !errors.Is(err, domain.ErrInvalid) {
				t.Fatalf("parseMessage() error = %v, want ErrInvalid", err)
			}
		})
	}
}
