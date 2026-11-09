package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/oowliq/acksy/internal/auth"
	"github.com/oowliq/acksy/internal/config"
	"github.com/oowliq/acksy/internal/domain"
	"github.com/oowliq/acksy/internal/netguard"
	"github.com/oowliq/acksy/internal/service"
)

// oidcProviderFactory builds the provider for the current sign-in settings.
// The issuer is entered by an admin and identity providers commonly live on
// the LAN, so private addresses are allowed (ADR 0006).
func oidcProviderFactory(redirectURL string) func(domain.OIDCSettings) service.OIDCProvider {
	client := netguard.NewClient(netguard.Options{AllowPrivate: true, Timeout: 10 * time.Second})
	return func(s domain.OIDCSettings) service.OIDCProvider {
		return auth.NewOIDCProvider(auth.OIDCConfig{
			Issuer: s.Issuer, ClientID: s.ClientID, ClientSecret: s.ClientSecret,
			RedirectURL: redirectURL, Scopes: s.Scopes,
			UsernameClaim: s.UsernameClaim, RoleClaim: s.RoleClaim,
			HTTPClient: client,
		})
	}
}

// importOIDCFromEnv moves the deprecated ACKSY_OIDC_* / `oidc:` config into
// the database once; afterwards the database is the only source (ADR 0006).
func importOIDCFromEnv(ctx context.Context, svc *service.Service, o config.OIDCConfig, log *slog.Logger) error {
	if o.Issuer == "" && o.ClientID == "" && !o.DisablePasswordLogin {
		return nil
	}
	settings := domain.OIDCSettings{
		Enabled: o.Issuer != "", Issuer: o.Issuer, ClientID: o.ClientID, ClientSecret: o.ClientSecret,
		Name: o.Name, Scopes: o.Scopes, UsernameClaim: o.UsernameClaim, RoleClaim: o.RoleClaim,
		AdminValues: o.AdminValues, AutoCreate: o.AutoCreate, DisablePasswordLogin: o.DisablePasswordLogin,
	}
	imported, err := svc.ImportOIDCSettings(ctx, settings)
	if err != nil {
		return err
	}
	if imported {
		log.InfoContext(ctx, "oidc settings imported from the environment; ACKSY_OIDC_* is now ignored, use Settings → Sign-in")
	} else {
		log.WarnContext(ctx, "ACKSY_OIDC_* is ignored: sign-in is configured in Settings → Sign-in (stored in the database)")
	}
	return nil
}
