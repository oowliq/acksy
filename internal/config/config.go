// Package config loads the Acksy startup configuration from an optional YAML
// file and environment variables. Environment variables take precedence over
// the file. Runtime settings live in the database (ADR 0008).
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the startup configuration: only what the server needs before it
// can read its database. Everything else (adapters, limits, Web Push,
// webhooks, actions, retention, sessions) is set by admins in the web UI and
// stored in the database (ADR 0008).
type Config struct {
	Listen          string        `yaml:"listen"`
	BaseURL         string        `yaml:"base_url"`
	DatabaseURL     string        `yaml:"database_url"`
	RedisURL        string        `yaml:"redis_url"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
	// SecureCookies forces the Secure flag on cookies. Defaults to true when
	// BaseURL is https.
	SecureCookies *bool     `yaml:"secure_cookies"`
	Log           LogConfig `yaml:"log"`
	// Admin bootstraps the first admin on an empty instance.
	Admin AdminConfig `yaml:"admin"`
	// Push optionally pins the Web Push keys.
	Push PushConfig `yaml:"push"`
	SMTP SMTPConfig `yaml:"smtp"`
	// Obsolete lists environment variables that are set but no longer read.
	Obsolete []string `yaml:"-"`
}

const SMTPModePlain = "plain"

// SMTPConfig binds the optional SMTP submission listener.
type SMTPConfig struct {
	Mode     string `yaml:"mode"`
	Listen   string `yaml:"listen"`
	Domain   string `yaml:"domain"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

var dnsLabel = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)

func (s SMTPConfig) validate() error {
	if s.Listen == "" {
		return nil
	}
	if s.Mode != "" && s.Mode != "tls" && s.Mode != SMTPModePlain {
		return errors.New("smtp.mode must be tls or plain")
	}
	if s.Mode != SMTPModePlain && (s.KeyFile == "" || s.CertFile == "") {
		return errors.New("smtp.cert_file and smtp.key_file are required with smtp.listen")
	}
	if len(s.Domain) > 253 {
		return errors.New("smtp.domain exceeds 253 bytes")
	}
	for _, label := range strings.Split(s.Domain, ".") {
		if !dnsLabel.MatchString(label) {
			return errors.New("smtp.domain must be a DNS hostname")
		}
	}
	return nil
}

// PushConfig pins the VAPID keys (RFC 8292). Without them Acksy generates
// keys on first start and stores them in the database.
type PushConfig struct {
	VAPIDPublicKey  string `yaml:"vapid_public_key"`
	VAPIDPrivateKey string `yaml:"vapid_private_key"`
}

// OIDCRedirectURL is the callback URL registered at the provider.
func (c Config) OIDCRedirectURL() string {
	return strings.TrimRight(c.BaseURL, "/") + "/api/v1/auth/oidc/callback"
}

// AdminConfig holds bootstrap admin credentials.
type AdminConfig struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// LogConfig controls structured logging.
type LogConfig struct {
	Level  string `yaml:"level"`  // debug | info | warn | error
	Format string `yaml:"format"` // text | json
}

// Default returns the configuration used when nothing is set.
func Default() Config {
	return Config{
		Listen:          ":8080",
		DatabaseURL:     "sqlite://data/acksy.db",
		ShutdownTimeout: 15 * time.Second,
		Log:             LogConfig{Level: "info", Format: "text"},
	}
}

// Load builds the configuration: defaults, then the YAML file at path (if
// non-empty), then ACKSY_* environment variables.
func Load(path string) (Config, error) {
	cfg := Default()

	if path != "" {
		raw, err := os.ReadFile(path) //nolint:gosec // path is an operator-supplied config file
		if err != nil {
			return Config{}, fmt.Errorf("read config file: %w", err)
		}
		if err := yaml.Unmarshal(raw, &cfg); err != nil {
			return Config{}, fmt.Errorf("parse config file %s: %w", path, err)
		}
	}

	if err := applyEnv(&cfg, os.LookupEnv); err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// obsoleteEnv are variables of settings that moved to the web UI (ADR 0006,
// ADR 0008). They are reported, not read.
var obsoleteEnv = []string{
	"ACKSY_SESSION_TTL", "ACKSY_EVENT_RETENTION",
	"ACKSY_OIDC_ISSUER", "ACKSY_OIDC_CLIENT_ID", "ACKSY_OIDC_CLIENT_SECRET", "ACKSY_OIDC_NAME",
	"ACKSY_OIDC_SCOPES", "ACKSY_OIDC_USERNAME_CLAIM", "ACKSY_OIDC_ROLE_CLAIM", "ACKSY_OIDC_ADMIN_VALUES",
	"ACKSY_OIDC_AUTO_CREATE", "ACKSY_OIDC_DISABLE_PASSWORD_LOGIN",
	"ACKSY_ADAPTERS_NTFY", "ACKSY_ADAPTERS_GOTIFY",
	"ACKSY_PUBLISH_LIMIT_TOKEN", "ACKSY_PUBLISH_LIMIT_USER",
	"ACKSY_PUSH_ENABLED", "ACKSY_VAPID_SUBJECT",
	"ACKSY_WEBHOOKS_ENABLED", "ACKSY_WEBHOOK_ALLOW_PRIVATE", "ACKSY_WEBHOOK_FAILED_RETENTION",
	"ACKSY_ACTIONS_ENABLED", "ACKSY_ACTIONS_ALLOW_PRIVATE",
	"ACKSY_RETENTION_READ", "ACKSY_RETENTION_READ_BY_PRIORITY", "ACKSY_RETENTION_UNREAD", "ACKSY_RETENTION_MAX_PER_USER",
}

type lookupFunc func(string) (string, bool)

func applyEnv(cfg *Config, lookup lookupFunc) error {
	str := map[string]*string{
		"ACKSY_LISTEN":         &cfg.Listen,
		"ACKSY_SMTP_MODE":      &cfg.SMTP.Mode,
		"ACKSY_SMTP_LISTEN":    &cfg.SMTP.Listen,
		"ACKSY_SMTP_DOMAIN":    &cfg.SMTP.Domain,
		"ACKSY_SMTP_CERT_FILE": &cfg.SMTP.CertFile,
		"ACKSY_SMTP_KEY_FILE":  &cfg.SMTP.KeyFile,
		"ACKSY_BASE_URL":       &cfg.BaseURL,
		"ACKSY_DATABASE_URL":   &cfg.DatabaseURL,
		"ACKSY_REDIS_URL":      &cfg.RedisURL,
		"ACKSY_LOG_LEVEL":      &cfg.Log.Level,
		"ACKSY_LOG_FORMAT":     &cfg.Log.Format,
		"ACKSY_ADMIN_USERNAME": &cfg.Admin.Username,
		"ACKSY_ADMIN_PASSWORD": &cfg.Admin.Password,

		"ACKSY_VAPID_PUBLIC_KEY":  &cfg.Push.VAPIDPublicKey,
		"ACKSY_VAPID_PRIVATE_KEY": &cfg.Push.VAPIDPrivateKey,
	}
	for key, dst := range str {
		if v, ok := lookup(key); ok {
			*dst = v
		}
	}
	if v, ok := lookup("ACKSY_SHUTDOWN_TIMEOUT"); ok {
		d, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("ACKSY_SHUTDOWN_TIMEOUT: %w", err)
		}
		cfg.ShutdownTimeout = d
	}
	if v, ok := lookup("ACKSY_SECURE_COOKIES"); ok {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("ACKSY_SECURE_COOKIES: %w", err)
		}
		cfg.SecureCookies = &b
	}
	cfg.Obsolete = nil
	for _, key := range obsoleteEnv {
		if _, ok := lookup(key); ok {
			cfg.Obsolete = append(cfg.Obsolete, key)
		}
	}
	return nil
}

// Validate reports configuration errors.
func (c Config) Validate() error {
	var errs []error
	if err := c.SMTP.validate(); err != nil {
		errs = append(errs, err)
	}
	if c.Listen == "" {
		errs = append(errs, errors.New("listen must not be empty"))
	}
	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("database_url must not be empty"))
	}
	if _, err := c.Log.SlogLevel(); err != nil {
		errs = append(errs, err)
	}
	switch strings.ToLower(c.Log.Format) {
	case "text", "json":
	default:
		errs = append(errs, fmt.Errorf("log.format must be text or json, got %q", c.Log.Format))
	}
	if c.ShutdownTimeout <= 0 {
		errs = append(errs, errors.New("shutdown_timeout must be positive"))
	}
	if (c.Admin.Username == "") != (c.Admin.Password == "") {
		errs = append(errs, errors.New("admin username and password must be set together"))
	}
	if (c.Push.VAPIDPublicKey == "") != (c.Push.VAPIDPrivateKey == "") {
		errs = append(errs, errors.New("push.vapid_public_key and push.vapid_private_key must be set together"))
	}
	return errors.Join(errs...)
}

// SlogLevel parses the configured level.
func (l LogConfig) SlogLevel() (slog.Level, error) {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(l.Level)); err != nil {
		return 0, fmt.Errorf("log.level: %w", err)
	}
	return lvl, nil
}

// NewLogger builds a slog logger according to the config.
func (l LogConfig) NewLogger() *slog.Logger {
	lvl, _ := l.SlogLevel()
	opts := &slog.HandlerOptions{Level: lvl}
	if strings.EqualFold(l.Format, "json") {
		return slog.New(slog.NewJSONHandler(os.Stderr, opts))
	}
	return slog.New(slog.NewTextHandler(os.Stderr, opts))
}

// UseSecureCookies resolves the effective Secure cookie setting.
func (c Config) UseSecureCookies() bool {
	if c.SecureCookies != nil {
		return *c.SecureCookies
	}
	return strings.HasPrefix(strings.ToLower(c.BaseURL), "https://")
}
