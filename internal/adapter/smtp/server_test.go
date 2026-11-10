package smtp

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/textproto"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/emersion/go-sasl"
	gosmtp "github.com/emersion/go-smtp"

	"github.com/oowliq/acksy/internal/broker/memory"
	"github.com/oowliq/acksy/internal/domain"
	"github.com/oowliq/acksy/internal/service"
	"github.com/oowliq/acksy/internal/store"
)

type pipeListener struct {
	conns chan net.Conn
	done  chan struct{}
}

func (l *pipeListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.conns:
		return c, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}
func (l *pipeListener) Addr() net.Addr { return &net.TCPAddr{} }
func (l *pipeListener) Close() error   { close(l.done); return nil }

func check(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func wantStatus(t *testing.T, err error, code int) {
	t.Helper()
	var se *gosmtp.SMTPError
	if !errors.As(err, &se) || se.Code != code {
		t.Fatalf("want SMTP %d, got %v", code, err)
	}
}

func TestSMTPSubmission(t *testing.T) {
	t.Run("tls", func(t *testing.T) { testSMTPSubmission(t, true) })
	t.Run("plain", func(t *testing.T) { testSMTPSubmission(t, false) })
}

func testSMTPSubmission(t *testing.T, secure bool) {
	ctx := t.Context()
	db, err := store.Open(ctx, "sqlite://:memory:")
	check(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Migrate(ctx)
	check(t, err)
	b := memory.New()
	t.Cleanup(func() { _ = b.Close() })
	svc := service.New(store.New(db), b, slog.New(slog.NewTextHandler(io.Discard, nil)), service.Options{})
	user, err := svc.CreateUser(ctx, service.SystemPrincipal(), service.CreateUserInput{Username: "monit", Password: "password123", Role: domain.RoleAdmin})
	check(t, err)
	p := service.UserPrincipal(user)
	_, err = svc.CreateTopic(ctx, p, service.CreateTopicInput{Name: "opnsense"})
	check(t, err)
	token, err := svc.CreateToken(ctx, p, service.CreateTokenInput{Name: "monit", Scopes: []string{string(domain.ScopePublish)}, Topics: []string{"opnsense"}})
	check(t, err)
	settings := svc.Settings(ctx)
	setEnabled := func(enabled bool) {
		settings.Adapters.SMTP = enabled
		_, _, err := svc.SaveServerSettings(ctx, p, settings)
		check(t, err)
	}
	setEnabled(true)
	const host = "notify.example.org"
	open := serveTestSMTP(t, svc, host, secure)
	dial := func() *gosmtp.Client {
		conn, _, _ := open()
		c := gosmtp.NewClient(conn)
		c.CommandTimeout = 3 * time.Second
		c.SubmissionTimeout = 3 * time.Second
		return c
	}
	const sender = "root@firewall.local"
	const recipient = "opnsense@NOTIFY.EXAMPLE.ORG"
	const letter = "From: root@firewall.local\r\nSubject: Monit alert\r\n\r\nService failed\r\n"
	c := dial()
	defer c.Close()
	wantStatus(t, c.Mail(sender, nil), 530)
	wantStatus(t, c.Auth(sasl.NewPlainClient("", "arbitrary", "invalid")), 535)
	check(t, c.Auth(sasl.NewLoginClient("arbitrary", token.Secret)))
	check(t, c.Mail(sender, nil))
	wantStatus(t, c.Rcpt("opnsense@elsewhere.example", nil), 550)
	wantStatus(t, c.Rcpt("bad+topic@"+host, nil), 550)
	check(t, c.Rcpt(recipient, nil))
	wantStatus(t, c.Rcpt("other@"+host, nil), 452)
	w, err := c.Data()
	check(t, err)
	_, err = io.WriteString(w, letter)
	check(t, err)
	check(t, w.Close())
	items, err := svc.ListNotifications(ctx, p, service.ListQuery{})
	check(t, err)
	if len(items.Items) != 1 {
		t.Fatalf("notifications: %d", len(items.Items))
	}
	n := items.Items[0]
	if n.Title != "Monit alert" || strings.TrimSpace(n.Body) != "Service failed" || n.Source != "smtp" || n.Topic == nil || n.Topic.Name != "opnsense" || n.TokenID != token.Token.ID {
		t.Fatalf("notification: %+v", n)
	}
	wantStatus(t, c.SendMail(sender, []string{"other@" + host}, strings.NewReader(letter)), 550)
	settings.PublishLimit.PerToken = 1
	setEnabled(true)
	wantStatus(t, c.SendMail(sender, []string{"other@" + host}, strings.NewReader(letter)), 550)
	wantStatus(t, c.SendMail(sender, []string{recipient}, strings.NewReader(letter)), 451)
	settings.PublishLimit.PerToken = 60
	setEnabled(false)
	wantStatus(t, c.Mail(sender, nil), 451)
	setEnabled(true)
	// A truncated final chunk must not publish a syntactically valid partial email.
	for _, transfer := range []string{"truncated-data", "partial-terminator", "bdat-after-data", "bdat-pipelined", "data-with-bdat-line", "limit", "oversized"} {
		t.Run(transfer, func(t *testing.T) {
			before, err := svc.ListNotifications(ctx, p, service.ListQuery{})
			check(t, err)
			conn, raw, closed := open()
			wire := textproto.NewConn(conn)
			defer wire.Close()
			response := func(code int) {
				_, _, err := wire.ReadResponse(code)
				check(t, err)
			}
			command := func(line string, code int) {
				check(t, wire.PrintfLine("%s", line))
				response(code)
			}
			response(220)
			check(t, wire.PrintfLine("EHLO monit.local"))
			_, caps, err := wire.ReadResponse(250)
			check(t, err)
			if strings.Contains(caps, "CHUNKING") || !strings.Contains(caps, "SIZE 262144") {
				t.Fatalf("capabilities: %s", caps)
			}
			command("AUTH PLAIN "+base64.StdEncoding.EncodeToString([]byte("\x00monit\x00"+token.Secret)), 235)
			command("MAIL FROM:<"+sender+">", 250)
			command("RCPT TO:<"+recipient+">", 250)
			switch transfer {
			case "truncated-data":
				command("DATA", 354)
				_, err = io.WriteString(conn, letter)
			case "partial-terminator":
				command("DATA", 354)
				_, err = io.WriteString(conn, letter+".\r")
			case "bdat-after-data":
				command("DATA", 354)
				_, err = io.WriteString(conn, letter+".\r\nBDAT 1000 LAST\r\n"+letter)
				response(250)
				response(502)
			case "bdat-pipelined":
				_, err = io.WriteString(conn, "bDaT 1000 LAST\r\n"+letter+"DATA\r\n"+letter+".\r\n")
				response(502)
			case "data-with-bdat-line":
				command("DATA", 354)
				_, err = io.WriteString(conn, letter+"BDAT 1000 LAST\r\n.\r\n")
				response(250)
			case "limit", "oversized":
				command("DATA", 354)
				size := maxMessageBytes
				if transfer == "oversized" {
					size++
				}
				// A preamble is ignored by the MIME parser; keep the published body small.
				const header = "Content-Type: multipart/mixed; boundary=cut\r\n\r\n"
				const body = "\r\n--cut\r\nContent-Type: text/plain\r\n\r\naccepted\r\n--cut--\r\n"
				preambleLen := size - len(header) - len(body)
				padding := strings.Repeat(strings.Repeat("a", 98)+"\r\n", preambleLen/100) + strings.Repeat("a", preambleLen%100)
				_, err = io.WriteString(conn, header+padding+body+".\r\n")
				if transfer == "limit" {
					response(250)
				} else {
					response(554)
				}

			}
			check(t, err)
			// Cut the transport without exchanging TLS close alerts on the unbuffered pipe.
			_ = raw.Close()
			select {
			case <-closed:
			case <-time.After(5 * time.Second):
				t.Fatal("SMTP handler did not close")
			}
			after, err := svc.ListNotifications(ctx, p, service.ListQuery{})
			check(t, err)
			want := 0
			if transfer == "data-with-bdat-line" || transfer == "bdat-after-data" || transfer == "limit" {
				want = 1
			}
			if delta := len(after.Items) - len(before.Items); delta != want {
				t.Fatalf("published %d messages, want %d", delta, want)
			}
		})
	}
	c2 := dial()
	defer c2.Close()
	check(t, c2.Auth(sasl.NewPlainClient("", "arbitrary", token.Secret)))
	check(t, c2.Mail(sender, nil))
	check(t, c2.Rcpt(recipient, nil))
	check(t, svc.DeleteToken(ctx, p, token.Token.ID))
	w, err = c2.Data()
	check(t, err)
	_, err = io.WriteString(w, letter)
	check(t, err)
	wantStatus(t, w.Close(), 535)
	items, err = svc.ListNotifications(ctx, p, service.ListQuery{})
	check(t, err)
	if len(items.Items) != 4 {
		t.Fatalf("rejected submission published: %d notifications", len(items.Items))
	}
}

type observedConn struct {
	net.Conn
	done chan struct{}
	once sync.Once
}

func (c *observedConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(func() { close(c.done) })
	return err
}

func serveTestSMTP(t *testing.T, svc *service.Service, host string, secure bool) func() (net.Conn, net.Conn, <-chan struct{}) {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	check(t, err)
	cert := &x509.Certificate{SerialNumber: big.NewInt(1), DNSNames: []string{host}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, pub, key)
	check(t, err)
	cert, err = x509.ParseCertificate(der)
	check(t, err)
	roots := x509.NewCertPool()
	roots.AddCert(cert)
	listener := &pipeListener{conns: make(chan net.Conn), done: make(chan struct{})}
	server := NewServer(t.Context(), svc, host)
	served := make(chan error, 1)
	var transport net.Listener = listener
	if secure {
		transport = tls.NewListener(listener, &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}})
	}
	go func() {
		served <- server.Serve(transport)
	}()
	t.Cleanup(func() {
		check(t, server.Shutdown(context.Background()))
		check(t, <-served)
	})
	return func() (net.Conn, net.Conn, <-chan struct{}) {
		s, c := net.Pipe()
		closed := make(chan struct{})
		listener.conns <- &observedConn{Conn: s, done: closed}
		conn := c
		if secure {
			conn = tls.Client(c, &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, ServerName: host})
		}
		check(t, conn.SetDeadline(time.Now().Add(5*time.Second)))
		return conn, c, closed
	}
}
