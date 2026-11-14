//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"testing"
	"time"

	fleetpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/adapters"
	fa "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	syncpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/adapters"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
)

// stagedMeterProof is the real Meter repository and proof. After the actual
// proof passes, it reports one of the real CheckHeldTx failure classes at a
// selected check: before the REVIEW receipt exists, or the handler's final
// check after the receipt was inserted in the same transaction.
type stagedMeterProof struct {
	*fleetpg.MeterStore
	final   bool
	failure error
	failed  int
}

func (p *stagedMeterProof) CheckHeldTx(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, outcome syncapp.Outcome) error {
	if err := p.MeterStore.CheckHeldTx(ctx, db, source, op, outcome); err != nil {
		return err
	}
	var receipt bool
	if err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM org_sync_acknowledgements WHERE org_id=$1 AND operation_id=$2)`, source.Scope.OrgID().String(), op.ID()).Scan(&receipt); err != nil {
		return err
	}
	if receipt == p.final {
		p.failed++
		return p.failure
	}
	return nil
}

// A REVIEW proof failure is a review-source failure at every check: the whole
// signed response is withheld (SourceDenied) and nothing is committed, whether
// the Store's pre-receipt check or the handler's final Confirm check fails.
func TestMeterIntakeReviewProofFailureWithholdsWholeResponseAtEveryCheck(t *testing.T) {
	for _, stage := range []struct {
		name  string
		final bool
	}{{"store-pre-receipt", false}, {"handler-final-confirm", true}} {
		for _, failure := range []struct {
			name string
			err  error
		}{{"receipt-mismatch", fa.ErrUnavailable}, {"source-binding", fa.ErrNotFound}} {
			t.Run(stage.name+"/"+failure.name, func(t *testing.T) {
				fx, meters, installed := meterEvidenceFixture(t)
				meters.Reviews = meterReviewStore(t, fx)
				proof := &stagedMeterProof{MeterStore: meters, final: stage.final, failure: failure.err}
				handler, err := fleetpg.NewMeterIntakeHandler(meters.Authority, proof)
				if err != nil {
					t.Fatal(err)
				}
				var declared time.Time
				if err := fx.f.Admin.QueryRow(t.Context(), `SELECT clock_timestamp()`).Scan(&declared); err != nil {
					t.Fatal(err)
				}
				op := meterReviewOperation(t, fx, meterObservedCommand(t, installed, declared, 7))
				matrixAssign(t, fx.deviceFixtureState, matrixPerson{member: fx.source.MembershipID}, "FINANCE", "ORG", "ACTIVE")
				ack, err := syncpg.NewStore(meterReviewIntakeConfig(fx)).Process(t.Context(), fx.source, op, protocol.DefaultLimits(), handler)
				if proof.failed != 1 {
					t.Fatal("selected actual proof check was not reached", proof.failed, err)
				}
				if !errors.Is(err, syncapp.ErrSourceDenied) || len(ack.CanonicalBytes()) != 0 {
					t.Fatalf("review proof failure did not withhold the whole response: %v", err)
				}
				var empty bool
				if err := fx.f.Admin.QueryRow(t.Context(), `SELECT NOT EXISTS(SELECT 1 FROM org_sync_operations WHERE org_id=$1 AND operation_id=$2) AND NOT EXISTS(SELECT 1 FROM org_sync_acknowledgements WHERE org_id=$1 AND operation_id=$2) AND NOT EXISTS(SELECT 1 FROM org_audit_events WHERE org_id=$1 AND operation_id=$2) AND NOT EXISTS(SELECT 1 FROM org_sync_review_items WHERE org_id=$1 AND operation_id=$2) AND NOT EXISTS(SELECT 1 FROM org_meter_held_readings WHERE org_id=$1 AND operation_id=$2)`, fx.scope.OrgID().String(), op.ID()).Scan(&empty); err != nil || !empty {
					t.Fatal("failed review proof left a claim, audit, item, fact or ACK", err)
				}
			})
		}
	}
}
