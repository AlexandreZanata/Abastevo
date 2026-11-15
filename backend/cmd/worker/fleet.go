package main

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

// fleetWorkerHandlers composes the reservation outcome job (EMP-FLEET-007)
// with the enterprise switch; it needs only the ordinary runtime pool and
// runs on the general dispatcher.
func fleetWorkerHandlers(enabled bool, pool *pgxpool.Pool) ([]jobs.Handler, error) {
	return enterprisesuite.FleetHandlers(enterprisesuite.Options{Enabled: enabled, Pool: pool})
}
