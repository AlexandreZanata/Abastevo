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
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/jackc/pgx/v5"
)

func meterReviewStore(t *testing.T, fx syncFixtureState) *fleetpg.MeterReviewStore {
	t.Helper()
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_sync_review_items,org_meter_held_readings,org_sync_review_resolutions TO "+role+";GRANT UPDATE(id) ON org_sync_review_items TO "+role); err != nil {
		t.Fatal(err)
	}
	catalog, err := audit.NewCatalog(fleetpg.MeterReviewAuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	return &fleetpg.MeterReviewStore{Pool: fx.f.Runtime, Authority: &orgpg.MeterReviewAuthority{Memberships: fx.legalFixtureState.store.Memberships, Roles: fx.legalFixtureState.store.Roles, Capabilities: fx.flags}, Audit: auditpg.NewStore(fx.f.Runtime, catalog), NewID: func() (string, error) { return onboardingID(t), nil }, ConfirmSource: fx.config.ConfirmSource}
}
func meterReviewOperation(t *testing.T, fx syncFixtureState, command app.MeterReadingCommand) protocol.Operation {
	t.Helper()
	in, err := command.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	return syncHTTPOperation(t, fx.deviceFixtureState, fx.source.EnrollmentID, func(raw map[string]any) {
		raw["operation_id"] = in.OperationID
		raw["type"] = fleetpg.MeterReadingOperationType
		raw["entity"] = map[string]any{"type": "meter_reading", "id": in.ReadingID, "expected_revision": nil}
		raw["occurred_at"] = orgtime.FormatInstant(in.Declared.OccurredAt)
		raw["entry_zone"] = in.Declared.EntryZone
		raw["device_offset_seconds"] = in.Declared.DeviceOffsetSeconds
		raw["payload"] = map[string]any{"asset_id": in.AssetID, "meter_id": in.MeterID, "segment_id": in.SegmentID, "kind": string(in.Value.Kind()), "value": "7", "method": string(in.Method)}
	})
}

func storeMeterHeldReview(t *testing.T, fx syncFixtureState, store *fleetpg.MeterReviewStore, command app.MeterReadingCommand) (protocol.Operation, app.ReviewReference, protocol.Acknowledgement, error) {
	t.Helper()
	op := meterReviewOperation(t, fx, command)
	v := op.Snapshot()
	var reference app.ReviewReference
	var ack protocol.Acknowledgement
	err := database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
		if _, err := fx.config.ConfirmSource(t.Context(), tx, fx.source); err != nil {
			return err
		}
		if _, err := tx.Exec(t.Context(), `INSERT INTO org_sync_operations(org_id,operation_id,owner_account_id,api_origin,membership_id,enrollment_id,operation_type,operation_schema,entity_type,entity_id,payload_digest)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, v.OrgID, op.ID(), v.OwnerAccountID, v.APIOrigin, v.MembershipID, v.EnrollmentID, v.Type, v.OperationSchema, v.Entity.Type, v.Entity.ID, op.PayloadDigest()); err != nil {
			return err
		}
		var err error
		reference, err = store.HoldMeterReadingTx(t.Context(), syncapp.Borrow(tx), fx.source, op, command, "PERMISSION_CHANGED")
		if err != nil {
			return err
		}
		outcome := syncapp.Outcome{Result: protocol.AcceptedForReview, Code: "PERMISSION_CHANGED", ReviewItemID: reference.ReviewItemID}
		if err := store.CheckHeldTx(t.Context(), syncapp.Borrow(tx), fx.source, op, outcome); err != nil {
			return err
		}
		_, err = fx.config.Audit.RecordChangeTx(t.Context(), tx, fx.scope, audit.Event{OperationID: op.ID(), ActorID: fx.source.Actor.AccountID, Action: "offl.operation.review", TargetType: "offl.operation", TargetID: op.ID(), ReasonCode: "intake", RequestHash: strings.TrimPrefix(op.PayloadDigest(), "sha256:")}, func(ctx context.Context, tx pgx.Tx) error {
			var at time.Time
			if err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&at); err != nil {
				return err
			}
			var err error
			ack, err = protocol.NewAcknowledgement(op, protocol.AcknowledgementSnapshot{Result: outcome.Result, Code: outcome.Code, ReviewItemID: reference.ReviewItemID, ReceivedAt: orgtime.FormatInstant(at)}, protocol.DefaultLimits())
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `INSERT INTO org_sync_acknowledgements(org_id,operation_id,result,code,review_item_id,received_at,canonical_ack,audit_hash)VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, v.OrgID, op.ID(), outcome.Result, outcome.Code, reference.ReviewItemID, at, ack.CanonicalBytes(), strings.TrimPrefix(op.PayloadDigest(), "sha256:"))
			return err
		})
		return err
	})
	return op, reference, ack, err
}

func TestMeterReviewKeepsActualHeldFactOutsideAppliedHistory(t *testing.T) {
	fx, _, installed := meterEvidenceFixture(t)
	store := meterReviewStore(t, fx)
	command := meterObservedCommand(t, installed, time.Now().UTC().Add(-time.Hour).Truncate(time.Second).Add(123456789*time.Nanosecond), 7)
	op, reference, ack, err := storeMeterHeldReview(t, fx, store, command)
	if err != nil || reference.Revision != 1 || ack.Snapshot().Result != protocol.AcceptedForReview {
		t.Fatal("actual typed held evidence was not committed", err)
	}
	var exact bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_sync_review_items WHERE org_id=$1)=1 AND(SELECT count(*) FROM org_meter_held_readings WHERE org_id=$1)=1 AND(SELECT count(*) FROM org_meter_readings WHERE org_id=$1)=1 AND NOT EXISTS(SELECT 1 FROM org_sync_effects WHERE org_id=$1) AND(SELECT review_proof_version FROM org_sync_acknowledgements WHERE org_id=$1 AND operation_id=$2)=1`, fx.scope.OrgID().String(), op.ID()).Scan(&exact); err != nil || !exact {
		t.Fatal("held fact became a business reading or readiness", err)
	}
	err = database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
		owned, err := store.CheckOwnReceiptTx(t.Context(), syncapp.Borrow(tx), fx.source, op)
		if err != nil {
			return err
		}
		if !owned {
			t.Fatal("original author cannot recover actual held receipt")
		}
		return store.CheckHeldTx(t.Context(), syncapp.Borrow(tx), fx.source, op, syncapp.Outcome{Result: protocol.AcceptedForReview, Code: "PERMISSION_CHANGED", ReviewItemID: reference.ReviewItemID})
	})
	if err != nil {
		t.Fatal("actual held source proof unavailable after commit", err)
	}
}

func TestMeterReviewReadsActualCurrentScopedFactAndPolicyDeadline(t *testing.T) {
	fx, _, installed := meterEvidenceFixture(t)
	store := meterReviewStore(t, fx)
	command := meterObservedCommand(t, installed, time.Now().UTC().Add(-time.Hour), 7)
	op, ref, ack, err := storeMeterHeldReview(t, fx, store, command)
	if err != nil {
		t.Fatal(err)
	}
	view, err := store.ReadMeterReview(t.Context(), fx.scope, fleetActor(fx.legalFixtureState), fx.member, ref.ReviewItemID)
	if err != nil || view.State != app.MeterReviewOpen || view.Revision != 1 || view.OperationID != op.ID() || view.PayloadDigest != op.PayloadDigest() || view.AuthorAccountID != fx.source.Actor.AccountID || view.AuthorMembershipID != fx.source.MembershipID || view.EnrollmentID != fx.source.EnrollmentID {
		t.Fatal("actual current reviewer source cannot read original held fact", err)
	}
	received, err := orgtime.ParseInstant(ack.Snapshot().ReceivedAt)
	if err != nil || !view.ReceivedAt.Equal(received) || !view.ExpiresAt.Equal(received.Add(60*24*time.Hour)) {
		t.Fatal("review expiry or original receipt was invented", err)
	}
	page, err := store.ListMeterReviews(t.Context(), fx.scope, fleetActor(fx.legalFixtureState), fx.member, app.MeterReviewFilter{Limit: 1, State: app.MeterReviewOpen})
	if err != nil || len(page.Items) != 1 || page.Items[0].ReviewItemID != ref.ReviewItemID || page.NextID != ref.ReviewItemID {
		t.Fatal("actual scoped review queue unavailable", err)
	}
}

func TestMeterReviewCurrentReviewerAppliesOnceWithoutRetaggingAuthorOrReceipt(t *testing.T) {
	fx, _, installed := meterEvidenceFixture(t)
	store := meterReviewStore(t, fx)
	command := meterObservedCommand(t, installed, time.Now().UTC().Add(-time.Hour).Truncate(time.Second).Add(987654321*time.Nanosecond), 7)
	op, ref, ack, err := storeMeterHeldReview(t, fx, store, command)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := app.NewMeterReviewDecision(app.MeterReviewDecisionInput{OperationID: onboardingID(t), ReviewItemID: ref.ReviewItemID, ExpectedRevision: 1, Decision: app.MeterReviewAccept})
	if err != nil {
		t.Fatal(err)
	}
	original, replay, err := store.ResolveMeterReview(t.Context(), fx.scope, fleetActor(fx.legalFixtureState), fx.member, decision)
	if err != nil || replay || original.Snapshot().Outcome != protocol.ReviewAccepted {
		t.Fatal("actual fresh current reviewer cannot accept held fact", err)
	}
	again, replay, err := store.ResolveMeterReview(t.Context(), fx.scope, fleetActor(fx.legalFixtureState), fx.member, decision)
	if err != nil || !replay || !bytes.Equal(original.CanonicalBytes(), again.CanonicalBytes()) {
		t.Fatal("same decision lost original immutable result", err)
	}
	in, _ := command.Snapshot()
	var exact bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT r.author_membership_id=$3::uuid AND r.enrollment_id=$4::uuid AND r.capture_kind='ENROLLED_DEVICE' AND r.review_resolution_id=$5::uuid AND r.received_at=a.received_at AND a.canonical_ack=$6::bytea AND a.result='ACCEPTED_FOR_REVIEW' AND(SELECT count(*) FROM org_sync_effects WHERE org_id=$1 AND operation_id=$2)=1 AND(SELECT count(*) FROM org_sync_review_resolutions WHERE org_id=$1)=1 AND(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND action='offl.review.resolved')=1 FROM org_meter_readings r JOIN org_sync_acknowledgements a ON(a.org_id,a.operation_id)=(r.org_id,r.operation_id) WHERE r.org_id=$1 AND r.operation_id=$2`, fx.scope.OrgID().String(), op.ID(), fx.source.MembershipID, fx.source.EnrollmentID, original.Snapshot().ResolutionID, ack.CanonicalBytes()).Scan(&exact); err != nil || !exact {
		t.Fatal("review retagged source or original receipt/ACK", err)
	}
	var sub int16
	var occurred time.Time
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT occurred_at,occurred_submicro FROM org_meter_readings WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), in.ReadingID).Scan(&occurred, &sub); err != nil || !occurred.Add(time.Duration(sub)*time.Nanosecond).Equal(in.Declared.OccurredAt) {
		t.Fatal("review narrowed original declaration", err)
	}
}

func TestMeterReviewRejectsActualAuthorSelfDecision(t *testing.T) {
	fx, _, installed := meterEvidenceFixture(t)
	store := meterReviewStore(t, fx)
	_, ref, _, err := storeMeterHeldReview(t, fx, store, meterObservedCommand(t, installed, time.Now().UTC().Add(-time.Hour), 7))
	if err != nil {
		t.Fatal(err)
	}
	decision, err := app.NewMeterReviewDecision(app.MeterReviewDecisionInput{OperationID: onboardingID(t), ReviewItemID: ref.ReviewItemID, ExpectedRevision: 1, Decision: app.MeterReviewRefuse, Reason: "Synthetic self review"})
	if err != nil {
		t.Fatal(err)
	}
	actor := app.FleetActor{AccountID: fx.person.web.AccountID, SessionID: fx.person.web.SessionID}
	if _, _, err := store.ResolveMeterReview(t.Context(), fx.scope, actor, fx.person.member, decision); !errors.Is(err, orgapi.SelfApproval) {
		t.Fatal("actual author could decide own held record", err)
	}
}

func TestMeterReviewOriginalAuthorReadsOnlyCorrelatedResult(t *testing.T) {
	fx, _, installed := meterEvidenceFixture(t)
	store := meterReviewStore(t, fx)
	op, _, _, err := storeMeterHeldReview(t, fx, store, meterObservedCommand(t, installed, time.Now().UTC().Add(-time.Hour), 7))
	if err != nil {
		t.Fatal(err)
	}
	results, err := store.OwnMeterReviewResults(t.Context(), fx.source, []protocol.ReviewSelector{{OperationID: op.ID(), PayloadDigest: op.PayloadDigest()}})
	if err != nil || len(results) != 1 || results[0].Resolution != nil || results[0].Selector.OperationID != op.ID() {
		t.Fatal("actual author cannot recover unresolved minimal result", err)
	}
}
