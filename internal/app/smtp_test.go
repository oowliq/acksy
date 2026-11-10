package app

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/oowliq/acksy/internal/config"
)

func TestSMTPShutdownInterruptsIdleHandshake(t *testing.T) {
	cfg := config.Default()
	cfg.SMTP.Domain = "notify.example.org"
	cfg.ShutdownTimeout = 25 * time.Millisecond
	a := &App{cfg: cfg}
	l := &pipeListener{conns: make(chan net.Conn), done: make(chan struct{})}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- a.serveSMTP(ctx, tls.NewListener(l, &tls.Config{MinVersion: tls.VersionTLS12})) }()
	client, err := l.dial(ctx, "", "")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SMTP shutdown blocked on client handshake")
	}
	if _, err := client.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Fatalf("client socket remains open: %v", err)
	}
}
