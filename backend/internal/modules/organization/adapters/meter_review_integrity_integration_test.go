//go:build integration

package adapters_test

import (
	"errors"
	"testing"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// A held fact reserves its exact reading identity: no ordinary reading may take
// it while held, and no hold may claim an identity an applied reading owns.
func TestMeterReviewHeldReadingIdentityIsReservedAcrossAppliedReadings(t *testing.T) {
	fx, meters, installed := meterEvidenceFixture(t)
	store := meterReviewStore(t, fx)
	command := meterObservedCommand(t, installed, time.Now().UTC().Add(-time.Hour), 7)
	held, err := command.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	_, ref, _, err := storeMeterHeldReview(t, fx, store, command)
	if err != nil {
		t.Fatal(err)
	}
	assets := fleetStore(t, fx.legalFixtureState)
	second, _, err := fleetWrite(t, fx.legalFixtureState, assets, fleetCommand(t, fleetCreateInput()))
	if err != nil {
		t.Fatal(err)
	}
	input := meterInstallInput(t, second.AssetID, time.Now().UTC().Add(-2*time.Hour))
	input.OpeningReadingID = held.ReadingID
	if _, _, err := meterInstall(t, fx.legalFixtureState, meters, meterInstallCommand(t, input)); !errors.Is(err, app.ErrOperationReused) {
		t.Fatal("an ordinary reading took the held reading identity", err)
	}
	var taken bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT EXISTS(SELECT 1 FROM org_meter_readings WHERE org_id=$1 AND id=$2)`, fx.scope.OrgID().String(), held.ReadingID).Scan(&taken); err != nil || taken {
		t.Fatal("held reading identity was applied while held", err)
	}
	decision, err := app.NewMeterReviewDecision(app.MeterReviewDecisionInput{OperationID: onboardingID(t), ReviewItemID: ref.ReviewItemID, ExpectedRevision: 1, Decision: app.MeterReviewAccept})
	if err != nil {
		t.Fatal(err)
	}
	if result, _, err := store.ResolveMeterReview(t.Context(), fx.scope, fleetActor(fx.legalFixtureState), fx.member, decision); err != nil || result.Snapshot().Outcome != protocol.ReviewAccepted {
		t.Fatal("reserved held identity could not be accepted", err)
	}
	// The reverse claim: a hold for the installed opening reading's identity.
	value, err := meter.NewValue(meter.Odometer, 7, nil)
	if err != nil {
		t.Fatal(err)
	}
	clash, err := app.NewMeterReadingCommand(app.MeterReadingInput{OperationID: onboardingID(t), AssetID: installed.AssetID, MeterID: installed.MeterID, SegmentID: installed.SegmentID, ReadingID: installed.OpeningReadingID, Value: value, Declared: orgtime.Declaration{OccurredAt: time.Now().UTC().Add(-time.Hour), EntryZone: "America/Cuiaba", DeviceOffsetSeconds: -14400}, Origin: meter.Standalone, Method: meter.Typed})
	if err != nil {
		t.Fatal(err)
	}
	op, _, _, err := storeMeterHeldReview(t, fx, store, clash)
	if err == nil {
		t.Fatal("a hold claimed an applied reading identity")
	}
	var empty bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT NOT EXISTS(SELECT 1 FROM org_sync_operations WHERE org_id=$1 AND operation_id=$2) AND NOT EXISTS(SELECT 1 FROM org_meter_held_readings WHERE org_id=$1 AND reading_id=$3)`, fx.scope.OrgID().String(), op.ID(), installed.OpeningReadingID).Scan(&empty); err != nil || !empty {
		t.Fatal("refused identity claim left a partial hold", err)
	}
}

// B-BR-EMP-164: an accepted resolution publishes readiness in the same
// transaction. If the effect is missing, nothing commits.
func TestMeterReviewAcceptedResolutionRequiresSameTransactionReadiness(t *testing.T) {
	fx, _, installed := meterEvidenceFixture(t)
	store := meterReviewStore(t, fx)
	op, ref, _, err := storeMeterHeldReview(t, fx, store, meterObservedCommand(t, installed, time.Now().UTC().Add(-time.Hour), 7))
	if err != nil {
		t.Fatal(err)
	}
	// Fault injection in this isolated database: the readiness row is silently
	// dropped, as a defective producer path that forgot it would behave.
	if _, err := fx.f.Admin.Exec(t.Context(), `CREATE FUNCTION synthetic_drop_effect() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RETURN NULL;END$$;CREATE TRIGGER synthetic_drop_effect BEFORE INSERT ON org_sync_effects FOR EACH ROW EXECUTE FUNCTION synthetic_drop_effect()`); err != nil {
		t.Fatal(err)
	}
	decision, err := app.NewMeterReviewDecision(app.MeterReviewDecisionInput{OperationID: onboardingID(t), ReviewItemID: ref.ReviewItemID, ExpectedRevision: 1, Decision: app.MeterReviewAccept})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.ResolveMeterReview(t.Context(), fx.scope, fleetActor(fx.legalFixtureState), fx.member, decision); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("accepted resolution committed without readiness", err)
	}
	var none bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT NOT EXISTS(SELECT 1 FROM org_sync_review_resolutions WHERE org_id=$1) AND NOT EXISTS(SELECT 1 FROM org_meter_readings WHERE org_id=$1 AND operation_id=$2) AND NOT EXISTS(SELECT 1 FROM org_audit_events WHERE org_id=$1 AND action='offl.review.resolved')`, fx.scope.OrgID().String(), op.ID()).Scan(&none); err != nil || !none {
		t.Fatal("accepted decision without readiness left partial history", err)
	}
	if _, err := fx.f.Admin.Exec(t.Context(), `DROP TRIGGER synthetic_drop_effect ON org_sync_effects`); err != nil {
		t.Fatal(err)
	}
	if result, _, err := store.ResolveMeterReview(t.Context(), fx.scope, fleetActor(fx.legalFixtureState), fx.member, decision); err != nil || result.Snapshot().Outcome != protocol.ReviewAccepted {
		t.Fatal("complete accepted decision unavailable", err)
	}
}

// A resolution instant must lie between the original receipt and the server
// clock; the immutable row can never be dated before the receipt it decides.
func TestMeterReviewResolutionTimeIsBoundedByReceiptAndServerClock(t *testing.T) {
	fx, _, installed := meterEvidenceFixture(t)
	store := meterReviewStore(t, fx)
	op, ref, ack, err := storeMeterHeldReview(t, fx, store, meterObservedCommand(t, installed, time.Now().UTC().Add(-time.Hour), 7))
	if err != nil {
		t.Fatal(err)
	}
	received, err := orgtime.ParseInstant(ack.Snapshot().ReceivedAt)
	if err != nil {
		t.Fatal(err)
	}
	var reviewer string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT employee_id::text FROM org_memberships WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), fx.member).Scan(&reviewer); err != nil {
		t.Fatal(err)
	}
	for name, at := range map[string]time.Time{"before-receipt": received.Add(-time.Second), "future": time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)} {
		t.Run(name, func(t *testing.T) {
			id := onboardingID(t)
			resolution, err := protocol.NewReviewResolution(protocol.ReviewResolutionSnapshot{OperationID: op.ID(), PayloadDigest: op.PayloadDigest(), ReviewItemID: ref.ReviewItemID, ResolutionID: id, ReviewRevision: 2, Outcome: protocol.ReviewRefused, Reason: "Synthetic misdated refusal", ResolvedAt: orgtime.FormatInstant(at)})
			if err != nil {
				t.Fatal(err)
			}
			err = database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
				_, err := tx.Exec(t.Context(), `INSERT INTO org_sync_review_resolutions(org_id,id,review_item_id,expected_revision,outcome,reason,reviewer_account_id,reviewer_membership_id,reviewer_employee_id,reviewer_session_id,request_hash,resolved_at,canonical_resolution)VALUES($1,$2,$3,1,'REFUSED','Synthetic misdated refusal',$4,$5,$6,$7,repeat('a',64),$8,$9)`, fx.scope.OrgID().String(), id, ref.ReviewItemID, fx.actor.AccountID, fx.member, reviewer, fx.actor.SessionID, at, resolution.CanonicalBytes())
				if err != nil {
					return err
				}
				_, err = tx.Exec(t.Context(), `SET CONSTRAINTS ALL IMMEDIATE`)
				return err
			})
			var pe *pgconn.PgError
			t.Logf("%s refusal: %v", name, err)
			if !errors.As(err, &pe) || pe.Code != "23514" || pe.Message == "review resolution publication unavailable" {
				t.Fatal("misdated resolution reached publication instead of the source guard", err)
			}
		})
	}
}
