package application

import (
	"context"

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
