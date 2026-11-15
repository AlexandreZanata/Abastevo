package main

import (
	"github.com/jackc/pgx/v5/pgxpool"

	mediapg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

// workerDocumentConfig carries the enterprise switch, the ordinary runtime
// pool and the private document bucket of the safety job (EMP-DOC-014).
type workerDocumentConfig struct {
	Enabled bool
	Pool    *pgxpool.Pool
	Storage *mediapg.StorageConfig
}

// documentWorkerHandlers composes the document safety handler only with the
// enterprise switch and a private document bucket. Like cmd/api, production
// configures no document bucket until provider qualification (EDR-153), so
// queued safety jobs wait without charging attempts (EDR-170).
func documentWorkerHandlers(c workerDocumentConfig) ([]jobs.Handler, error) {
	if !c.Enabled || c.Storage == nil {
		return nil, nil
	}
	return enterprisesuite.DocumentHandlers(enterprisesuite.Options{Enabled: true, Pool: c.Pool, DocumentStorage: c.Storage})
}
