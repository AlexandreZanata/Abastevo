package application

import (
	"context"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
)

// MeterDisposition is source evidence, not a client claim. Historical coverage
// may permit quarantine; only current unchanged coverage can permit application.
type MeterDisposition string

const (
	MeterApply                 MeterDisposition = "APPLY"
	MeterHoldPermissionChanged MeterDisposition = "HOLD_PERMISSION_CHANGED"
)

// MeterIntakeAuthority checks current enrolled proof before classifying complete
// retained permission periods. Unknown history is never a never-authorized fact.
type MeterIntakeAuthority interface {
	ClassifyMeterTx(context.Context, syncapp.DBTX, kernel.OrgScope, MeterSource, string, MeterReadingCommand) (MeterDisposition, error)
}

type ReviewReference struct {
	private
	ReviewItemID string
	Revision     int64
}

// MeterHeldRepository stores typed held evidence in the original intake
// transaction, without applying a reading or changing transaction lifecycle.
type MeterHeldRepository interface {
	HoldMeterReadingTx(context.Context, syncapp.DBTX, syncapp.Source, protocol.Operation, MeterReadingCommand, string) (ReviewReference, error)
}

// MeterAssetLifecycle reads the asset's current disposal instant inside the
// caller's intake transaction, after taking the organization root and asset
// row locks in that order. A nil instant means no current disposal; a field
// reading declared at or after a returned instant is held (B-BR-EMP-206).
type MeterAssetLifecycle interface {
	MeterAssetDisposalTx(context.Context, syncapp.DBTX, kernel.OrgScope, string) (*time.Time, error)
}
