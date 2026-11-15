package application

import "time"

// Late intake end sources (EMP-OFFL-009, EDR-197).
const (
	LateEndMembership = "MEMBERSHIP"
	LateEndDevice     = "DEVICE"
)

// LateIntakeDevice is the server end evidence of one exact owned enrollment
// period revoked ACCESS_ENDED. It is intake-only: it carries no membership,
// role, record, receipt or review-result authority.
type LateIntakeDevice struct {
	privateRegister
	End     string
	EndedAt time.Time
}

func (v LateIntakeDevice) Validate() error {
	if v.End != LateEndMembership && v.End != LateEndDevice || v.EndedAt.IsZero() || v.EndedAt.UTC().Year() < 1 || v.EndedAt.UTC().Year() > 9999 {
		return ErrRegisterUnavailable
	}
	return nil
}
