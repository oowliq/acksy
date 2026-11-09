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
		{"short session", func(c *Config) { c.SessionTTL = time.Minute }},
		{"short event retention", func(c *Config) { c.EventRetention = time.Minute }},
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

func TestOIDCEnv(t *testing.T) {
	cfg := Default()
	err := applyEnv(&cfg, mapLookup(map[string]string{
		"ACKSY_BASE_URL":                    "https://acksy.example/",
		"ACKSY_OIDC_ISSUER":                 "https://idp.example/app",
		"ACKSY_OIDC_CLIENT_ID":              "acksy",
		"ACKSY_OIDC_CLIENT_SECRET":          "s3cret",
		"ACKSY_OIDC_SCOPES":                 "openid profile email groups",
		"ACKSY_OIDC_ADMIN_VALUES":           "acksy-admins, ops",
		"ACKSY_OIDC_AUTO_CREATE":            "false",
		"ACKSY_OIDC_DISABLE_PASSWORD_LOGIN": "true",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	o := cfg.OIDC
	if !o.Enabled() || o.ClientSecret != "s3cret" || len(o.Scopes) != 4 || o.AutoCreate || !o.DisablePasswordLogin {
		t.Fatalf("oidc env not applied: %+v", o)
	}
	if len(o.AdminValues) != 2 || o.AdminValues[1] != "ops" {
		t.Fatalf("admin values: %q", o.AdminValues)
	}
	if got := cfg.OIDCRedirectURL(); got != "https://acksy.example/api/v1/auth/oidc/callback" {
		t.Fatalf("redirect url %q", got)
	}
}

func TestPublishLimitEnv(t *testing.T) {
	cfg := Default()
	if cfg.PublishLimit.PerToken != 60 || cfg.PublishLimit.PerUser != 120 {
		t.Fatalf("defaults: %+v", cfg.PublishLimit)
	}
	if err := applyEnv(&cfg, mapLookup(map[string]string{
		"ACKSY_PUBLISH_LIMIT_TOKEN": "10", "ACKSY_PUBLISH_LIMIT_USER": "0",
	})); err != nil {
		t.Fatal(err)
	}
	if cfg.PublishLimit.PerToken != 10 || cfg.PublishLimit.PerUser != 0 || cfg.Validate() != nil {
		t.Fatalf("env: %+v %v", cfg.PublishLimit, cfg.Validate())
	}
	if err := applyEnv(&cfg, mapLookup(map[string]string{"ACKSY_PUBLISH_LIMIT_USER": "lots"})); err == nil {
		t.Fatal("non-integer accepted")
	}
	cfg.PublishLimit.PerToken = -1
	if cfg.Validate() == nil {
		t.Fatal("negative limit accepted")
	}
}

func TestPushConfig(t *testing.T) {
	cfg := Default()
	if !cfg.Push.Enabled || cfg.PushSubject() != "" {
		t.Fatalf("defaults: %+v %q", cfg.Push, cfg.PushSubject())
	}
	cfg.BaseURL = "https://acksy.example/"
	if got := cfg.PushSubject(); got != "https://acksy.example" {
		t.Fatalf("subject from base url: %q", got)
	}
	if err := applyEnv(&cfg, mapLookup(map[string]string{
		"ACKSY_VAPID_SUBJECT": "mailto:ops@example.com", "ACKSY_PUSH_ENABLED": "false",
	})); err != nil {
		t.Fatal(err)
	}
	if cfg.PushSubject() != "ops@example.com" || cfg.Push.Enabled {
		t.Fatalf("env: %+v %q", cfg.Push, cfg.PushSubject())
	}
	cfg.Push.VAPIDPublicKey = "only-half"
	if cfg.Validate() == nil {
		t.Fatal("half a key pair accepted")
	}
	cfg.Push.VAPIDPublicKey, cfg.Push.Subject = "", "ops"
	if cfg.Validate() == nil {
		t.Fatal("bad subject accepted")
	}
}

func TestWebhooksConfig(t *testing.T) {
	cfg := Default()
	if !cfg.Webhooks.Enabled || cfg.Webhooks.AllowPrivate || cfg.Webhooks.FailedRetention != 7*24*time.Hour {
		t.Fatalf("defaults: %+v", cfg.Webhooks)
	}
	if err := applyEnv(&cfg, mapLookup(map[string]string{
		"ACKSY_WEBHOOK_ALLOW_PRIVATE": "true", "ACKSY_WEBHOOK_FAILED_RETENTION": "72h",
	})); err != nil {
		t.Fatal(err)
	}
	if !cfg.Webhooks.AllowPrivate || cfg.Webhooks.FailedRetention != 72*time.Hour || cfg.Validate() != nil {
		t.Fatalf("env: %+v", cfg.Webhooks)
	}
	cfg.Webhooks.FailedRetention = time.Minute
	if cfg.Validate() == nil {
		t.Fatal("tiny retention accepted")
	}
}

func TestRetentionConfig(t *testing.T) {
	cfg := Default()
	if time.Duration(cfg.Retention.ReadTTL) != 90*24*time.Hour || cfg.Retention.UnreadTTL != 0 || cfg.Retention.MaxPerUser != 10_000 {
		t.Fatalf("defaults: %+v", cfg.Retention)
	}
	if err := applyEnv(&cfg, mapLookup(map[string]string{
		"ACKSY_RETENTION_READ": "30d", "ACKSY_RETENTION_UNREAD": "8760h",
		"ACKSY_RETENTION_READ_BY_PRIORITY": "1=7d, 2=14d", "ACKSY_RETENTION_MAX_PER_USER": "500",
	})); err != nil {
		t.Fatal(err)
	}
	r := cfg.Retention
	if time.Duration(r.ReadTTL) != 30*24*time.Hour || time.Duration(r.UnreadTTL) != 8760*time.Hour ||
		time.Duration(r.ReadTTLByPriority[2]) != 14*24*time.Hour || r.MaxPerUser != 500 || cfg.Validate() != nil {
		t.Fatalf("env: %+v %v", r, cfg.Validate())
	}
	for _, bad := range []string{"1=soon", "x=1d", "7"} {
		c := Default()
		if err := applyEnv(&c, mapLookup(map[string]string{"ACKSY_RETENTION_READ_BY_PRIORITY": bad})); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	cfg.Retention.ReadTTLByPriority = map[int]Duration{9: 1}
	if cfg.Validate() == nil {
		t.Fatal("priority 9 accepted")
	}
}

func TestRetentionYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "acksy.yaml")
	yml := "retention:\n  read_ttl: 14d\n  read_ttl_by_priority:\n    1: 2d\n  max_per_user: 0\n"
	if err := os.WriteFile(path, []byte(yml), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if time.Duration(cfg.Retention.ReadTTL) != 14*24*time.Hour || time.Duration(cfg.Retention.ReadTTLByPriority[1]) != 48*time.Hour ||
		cfg.Retention.MaxPerUser != 0 {
		t.Fatalf("yaml: %+v", cfg.Retention)
	}
}

func TestActionsConfig(t *testing.T) {
	cfg := Default()
	if !cfg.Actions.Enabled || cfg.Actions.AllowPrivate {
		t.Fatalf("defaults: %+v", cfg.Actions)
	}
	if err := applyEnv(&cfg, mapLookup(map[string]string{"ACKSY_ACTIONS_ALLOW_PRIVATE": "true", "ACKSY_ACTIONS_ENABLED": "false"})); err != nil {
		t.Fatal(err)
	}
	if !cfg.Actions.AllowPrivate || cfg.Actions.Enabled {
		t.Fatalf("env: %+v", cfg.Actions)
	}
}
