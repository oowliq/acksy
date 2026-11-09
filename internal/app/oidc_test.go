package app

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/oowliq/acksy/internal/broker/memory"
	"github.com/oowliq/acksy/internal/config"
	"github.com/oowliq/acksy/internal/service"
	"github.com/oowliq/acksy/internal/store"
)

func newTestService(t *testing.T) *service.Service {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, "sqlite://:memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	return service.New(store.New(db), memory.New(), slog.New(slog.DiscardHandler), service.Options{
		OIDCRedirectURL: "https://acksy.example/api/v1/auth/oidc/callback",
	})
}

func envOIDC() config.OIDCConfig {
	o := config.Default().OIDC
	o.Issuer, o.ClientID, o.ClientSecret, o.AutoCreate = "https://idp.example", "acksy", "s3cret", false
	return o
}

func TestImportOIDCFromEnv(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	var logs bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logs, nil))

	if err := importOIDCFromEnv(ctx, svc, envOIDC(), log); err != nil {
		t.Fatal(err)
	}
	s, _, err := svc.OIDCSettings(ctx, service.SystemPrincipal())
	if err != nil || !s.Enabled || s.Issuer != "https://idp.example" || s.ClientSecret != "s3cret" || s.AutoCreate {
		t.Fatalf("imported: %+v %v", s, err)
	}
	if !strings.Contains(logs.String(), "imported") {
		t.Fatalf("no import log: %s", logs.String())
	}

	// A second start with different env values changes nothing and warns.
	logs.Reset()
	other := envOIDC()
	other.Issuer = "https://elsewhere.example"
	if err := importOIDCFromEnv(ctx, svc, other, log); err != nil {
		t.Fatal(err)
	}
	if s, _, _ := svc.OIDCSettings(ctx, service.SystemPrincipal()); s.Issuer != "https://idp.example" {
		t.Fatalf("overwritten: %+v", s)
	}
	if !strings.Contains(logs.String(), "level=WARN") {
		t.Fatalf("no warning: %s", logs.String())
	}
}

func TestImportOIDCFromEnvNothingSet(t *testing.T) {
	svc := newTestService(t)
	var logs bytes.Buffer
	if err := importOIDCFromEnv(context.Background(), svc, config.Default().OIDC, slog.New(slog.NewTextHandler(&logs, nil))); err != nil {
		t.Fatal(err)
	}
	if logs.Len() != 0 {
		t.Fatalf("unexpected logs: %s", logs.String())
	}
}

func TestImportOIDCFromEnvNeedsBaseURL(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "sqlite://:memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	svc := service.New(store.New(db), memory.New(), slog.New(slog.DiscardHandler), service.Options{})
	if err := importOIDCFromEnv(ctx, svc, envOIDC(), slog.New(slog.DiscardHandler)); err == nil {
		t.Fatal("startup must fail: OIDC from the environment without ACKSY_BASE_URL")
	}
}
