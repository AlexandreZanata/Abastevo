//go:build integration

package adapters_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	fleetpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/adapters"
	fa "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	memberpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	syncpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/adapters"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/jackc/pgx/v5"
)

// lateFixture is the actual meter owner with its OFFL-008 held-review store,
// the real signed intake store and the EMP-OFFL-009 late source composed as
// the runtime composes it (membership device store → orgsync store).
type lateFixture struct {
	syncFixtureState
	meters    *fleetpg.MeterStore
	installed fa.MeterInstallResult
	handler   syncapp.Handler
	intake    *syncpg.Store
}

func lateIntakeConfig(fx syncFixtureState) syncpg.Config {
	config := meterReviewIntakeConfig(fx)
	config.ConfirmLate = func(ctx context.Context, tx pgx.Tx, s syncapp.Source) (syncapp.LateAccess, time.Time, error) {
		v, at, err := fx.devices.CheckLateIntakeDeviceTx(ctx, tx, s.Scope, s.Actor, s.MembershipID, s.EnrollmentID)
		if err != nil {
			return syncapp.LateAccess{}, time.Time{}, syncapp.SourceDenied(err)
		}
		return syncapp.LateAccess{End: syncapp.LateEnd(v.End), EndedAt: v.EndedAt}, at, nil
	}
	return config
}

func lateIntakeFixture(t *testing.T) lateFixture {
	t.Helper()
	return lateIntakeFrom(t, syncFixture(t))
}

func lateIntakeFrom(t *testing.T, base syncFixtureState) lateFixture {
	t.Helper()
	fx, _, meters, installed, handler, _ := lifecycleIntakeFixtureOn(t, base)
	fx.f.GrantLateIntake(t)
	return lateFixture{syncFixtureState: fx, meters: meters, installed: installed, handler: handler, intake: syncpg.NewStore(lateIntakeConfig(fx))}
}

// end ends the member's period exactly as offboarding does at the database
// (account → root → membership): the 194 trigger revokes its enrollment
// ACCESS_ENDED with source MEMBERSHIP. It returns the server end instant.
func (x lateFixture) end(t *testing.T) time.Time {
	t.Helper()
	if err := lateEndMembership(t.Context(), x.syncFixtureState); err != nil {
		t.Fatal(err)
	}
	var ended time.Time
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT ended_at FROM org_memberships WHERE org_id=$1 AND id=$2 AND state='ENDED'`, x.scope.OrgID().String(), x.source.MembershipID).Scan(&ended); err != nil {
		t.Fatal(err)
	}
	return ended.UTC()
}

func lateEndMembership(ctx context.Context, fx syncFixtureState) error {
	return pgx.BeginFunc(ctx, fx.f.Runtime, func(tx pgx.Tx) error {
		org := fx.scope.OrgID().String()
		if _, err := tx.Exec(ctx, `SELECT set_config('anpfuel.org_id',$1,true)`, org); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `SELECT 1 FROM accounts WHERE id=$1 FOR UPDATE`, fx.source.Actor.AccountID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `SELECT 1 FROM organizations WHERE org_id=$1 FOR UPDATE`, org); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE org_memberships SET state='ENDED',end_reason='OFFBOARDED',ended_at=clock_timestamp(),revision=revision+1 WHERE org_id=$1 AND id=$2`, org, fx.source.MembershipID)
		return err
	})
}

func (x lateFixture) installedAt(t *testing.T) time.Time {
	t.Helper()
	var at time.Time
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT installed_at FROM org_meters WHERE org_id=$1 AND id=$2`, x.scope.OrgID().String(), x.installed.MeterID).Scan(&at); err != nil {
		t.Fatal(err)
	}
	return at.UTC()
}

func (x lateFixture) op(t *testing.T, declared time.Time) protocol.Operation {
	t.Helper()
	return meterReviewOperation(t, x.syncFixtureState, meterObservedCommand(t, x.installed, declared, 7))
}

func (x lateFixture) late(t *testing.T, op protocol.Operation) (protocol.Acknowledgement, error) {
	t.Helper()
	return x.intake.ProcessLate(t.Context(), x.source, op, protocol.DefaultLimits(), x.handler)
}

type lateRow struct {
	end, disposition, result, code string
	ended, received                time.Time
	window                         int
	revision                       int64
}

func (x lateFixture) row(t *testing.T, op protocol.Operation) (lateRow, bool) {
	t.Helper()
	var r lateRow
	err := x.f.Admin.QueryRow(t.Context(), `SELECT end_source,disposition,result,code,ended_at,received_at,window_days,COALESCE(settings_revision,0) FROM org_sync_late_intakes WHERE org_id=$1 AND operation_id=$2`, x.scope.OrgID().String(), op.ID()).Scan(&r.end, &r.disposition, &r.result, &r.code, &r.ended, &r.received, &r.window, &r.revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return lateRow{}, false
	}
	if err != nil {
		t.Fatal(err)
	}
	return r, true
}

func (x lateFixture) counts(t *testing.T, op protocol.Operation) (claims, acks, readings, held, effects, items int) {
	t.Helper()
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_sync_operations WHERE org_id=$1 AND operation_id=$2),(SELECT count(*) FROM org_sync_acknowledgements WHERE org_id=$1 AND operation_id=$2),(SELECT count(*) FROM org_meter_readings WHERE org_id=$1 AND operation_id=$2),(SELECT count(*) FROM org_meter_held_readings WHERE org_id=$1 AND operation_id=$2),(SELECT count(*) FROM org_sync_effects WHERE org_id=$1 AND operation_id=$2),(SELECT count(*) FROM org_sync_review_items WHERE org_id=$1 AND operation_id=$2)`, x.scope.OrgID().String(), op.ID()).Scan(&claims, &acks, &readings, &held, &effects, &items); err != nil {
		t.Fatal(err)
	}
	return
}

// held proves quarantine: ACCEPTED_FOR_REVIEW with the late code, one real
// review item and held reading, no reading, no effect and one decision row
// carrying the server evidence.
func (x lateFixture) held(t *testing.T, op protocol.Operation, ack protocol.Acknowledgement, err error, ended time.Time, disposition string) {
	t.Helper()
	if err != nil || ack.Result() != protocol.AcceptedForReview || ack.Code() != syncapp.LateHeldCode || ack.Snapshot().ReviewItemID == "" || ack.Snapshot().EntityRevision != nil {
		t.Fatal("pre-end operation was not quarantined", err, ack.Result(), ack.Code())
	}
	if claims, acks, readings, held, effects, items := x.counts(t, op); claims != 1 || acks != 1 || readings != 0 || held != 1 || effects != 0 || items != 1 {
		t.Fatal("quarantine applied or lost a fact", claims, acks, readings, held, effects, items)
	}
	row, ok := x.row(t, op)
	received, perr := orgtime.ParseInstant(ack.Snapshot().ReceivedAt)
	if !ok || perr != nil || row.disposition != disposition || row.result != protocol.AcceptedForReview || row.code != syncapp.LateHeldCode || !row.ended.Equal(ended) || !row.received.Equal(received) || row.end != string(syncapp.LateEndMembership) && row.end != string(syncapp.LateEndDevice) {
		t.Fatal("late decision evidence missing or different", row, perr)
	}
	var author bool
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT i.author_employee_id=m.employee_id AND i.code=$3 AND a.review_proof_version=1 FROM org_sync_review_items i JOIN org_sync_operations o ON(o.org_id,o.operation_id)=(i.org_id,i.operation_id) JOIN org_memberships m ON(m.org_id,m.id)=(o.org_id,o.membership_id) JOIN org_sync_acknowledgements a ON(a.org_id,a.operation_id)=(i.org_id,i.operation_id) WHERE i.org_id=$1 AND i.operation_id=$2`, x.scope.OrgID().String(), op.ID(), syncapp.LateHeldCode).Scan(&author); err != nil || !author {
		t.Fatal("held review item lacks its author or version-1 proof", err)
	}
}

// refused proves a final late refusal with its decision row and no fact.
func (x lateFixture) refused(t *testing.T, op protocol.Operation, ack protocol.Acknowledgement, err error, code, disposition string) {
	t.Helper()
	if err != nil || ack.Result() != protocol.Rejected || ack.Code() != code || !ack.IsFinal() {
		t.Fatal("late operation not refused", code, err, ack.Result(), ack.Code())
	}
	if claims, acks, readings, held, effects, items := x.counts(t, op); claims != 1 || acks != 1 || readings != 0 || held != 0 || effects != 0 || items != 0 {
		t.Fatal("late refusal stored a fact", claims, acks, readings, held, effects, items)
	}
	if row, ok := x.row(t, op); !ok || row.disposition != disposition || row.result != protocol.Rejected || row.code != code {
		t.Fatal("late refusal decision row missing or different", row)
	}
}

func (x lateFixture) untouched(t *testing.T, op protocol.Operation) {
	t.Helper()
	if claims, acks, readings, held, effects, items := x.counts(t, op); claims+acks+readings+held+effects+items != 0 {
		t.Fatal("refused or unavailable late source stored something", claims, acks, readings, held, effects, items)
	}
	if _, ok := x.row(t, op); ok {
		t.Fatal("refused late source recorded a decision")
	}
}

func (x lateFixture) replays(t *testing.T, op protocol.Operation, original protocol.Acknowledgement) {
	t.Helper()
	for range 2 {
		again, err := x.late(t, op)
		if err != nil || !bytes.Equal(again.CanonicalBytes(), original.CanonicalBytes()) {
			t.Fatal("late replay did not return the stored acknowledgement byte for byte", err)
		}
	}
}

// lateSourceDenied proves a whole-response source refusal carrying its exact
// cause (mapped to the bounded transport code by the signed route).
func lateSourceDenied(t *testing.T, err error, cause error) {
	t.Helper()
	if !errors.Is(err, syncapp.ErrSourceDenied) || cause != nil && !errors.Is(err, cause) {
		t.Fatalf("expected whole-response %v, got %v", cause, err)
	}
}

// VAL-04 shape on the actual meter owner: readings declared before the end are
// held, one declared at or after it is MEMBERSHIP_ENDED, a forged early
// declaration is only held, resends replay, nothing is applied or read.
func TestLateIntakeHoldsPreEndReadingsAndRefusesLaterOnesFromServerHistory(t *testing.T) {
	x := lateIntakeFixture(t)
	// Retained ADMIN coverage starts when the fixture assigned it: a current
	// reading declared now applies through the normal path.
	start := lifecycleDBNow(t, x.legalFixtureState)
	accepted := x.op(t, start)
	original, err := x.intake.Process(t.Context(), x.source, accepted, protocol.DefaultLimits(), x.handler)
	if err != nil || original.Result() != protocol.Accepted {
		t.Fatal("pre-end reading was not accepted while access was current", err)
	}
	if _, err := x.late(t, x.op(t, start)); err == nil {
		t.Fatal("a current period was admitted to the late path")
	}
	ended := x.end(t)
	if row, ok := x.row(t, accepted); ok {
		t.Fatal("normal acceptance recorded a late decision", row)
	}
	pre := x.op(t, ended.Add(-30*time.Minute))
	if ack, err := x.intake.Process(t.Context(), x.source, pre, protocol.DefaultLimits(), x.handler); err == nil || len(ack.CanonicalBytes()) != 0 {
		t.Fatal("the normal path admitted an ended period", err)
	}
	x.untouched(t, pre)

	declared := []time.Time{ended.Add(-6 * time.Hour), ended.Add(-30 * time.Minute), ended.Add(-time.Nanosecond)}
	for _, at := range declared {
		op := x.op(t, at)
		ack, err := x.late(t, op)
		x.held(t, op, ack, err, ended, string(syncapp.LateHoldDeclared))
		x.replays(t, op, ack)
		if row, _ := x.row(t, op); row.end != string(syncapp.LateEndMembership) || row.window != 15 {
			t.Fatal("membership end or default window not recorded", row)
		}
	}
	for _, at := range []time.Time{ended, ended.Add(time.Nanosecond), ended.Add(time.Hour)} {
		op := x.op(t, at)
		ack, err := x.late(t, op)
		x.refused(t, op, ack, err, syncapp.LateMembershipEndedCode, string(syncapp.LateEnded))
		x.replays(t, op, ack)
	}

	t.Run("forged-early-declaration-is-only-evidence", func(t *testing.T) {
		forged := x.op(t, x.installedAt(t).Add(time.Nanosecond))
		ack, err := x.late(t, forged)
		x.held(t, forged, ack, err, ended, string(syncapp.LateHoldDeclared))
		received, _ := orgtime.ParseInstant(ack.Snapshot().ReceivedAt)
		if !received.After(ended) {
			t.Fatal("the receipt instant came from the device declaration")
		}
	})
	t.Run("reused-identifier-other-digest", func(t *testing.T) {
		first := x.op(t, ended.Add(-time.Minute))
		ack, err := x.late(t, first)
		x.held(t, first, ack, err, ended, string(syncapp.LateHoldDeclared))
		in, _ := meterObservedCommand(t, x.installed, ended.Add(-2*time.Minute), 9).Snapshot()
		changed := syncHTTPOperation(t, x.deviceFixtureState, x.source.EnrollmentID, func(raw map[string]any) {
			raw["operation_id"] = first.ID()
			raw["type"] = fleetpg.MeterReadingOperationType
			raw["entity"] = map[string]any{"type": "meter_reading", "id": first.Snapshot().Entity.ID, "expected_revision": nil}
			raw["occurred_at"] = orgtime.FormatInstant(in.Declared.OccurredAt)
			raw["payload"] = map[string]any{"asset_id": in.AssetID, "meter_id": in.MeterID, "segment_id": in.SegmentID, "kind": "ODOMETER", "value": "9", "method": "TYPED"}
		})
		reused, err := x.late(t, changed)
		if err != nil || reused.Result() != protocol.Rejected || reused.Code() != "OPERATION_ID_REUSED" {
			t.Fatal("changed intent under a late identifier", err, reused.Code())
		}
		x.replays(t, first, ack)
	})
	t.Run("unknown-target-is-malformed", func(t *testing.T) {
		in, _ := meterObservedCommand(t, x.installed, ended.Add(-time.Minute), 7).Snapshot()
		op := syncHTTPOperation(t, x.deviceFixtureState, x.source.EnrollmentID, func(raw map[string]any) {
			raw["type"] = fleetpg.MeterReadingOperationType
			raw["entity"] = map[string]any{"type": "meter_reading", "id": onboardingID(t), "expected_revision": nil}
			raw["occurred_at"] = orgtime.FormatInstant(in.Declared.OccurredAt)
			raw["payload"] = map[string]any{"asset_id": in.AssetID, "meter_id": onboardingID(t), "segment_id": in.SegmentID, "kind": "ODOMETER", "value": "7", "method": "TYPED"}
		})
		ack, err := x.late(t, op)
		x.refused(t, op, ack, err, "MALFORMED", string(syncapp.LateHoldDeclared))
		x.replays(t, op, ack)
	})
	t.Run("pre-end-normal-receipt-is-not-reopened", func(t *testing.T) {
		if ack, err := x.late(t, accepted); !errors.Is(err, syncapp.ErrUnavailable) || errors.Is(err, syncapp.ErrSourceDenied) || len(ack.CanonicalBytes()) != 0 {
			t.Fatal("an ended member read a normal receipt through the late path", err)
		}
		var kept bool
		if err := x.f.Admin.QueryRow(t.Context(), `SELECT canonical_ack=$3 FROM org_sync_acknowledgements WHERE org_id=$1 AND operation_id=$2`, x.scope.OrgID().String(), accepted.ID(), original.CanonicalBytes()).Scan(&kept); err != nil || !kept {
			t.Fatal("late resend changed the original receipt", err)
		}
		if row, ok := x.row(t, accepted); ok {
			t.Fatal("late resend recorded a decision for a normal receipt", row)
		}
	})
	// Five held (three pre-end, the forged early one, the reused identifier's
	// original), three ENDED and one MALFORMED decision; only the normal
	// acceptance (beside the installation's opening reading) was applied.
	var items, held, intakeReadings, effects, intents, decisions int
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_sync_review_items WHERE org_id=$1 AND code='ACCESS_ENDED_BEFORE_RECEIPT'),(SELECT count(*) FROM org_meter_held_readings WHERE org_id=$1),
 (SELECT count(*) FROM org_meter_readings r WHERE r.org_id=$1 AND EXISTS(SELECT 1 FROM org_sync_operations o WHERE o.org_id=r.org_id AND o.operation_id=r.operation_id)),(SELECT count(*) FROM org_sync_effects WHERE org_id=$1),(SELECT count(*) FROM org_event_intents WHERE org_id=$1),
 (SELECT count(*) FROM org_sync_late_intakes WHERE org_id=$1)`, x.scope.OrgID().String()).Scan(&items, &held, &intakeReadings, &effects, &intents, &decisions); err != nil || items != 5 || held != 5 || intakeReadings != 1 || effects != 1 || intents != 0 || decisions != 9 {
		t.Fatal("late intake duplicated, lost or applied facts", err, items, held, intakeReadings, effects, intents, decisions)
	}
	// The panel queue lists the late items like any other held reading.
	page, err := x.meters.Reviews.ListMeterReviews(t.Context(), x.scope, fleetActor(x.legalFixtureState), x.member, fa.MeterReviewFilter{Limit: 20, State: fa.MeterReviewOpen})
	if err != nil || len(page.Items) != 5 {
		t.Fatal("late held items are not listed for a reviewer", err, len(page.Items))
	}
	for _, item := range page.Items {
		if item.Code != syncapp.LateHeldCode || item.AuthorMembershipID != x.source.MembershipID {
			t.Fatal("late held item lost its code or author", item.Code)
		}
	}
}

// Only a reviewer who is not the author resolves a late item; acceptance
// applies once with the original author and receipt, and the author's late
// replay keeps the original held acknowledgement.
func TestLateIntakeHeldReadingIsResolvedOnlyByAnotherReviewer(t *testing.T) {
	x := lateIntakeFixture(t)
	ended := x.end(t)
	op := x.op(t, ended.Add(-time.Hour))
	ack, err := x.late(t, op)
	x.held(t, op, ack, err, ended, string(syncapp.LateHoldDeclared))
	decision, err := fa.NewMeterReviewDecision(fa.MeterReviewDecisionInput{OperationID: onboardingID(t), ReviewItemID: ack.Snapshot().ReviewItemID, ExpectedRevision: 1, Decision: fa.MeterReviewAccept})
	if err != nil {
		t.Fatal(err)
	}
	resolution, replay, err := x.meters.Reviews.ResolveMeterReview(t.Context(), x.scope, fleetActor(x.legalFixtureState), x.member, decision)
	if err != nil || replay || resolution.Snapshot().Outcome != protocol.ReviewAccepted {
		t.Fatal("a current reviewer could not decide a late held reading", err)
	}
	var exact bool
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT r.author_membership_id=$3 AND r.enrollment_id=$4 AND r.received_at=a.received_at AND(SELECT count(*) FROM org_sync_effects e WHERE e.org_id=$1 AND e.operation_id=$2)=1
 FROM org_meter_readings r JOIN org_sync_acknowledgements a ON(a.org_id,a.operation_id)=(r.org_id,r.operation_id) WHERE r.org_id=$1 AND r.operation_id=$2`, x.scope.OrgID().String(), op.ID(), x.source.MembershipID, x.source.EnrollmentID).Scan(&exact); err != nil || !exact {
		t.Fatal("accepted late reading lost its original author, capture or receipt", err)
	}
	x.replays(t, op, ack)
}

// The window is the one in force at the end instant (ORG-008 settings); a
// later change neither reopens nor shortens it. An earlier claim cannot
// extend it and the declared instant cannot move it.
func TestLateIntakeWindowIsTheSettingsInForceAtTheEnd(t *testing.T) {
	x := lateIntakeFixture(t)
	x.f.GrantSettingsLifecycle(t)
	if _, err := x.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_settings_commands TO "+pgx.Identifier{x.f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	settings := settingsStore(t, x.legalFixtureState)
	current := settingsRead(t, x.legalFixtureState, settings, 0)
	values := current.Values
	values.Policies.IntakeWindowDays = 0
	closedRevision, _, err := settings.ChangeSettings(t.Context(), x.scope, x.actor, x.member, settingsCommand(t, current.Revision, values, ""))
	if err != nil {
		t.Fatal("window settings change", err)
	}
	ended := x.end(t)
	pre := x.op(t, ended.Add(-time.Hour))
	ack, err := x.late(t, pre)
	x.refused(t, pre, ack, err, syncapp.LateWindowClosedCode, string(syncapp.LateClosed))
	if row, _ := x.row(t, pre); row.window != 0 || row.revision != closedRevision.Revision {
		t.Fatal("window not read from the settings in force at the end", row)
	}
	values.Policies.IntakeWindowDays = 60
	if _, _, err := settings.ChangeSettings(t.Context(), x.scope, x.actor, x.member, settingsCommand(t, closedRevision.Revision, values, "")); err != nil {
		t.Fatal(err)
	}
	for _, at := range []time.Time{ended.Add(-time.Minute), ended.Add(time.Minute), time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Hour)} {
		op := x.op(t, at)
		ack, err := x.late(t, op)
		x.refused(t, op, ack, err, syncapp.LateWindowClosedCode, string(syncapp.LateClosed))
	}
	x.replays(t, pre, ack)
}

// EMP-IDN-015: an ACCESS_ENDED revocation command ends one device while the
// membership stays active; its end instant is the revocation's server time.
func TestLateIntakeDeviceCommandEndUsesTheRevocationInstant(t *testing.T) {
	x := lateIntakeFixture(t)
	record, err := x.devices.CheckDevice(t.Context(), x.scope, x.source.Actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := x.devices.RevokeDevice(t.Context(), x.scope, x.person.web, "", x.person.member, true, deviceRevokeCommand(t, x.scope, record, d.AccessEnded)); err != nil {
		t.Fatal(err)
	}
	var revoked time.Time
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT revoked_at FROM org_device_enrollments WHERE org_id=$1 AND id=$2 AND source_type='COMMAND'`, x.scope.OrgID().String(), x.source.EnrollmentID).Scan(&revoked); err != nil {
		t.Fatal(err)
	}
	revoked = revoked.UTC()
	before := x.op(t, revoked.Add(-time.Nanosecond))
	ack, err := x.late(t, before)
	x.held(t, before, ack, err, revoked, string(syncapp.LateHoldDeclared))
	if row, _ := x.row(t, before); row.end != string(syncapp.LateEndDevice) {
		t.Fatal("device command end not recorded", row)
	}
	after := x.op(t, revoked)
	ack, err = x.late(t, after)
	x.refused(t, after, ack, err, syncapp.LateDeviceRevokedCode, string(syncapp.LateEnded))
}

// LOST_OR_COMPROMISED is refused as a whole with nothing stored, also when
// the key was barred in another period; an unknown or still-enrolled period
// and another organization's period are not late sources.
func TestLateIntakeRefusesLostKeysForeignAndCurrentPeriods(t *testing.T) {
	t.Run("lost-period", func(t *testing.T) {
		x := lateIntakeFixture(t)
		record, err := x.devices.CheckDevice(t.Context(), x.scope, x.source.Actor)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := x.devices.RevokeDevice(t.Context(), x.scope, employeeActor(x.legalFixtureState), x.member, x.person.member, false, deviceRevokeCommand(t, x.scope, record, d.LostOrCompromised)); err != nil {
			t.Fatal(err)
		}
		x.end(t)
		op := x.op(t, lifecycleDBNow(t, x.legalFixtureState).Add(-time.Hour))
		ack, err := x.late(t, op)
		lateSourceDenied(t, err, app.ErrDeviceRevoked)
		if len(ack.CanonicalBytes()) != 0 {
			t.Fatal("lost device received an acknowledgement")
		}
		x.untouched(t, op)
		if _, err := x.devices.CheckLateIntakeDevice(t.Context(), x.scope, x.source.Actor, x.source.MembershipID, x.source.EnrollmentID); !errors.Is(err, app.ErrDeviceRevoked) {
			t.Fatal("lost device passed the late source check", err)
		}
	})
	t.Run("key-barred-in-a-later-period", func(t *testing.T) {
		x := lateIntakeFixture(t)
		ended := x.end(t)
		pre := x.op(t, ended.Add(-time.Hour))
		ack, err := x.late(t, pre)
		x.held(t, pre, ack, err, ended, string(syncapp.LateHoldDeclared))
		second := lateRehire(t, x)
		record, err := x.devices.CheckDevice(t.Context(), x.scope, x.source.Actor)
		if err != nil {
			t.Fatal("rehired period device", err)
		}
		if s, _ := record.Enrollment.Snapshot(x.scope); s.MembershipID != second {
			t.Fatal("rehire did not open a new enrolled period")
		}
		if _, err := x.devices.RevokeDevice(t.Context(), x.scope, employeeActor(x.legalFixtureState), x.member, second, false, deviceRevokeCommand(t, x.scope, record, d.LostOrCompromised)); err != nil {
			t.Fatal(err)
		}
		// Neither a new late operation nor a replay of the held one passes a barred key.
		for _, op := range []protocol.Operation{x.op(t, ended.Add(-2*time.Hour)), pre} {
			replay, err := x.late(t, op)
			lateSourceDenied(t, err, app.ErrDeviceRevoked)
			if len(replay.CanonicalBytes()) != 0 {
				t.Fatal("barred key received an acknowledgement")
			}
		}
	})
	t.Run("current-and-foreign-periods", func(t *testing.T) {
		x := lateIntakeFixture(t)
		op := x.op(t, lifecycleDBNow(t, x.legalFixtureState).Add(-time.Hour))
		ack, err := x.late(t, op)
		lateSourceDenied(t, err, app.ErrEmployeeNotFound)
		if len(ack.CanonicalBytes()) != 0 {
			t.Fatal("current period received a late acknowledgement")
		}
		x.untouched(t, op)
		ended := x.end(t)
		other := legalFixtureFor(t, x.f, x.onboarding, x.actor, x.flags)
		teamDriver(t, other, x.person.auth.Account.ID)
		if _, _, err := other.onboarding.Repository.ActivateOrganization(t.Context(), other.scope, other.actor, other.member, onboardingID(t), 1); err != nil {
			t.Fatal(err)
		}
		x.f.GrantLateIntake(t)
		foreign := x.source
		foreign.Scope = other.scope
		if _, err := x.devices.CheckLateIntakeDevice(t.Context(), other.scope, x.source.Actor, x.source.MembershipID, x.source.EnrollmentID); !errors.Is(err, app.ErrEmployeeNotFound) {
			t.Fatal("another organization's period is a late source", err)
		}
		pre := x.op(t, ended.Add(-time.Hour))
		if ack, err := x.intake.ProcessLate(t.Context(), foreign, pre, protocol.DefaultLimits(), x.handler); !errors.Is(err, syncapp.ErrSourceDenied) || len(ack.CanonicalBytes()) != 0 {
			t.Fatal("foreign organization admitted a late operation", err)
		}
		x.untouched(t, pre)
	})
}

// lateRehire gives the ended member a fresh invitation and a new ACTIVE
// membership with the same still-valid personal key (EMP-IDN-015 new period).
func lateRehire(t *testing.T, x lateFixture) string {
	t.Helper()
	catalog, err := audit.NewCatalog(memberpg.InvitationAuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	invites := &memberpg.InvitationStore{Pool: x.f.Runtime, Authority: &orgpg.InvitationAuthority{Memberships: x.legalFixtureState.store.Memberships, Roles: x.legalFixtureState.store.Roles, Capabilities: x.flags, Settings: &orgpg.SettingsStore{Pool: x.f.Runtime}}, Memberships: x.legalFixtureState.store.Memberships, Audit: auditpg.NewStore(x.f.Runtime, catalog)}
	invFx := invitationFixtureState{legalFixtureState: x.legalFixtureState, invites: invites}
	invitation, code := contactInvite(t, invFx, x.person.employee, "DRIVER")
	r := redemptionStore(t, invFx)
	preview, err := r.Redeem(t.Context(), x.person.web, app.RedemptionCommand{Action: "PREVIEW", Code: code})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Redeem(t.Context(), x.person.web, app.RedemptionCommand{Action: "ACCEPT", Code: code, OperationID: onboardingID(t), Consent: true, ConsentDigest: preview.ConsentDigest}); err != nil {
		t.Fatal(err)
	}
	if _, err := x.devices.EnrollDevice(t.Context(), x.scope, x.person.actor, deviceEnrollCommand(t, "Synthetic rehired period")); err != nil {
		t.Fatal(err)
	}
	return invitation.MembershipID
}

// Twelve identical late deliveries leave one claim, one decision and one
// held fact, with identical acknowledgements (V-IDEM).
func TestLateIntakeConcurrentIdenticalDeliveriesKeepOneDecision(t *testing.T) {
	x := lateIntakeFixture(t)
	ended := x.end(t)
	op := x.op(t, ended.Add(-time.Hour))
	acks, errs := make([]protocol.Acknowledgement, 12), make([]error, 12)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range acks {
		wg.Go(func() {
			<-start
			acks[i], errs[i] = x.intake.ProcessLate(t.Context(), x.source, op, protocol.DefaultLimits(), x.handler)
		})
	}
	close(start)
	wg.Wait()
	var first protocol.Acknowledgement
	for i, err := range errs {
		if err != nil {
			if errors.Is(err, syncapp.ErrSourceDenied) {
				t.Fatal("concurrent late delivery withheld the source", err)
			}
			continue
		}
		if first.CanonicalBytes() == nil {
			first = acks[i]
		} else if !bytes.Equal(first.CanonicalBytes(), acks[i].CanonicalBytes()) {
			t.Fatal("concurrent late deliveries produced different acknowledgements")
		}
	}
	if first.CanonicalBytes() == nil {
		t.Fatal("no concurrent late delivery completed")
	}
	x.held(t, op, first, nil, ended, string(syncapp.LateHoldDeclared))
	x.replays(t, op, first)
}

// The membership end and a normal delivery race in the root-lock order. End
// first: the delivery is refused as a whole and the late path then holds it.
// Delivery first: it is accepted, and a later late resend reads nothing.
func TestLateIntakeRacesTheMembershipEndInBothOrders(t *testing.T) {
	for _, endFirst := range []bool{true, false} {
		name := map[bool]string{true: "end-first-then-held", false: "intake-first-accepted"}[endFirst]
		t.Run(name, func(t *testing.T) {
			x := lateIntakeFixture(t)
			org := x.scope.OrgID().String()
			op := x.op(t, lifecycleDBNow(t, x.legalFixtureState))
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()
			holder, err := x.f.Runtime.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer holder.Rollback(ctx)
			if _, err := holder.Exec(ctx, `SELECT set_config('anpfuel.org_id',$1,true)`, org); err != nil {
				t.Fatal(err)
			}
			if _, err := holder.Exec(ctx, `SELECT 1 FROM organizations WHERE org_id=$1 FOR UPDATE`, org); err != nil {
				t.Fatal(err)
			}
			waiting := func(n int) {
				ticker := time.NewTicker(5 * time.Millisecond)
				defer ticker.Stop()
				for {
					var count int
					if err := x.f.Admin.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND usename=$1 AND wait_event_type='Lock'`, x.f.RuntimeRole).Scan(&count); err != nil {
						t.Fatal(err)
					}
					if count >= n {
						return
					}
					select {
					case <-ticker.C:
					case <-ctx.Done():
						t.Fatal("no lock wait observed", n)
					}
				}
			}
			ended := make(chan error, 1)
			type result struct {
				ack protocol.Acknowledgement
				err error
			}
			intake := make(chan result, 1)
			startEnd := func() { go func() { ended <- lateEndMembership(ctx, x.syncFixtureState) }() }
			startIntake := func() {
				go func() {
					ack, err := x.intake.Process(ctx, x.source, op, protocol.DefaultLimits(), x.handler)
					intake <- result{ack, err}
				}()
			}
			if endFirst {
				startEnd()
				waiting(1)
				startIntake()
			} else {
				startIntake()
				waiting(1)
				startEnd()
			}
			waiting(2)
			if err := holder.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			if err := <-ended; err != nil {
				t.Fatal("concurrent membership end failed", err)
			}
			got := <-intake
			var end time.Time
			if err := x.f.Admin.QueryRow(t.Context(), `SELECT ended_at FROM org_memberships WHERE org_id=$1 AND id=$2`, org, x.source.MembershipID).Scan(&end); err != nil {
				t.Fatal(err)
			}
			if endFirst {
				if !errors.Is(got.err, syncapp.ErrSourceDenied) || len(got.ack.CanonicalBytes()) != 0 {
					t.Fatal("an ended period was accepted after the end", got.err)
				}
				x.untouched(t, op)
				ack, err := x.late(t, op)
				x.held(t, op, ack, err, end.UTC(), string(syncapp.LateHoldDeclared))
				return
			}
			if got.err != nil || got.ack.Result() != protocol.Accepted {
				t.Fatal("delivery committed before the end was not accepted", got.err)
			}
			received, _ := orgtime.ParseInstant(got.ack.Snapshot().ReceivedAt)
			if !received.Before(end) {
				t.Fatal("acceptance recorded after the end", received, end)
			}
			if ack, err := x.late(t, op); !errors.Is(err, syncapp.ErrUnavailable) || errors.Is(err, syncapp.ErrSourceDenied) || len(ack.CanonicalBytes()) != 0 {
				t.Fatal("late resend reopened a normal receipt", err)
			}
		})
	}
}

// Late intake requires an ACTIVE organization with its capability enabled;
// suspension and plan state remain EMP-OFFL-012's decision (EDR-199).
func TestLateIntakeKeepsOrganizationRefusalsWhole(t *testing.T) {
	x := lateIntakeFixture(t)
	ended := x.end(t)
	var revision int64
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT revision FROM org_capabilities WHERE org_id=$1 AND capability='organizations'`, x.scope.OrgID().String()).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	if _, err := x.flags.Set(t.Context(), x.scope, capability.Actor{AccountID: x.actor.AccountID}, capability.Change{OperationID: onboardingID(t), Capability: "organizations", ExpectedRevision: revision, Enabled: false}); err != nil {
		t.Fatal(err)
	}
	op := x.op(t, ended.Add(-time.Hour))
	ack, err := x.late(t, op)
	lateSourceDenied(t, err, access.ErrEntitlementRequired)
	if len(ack.CanonicalBytes()) != 0 {
		t.Fatal("disabled capability acknowledged a late operation")
	}
	x.untouched(t, op)
}

// lateGenericHolder gives the synthetic validation owner a typed held fact,
// so the generic path proves claim evidence and dependency order end to end.
type lateGenericHolder struct{ *syncSQLHandler }

func (h lateGenericHolder) HoldLate(ctx context.Context, db syncapp.DBTX, s syncapp.Source, op protocol.Operation, code string) (syncapp.Outcome, error) {
	var payload struct {
		Value string `json:"value"`
	}
	if json.Unmarshal(op.PayloadBytes(), &payload) != nil || payload.Value == "" {
		return syncapp.Outcome{Result: protocol.Rejected, Code: "MALFORMED"}, nil
	}
	outcome := syncapp.Outcome{Result: protocol.AcceptedForReview, Code: code, ReviewItemID: op.ID()}
	if _, err := db.Exec(ctx, `INSERT INTO org_sync_test_records(org_id,operation_id,target_id,value,held,review_id,author_member,links,proposed_refs) VALUES($1,$2,$3,$4,true,$2,$5,'{}','{}')`, s.Scope.OrgID().String(), op.ID(), op.Snapshot().Entity.ID, payload.Value, s.MembershipID); err != nil {
		return syncapp.Outcome{}, err
	}
	return outcome, syncRecordReview(ctx, db, s, op, outcome)
}

func lateSyncOperation(t *testing.T, fx syncFixtureState, value string, deps []string, declared time.Time) protocol.Operation {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "owner_account_id": fx.source.Actor.AccountID, "api_origin": fx.source.Origin, "org_id": fx.scope.OrgID().String(), "membership_id": fx.source.MembershipID, "enrollment_id": fx.source.EnrollmentID, "type": syncTestType, "operation_schema": 1, "entity": map[string]any{"type": "setting", "id": fx.target, "expected_revision": nil}, "occurred_at": orgtime.FormatInstant(declared), "entry_zone": "UTC", "device_offset_seconds": 0, "depends_on": append([]string{}, deps...), "soft_references": []any{}, "attachments": []any{}, "payload": map[string]any{"value": value}})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := protocol.NewBinding(fx.source.Actor.AccountID, fx.source.Origin, fx.scope.OrgID().String())
	if err != nil {
		t.Fatal(err)
	}
	op, err := protocol.Seal(raw, binding)
	if err != nil {
		t.Fatal(err)
	}
	return op
}

// EDR-197: a claim the server already held before the end is server evidence
// that outranks a device clock running ahead; the late path keeps BR-OFFL-007
// dependency order (pending until accepted, claim kept) and applies nothing.
func TestLateIntakeEarlierClaimIsEvidenceAndDependenciesKeepTheirOrder(t *testing.T) {
	fx := syncFixture(t)
	fx.f.GrantLateIntake(t)
	config := fx.config
	config.ConfirmLate = lateIntakeConfig(fx).ConfirmLate
	store := syncpg.NewStore(config)
	handler := lateGenericHolder{fx.handler}
	now := lifecycleDBNow(t, fx.legalFixtureState)
	parent := lateSyncOperation(t, fx, "Synthetic parent", nil, now)
	// Declared two hours ahead: by the device clock this is after the end.
	child := lateSyncOperation(t, fx, "Synthetic fast-clock child", []string{parent.ID()}, now.Add(2*time.Hour))
	pending, err := store.Process(t.Context(), fx.source, child, protocol.DefaultLimits(), handler)
	if err != nil || pending.Result() != protocol.Retry || pending.Code() != "DEPENDENCY_PENDING" {
		t.Fatal("child was not claimed pending its parent", err, pending.Code())
	}
	if ack, err := store.Process(t.Context(), fx.source, parent, protocol.DefaultLimits(), handler); err != nil || ack.Result() != protocol.Accepted {
		t.Fatal("parent not accepted while access was current", err)
	}
	if err := lateEndMembership(t.Context(), fx); err != nil {
		t.Fatal(err)
	}
	var ended, claimed time.Time
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT m.ended_at,o.claimed_at FROM org_memberships m JOIN org_sync_operations o ON(o.org_id,o.membership_id)=(m.org_id,m.id) WHERE m.org_id=$1 AND o.operation_id=$2`, fx.scope.OrgID().String(), child.ID()).Scan(&ended, &claimed); err != nil {
		t.Fatal(err)
	}
	if !claimed.Before(ended) || !now.Add(2*time.Hour).After(ended) {
		t.Fatal("fixture did not produce a pre-end claim declared after the end")
	}
	held, err := store.ProcessLate(t.Context(), fx.source, child, protocol.DefaultLimits(), handler)
	if err != nil || held.Result() != protocol.AcceptedForReview || held.Code() != syncapp.LateHeldCode {
		t.Fatal("server claim evidence did not hold the child", err, held.Result(), held.Code())
	}
	var disposition string
	var effects int
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT l.disposition,(SELECT count(*) FROM org_sync_effects e WHERE e.org_id=l.org_id AND e.operation_id=l.operation_id) FROM org_sync_late_intakes l WHERE l.org_id=$1 AND l.operation_id=$2`, fx.scope.OrgID().String(), child.ID()).Scan(&disposition, &effects); err != nil || disposition != string(syncapp.LateHoldClaim) || effects != 0 {
		t.Fatal("claim basis not recorded or an effect applied", err, disposition, effects)
	}
	again, err := store.ProcessLate(t.Context(), fx.source, child, protocol.DefaultLimits(), handler)
	if err != nil || !bytes.Equal(again.CanonicalBytes(), held.CanonicalBytes()) {
		t.Fatal("claim-held child did not replay", err)
	}
	// Without a claim the same fast clock is a refusal after the end.
	fast := lateSyncOperation(t, fx, "Synthetic unclaimed fast clock", nil, now.Add(2*time.Hour))
	refused, err := store.ProcessLate(t.Context(), fx.source, fast, protocol.DefaultLimits(), handler)
	if err != nil || refused.Code() != syncapp.LateMembershipEndedCode {
		t.Fatal("an unclaimed post-end declaration was not refused", err, refused.Code())
	}
	// A late operation waiting on an unaccepted parent keeps its claim.
	orphan := lateSyncOperation(t, fx, "Synthetic waiting child", []string{onboardingID(t)}, ended.Add(-time.Minute))
	for range 2 {
		waiting, err := store.ProcessLate(t.Context(), fx.source, orphan, protocol.DefaultLimits(), handler)
		if err != nil || waiting.Result() != protocol.Retry || waiting.Code() != "DEPENDENCY_PENDING" {
			t.Fatal("late dependency order not kept", err, waiting.Code())
		}
	}
	var claims, decisions int
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_sync_operations WHERE org_id=$1 AND operation_id=$2),(SELECT count(*) FROM org_sync_late_intakes WHERE org_id=$1 AND operation_id=$2)`, fx.scope.OrgID().String(), orphan.ID()).Scan(&claims, &decisions); err != nil || claims != 1 || decisions != 0 {
		t.Fatal("pending late operation lost its claim or recorded a decision", err, claims, decisions)
	}
}
