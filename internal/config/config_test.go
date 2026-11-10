package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func mapLookup(m map[string]string) lookupFunc {
	return func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}
}

func TestDefaultIsValid(t *testing.T) {
	if err := Default().Validate(); err != nil {
		t.Fatalf("default config invalid: %v", err)
	}
}

func TestApplyEnv(t *testing.T) {
	cfg := Default()
	err := applyEnv(&cfg, mapLookup(map[string]string{
		"ACKSY_LISTEN":           ":9000",
		"ACKSY_DATABASE_URL":     "postgres://x",
		"ACKSY_REDIS_URL":        "redis://r:6379/0",
		"ACKSY_LOG_FORMAT":       "json",
		"ACKSY_SHUTDOWN_TIMEOUT": "3s",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != ":9000" || cfg.DatabaseURL != "postgres://x" || cfg.RedisURL != "redis://r:6379/0" ||
		cfg.Log.Format != "json" || cfg.ShutdownTimeout != 3*time.Second {
		t.Fatalf("env not applied: %+v", cfg)
	}
}

func TestApplyEnvBadDuration(t *testing.T) {
	cfg := Default()
	if err := applyEnv(&cfg, mapLookup(map[string]string{"ACKSY_SHUTDOWN_TIMEOUT": "soon"})); err == nil {
		t.Fatal("expected error")
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{"empty listen", func(c *Config) { c.Listen = "" }},
		{"empty db", func(c *Config) { c.DatabaseURL = "" }},
		{"bad level", func(c *Config) { c.Log.Level = "loud" }},
		{"bad format", func(c *Config) { c.Log.Format = "xml" }},
		{"zero timeout", func(c *Config) { c.ShutdownTimeout = 0 }},
		{"half a vapid key pair", func(c *Config) { c.Push.VAPIDPublicKey = "only-half" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			tt.mutate(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestLoadYAMLThenEnv(t *testing.T) {
	path := filepath.Join(t.TempDir(), "acksy.yaml")
	yaml := "listen: \":7000\"\nredis_url: redis://file\nlog:\n  level: debug\n"
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ACKSY_REDIS_URL", "redis://env")

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != ":7000" {
		t.Errorf("listen from yaml: got %q", cfg.Listen)
	}
	if cfg.RedisURL != "redis://env" {
		t.Errorf("env must override yaml: got %q", cfg.RedisURL)
	}
	if cfg.Log.Level != "debug" || cfg.Log.Format != "text" {
		t.Errorf("log merged incorrectly: %+v", cfg.Log)
	}
}

func TestSecureCookies(t *testing.T) {
	cfg := Default()
	if cfg.UseSecureCookies() {
		t.Fatal("plain default must not force secure")
	}
	cfg.BaseURL = "https://acksy.example.com"
	if !cfg.UseSecureCookies() {
		t.Fatal("https base url must imply secure")
	}
	if err := applyEnv(&cfg, mapLookup(map[string]string{"ACKSY_SECURE_COOKIES": "false"})); err != nil {
		t.Fatal(err)
	}
	if cfg.UseSecureCookies() {
		t.Fatal("explicit false must win")
	}
}

func TestAdminMustBeComplete(t *testing.T) {
	cfg := Default()
	cfg.Admin.Username = "admin"
	if cfg.Validate() == nil {
		t.Fatal("username without password accepted")
	}
	cfg.Admin.Password = "secret"
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestRedirectURL(t *testing.T) {
	cfg := Default()
	cfg.BaseURL = "https://acksy.example/"
	if got := cfg.OIDCRedirectURL(); got != "https://acksy.example/api/v1/auth/oidc/callback" {
		t.Fatalf("redirect url %q", got)
	}
}

func TestVAPIDKeysEnv(t *testing.T) {
	cfg := Default()
	if err := applyEnv(&cfg, mapLookup(map[string]string{
		"ACKSY_VAPID_PUBLIC_KEY": "pub", "ACKSY_VAPID_PRIVATE_KEY": "priv",
	})); err != nil {
		t.Fatal(err)
	}
	if cfg.Push.VAPIDPublicKey != "pub" || cfg.Push.VAPIDPrivateKey != "priv" || cfg.Validate() != nil {
		t.Fatalf("env: %+v", cfg.Push)
	}
}

// Settings that moved to the web UI are reported, not read (ADR 0008).
func TestObsoleteEnvReported(t *testing.T) {
	cfg := Default()
	if err := applyEnv(&cfg, mapLookup(map[string]string{
		"ACKSY_RETENTION_READ": "not even a duration", "ACKSY_OIDC_ISSUER": "https://idp", "ACKSY_LISTEN": ":9000",
	})); err != nil {
		t.Fatalf("obsolete values must not be parsed: %v", err)
	}
	if len(cfg.Obsolete) != 2 || cfg.Obsolete[0] != "ACKSY_OIDC_ISSUER" || cfg.Obsolete[1] != "ACKSY_RETENTION_READ" {
		t.Fatalf("obsolete: %q", cfg.Obsolete)
	}
	clean := Default()
	if err := applyEnv(&clean, mapLookup(map[string]string{"ACKSY_LISTEN": ":9000"})); err != nil || clean.Obsolete != nil {
		t.Fatalf("clean env: %q %v", clean.Obsolete, err)
	}
}
