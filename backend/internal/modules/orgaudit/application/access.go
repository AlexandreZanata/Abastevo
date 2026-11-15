package application

import (
	"context"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
)

// AccessRecorder commits one sensitive-access record (EMP-PRIV-005) inside the
// request's organization scope. The owning read calls it only after its own
// scope, membership, session and category-permission checks, and mints a
// capability or releases content only after it returns nil. Every error,
// including domain.ErrReplayed and domain.ErrReused, means serve nothing.
//
// Consumers: private documents (orgmedia, DOCUMENT_CONTENT). EXPORT_FILE
// (EMP-RPT-001) and LOCATION_LIVE/LOCATION_TRACK (EMP-GPS-012) are declared
// categories without a producer yet.
type AccessRecorder interface {
	RecordAccess(context.Context, kernel.OrgScope, domain.SensitiveAccess) error
}
