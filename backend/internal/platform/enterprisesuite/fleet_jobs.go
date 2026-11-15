package enterprisesuite

import (
	"errors"

	fleetpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/adapters"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

var errFleetComposition = errors.New("enterprisesuite: incomplete fleet job composition")

// FleetHandlers composes the reservation outcome job (EMP-FLEET-007,
// EDR-193) over the ordinary runtime pool. The job settles server-owned
// time-driven outcomes only; it never acts for a session.
func FleetHandlers(o Options) ([]jobs.Handler, error) {
	if !o.Enabled {
		return nil, nil
	}
	if o.Pool == nil {
		return nil, errFleetComposition
	}
	catalog, err := audit.NewCatalog(fleetpg.AssetAuditDefinitions())
	if err != nil {
		return nil, err
	}
	store := &fleetpg.AssetStore{Pool: o.Pool, Audit: auditpg.NewStore(o.Pool, catalog), NewID: jobs.NewUUIDv4}
	return []jobs.Handler{fleetpg.ReservationOutcomeHandler{Outcomes: store}}, nil
}
