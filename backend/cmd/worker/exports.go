package main

import (
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters/storage"
	orgexport "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/config"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5/pgxpool"
)

// workerExportConfig carries the enterprise switch, the ordinary runtime
// pool and the private bucket of the organization export worker.
type workerExportConfig struct {
	Enabled bool
	Pool    *pgxpool.Pool
	Storage *orgexport.StorageConfig
}

// exportStorage maps the existing private bucket configuration.
func exportStorage(r2 *config.R2Config) *orgexport.StorageConfig {
	if r2 == nil {
		return nil
	}
	return &orgexport.StorageConfig{Endpoint: r2.Endpoint, Bucket: r2.Bucket, Region: r2.Region,
		Credentials: storage.Credentials{AccessKeyID: r2.AccessKeyID, SecretAccessKey: r2.SecretAccessKey}}
}

// exportWorkerHandlers composes the organization export handlers only when
// the enterprise switch is on and private storage is configured; otherwise
// queued export jobs wait, excluded from the general dispatcher, without
// charging attempts (BR-RPT-009, EDR-156). Production admits no format yet.
func exportWorkerHandlers(c workerExportConfig) ([]jobs.Handler, error) {
	if !c.Enabled || c.Storage == nil {
		return nil, nil
	}
	return enterprisesuite.ExportHandlers(enterprisesuite.Options{Enabled: true, Pool: c.Pool, ExportStorage: c.Storage})
}
