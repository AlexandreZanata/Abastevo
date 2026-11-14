//go:build integration

package adapters_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	fleetpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	syncpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/adapters"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

// lifecycleIntakeFixture is an ACTIVE powered-free asset with an actual meter,
// the real signed intake store and the actual held-review owner.
func lifecycleIntakeFixture(t *testing.T) (syncFixtureState, *fleetpg.AssetStore, *fleetpg.MeterStore, app.MeterInstallResult, syncapp.Handler, int64) {
	t.Helper()
	fx := syncFixture(t)
	assets := lifecycleStore(t, fx.legalFixtureState)
	var unit string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT id::text FROM org_units WHERE org_id=$1`, fx.scope.OrgID().String()).Scan(&unit); err != nil {
		t.Fatal(err)
	}
	input := fleetCreateInput()
	input.UnitID = unit
	created, _, err := fleetWrite(t, fx.legalFixtureState, assets, fleetCommand(t, input))
	if err != nil {
		t.Fatal(err)
	}
	meters := meterStore(t, fx.legalFixtureState)
	meters.Authority = &orgpg.MeterAuthority{Memberships: fx.legalFixtureState.store.Memberships, Roles: fx.legalFixtureState.store.Roles, Capabilities: fx.flags, Devices: fx.devices}
	installed, _, err := meterInstall(t, fx.legalFixtureState, meters, meterInstallCommand(t, meterInstallInput(t, created.AssetID, lifecycleDBNow(t, fx.legalFixtureState).Add(-6*time.Hour))))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := fleetWrite(t, fx.legalFixtureState, assets, fleetCommand(t, app.AssetWriteInput{Kind: app.ActivateAsset, AssetID: created.AssetID, ExpectedRevision: installed.AssetRevision})); err != nil {
		t.Fatal(err)
	}
	meters.Reviews = meterReviewStore(t, fx)
	handler, err := fleetpg.NewMeterIntakeHandler(meters.Authority, meters)
	if err != nil {
		t.Fatal(err)
	}
	return fx, assets, meters, installed, handler, installed.AssetRevision + 1
}

func lifecycleIntakeCount(t *testing.T, fx syncFixtureState, op protocol.Operation) (readings, held, effects int) {
	t.Helper()
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_meter_readings WHERE org_id=$1 AND operation_id=$2),(SELECT count(*) FROM org_meter_held_readings WHERE org_id=$1 AND operation_id=$2),(SELECT count(*) FROM org_sync_effects WHERE org_id=$1 AND operation_id=$2)`, fx.scope.OrgID().String(), op.ID()).Scan(&readings, &held, &effects); err != nil {
		t.Fatal(err)
	}
	return
}

// BUC-EMP-105: the exact disposal instant separates normal handling from a
// held ASSET_UNAVAILABLE review through the existing OFFL-008 owner.
func TestFleetDisposalHoldsSignedReadingsAtOrAfterTheExactInstant(t *testing.T) {
	fx, assets, meters, installed, handler, revision := lifecycleIntakeFixture(t)
	store := syncpg.NewStore(meterReviewIntakeConfig(fx))
	// Retained ADMIN coverage starts when the fixture assigned it, so every
	// declared instant stays after that start and at or before the server clock.
	disposal := lifecycleDBNow(t, fx.legalFixtureState).Truncate(time.Microsecond).Add(-time.Microsecond).Add(500 * time.Nanosecond)
	dispose := lifecycleInputFor(t, fx.legalFixtureState, fleet.Dispose, installed.AssetID, revision)
	dispose.At = disposal
	disposed, _, err := lifecycleWrite(t, fx.legalFixtureState, assets, lifecycleCommand(t, dispose))
	if err != nil {
		t.Fatal(err)
	}
	before := meterReviewOperation(t, fx, meterObservedCommand(t, installed, disposal.Add(-time.Nanosecond), 7))
	ack, err := store.Process(t.Context(), fx.source, before, protocol.DefaultLimits(), handler)
	if err != nil || ack.Result() != protocol.Accepted {
		t.Fatal("reading declared before the disposal lost normal handling", err)
	}
	if readings, held, effects := lifecycleIntakeCount(t, fx, before); readings != 1 || held != 0 || effects != 1 {
		t.Fatal("pre-disposal reading was not applied exactly once")
	}
	var reviews []string
	for _, offset := range []time.Duration{0, time.Nanosecond, time.Millisecond} {
		op := meterReviewOperation(t, fx, meterObservedCommand(t, installed, disposal.Add(offset), 7))
		ack, err := store.Process(t.Context(), fx.source, op, protocol.DefaultLimits(), handler)
		if err != nil || ack.Result() != protocol.AcceptedForReview || ack.Code() != "ASSET_UNAVAILABLE" || ack.Snapshot().ReviewItemID == "" {
			t.Fatal("post-disposal reading was not held for review", offset, err)
		}
		if readings, held, effects := lifecycleIntakeCount(t, fx, op); readings != 0 || held != 1 || effects != 0 {
			t.Fatal("held post-disposal reading applied an effect", offset)
		}
		replay, err := store.Process(t.Context(), fx.source, op, protocol.DefaultLimits(), handler)
		if err != nil || !bytes.Equal(replay.CanonicalBytes(), ack.CanonicalBytes()) {
			t.Fatal("held disposal receipt was rewritten on replay", err)
		}
		reviews = append(reviews, ack.Snapshot().ReviewItemID)
	}
	var proof bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_sync_review_items WHERE org_id=$1 AND code='ASSET_UNAVAILABLE')=3 AND(SELECT count(*) FROM org_sync_acknowledgements WHERE org_id=$1 AND code='ASSET_UNAVAILABLE' AND review_proof_version=1)=3`, fx.scope.OrgID().String()).Scan(&proof); err != nil || !proof {
		t.Fatal("held disposal items lack version-1 proof", err)
	}
	// The existing reviewer sees the typed item and decides it unchanged.
	reviewer := meters.Reviews
	item, err := reviewer.ReadMeterReview(t.Context(), fx.scope, fleetActor(fx.legalFixtureState), fx.member, reviews[0])
	if err != nil || item.Code != "ASSET_UNAVAILABLE" {
		t.Fatal("held disposal item unreadable by the existing reviewer", err)
	}
	decision, err := app.NewMeterReviewDecision(app.MeterReviewDecisionInput{OperationID: onboardingID(t), ReviewItemID: reviews[0], ExpectedRevision: 1, Decision: app.MeterReviewAccept})
	if err != nil {
		t.Fatal(err)
	}
	if result, _, err := reviewer.ResolveMeterReview(t.Context(), fx.scope, fleetActor(fx.legalFixtureState), fx.member, decision); err != nil || result.Snapshot().Outcome != protocol.ReviewAccepted {
		t.Fatal("existing review decision unavailable for a disposal hold", err)
	}
	// SQL defence: no live observation at or after the disposal instant.
	live := func(offset time.Duration) error {
		target := disposal.Add(offset)
		at, sub := target.UTC().Truncate(time.Microsecond), int16(target.Nanosecond()%1000)
		return database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
			_, err := tx.Exec(t.Context(), `INSERT INTO org_meter_readings(org_id,id,meter_id,segment_id,value,occurred_at,occurred_submicro,entry_zone,device_offset_seconds,author_membership_id,capture_kind,enrollment_id,operation_id,request_hash,origin,method,state) VALUES($1,$2,$3,$4,7,$5,$6,'UTC',0,$7,'ENROLLED_DEVICE',$8,$9,$10,'STANDALONE','TYPED','VALID')`,
				fx.scope.OrgID().String(), onboardingID(t), installed.MeterID, installed.SegmentID, at, sub, fx.source.MembershipID, fx.source.EnrollmentID, onboardingID(t), strings.Repeat("a", 64))
			return err
		})
	}
	if code := lifecycleSQLCode(live(0)); code != "23514" {
		t.Fatal("database accepted a live observation at the disposal instant", code)
	}
	if code := lifecycleSQLCode(live(-time.Nanosecond)); code != "" {
		t.Fatal("database refused a live observation before the disposal instant", code)
	}
	// After a revert there is no current disposal: normal handling returns,
	// while earlier held items stay held.
	if _, _, err := lifecycleWrite(t, fx.legalFixtureState, assets, lifecycleCommand(t, lifecycleInputFor(t, fx.legalFixtureState, fleet.RevertDisposal, installed.AssetID, disposed.Revision))); err != nil {
		t.Fatal(err)
	}
	after := meterReviewOperation(t, fx, meterObservedCommand(t, installed, disposal.Add(2*time.Millisecond), 7))
	if ack, err := store.Process(t.Context(), fx.source, after, protocol.DefaultLimits(), handler); err != nil || ack.Result() != protocol.Accepted {
		t.Fatal("reverted disposal still held readings", err)
	}
	var still bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_meter_held_readings WHERE org_id=$1)=3 AND NOT EXISTS(SELECT 1 FROM org_sync_review_resolutions WHERE org_id=$1 AND review_item_id=ANY($2::uuid[]))`, fx.scope.OrgID().String(), reviews[1:]).Scan(&still); err != nil || !still {
		t.Fatal("revert resolved or removed earlier held items", err)
	}
}

// A held ASSET_UNAVAILABLE fact needs an actual disposal at or before its instant.
func TestFleetDisposalHoldCannotBeFabricatedForAnAvailableAsset(t *testing.T) {
	fx, _, meters, installed, _, _ := lifecycleIntakeFixture(t)
	command := meterObservedCommand(t, installed, lifecycleDBNow(t, fx.legalFixtureState).Add(-time.Hour), 7)
	op := meterReviewOperation(t, fx, command)
	v := op.Snapshot()
	err := database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
		if _, err := fx.config.ConfirmSource(t.Context(), tx, fx.source); err != nil {
			return err
		}
		if _, err := tx.Exec(t.Context(), `INSERT INTO org_sync_operations(org_id,operation_id,owner_account_id,api_origin,membership_id,enrollment_id,operation_type,operation_schema,entity_type,entity_id,payload_digest)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, v.OrgID, op.ID(), v.OwnerAccountID, v.APIOrigin, v.MembershipID, v.EnrollmentID, v.Type, v.OperationSchema, v.Entity.Type, v.Entity.ID, op.PayloadDigest()); err != nil {
			return err
		}
		_, err := meters.Reviews.HoldMeterReadingTx(t.Context(), syncapp.Borrow(tx), fx.source, op, command, "ASSET_UNAVAILABLE")
		return err
	})
	if !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("fabricated disposal hold accepted for an available asset", err)
	}
	var none bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT NOT EXISTS(SELECT 1 FROM org_meter_held_readings WHERE org_id=$1) AND NOT EXISTS(SELECT 1 FROM org_sync_review_items WHERE org_id=$1)`, fx.scope.OrgID().String()).Scan(&none); err != nil || !none {
		t.Fatal("fabricated hold left a review item", err)
	}
	if _, err := meters.Reviews.HoldMeterReadingTx(context.Background(), nil, fx.source, op, command, "ASSET_BUSY"); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("undeclared held code accepted", err)
	}
}
