package app

import (
	"time"

	"github.com/oowliq/acksy/internal/auth"
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
