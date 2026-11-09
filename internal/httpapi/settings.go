package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/oowliq/acksy/internal/domain"
	"github.com/oowliq/acksy/internal/netguard"
	"github.com/oowliq/acksy/internal/service"
)

// oidcSettingsView is what clients see: never the client secret.
type oidcSettingsView struct {
	Enabled              bool       `json:"enabled"`
	Issuer               string     `json:"issuer"`
	ClientID             string     `json:"client_id"`
	ClientSecretSet      bool       `json:"client_secret_set"`
	Name                 string     `json:"name"`
	Scopes               []string   `json:"scopes"`
	UsernameClaim        string     `json:"username_claim"`
	RoleClaim            string     `json:"role_claim"`
	AdminValues          []string   `json:"admin_values"`
	AutoCreate           bool       `json:"auto_create"`
	DisablePasswordLogin bool       `json:"disable_password_login"`
	RedirectURL          string     `json:"redirect_url"`
	UpdatedAt            *time.Time `json:"updated_at"`
}

func (a *api) oidcView(s domain.OIDCSettings, at time.Time) oidcSettingsView {
	v := oidcSettingsView{
		Enabled: s.Enabled, Issuer: s.Issuer, ClientID: s.ClientID, ClientSecretSet: s.ClientSecret != "",
		Name: s.Name, Scopes: nonNilStrings(s.Scopes), UsernameClaim: s.UsernameClaim, RoleClaim: s.RoleClaim,
		AdminValues: nonNilStrings(s.AdminValues), AutoCreate: s.AutoCreate, DisablePasswordLogin: s.DisablePasswordLogin,
		RedirectURL: a.svc.OIDCRedirectURL(),
	}
	if !at.IsZero() {
		v.UpdatedAt = &at
	}
	return v
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// oidcSettingsInput mirrors domain.OIDCSettings; an absent client_secret
// keeps the stored one, "" removes it.
type oidcSettingsInput struct {
	Enabled              bool     `json:"enabled"`
	Issuer               string   `json:"issuer"`
	ClientID             string   `json:"client_id"`
	ClientSecret         *string  `json:"client_secret"`
	Name                 string   `json:"name"`
	Scopes               []string `json:"scopes"`
	UsernameClaim        string   `json:"username_claim"`
	RoleClaim            string   `json:"role_claim"`
	AdminValues          []string `json:"admin_values"`
	AutoCreate           bool     `json:"auto_create"`
	DisablePasswordLogin bool     `json:"disable_password_login"`
}

func (in oidcSettingsInput) toService() service.OIDCSettingsInput {
	return service.OIDCSettingsInput{
		OIDCSettings: domain.OIDCSettings{
			Enabled: in.Enabled, Issuer: in.Issuer, ClientID: in.ClientID, Name: in.Name, Scopes: in.Scopes,
			UsernameClaim: in.UsernameClaim, RoleClaim: in.RoleClaim, AdminValues: in.AdminValues,
			AutoCreate: in.AutoCreate, DisablePasswordLogin: in.DisablePasswordLogin,
		},
		ClientSecret: in.ClientSecret,
	}
}

func (a *api) getOIDCSettings(w http.ResponseWriter, r *http.Request) {
	s, at, err := a.svc.OIDCSettings(r.Context(), principal(r))
	if err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	writeJSON(w, http.StatusOK, a.oidcView(s, at))
}

func (a *api) putOIDCSettings(w http.ResponseWriter, r *http.Request) {
	var in oidcSettingsInput
	if err := decodeJSON(w, r, &in); err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	s, at, err := a.svc.SaveOIDCSettings(r.Context(), principal(r), in.toService())
	if err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	writeJSON(w, http.StatusOK, a.oidcView(s, at))
}

func (a *api) testOIDCSettings(w http.ResponseWriter, r *http.Request) {
	var in oidcSettingsInput
	if err := decodeJSON(w, r, &in); err != nil {
		respondErr(w, r, a.log, err)
		return
	}
	type result struct {
		OK    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
	}
	err := a.svc.TestOIDCSettings(r.Context(), principal(r), in.toService())
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, result{OK: true})
	case errors.Is(err, domain.ErrForbidden), errors.Is(err, domain.ErrUnauthorized):
		respondErr(w, r, a.log, err)
	default:
		// Validation and discovery failures are the answer, not an API error.
		writeJSON(w, http.StatusOK, result{Error: netguard.Describe(err)})
	}
}
