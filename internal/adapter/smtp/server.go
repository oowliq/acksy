package smtp

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"strings"
	"time"

	"github.com/emersion/go-sasl"
	gosmtp "github.com/emersion/go-smtp"

	"github.com/oowliq/acksy/internal/domain"
	"github.com/oowliq/acksy/internal/service"
)

// Server accepts submissions on the listener provided by the application.
type Server struct{ smtp *gosmtp.Server }

func (s *Server) Serve(l net.Listener) error {
	return s.smtp.Serve(submissionListener{Listener: l})
}

func (s *Server) Shutdown(ctx context.Context) error { return s.smtp.Shutdown(ctx) }

func NewServer(ctx context.Context, svc *service.Service, domainName string) *Server {
	server := gosmtp.NewServer(gosmtp.BackendFunc(func(_ *gosmtp.Conn) (gosmtp.Session, error) {
		return &session{ctx: ctx, svc: svc, domain: domainName}, nil
	}))
	server.Domain = domainName
	server.MaxRecipients = 1
	// The application selects implicit TLS or explicit plaintext. Both
	// transports require SMTP AUTH through the same token checks.
	server.AllowInsecureAuth = true
	// parseMessage enforces the inclusive byte limit, including the case
	// where the terminator follows a message of exactly maxMessageBytes.
	server.ReadTimeout = time.Minute
	server.WriteTimeout = 30 * time.Second
	// Untrusted peer input can appear in transport errors. Keep wire diagnostics private.
	server.ErrorLog = log.New(io.Discard, "", 0)
	return &Server{smtp: server}
}

type session struct {
	ctx    context.Context
	svc    *service.Service
	domain string
	secret string
	topic  string
}

func (s *session) Reset() { s.topic = "" }
func (s *session) Logout() error {
	s.secret = ""
	s.Reset()
	return nil
}
func (*session) AuthMechanisms() []string { return []string{sasl.Plain, sasl.Login} }

func (s *session) Auth(mech string) (sasl.Server, error) {
	switch mech {
	case sasl.Plain:
		return sasl.NewPlainServer(func(identity, username, password string) error {
			if identity != "" && identity != username {
				return gosmtp.ErrAuthFailed
			}
			return s.authenticate(password)
		}), nil
	case sasl.Login:
		return &loginServer{authenticate: s.authenticate}, nil
	default:
		return nil, gosmtp.ErrAuthUnknownMechanism
	}
}

// LOGIN accepts an optional initial username followed by the token as password.
type loginServer struct {
	authenticate func(string) error
	wantPassword bool
}

func (l *loginServer) Next(response []byte) ([]byte, bool, error) {
	if !l.wantPassword {
		if response == nil {
			return []byte("Username:"), false, nil
		}
		l.wantPassword = true
		return []byte("Password:"), false, nil
	}
	return nil, true, l.authenticate(string(response))
}

func (s *session) enabled() error {
	if !s.svc.Settings(s.ctx).Adapters.SMTP {
		return &gosmtp.SMTPError{Code: 451, EnhancedCode: gosmtp.EnhancedCode{4, 3, 2}, Message: "SMTP submission is disabled"}
	}
	return nil
}

func (s *session) authenticate(secret string) error {
	if err := s.enabled(); err != nil {
		return err
	}
	if _, err := s.svc.AuthenticateToken(s.ctx, secret); err != nil {
		return submissionError(err)
	}
	s.secret = secret
	return nil
}

func (s *session) Mail(_ string, opts *gosmtp.MailOptions) error {
	if s.secret == "" {
		return &gosmtp.SMTPError{Code: 530, EnhancedCode: gosmtp.EnhancedCode{5, 7, 0}, Message: "Authentication required"}
	}
	if opts.Size > maxMessageBytes {
		return gosmtp.ErrDataTooLarge
	}
	return s.enabled()
}

func (s *session) Rcpt(to string, _ *gosmtp.RcptOptions) error {
	name, host, ok := strings.Cut(to, "@")
	topic := strings.ToLower(name)
	if !ok || !strings.EqualFold(host, s.domain) || domain.ValidateTopicName(topic) != nil {
		return &gosmtp.SMTPError{Code: 550, EnhancedCode: gosmtp.EnhancedCode{5, 1, 1}, Message: "Invalid recipient"}
	}
	s.topic = topic
	return nil
}

func (s *session) Data(r io.Reader) error {
	m, err := parseMessage(r, s.topic)
	if err != nil {
		return submissionError(err)
	}
	if err := s.enabled(); err != nil {
		return err
	}
	p, err := s.svc.AuthenticateToken(s.ctx, s.secret)
	if err != nil {
		return submissionError(err)
	}
	_, err = s.svc.Publish(s.ctx, p, m)
	return submissionError(err)
}

func submissionError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, domain.ErrUnauthorized):
		return gosmtp.ErrAuthFailed
	case errors.Is(err, domain.ErrInvalid):
		return &gosmtp.SMTPError{Code: 554, EnhancedCode: gosmtp.EnhancedCode{5, 6, 0}, Message: "Invalid message"}
	case errors.Is(err, domain.ErrForbidden), errors.Is(err, domain.ErrNotFound), errors.Is(err, domain.ErrConflict):
		return &gosmtp.SMTPError{Code: 550, EnhancedCode: gosmtp.EnhancedCode{5, 7, 1}, Message: "Publication denied"}
	default:
		return &gosmtp.SMTPError{Code: 451, EnhancedCode: gosmtp.EnhancedCode{4, 3, 0}, Message: "Publication temporarily unavailable"}
	}
}
