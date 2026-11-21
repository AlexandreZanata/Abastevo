package domain

import (
	"errors"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

var (
	ErrLimitReached = errors.New("orgbilling: plan limit reached")
	ErrInvalidUsage = errors.New("orgbilling: invalid usage increase")
)

// CapabilityAllowed resolves plan facts only. The caller applies the
// independently verified organization role and record scope.
func CapabilityAllowed(scope kernel.OrgScope, sub Subscription, plan Plan, code string, now time.Time) bool {
	current := sub.Snapshot()
	if scope.Validate() != nil || scope != current.Scope || current.Plan != plan.Key() ||
		now.Before(current.LastEventAt) || !current.Period.contains(now) || !plan.Includes(code) {
		return false
	}
	state := current.State
	if state == Cancelling {
		state = current.PriorState
	}
	return state == Trialing || state == Active
}

// CheckIncrease checks an already measured amount and a proposed positive
// increment. The owning transaction must serialize measurement with writes.
func (plan Plan) CheckIncrease(meter Meter, used, additional int64) error {
	if used < 0 || additional <= 0 {
		return ErrInvalidUsage
	}
	ceiling, err := plan.Limit(meter)
	if err != nil {
		return err
	}
	if additional > ceiling-used {
		return ErrLimitReached
	}
	return nil
}
