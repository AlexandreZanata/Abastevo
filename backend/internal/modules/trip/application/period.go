package application

import (
	"errors"

	reviewapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/application"
)

// EMP-TRIP-016 period lock of trip review decisions (B-BR-EMP-359..362,
// EDR-232/233). Field intake never consults the period (B-BR-EMP-360).

// ErrPeriodLocked: the period of the trip's stored start reporting day is
// LOCKED; nothing is written (B-BR-EMP-359/361).
var ErrPeriodLocked = errors.New("trip: reporting period locked")

// DecisionPeriod admits a review decision only when the APPR-006 period
// holding the trip's stored start reporting day is writable: OPEN, or
// REOPENED with its stored reasoned reopening. The fact must be the gate's
// answer for exactly that stored day (EDR-182, EDR-232).
func DecisionPeriod(reportingDay string, fact reviewapp.PeriodFact) error {
	if reportingDay == "" || fact.Day != reportingDay || fact.Validate() != nil {
		return ErrUnavailable
	}
	if !fact.Writable() {
		return ErrPeriodLocked
	}
	return nil
}
