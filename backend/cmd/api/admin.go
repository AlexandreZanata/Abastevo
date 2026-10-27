package main

import (
	"net/netip"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/adapters"
	adminhttp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/adapters/http"
	adminapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/application"
	admindomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/domain"
	identityhttp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters/http"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/config"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// registerAdminRoutes mounts the administrative surface only when at least
// one key hash is configured. The trusted-proxy resolver is reused through a
// narrow closure so the admin module never imports identity adapters.
func registerAdminRoutes(r chi.Router, cfg config.Config, pool *pgxpool.Pool, trusted []netip.Prefix) bool {
	if len(cfg.AdminKeyHashes) == 0 {
		return false
	}
	keys := make([]admindomain.ConfiguredKey, 0, len(cfg.AdminKeyHashes))
	for _, hash := range cfg.AdminKeyHashes {
		keys = append(keys, admindomain.ConfiguredKey{Hash: hash, Fingerprint: admindomain.Fingerprint(hash)})
	}
	resolver := identityhttp.Resolver{TrustedProxies: trusted}
	service := &adminapp.Service{Store: adapters.NewStore(pool), Keys: keys, Clock: time.Now}
	adminhttp.Handler{
		Service:      service,
		Origin:       cfg.AdminOrigin,
		AllowedCIDRs: cfg.AdminAllowedCIDRs,
		ClientIP:     resolver.ClientIP,
	}.RegisterRoutes(r)
	return true
}
