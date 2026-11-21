package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	qsync "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgsync"
	fueling "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/application"

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

func (a IntakeStateAssessor) AssessStateTx(ctx context.Context, tx pgx.Tx, source syncapp.Source, op protocol.Operation, descriptor syncapp.Descriptor, declared time.Time) (syncapp.StateDecision, error) {
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
		q := qsync.New(tx)
		declaredAt := pgtype.Timestamptz{Time: declared.UTC().Truncate(time.Microsecond), Valid: true}
		receivedAt := pgtype.Timestamptz{Time: intake.ReceivedAt, Valid: true}
		if descriptor.Type == fueling.OperationType && capability == "organizations" {
			archived, err := assessVehiclePlanArchive(ctx, q, source.Scope, op, declaredAt, receivedAt, &decision)
			if err != nil {
				return syncapp.StateDecision{}, syncapp.SourceDenied(err)
			}
			if archived {
				if decision.Validate() != nil {
					return syncapp.StateDecision{}, syncapp.SourceDenied(syncapp.ErrUnavailable)
				}
				return decision, nil
			}
		}
		proof, err := q.InactiveSubscriptionIntakeProof(ctx, qsync.InactiveSubscriptionIntakeProofParams{
			OrgID: source.Scope.OrgID().String(), Capability: capability, DeclaredAt: declaredAt, ReceivedAt: receivedAt,
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

// assessVehiclePlanArchive resolves only the vehicle selector. The owning
// fueling handler still validates the signed payload and recorded author
// authority before it can create a review item.
func assessVehiclePlanArchive(ctx context.Context, q *qsync.Queries, scope kernel.OrgScope, op protocol.Operation,
	declared, received pgtype.Timestamptz, decision *syncapp.StateDecision,
) (bool, error) {
	var target struct {
		Asset string `json:"asset_id"`
	}
	if err := json.Unmarshal(op.PayloadBytes(), &target); err != nil || legalIdentity(scope, target.Asset) != nil {
		return false, nil
	}
	proof, err := q.VehiclePlanArchiveForIntake(ctx, qsync.VehiclePlanArchiveForIntakeParams{
		OrgID: scope.OrgID().String(), AssetID: target.Asset, DeclaredAt: declared, ReceivedAt: received,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if proof.SubscriptionRevision == 0 {
		return false, orgapi.NotFound
	}
	decision.VehiclePlanSelectionID, decision.VehiclePlanAssetID = proof.SelectionID, target.Asset
	decision.SubscriptionID, decision.SubscriptionRevision = proof.SubscriptionID, proof.SubscriptionRevision
	return true, nil
}
