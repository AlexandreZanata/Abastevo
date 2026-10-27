package main

import (
	"net/netip"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/adapters"
	adminhttp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/adapters/http"
	adminapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/application"
	admindomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/domain"
	entitlementadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/entitlement/adapters"
	identityhttp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters/http"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/config"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// registerAdminRoutes mounts the administrative surface only when at least
// one key hash is configured. The trusted-proxy resolver is reused through a
// narrow closure so the admin module never imports identity adapters.
func newAdminHandler(cfg config.Config, pool *pgxpool.Pool, trusted []netip.Prefix) (*adminhttp.Handler, bool) {
	if len(cfg.AdminKeyHashes) == 0 {
		return nil, false
	}
	keys := make([]admindomain.ConfiguredKey, 0, len(cfg.AdminKeyHashes))
	for _, hash := range cfg.AdminKeyHashes {
		keys = append(keys, admindomain.ConfiguredKey{Hash: hash, Fingerprint: admindomain.Fingerprint(hash)})
	}
	resolver := identityhttp.Resolver{TrustedProxies: trusted}
	adminStore := adapters.NewStore(pool)
	service := &adminapp.Service{
		Store: adminStore, Grants: entitlementadapters.NewStore(pool),
		Keys: keys, Clock: time.Now, NewID: newUUID,
	}
	handler := &adminhttp.Handler{
		Service:      service,
		Origin:       cfg.AdminOrigin,
		AllowedCIDRs: cfg.AdminAllowedCIDRs,
		ClientIP:     resolver.ClientIP,
		CursorSecret: cfg.CursorSecret,
	}
	return handler, true
}

func registerAdminRoutes(r chi.Router, cfg config.Config, pool *pgxpool.Pool, trusted []netip.Prefix) bool {
	handler, ok := newAdminHandler(cfg, pool, trusted)
	if !ok {
		return false
	}
	handler.RegisterRoutes(r)
	return true
}
