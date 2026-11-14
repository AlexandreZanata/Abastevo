package enterprisesuite

import (
	adminpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/adapters"
	adminhttp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/adapters/http"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	orgweb "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters/http"
	org "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/go-chi/chi/v5"
)

// RegisterAdmin adds only the operator lifecycle surface. The admin credential
// plane and enterprise flag must both be enabled; company roles are unrelated.
func RegisterAdmin(router chi.Router, o Options, admin *adminhttp.Handler) error {
	if !o.Enabled || admin == nil || admin.Service == nil || !admin.Service.Enabled() {
		return nil
	}
	if o.Pool == nil {
		return org.ErrUnavailable
	}
	runner, ok := admin.Service.Store.(*adminpg.Store)
	if !ok || runner.Pool != o.Pool {
		return org.ErrUnavailable
	}
	store := &orgpg.SuspensionStore{Roots: orgpg.NewStore(o.Pool), Proof: runner, NewID: jobs.NewUUIDv4}
	handler := orgweb.SuspensionAdmin{Service: org.SuspensionService{Enabled: true, Runner: runner, Repository: store}, Admin: admin}
	router.Group(func(g chi.Router) { g.Use(admin.AdminMiddleware()...); handler.RegisterRoutes(g) })
	return nil
}
