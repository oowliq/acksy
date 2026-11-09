package domain

import (
	"errors"
	"net/url"
	"slices"
	"strings"
)

// OIDCSettingsKey is the instance_settings key holding OIDCSettings as JSON.
const OIDCSettingsKey = "oidc"

// OIDCSettings configure sign-in through the single OpenID Connect provider.
// They live in the database and are edited by admins (ADR 0006).
type OIDCSettings struct {
	Enabled      bool   `json:"enabled"`
	Issuer       string `json:"issuer"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret,omitempty"` // stored; never sent to clients
	// Name labels the sign-in button ("Continue with <Name>").
	Name   string   `json:"name"`
	Scopes []string `json:"scopes"`
	// UsernameClaim seeds the username of auto-created users.
	UsernameClaim string `json:"username_claim"`
	// RoleClaim holds a string or list of strings (e.g. groups). When any value
	// is in AdminValues the user is admin, otherwise user; synced on every
	// login. Empty AdminValues leaves roles to Acksy.
	RoleClaim   string   `json:"role_claim"`
	AdminValues []string `json:"admin_values"`
	// AutoCreate provisions a local user on the first OIDC login.
	AutoCreate bool `json:"auto_create"`
	// DisablePasswordLogin turns off password sign-in (the CLI keeps working).
	DisablePasswordLogin bool `json:"disable_password_login"`
}

// DefaultOIDCSettings is what an instance starts with: OIDC off.
func DefaultOIDCSettings() OIDCSettings {
	return OIDCSettings{
		Name:          "SSO",
		Scopes:        []string{"openid", "profile", "email"},
		UsernameClaim: "preferred_username",
		RoleClaim:     "groups",
		AutoCreate:    true,
	}
}

// Active reports whether OIDC sign-in is on.
func (s OIDCSettings) Active() bool { return s.Enabled && s.Issuer != "" }

// Validate checks the settings are usable.
func (s OIDCSettings) Validate() error {
	var errs []error
	if s.DisablePasswordLogin && !s.Enabled {
		errs = append(errs, errors.New("password sign-in can only be turned off while single sign-on is on"))
	}
	if len(s.AdminValues) > 0 && s.RoleClaim == "" {
		errs = append(errs, errors.New("admin values need a role claim"))
	}
	if s.Enabled {
		if u, err := url.Parse(s.Issuer); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			errs = append(errs, errors.New("issuer must be an absolute http(s) URL"))
		} else if u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(s.Issuer, "#") {
			errs = append(errs, errors.New("issuer must not have a query or fragment"))
		}
		if s.ClientID == "" {
			errs = append(errs, errors.New("client ID is required"))
		}
		if !slices.Contains(s.Scopes, "openid") {
			errs = append(errs, errors.New("scopes must include openid"))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return Invalid("%s", err.Error())
	}
	return nil
}
