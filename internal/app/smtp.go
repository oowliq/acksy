package app

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"

	"github.com/oowliq/acksy/internal/adapter/smtp"
	"github.com/oowliq/acksy/internal/config"
)

// smtpListener closes any remaining client sockets when the shutdown grace
// period expires, including clients still negotiating TLS.
type smtpListener struct {
	net.Listener
	connections context.Context
}

type smtpConn struct {
	net.Conn
	stop func() bool
}

func (c *smtpConn) Close() error {
	c.stop()
	return c.Conn.Close()
}

func (l smtpListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	stop := context.AfterFunc(l.connections, func() { _ = c.Close() })
	return &smtpConn{Conn: c, stop: stop}, nil
}

func listenSMTP(ctx context.Context, cfg config.SMTPConfig) (net.Listener, *tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
	if err != nil {
		return nil, nil, fmt.Errorf("load SMTP certificate: %w", err)
	}
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", cfg.Listen)
	if err != nil {
		return nil, nil, fmt.Errorf("listen SMTP %s: %w", cfg.Listen, err)
	}
	return ln, &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}, nil
}

func (a *App) serveSMTP(ctx context.Context, ln net.Listener, tlsConfig *tls.Config) error {
	// Allow in-flight publications to finish during graceful shutdown.
	base := context.WithoutCancel(ctx)
	connections, closeClients := context.WithCancel(base)
	defer closeClients()
	server := smtp.NewServer(connections, a.svc, a.cfg.SMTP.Domain)
	done := make(chan error, 1)
	go func() {
		done <- server.ServeTLS(smtpListener{ln, connections}, tlsConfig)
	}()
	var serveErr error
	select {
	case <-ctx.Done():
		// Stop Accept before Shutdown waits on the server's connection group.
		_ = ln.Close()
		serveErr = <-done
	case serveErr = <-done:
	}
	if errors.Is(serveErr, net.ErrClosed) {
		serveErr = nil
	}
	grace, cancel := context.WithTimeout(base, a.cfg.ShutdownTimeout)
	defer cancel()
	stop := context.AfterFunc(grace, closeClients)
	defer stop()
	// Wait for handlers to release the service before App.Close closes the DB.
	err := server.Shutdown(base)
	if errors.Is(err, net.ErrClosed) {
		err = nil
	}
	return errors.Join(serveErr, err)
}
