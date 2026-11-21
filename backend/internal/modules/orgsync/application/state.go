package application

import (
	"errors"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
)

// Organization-state and plan disposition of queued operations (EMP-OFFL-012,
// EDR-200/201). Server history decides before the owning handler; the device
// clock only chooses review over refusal.

const (
	StateSuspension = "SUSPENSION"
	StatePlan       = "PLAN"

	StateGraceCode     = "ORG_SUSPENDED_GRACE"
	StateSuspendedCode = "ORG_SUSPENDED"
	StatePlanCode      = "PLAN_CHANGED"

	StateHold   = "HOLD"
	StateRefuse = "REFUSE"
	StatePolicy = "state-intake-v1"
)

// StateDecision restricts an operation before its owner. The zero value
// allows the owner to decide (normal path, including its own refusals).
type StateDecision struct {
	privateValue
	Condition, Disposition, Result, Code string
	// Instant is the plan's downgrade instant; suspension history is
	// re-derived by the database from its recorded periods.
	Instant              time.Time
	Capability           string
	CapabilityRevision   int64
	SubscriptionID       string
	SubscriptionRevision int64
}

func (decision StateDecision) Restricts() bool { return decision.Condition != "" }
func (decision StateDecision) Holds() bool     { return decision.Disposition == StateHold }

// Same compares two decisions taken in one transaction.
func (decision StateDecision) Same(other StateDecision) bool {
	return decision.Condition == other.Condition && decision.Disposition == other.Disposition && decision.Result == other.Result && decision.Code == other.Code &&
		decision.Instant.Equal(other.Instant) && decision.Capability == other.Capability && decision.CapabilityRevision == other.CapabilityRevision &&
		decision.SubscriptionID == other.SubscriptionID && decision.SubscriptionRevision == other.SubscriptionRevision
}

func (decision StateDecision) Validate() error {
	hasSubscriptionProof := decision.SubscriptionID != "" || decision.SubscriptionRevision != 0
	switch {
	case !decision.Restricts():
		if decision != (StateDecision{}) {
			return ErrUnavailable
		}
	case decision.Condition == StateSuspension && decision.Disposition == StateHold && decision.Result == protocol.AcceptedForReview && decision.Code == StateGraceCode,
		decision.Condition == StateSuspension && decision.Disposition == StateRefuse && decision.Result == protocol.Rejected && decision.Code == StateSuspendedCode:
		if !decision.Instant.IsZero() || decision.Capability != "" || decision.CapabilityRevision != 0 || hasSubscriptionProof {
			return ErrUnavailable
		}
	case decision.Condition == StatePlan && decision.Disposition == StateHold && decision.Result == protocol.AcceptedForReview && decision.Code == StatePlanCode:
		if decision.Capability == "" {
			return ErrUnavailable
		}
		if hasSubscriptionProof {
			id, err := kernel.ParseOrgID(decision.SubscriptionID)
			if err != nil || id.String() != decision.SubscriptionID || decision.SubscriptionRevision < 1 || !decision.Instant.IsZero() || decision.CapabilityRevision != 0 {
				return ErrUnavailable
			}
		} else if decision.Instant.IsZero() || decision.CapabilityRevision < 2 {
			return ErrUnavailable
		}
	default:
		return ErrUnavailable
	}
	return nil
}

// Plan states of the operation's write capability row.
const (
	PlanAbsent   = "ABSENT"
	PlanEnabled  = "ENABLED"
	PlanDisabled = "DISABLED"

	PlanApply = "APPLY"
	PlanDeny  = "DENY"
)

// PlanState is the capability row at receipt: its state, revision and the
// instant that state was set (`org_capabilities.updated_at`).
type PlanState struct {
	privateValue
	State     string
	Revision  int64
	ChangedAt time.Time
}

var errPlanEvidence = errors.New("orgsync: plan evidence unavailable")

// DecidePlan is the EDR-201 rule shared with org_sync_plan_disposition:
// APPLY (owner decides), HOLD (PLAN_CHANGED) or DENY (new forbidden work,
// the owner keeps refusing it).
func DecidePlan(state PlanState, declared *time.Time, received time.Time) (string, error) {
	if declared == nil || received.IsZero() {
		return "", errPlanEvidence
	}
	switch state.State {
	case PlanAbsent:
		if state.Revision != 0 || !state.ChangedAt.IsZero() {
			return "", errPlanEvidence
		}
		return PlanDeny, nil
	case PlanEnabled, PlanDisabled:
	default:
		return "", errPlanEvidence
	}
	if state.Revision < 1 || state.ChangedAt.IsZero() || state.ChangedAt.After(received) {
		return "", errPlanEvidence
	}
	if state.State == PlanEnabled {
		return PlanApply, nil
	}
	if state.Revision > 1 && declared.Before(state.ChangedAt) {
		return StateHold, nil
	}
	return PlanDeny, nil
}
