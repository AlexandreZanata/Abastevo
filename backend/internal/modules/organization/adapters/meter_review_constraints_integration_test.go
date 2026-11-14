//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/fleet"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// meterReviewReceiptAttempt runs one real current-source intake transaction:
// original operation, audit and the caller's review/ACK rows, then COMMIT.
func meterReviewReceiptAttempt(t *testing.T, fx syncFixtureState, op protocol.Operation, rows func(ctx context.Context, tx pgx.Tx, at time.Time) error) error {
	t.Helper()
	v := op.Snapshot()
	return database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
		if _, err := fx.config.ConfirmSource(t.Context(), tx, fx.source); err != nil {
			return err
		}
		if _, err := tx.Exec(t.Context(), `INSERT INTO org_sync_operations(org_id,operation_id,owner_account_id,api_origin,membership_id,enrollment_id,operation_type,operation_schema,entity_type,entity_id,payload_digest)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, v.OrgID, op.ID(), v.OwnerAccountID, v.APIOrigin, v.MembershipID, v.EnrollmentID, v.Type, v.OperationSchema, v.Entity.Type, v.Entity.ID, op.PayloadDigest()); err != nil {
			return err
		}
		_, err := fx.config.Audit.RecordChangeTx(t.Context(), tx, fx.scope, audit.Event{OperationID: op.ID(), ActorID: fx.source.Actor.AccountID, Action: "offl.operation.review", TargetType: "offl.operation", TargetID: op.ID(), ReasonCode: "intake", RequestHash: strings.TrimPrefix(op.PayloadDigest(), "sha256:")}, func(ctx context.Context, tx pgx.Tx) error {
			var at time.Time
			if err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&at); err != nil {
				return err
			}
			return rows(ctx, tx, at)
		})
		return err
	})
}

func meterReviewAck(ctx context.Context, tx pgx.Tx, op protocol.Operation, at time.Time, result, code, review string, proof any) error {
	v := op.Snapshot()
	var revision *int64
	if result == protocol.Accepted {
		one := int64(1)
		revision = &one
	}
	ack, err := protocol.NewAcknowledgement(op, protocol.AcknowledgementSnapshot{Result: result, Code: code, EntityRevision: revision, ReviewItemID: review, ReceivedAt: orgtime.FormatInstant(at)}, protocol.DefaultLimits())
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO org_sync_acknowledgements(org_id,operation_id,result,code,entity_revision,review_item_id,received_at,canonical_ack,audit_hash,review_proof_version)VALUES($1,$2,$3,$4,$5,NULLIF($6,'')::uuid,$7,$8,$9,$10)`, v.OrgID, op.ID(), result, code, revision, review, at, ack.CanonicalBytes(), strings.TrimPrefix(op.PayloadDigest(), "sha256:"), proof)
	return err
}

func TestMeterReviewNewReceiptsRequireVersionOneProofAndRealHeldFact(t *testing.T) {
	fx, _, installed := meterEvidenceFixture(t)
	store := meterReviewStore(t, fx)
	org := fx.scope.OrgID().String()
	type refusal struct {
		code, constraint string
	}
	cases := map[string]struct {
		want refusal
		rows func(t *testing.T, ctx context.Context, tx pgx.Tx, op protocol.Operation, command app.MeterReadingCommand, at time.Time) error
	}{
		// MATCH SIMPLE skips only retained legacy NULL rows: a new REVIEW with
		// an explicit NULL marker is forced to version1 and needs an actual item.
		"new-null-marker": {refusal{"23503", "org_sync_ack_review_proof_fkey"}, func(t *testing.T, ctx context.Context, tx pgx.Tx, op protocol.Operation, _ app.MeterReadingCommand, at time.Time) error {
			return meterReviewAck(ctx, tx, op, at, protocol.AcceptedForReview, "PERMISSION_CHANGED", onboardingID(t), nil)
		}},
		"unknown-proof-version": {refusal{"23514", ""}, func(t *testing.T, ctx context.Context, tx pgx.Tx, op protocol.Operation, _ app.MeterReadingCommand, at time.Time) error {
			return meterReviewAck(ctx, tx, op, at, protocol.AcceptedForReview, "PERMISSION_CHANGED", onboardingID(t), 2)
		}},
		"proof-on-final-non-review": {refusal{"23514", ""}, func(t *testing.T, ctx context.Context, tx pgx.Tx, op protocol.Operation, _ app.MeterReadingCommand, at time.Time) error {
			return meterReviewAck(ctx, tx, op, at, protocol.Rejected, "FORBIDDEN", "", 1)
		}},
		// A real version1 item and ACK without the typed owner fact never commits.
		"item-without-typed-fact": {refusal{"23514", ""}, func(t *testing.T, ctx context.Context, tx pgx.Tx, op protocol.Operation, _ app.MeterReadingCommand, at time.Time) error {
			review := onboardingID(t)
			if n, err := q.New(tx).InsertMeterReviewItem(ctx, q.InsertMeterReviewItemParams{OrgID: fx.scope.OrgID().String(), ID: review, OperationID: op.ID(), Code: "PERMISSION_CHANGED"}); err != nil || n != 1 {
				t.Fatal("actual review item insert", err)
			}
			return meterReviewAck(ctx, tx, op, at, protocol.AcceptedForReview, "PERMISSION_CHANGED", review, nil)
		}},
		// The typed fact must be the exact signed entity, not another reading.
		"fact-for-another-reading": {refusal{"23514", ""}, func(t *testing.T, ctx context.Context, tx pgx.Tx, op protocol.Operation, command app.MeterReadingCommand, at time.Time) error {
			review := onboardingID(t)
			if n, err := q.New(tx).InsertMeterReviewItem(ctx, q.InsertMeterReviewItemParams{OrgID: fx.scope.OrgID().String(), ID: review, OperationID: op.ID(), Code: "PERMISSION_CHANGED"}); err != nil || n != 1 {
				t.Fatal("actual review item insert", err)
			}
			in, err := command.Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			return q.New(tx).InsertMeterHeldReading(ctx, q.InsertMeterHeldReadingParams{OrgID: fx.scope.OrgID().String(), ReviewItemID: review, OperationID: op.ID(), ReadingID: onboardingID(t), AssetID: in.AssetID, MeterID: in.MeterID, SegmentID: in.SegmentID, Value: in.Value.Count(), OccurredAt: pgtype.Timestamptz{Time: in.Declared.OccurredAt.Truncate(time.Microsecond), Valid: true}, OccurredSubmicro: int16(in.Declared.OccurredAt.Nanosecond() % 1000), EntryZone: in.Declared.EntryZone, DeviceOffsetSeconds: in.Declared.DeviceOffsetSeconds, Origin: string(in.Origin), Method: string(in.Method), RequestHash: command.Digest(fx.scope)})
		}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			command := meterObservedCommand(t, installed, time.Now().UTC().Add(-time.Hour), 7)
			op := meterReviewOperation(t, fx, command)
			err := meterReviewReceiptAttempt(t, fx, op, func(ctx context.Context, tx pgx.Tx, at time.Time) error { return c.rows(t, ctx, tx, op, command, at) })
			var pe *pgconn.PgError
			if !errors.As(err, &pe) || pe.Code != c.want.code || c.want.constraint != "" && pe.ConstraintName != c.want.constraint {
				t.Fatalf("new receipt bypassed actual review proof: %v", err)
			}
			var empty bool
			if err := fx.f.Admin.QueryRow(t.Context(), `SELECT NOT EXISTS(SELECT 1 FROM org_sync_operations WHERE org_id=$1 AND operation_id=$2) AND NOT EXISTS(SELECT 1 FROM org_sync_acknowledgements WHERE org_id=$1 AND operation_id=$2) AND NOT EXISTS(SELECT 1 FROM org_audit_events WHERE org_id=$1 AND operation_id=$2) AND NOT EXISTS(SELECT 1 FROM org_sync_review_items WHERE org_id=$1 AND operation_id=$2) AND NOT EXISTS(SELECT 1 FROM org_meter_held_readings WHERE org_id=$1 AND operation_id=$2)`, org, op.ID()).Scan(&empty); err != nil || !empty {
				t.Fatal("refused review proof left a partial claim, audit, item or ACK", err)
			}
		})
	}
	// The same transaction shape with the actual typed fact commits version1.
	_, ref, _, err := storeMeterHeldReview(t, fx, store, meterObservedCommand(t, installed, time.Now().UTC().Add(-time.Hour), 7))
	if err != nil {
		t.Fatal("actual typed review proof did not commit", err)
	}
	var proven bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT a.review_proof_version=1 AND i.proof_version=1 FROM org_sync_review_items i JOIN org_sync_acknowledgements a ON(a.org_id,a.operation_id,a.review_item_id)=(i.org_id,i.operation_id,i.id) WHERE i.org_id=$1 AND i.id=$2`, org, ref.ReviewItemID).Scan(&proven); err != nil || !proven {
		t.Fatal("new REVIEW lacks version1 proof", err)
	}
	decision, err := app.NewMeterReviewDecision(app.MeterReviewDecisionInput{OperationID: onboardingID(t), ReviewItemID: ref.ReviewItemID, ExpectedRevision: 1, Decision: app.MeterReviewRefuse, Reason: "Synthetic explicit refusal"})
	if err != nil {
		t.Fatal(err)
	}
	if result, _, err := store.ResolveMeterReview(t.Context(), fx.scope, fleetActor(fx.legalFixtureState), fx.member, decision); err != nil || result.Snapshot().Outcome != protocol.ReviewRefused {
		t.Fatal("actual append-only refusal unavailable", err)
	}
	// Even with explicit maintenance privileges, populated review evidence stays
	// append-only (TRUNCATE targets the unreferenced held-fact table).
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT UPDATE,DELETE,TRUNCATE ON org_sync_review_items,org_meter_held_readings,org_sync_review_resolutions TO "+role); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{`UPDATE org_sync_review_items SET code=code`, `UPDATE org_sync_review_resolutions SET reason=reason`, `DELETE FROM org_sync_review_resolutions`, `DELETE FROM org_meter_held_readings`, `TRUNCATE org_meter_held_readings`} {
		err := database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error { _, err := tx.Exec(t.Context(), statement); return err })
		var pe *pgconn.PgError
		if !errors.As(err, &pe) || pe.Code != "55000" {
			t.Fatal("ordinary maintenance could rewrite review evidence", statement, err)
		}
	}
	var retained bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_sync_review_items WHERE org_id=$1)=1 AND(SELECT count(*) FROM org_meter_held_readings WHERE org_id=$1)=1 AND(SELECT count(*) FROM org_sync_review_resolutions WHERE org_id=$1 AND outcome='REFUSED')=1`, org).Scan(&retained); err != nil || !retained {
		t.Fatal("review evidence was not retained", err)
	}
}
