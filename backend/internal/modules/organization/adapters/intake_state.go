package adapters

import (
	"context"
	"errors"
	"slices"
	"time"

	qsync "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgsync"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// IntakePlanStates reads the operation's write capability row (EDR-201).
type IntakePlanStates interface {
	PlanStateDBTX(context.Context, database.OrgDBTX, kernel.OrgScope, string) (bool, bool, int64, time.Time, error)
}

// IntakeStateAssessor composes ORG-009's suspension assessment and the plan
// rule before the owning handler (EMP-OFFL-012, EDR-200/201). It reads only
// server history and the database clock; the declared instant is evidence.
type IntakeStateAssessor struct {
	Suspensions       *SuspensionStore
	Plans             IntakePlanStates
	SubscriptionPlans bool
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
	decision := syncapp.StateDecision{
		Condition: syncapp.StatePlan, Disposition: syncapp.StateHold, Result: protocol.AcceptedForReview,
		Code: syncapp.StatePlanCode, Capability: capability,
	}
	switch {
	case disposition == syncapp.StateHold:
		decision.Instant, decision.CapabilityRevision = changed, revision
	case a.SubscriptionPlans && (!present || enabled) && slices.Contains(descriptor.Results(), protocol.AcceptedForReview):
		proof, err := qsync.New(tx).InactiveSubscriptionIntakeProof(ctx, qsync.InactiveSubscriptionIntakeProofParams{
			OrgID: source.Scope.OrgID().String(), Capability: capability,
			DeclaredAt: pgtype.Timestamptz{Time: declared.UTC().Truncate(time.Microsecond), Valid: true},
			ReceivedAt: pgtype.Timestamptz{Time: intake.ReceivedAt, Valid: true},
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return syncapp.StateDecision{}, nil
		}
		if err != nil {
			return syncapp.StateDecision{}, syncapp.SourceDenied(syncapp.ErrUnavailable)
		}
		decision.SubscriptionID, decision.SubscriptionRevision = proof.SubscriptionID, proof.SubscriptionRevision
	default:
		// The owning handler evaluates current write authority.
		return syncapp.StateDecision{}, nil
	}
	if decision.Validate() != nil {
		return syncapp.StateDecision{}, syncapp.SourceDenied(errors.New("orgsync: invalid plan decision"))
	}
	return decision, nil
}
