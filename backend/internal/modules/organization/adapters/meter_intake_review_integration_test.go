//go:build integration

package adapters_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	fleetpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/adapters"
	syncpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/adapters"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/jackc/pgx/v5"
)

func meterReviewIntakeConfig(fx syncFixtureState) syncpg.Config {
	config := fx.config
	config.NoAsyncPlans = map[syncapp.OperationKey]map[string]syncapp.FollowupMode{{Type: fleetpg.MeterReadingOperationType, Schema: 1}: {protocol.Accepted: syncapp.NoAsyncFollowup, protocol.AcceptedForReview: syncapp.AtomicReview, protocol.Rejected: syncapp.NoAsyncFollowup}}
	config.ConfirmSource = func(ctx context.Context, tx pgx.Tx, source syncapp.Source) (time.Time, error) {
		_, at, err := fx.devices.CheckCurrentIntakeDeviceTx(ctx, tx, source.Scope, source.Actor, source.MembershipID, source.EnrollmentID)
		if err != nil {
			return time.Time{}, syncapp.SourceDenied(err)
		}
		return at, nil
	}
	return config
}
func TestMeterIntakeRetainsRealPermissionChangeWithoutApplyingReading(t *testing.T) {
	fx, meters, installed := meterEvidenceFixture(t)
	meters.Reviews = meterReviewStore(t, fx)
	handler, err := fleetpg.NewMeterIntakeHandler(meters.Authority, meters)
	if err != nil {
		t.Fatal(err)
	}
	var declared time.Time
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT clock_timestamp()`).Scan(&declared); err != nil {
		t.Fatal(err)
	}
	command := meterObservedCommand(t, installed, declared, 7)
	op := meterReviewOperation(t, fx, command)
	matrixAssign(t, fx.deviceFixtureState, matrixPerson{member: fx.source.MembershipID}, "FINANCE", "ORG", "ACTIVE")
	store := syncpg.NewStore(meterReviewIntakeConfig(fx))
	ack, err := store.Process(t.Context(), fx.source, op, protocol.DefaultLimits(), handler)
	if err != nil || ack.Result() != protocol.AcceptedForReview || ack.Code() != "PERMISSION_CHANGED" {
		t.Fatal("actual ended permission was not retained for review", err)
	}
	var exact bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_sync_review_items WHERE org_id=$1 AND operation_id=$2)=1 AND(SELECT count(*) FROM org_meter_held_readings WHERE org_id=$1 AND operation_id=$2)=1 AND NOT EXISTS(SELECT 1 FROM org_meter_readings WHERE org_id=$1 AND operation_id=$2) AND NOT EXISTS(SELECT 1 FROM org_sync_effects WHERE org_id=$1 AND operation_id=$2) AND(SELECT count(*) FROM org_sync_acknowledgements WHERE org_id=$1 AND operation_id=$2 AND review_proof_version=1)=1`, fx.scope.OrgID().String(), op.ID()).Scan(&exact); err != nil || !exact {
		t.Fatal("held receipt lacked actual source or applied an effect", err)
	}
	// Current source is still mandatory, but the original author's minimal held
	// ACK does not depend on fleet.read after a later role reduction.
	matrixAssign(t, fx.deviceFixtureState, matrixPerson{member: fx.source.MembershipID}, "DRIVER", "SELF", "ACTIVE")
	replay, err := store.Process(t.Context(), fx.source, op, protocol.DefaultLimits(), handler)
	if err != nil || !bytes.Equal(replay.CanonicalBytes(), ack.CanonicalBytes()) {
		t.Fatal("minimal original held receipt was rewritten or required business read", err)
	}
	// A current-source failure after proof lookup must withhold the whole result.
	config := meterReviewIntakeConfig(fx)
	calls := 0
	sourceGuard := config.ConfirmSource
	config.ConfirmSource = func(ctx context.Context, tx pgx.Tx, s syncapp.Source) (time.Time, error) {
		calls++
		at, err := sourceGuard(ctx, tx, s)
		if err == nil && calls > 1 {
			return time.Time{}, errors.New("synthetic unavailable final source")
		}
		return at, err
	}
	denied, err := syncpg.NewStore(config).Process(t.Context(), fx.source, op, protocol.DefaultLimits(), handler)
	if !errors.Is(err, syncapp.ErrSourceDenied) || len(denied.CanonicalBytes()) != 0 {
		t.Fatal("final current-source failure exposed held receipt", err)
	}
}

type missingMeterHeldSource struct {
	syncapp.Handler
	proof syncapp.ReviewProof
}

func (h missingMeterHeldSource) ReviewProof() syncapp.ReviewProof { return h.proof }
func (h missingMeterHeldSource) Apply(context.Context, syncapp.DBTX, syncapp.Source, protocol.Operation) (syncapp.Outcome, error) {
	return syncapp.Outcome{Result: protocol.AcceptedForReview, Code: "PERMISSION_CHANGED", ReviewItemID: "7b0e2a54-0000-4000-8000-000000000099"}, nil
}
func TestMeterIntakeCannotCommitReviewIdentifierWithoutRealHeldSource(t *testing.T) {
	fx, meters, installed := meterEvidenceFixture(t)
	meters.Reviews = meterReviewStore(t, fx)
	handler, err := fleetpg.NewMeterIntakeHandler(meters.Authority, meters)
	if err != nil {
		t.Fatal(err)
	}
	var declared time.Time
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT clock_timestamp()`).Scan(&declared); err != nil {
		t.Fatal(err)
	}
	op := meterReviewOperation(t, fx, meterObservedCommand(t, installed, declared, 7))
	bad := missingMeterHeldSource{Handler: handler, proof: meters}
	ack, err := syncpg.NewStore(meterReviewIntakeConfig(fx)).Process(t.Context(), fx.source, op, protocol.DefaultLimits(), bad)
	if !errors.Is(err, syncapp.ErrSourceDenied) || len(ack.CanonicalBytes()) != 0 {
		t.Fatal("identifier without source proof exposed a final ACK", err)
	}
	var empty bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT NOT EXISTS(SELECT 1 FROM org_sync_operations WHERE org_id=$1 AND operation_id=$2) AND NOT EXISTS(SELECT 1 FROM org_sync_acknowledgements WHERE org_id=$1 AND operation_id=$2) AND NOT EXISTS(SELECT 1 FROM org_audit_events WHERE org_id=$1 AND operation_id=$2) AND NOT EXISTS(SELECT 1 FROM org_sync_review_items WHERE org_id=$1 AND operation_id=$2)`, fx.scope.OrgID().String(), op.ID()).Scan(&empty); err != nil || !empty {
		t.Fatal("failed proof left partial claim/audit/ACK", err)
	}
}
