package main

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

// trackingWorkerHandlers composes the track processing job (EMP-GPS-005)
// with the enterprise switch; it needs only the ordinary runtime pool and
// runs on the general dispatcher (EDR-298).
func trackingWorkerHandlers(enabled bool, pool *pgxpool.Pool) ([]jobs.Handler, error) {
	return enterprisesuite.TrackingHandlers(enterprisesuite.Options{Enabled: enabled, Pool: pool})
}
