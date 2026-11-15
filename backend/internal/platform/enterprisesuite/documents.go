package enterprisesuite

import (
	"errors"

	documentpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/adapters"
	documentapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/application"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	mediapg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/adapters"
	media "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

var errDocumentComposition = errors.New("enterprisesuite: incomplete document composition")

// documentObjects is the private document bucket as both the bounded
// transfer port and the safety check's read and removal port.
type documentObjects interface {
	media.Objects
	documentapp.SafetyObjects
}

// DocumentHandlers composes the document safety job (EMP-DOC-014) over the
// same private bucket and inspector as the ingress. Without a configured
// bucket it composes none, so queued jobs wait, excluded from the general
// dispatcher, without charging attempts (EDR-170).
func DocumentHandlers(o Options) ([]jobs.Handler, error) {
	if !o.Enabled || o.DocumentStorage == nil {
		return nil, nil
	}
	if o.Pool == nil {
		return nil, errDocumentComposition
	}
	objects, err := mediapg.NewS3(*o.DocumentStorage)
	if err != nil {
		return nil, err
	}
	catalog, err := audit.NewCatalog(documentpg.AuditDefinitions())
	if err != nil {
		return nil, err
	}
	// The job settles server-owned state only; it never acts for a session.
	store := &documentpg.Store{Pool: o.Pool, Audit: auditpg.NewStore(o.Pool, catalog)}
	checker := documentapp.Checker{Repository: store, Objects: objects, Inspector: mediapg.JPEGInspector{}, NewID: jobs.NewUUIDv4}
	return []jobs.Handler{documentpg.SafetyHandler{Safety: checker}}, nil
}
