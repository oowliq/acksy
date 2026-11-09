// Package config loads Acksy configuration from an optional YAML file and
// environment variables. Environment variables take precedence over the file.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the full server configuration.
type Config struct {
	Listen          string        `yaml:"listen"`
	BaseURL         string        `yaml:"base_url"`
	DatabaseURL     string        `yaml:"database_url"`
	RedisURL        string        `yaml:"redis_url"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
	SessionTTL      time.Duration `yaml:"session_ttl"`
	// EventRetention is how long realtime events are kept for resuming streams.
	EventRetention time.Duration `yaml:"event_retention"`
	// SecureCookies forces the Secure flag on cookies. Defaults to true when
	// BaseURL is https.
	SecureCookies *bool     `yaml:"secure_cookies"`
	Log           LogConfig `yaml:"log"`
	// Admin bootstraps the first admin on an empty instance.
	Admin AdminConfig `yaml:"admin"`
	// OIDC enables sign-in through an external OpenID Connect provider.
	OIDC OIDCConfig `yaml:"oidc"`
	// Adapters toggles the ingest adapters.
	Adapters AdaptersConfig `yaml:"adapters"`
	// PublishLimit caps publish requests per minute; 0 disables a limit.
	PublishLimit PublishLimitConfig `yaml:"publish_limit"`
	// Push configures Web Push.
	Push PushConfig `yaml:"push"`
	// Webhooks configures outgoing webhooks.
	Webhooks WebhooksConfig `yaml:"webhooks"`
	// Retention deletes old notifications.
	Retention RetentionConfig `yaml:"retention"`
	// Actions configures http action buttons.
	Actions ActionsConfig `yaml:"actions"`
}

// ActionsConfig configures http actions run by the server.
type ActionsConfig struct {
	// Enabled lets recipients run http actions (default true).
	Enabled bool `yaml:"enabled"`
	// AllowPrivate lets actions reach private and loopback addresses (LAN
	// services such as Home Assistant). The URL is set by the publisher,
	// so enable it only if you trust everyone who can publish.
	AllowPrivate bool `yaml:"allow_private"`
}

// RetentionConfig is the instance-wide notification retention; topics can
// override ReadTTL and add a count limit. Zero disables a rule.
type RetentionConfig struct {
	// ReadTTL deletes read/archived notifications after their last change.
	ReadTTL Duration `yaml:"read_ttl"`
	// ReadTTLByPriority overrides ReadTTL for some priorities, e.g. {1: 7d}.
	ReadTTLByPriority map[int]Duration `yaml:"read_ttl_by_priority"`
	// UnreadTTL deletes unread notifications (off by default).
	UnreadTTL Duration `yaml:"unread_ttl"`
	// MaxPerUser caps each user's notifications, oldest read ones first.
	MaxPerUser int `yaml:"max_per_user"`
}

// Duration is a time.Duration that also accepts whole days ("90d").
type Duration time.Duration

// ParseDuration parses Go durations ("36h") and days ("90d").
func ParseDuration(v string) (Duration, error) {
	v = strings.TrimSpace(v)
	if days, ok := strings.CutSuffix(v, "d"); ok {
		n, err := strconv.Atoi(days)
		if err != nil || n < 0 {
			return 0, fmt.Errorf("invalid duration %q", v)
		}
		return Duration(time.Duration(n) * 24 * time.Hour), nil
	}
	d, err := time.ParseDuration(v)
	return Duration(d), err
}

// UnmarshalYAML accepts the same forms as ParseDuration.
func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	parsed, err := ParseDuration(node.Value)
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}

// WebhooksConfig configures outgoing webhooks.
type WebhooksConfig struct {
	// Enabled turns webhook delivery on (default true).
	Enabled bool `yaml:"enabled"`
	// AllowPrivate lets webhooks reach private and loopback addresses
	// (LAN services such as Home Assistant). Off by default: users could
	// otherwise make the server call into its own network (SSRF).
	AllowPrivate bool `yaml:"allow_private"`
	// FailedRetention is how long failed deliveries are kept for inspection.
	FailedRetention time.Duration `yaml:"failed_retention"`
}

// PushConfig configures Web Push (VAPID, RFC 8292).
type PushConfig struct {
	// Enabled turns Web Push on (default true).
	Enabled bool `yaml:"enabled"`
	// VAPIDPublicKey / VAPIDPrivateKey override the keys Acksy generates
	// and stores in the database on first start. Set both or neither.
	VAPIDPublicKey  string `yaml:"vapid_public_key"`
	VAPIDPrivateKey string `yaml:"vapid_private_key"`
	// Subject identifies the operator to push services: an https URL or an
	// e-mail address. Defaults to BaseURL when that is https. Safari's push
	// service rejects pushes without a valid subject.
	Subject string `yaml:"subject"`
}

// PushSubject resolves the VAPID subject ("" when none is usable).
func (c Config) PushSubject() string {
	if c.Push.Subject != "" {
		return strings.TrimPrefix(c.Push.Subject, "mailto:")
	}
	if strings.HasPrefix(strings.ToLower(c.BaseURL), "https://") {
		return strings.TrimRight(c.BaseURL, "/")
	}
	return ""
}

// PublishLimitConfig caps publishes per minute per API token and per user
// (across all of the user's tokens). Bursts of the same size are allowed.
type PublishLimitConfig struct {
	PerToken int `yaml:"per_token"`
	PerUser  int `yaml:"per_user"`
}

// AdaptersConfig enables publish-compatible endpoints for other services.
type AdaptersConfig struct {
	NTFY   bool `yaml:"ntfy"`   // /ntfy/<topic>
	Gotify bool `yaml:"gotify"` // /gotify/message
}

// Enabled maps adapter names to their switch.
func (a AdaptersConfig) Enabled() map[string]bool {
	return map[string]bool{"ntfy": a.NTFY, "gotify": a.Gotify}
}

// OIDCConfig is the deprecated environment/YAML OIDC configuration. It is only
// read once, to import it into the database when no sign-in settings are
// stored yet; Settings → Sign-in is the source of truth (ADR 0006).
type OIDCConfig struct {
	Issuer       string `yaml:"issuer"`
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	// Name labels the sign-in button ("Continue with <Name>").
	Name   string   `yaml:"name"`
	Scopes []string `yaml:"scopes"`
	// UsernameClaim seeds the username of auto-created users.
	UsernameClaim string `yaml:"username_claim"`
	// RoleClaim holds a string or list of strings (e.g. groups). When any
	// value is in AdminValues the user is admin, otherwise user. The role is
	// synced on every OIDC login; empty AdminValues disables the mapping.
	RoleClaim   string   `yaml:"role_claim"`
	AdminValues []string `yaml:"admin_values"`
	// AutoCreate provisions a local user on the first OIDC login. When false,
	// identities must be linked with `acksy user link-oidc`.
	AutoCreate bool `yaml:"auto_create"`
	// DisablePasswordLogin turns off POST /auth/login (the CLI keeps working).
	DisablePasswordLogin bool `yaml:"disable_password_login"`
}

// Enabled reports whether OIDC sign-in is configured.
func (o OIDCConfig) Enabled() bool { return o.Issuer != "" }

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
		SessionTTL:      30 * 24 * time.Hour,
		EventRetention:  7 * 24 * time.Hour,
		Log:             LogConfig{Level: "info", Format: "text"},
		OIDC: OIDCConfig{
			Name:          "SSO",
			Scopes:        []string{"openid", "profile", "email"},
			UsernameClaim: "preferred_username",
			RoleClaim:     "groups",
			AutoCreate:    true,
		},
		Adapters:     AdaptersConfig{NTFY: true, Gotify: true},
		PublishLimit: PublishLimitConfig{PerToken: 60, PerUser: 120},
		Push:         PushConfig{Enabled: true},
		Webhooks:     WebhooksConfig{Enabled: true, FailedRetention: 7 * 24 * time.Hour},
		Retention:    RetentionConfig{ReadTTL: Duration(90 * 24 * time.Hour), MaxPerUser: 10_000},
		Actions:      ActionsConfig{Enabled: true},
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

type lookupFunc func(string) (string, bool)

func applyEnv(cfg *Config, lookup lookupFunc) error {
	str := map[string]*string{
		"ACKSY_LISTEN":         &cfg.Listen,
		"ACKSY_BASE_URL":       &cfg.BaseURL,
		"ACKSY_DATABASE_URL":   &cfg.DatabaseURL,
		"ACKSY_REDIS_URL":      &cfg.RedisURL,
		"ACKSY_LOG_LEVEL":      &cfg.Log.Level,
		"ACKSY_LOG_FORMAT":     &cfg.Log.Format,
		"ACKSY_ADMIN_USERNAME": &cfg.Admin.Username,
		"ACKSY_ADMIN_PASSWORD": &cfg.Admin.Password,

		"ACKSY_OIDC_ISSUER":         &cfg.OIDC.Issuer,
		"ACKSY_OIDC_CLIENT_ID":      &cfg.OIDC.ClientID,
		"ACKSY_OIDC_CLIENT_SECRET":  &cfg.OIDC.ClientSecret,
		"ACKSY_OIDC_NAME":           &cfg.OIDC.Name,
		"ACKSY_OIDC_USERNAME_CLAIM": &cfg.OIDC.UsernameClaim,
		"ACKSY_OIDC_ROLE_CLAIM":     &cfg.OIDC.RoleClaim,

		"ACKSY_VAPID_PUBLIC_KEY":  &cfg.Push.VAPIDPublicKey,
		"ACKSY_VAPID_PRIVATE_KEY": &cfg.Push.VAPIDPrivateKey,
		"ACKSY_VAPID_SUBJECT":     &cfg.Push.Subject,
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
	if v, ok := lookup("ACKSY_SESSION_TTL"); ok {
		d, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("ACKSY_SESSION_TTL: %w", err)
		}
		cfg.SessionTTL = d
	}
	if v, ok := lookup("ACKSY_EVENT_RETENTION"); ok {
		d, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("ACKSY_EVENT_RETENTION: %w", err)
		}
		cfg.EventRetention = d
	}
	if v, ok := lookup("ACKSY_OIDC_SCOPES"); ok {
		cfg.OIDC.Scopes = splitList(v)
	}
	if v, ok := lookup("ACKSY_OIDC_ADMIN_VALUES"); ok {
		cfg.OIDC.AdminValues = splitList(v)
	}
	bools := map[string]*bool{
		"ACKSY_OIDC_AUTO_CREATE":            &cfg.OIDC.AutoCreate,
		"ACKSY_OIDC_DISABLE_PASSWORD_LOGIN": &cfg.OIDC.DisablePasswordLogin,
		"ACKSY_ADAPTERS_NTFY":               &cfg.Adapters.NTFY,
		"ACKSY_ADAPTERS_GOTIFY":             &cfg.Adapters.Gotify,
		"ACKSY_PUSH_ENABLED":                &cfg.Push.Enabled,
		"ACKSY_WEBHOOKS_ENABLED":            &cfg.Webhooks.Enabled,
		"ACKSY_WEBHOOK_ALLOW_PRIVATE":       &cfg.Webhooks.AllowPrivate,
		"ACKSY_ACTIONS_ENABLED":             &cfg.Actions.Enabled,
		"ACKSY_ACTIONS_ALLOW_PRIVATE":       &cfg.Actions.AllowPrivate,
	}
	for key, dst := range bools {
		if v, ok := lookup(key); ok {
			b, err := strconv.ParseBool(v)
			if err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
			*dst = b
		}
	}
	ints := map[string]*int{
		"ACKSY_PUBLISH_LIMIT_TOKEN": &cfg.PublishLimit.PerToken,
		"ACKSY_PUBLISH_LIMIT_USER":  &cfg.PublishLimit.PerUser,
	}
	for key, dst := range ints {
		if v, ok := lookup(key); ok {
			n, err := strconv.Atoi(v)
			if err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
			*dst = n
		}
	}
	if v, ok := lookup("ACKSY_WEBHOOK_FAILED_RETENTION"); ok {
		d, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("ACKSY_WEBHOOK_FAILED_RETENTION: %w", err)
		}
		cfg.Webhooks.FailedRetention = d
	}
	for key, dst := range map[string]*Duration{
		"ACKSY_RETENTION_READ":   &cfg.Retention.ReadTTL,
		"ACKSY_RETENTION_UNREAD": &cfg.Retention.UnreadTTL,
	} {
		if v, ok := lookup(key); ok {
			d, err := ParseDuration(v)
			if err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
			*dst = d
		}
	}
	if v, ok := lookup("ACKSY_RETENTION_READ_BY_PRIORITY"); ok {
		m, err := parsePriorityDurations(v)
		if err != nil {
			return fmt.Errorf("ACKSY_RETENTION_READ_BY_PRIORITY: %w", err)
		}
		cfg.Retention.ReadTTLByPriority = m
	}
	if v, ok := lookup("ACKSY_RETENTION_MAX_PER_USER"); ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("ACKSY_RETENTION_MAX_PER_USER: %w", err)
		}
		cfg.Retention.MaxPerUser = n
	}
	if v, ok := lookup("ACKSY_SECURE_COOKIES"); ok {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("ACKSY_SECURE_COOKIES: %w", err)
		}
		cfg.SecureCookies = &b
	}
	return nil
}

// Validate reports configuration errors.
func (c Config) Validate() error {
	var errs []error
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
	if c.EventRetention < time.Hour {
		errs = append(errs, errors.New("event_retention must be at least 1h"))
	}
	if c.SessionTTL < time.Hour {
		errs = append(errs, errors.New("session_ttl must be at least 1h"))
	}
	if (c.Admin.Username == "") != (c.Admin.Password == "") {
		errs = append(errs, errors.New("admin username and password must be set together"))
	}
	if c.PublishLimit.PerToken < 0 || c.PublishLimit.PerUser < 0 {
		errs = append(errs, errors.New("publish_limit values must not be negative (0 disables)"))
	}
	if (c.Push.VAPIDPublicKey == "") != (c.Push.VAPIDPrivateKey == "") {
		errs = append(errs, errors.New("push.vapid_public_key and push.vapid_private_key must be set together"))
	}
	if sub := c.Push.Subject; sub != "" && !strings.HasPrefix(sub, "https://") && !strings.Contains(sub, "@") {
		errs = append(errs, errors.New("push.subject must be an https URL or an e-mail address"))
	}
	if c.Webhooks.FailedRetention < time.Hour {
		errs = append(errs, errors.New("webhooks.failed_retention must be at least 1h"))
	}
	r := c.Retention
	if r.ReadTTL < 0 || r.UnreadTTL < 0 || r.MaxPerUser < 0 {
		errs = append(errs, errors.New("retention values must not be negative (0 disables)"))
	}
	for p, d := range r.ReadTTLByPriority {
		if p < 1 || p > 5 || d < 0 {
			errs = append(errs, fmt.Errorf("retention.read_ttl_by_priority: priority %d must be 1-5 with a non-negative duration", p))
		}
	}
	if r.MaxPerUser > 0 && r.MaxPerUser < 100 {
		errs = append(errs, errors.New("retention.max_per_user must be 0 (off) or at least 100"))
	}
	return errors.Join(errs...)
}

// parsePriorityDurations parses "1=7d,2=14d".
func parsePriorityDurations(v string) (map[int]Duration, error) {
	out := map[int]Duration{}
	for _, item := range splitList(v) {
		k, val, ok := strings.Cut(item, "=")
		prio, err := strconv.Atoi(k)
		if !ok || err != nil {
			return nil, fmt.Errorf("expected priority=duration, got %q", item)
		}
		d, err := ParseDuration(val)
		if err != nil {
			return nil, err
		}
		out[prio] = d
	}
	return out, nil
}

// splitList splits a comma- or space-separated list, dropping empty items.
func splitList(v string) []string {
	return strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ' ' })
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
