package application

import (
	"context"
	"time"

	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

// MeterUsageMethod versions the usage-between-instants answer (EDR-316).
const MeterUsageMethod = "meter-usage-v1"

// MeterUsageAnchor is one reading a usage answer used.
type MeterUsageAnchor struct {
	ReadingID  string
	Value      int64
	OccurredAt time.Time
}

// MeterUsage answers usage(from, to) for one asset and kind (B-BR-EMP-489).
// Reason is empty when the usage is known; anchors are nil when absent.
type MeterUsage struct {
	AssetID  string
	Kind     meter.Kind
	From, To time.Time
	Reason   meter.Insufficient
	Usage    int64
	Start    *MeterUsageAnchor
	End      *MeterUsageAnchor
	Method   string
	Revision int64
}

func (u MeterUsage) Known() bool { return u.Reason == "" }

// MeterUsagePort is the EMP-METER-006 port for other modules (UC-METER-08).
type MeterUsagePort interface {
	UsageBetween(ctx context.Context, scope kernel.OrgScope, asset string, kind meter.Kind, from, to time.Time) (MeterUsage, error)
}
