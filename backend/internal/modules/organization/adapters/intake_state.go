package adapters

import (
	"context"
	"errors"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/jackc/pgx/v5"
)

// IntakePlanStates reads the operation's write capability row (EDR-201).
type IntakePlanStates interface {
	PlanStateDBTX(context.Context, database.OrgDBTX, kernel.OrgScope, string) (bool, bool, int64, time.Time, error)
}

// IntakeStateAssessor composes ORG-009's suspension assessment and the plan
// rule before the owning handler (EMP-OFFL-012, EDR-200/201). It reads only
// server history and the database clock; the declared instant is evidence.
type IntakeStateAssessor struct {
	Suspensions *SuspensionStore
	Plans       IntakePlanStates
}

func (a IntakeStateAssessor) AssessStateTx(ctx context.Context, tx pgx.Tx, source syncapp.Source, descriptor syncapp.Descriptor, declared time.Time) (syncapp.StateDecision, error) {
	if a.Suspensions == nil || a.Plans == nil || source.Validate() != nil || descriptor.Validate() != nil {
		return syncapp.StateDecision{}, syncapp.SourceDenied(syncapp.ErrUnavailable)
	}
	intake, err := a.Suspensions.AssessSuspensionTx(ctx, tx, source.Scope, declared, time.Time{})
	if err != nil {
		return syncapp.StateDecision{}, syncapp.SourceDenied(syncapp.ErrUnavailable)
	}
	switch intake.Assessment.Disposition {
	case d.SuspensionReview:
		return syncapp.StateDecision{Condition: syncapp.StateSuspension, Disposition: syncapp.StateHold, Result: protocol.AcceptedForReview, Code: syncapp.StateGraceCode}, nil
	case d.SuspensionRefuse:
		switch intake.Assessment.Code {
		case syncapp.StateSuspendedCode:
			return syncapp.StateDecision{Condition: syncapp.StateSuspension, Disposition: syncapp.StateRefuse, Result: protocol.Rejected, Code: syncapp.StateSuspendedCode}, nil
		case string(orgapi.OrgArchived):
			return syncapp.StateDecision{}, syncapp.SourceDenied(orgapi.OrgArchived)
		case string(orgapi.OrgSetupIncomplete):
			return syncapp.StateDecision{}, syncapp.SourceDenied(orgapi.OrgSetupIncomplete)
		}
		return syncapp.StateDecision{}, syncapp.SourceDenied(orgapi.NotFound)
	case d.SuspensionAllow:
	default:
		return syncapp.StateDecision{}, syncapp.SourceDenied(syncapp.ErrUnavailable)
	}
	capability := descriptor.WritePolicy.Capability
	present, enabled, revision, changed, err := a.Plans.PlanStateDBTX(ctx, tx, source.Scope, capability)
	if err != nil {
		return syncapp.StateDecision{}, syncapp.SourceDenied(syncapp.ErrUnavailable)
	}
	state := syncapp.PlanState{State: syncapp.PlanAbsent}
	if present {
		state = syncapp.PlanState{State: syncapp.PlanDisabled, Revision: revision, ChangedAt: changed}
		if enabled {
			state.State = syncapp.PlanEnabled
		}
	}
	disposition, err := syncapp.DecidePlan(state, &declared, intake.ReceivedAt)
	if err != nil {
		return syncapp.StateDecision{}, syncapp.SourceDenied(syncapp.ErrUnavailable)
	}
	if disposition != syncapp.StateHold {
		// APPLY and DENY leave the owner to decide (DENY: ENTITLEMENT_REQUIRED).
		return syncapp.StateDecision{}, nil
	}
	decision := syncapp.StateDecision{Condition: syncapp.StatePlan, Disposition: syncapp.StateHold, Result: protocol.AcceptedForReview, Code: syncapp.StatePlanCode, Instant: changed, Capability: capability, CapabilityRevision: revision}
	if decision.Validate() != nil {
		return syncapp.StateDecision{}, syncapp.SourceDenied(errors.New("orgsync: invalid plan decision"))
	}
	return decision, nil
}
