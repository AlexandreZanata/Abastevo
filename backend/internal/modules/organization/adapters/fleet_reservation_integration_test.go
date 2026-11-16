//go:build integration

package adapters_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	fleetpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/adapters"
	fa "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
)

// reservationStore is the assignment store plus the exact EMP-FLEET-007
// grants and the explicit "no source" availability ports.
func reservationStore(t *testing.T, fx legalFixtureState) *fleetpg.AssetStore {
	t.Helper()
	s := assignmentStore(t, fx)
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON org_asset_reservations,org_asset_blocks TO "+role+";GRANT SELECT,INSERT ON org_asset_reservation_commands TO "+role+";GRANT SELECT,INSERT ON job_queue TO "+role); err != nil {
		t.Fatal(err)
	}
	// EMP-FLEET-011: once installed, the availability read follows an
	// implement's attachment, so the Fleet principal reads that table too.
	var attachments bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT to_regclass('public.org_asset_implement_attachments') IS NOT NULL`).Scan(&attachments); err != nil {
		t.Fatal(err)
	}
	if attachments {
		if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT ON org_asset_implement_attachments TO "+role); err != nil {
			t.Fatal(err)
		}
	}
	s.ActiveUse, s.MaintenanceHolds = fleetpg.NoActiveUse{}, fleetpg.NoMaintenanceHolds{}
	return s
}

// exactNow is the database clock floored to the microsecond.
func exactNow(t *testing.T, fx legalFixtureState) time.Time {
	t.Helper()
	return lifecycleDBNow(t, fx).Truncate(time.Microsecond)
}

// waitDBClock waits until the database clock reaches the instant (no sleep
// on a fixed duration: the database clock is observed).
func waitDBClock(t *testing.T, fx legalFixtureState, at time.Time) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var reached bool
		if err := fx.f.Admin.QueryRow(ctx, `SELECT clock_timestamp()>=$1`, at).Scan(&reached); err != nil {
			t.Fatal(err)
		}
		if reached {
			return
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal("database clock did not reach the instant")
		}
	}
}

func reserve(t *testing.T, fx legalFixtureState, s *fleetpg.AssetStore, v fa.ReservationRequestInput) (fa.ReservationResult, bool, error) {
	t.Helper()
	if v.OperationID == "" {
		v.OperationID = onboardingID(t)
	}
	if v.Purpose == "" {
		v.Purpose = "Synthetic reservation purpose"
	}
	c, err := fa.NewReservationRequest(v)
	if err != nil {
		t.Fatal("invalid fixture reservation", err)
	}
	return s.RequestReservation(t.Context(), fx.scope, fleetActor(fx), fx.member, onboardingID(t), c)
}

func actOn(t *testing.T, fx legalFixtureState, s *fleetpg.AssetStore, confirm bool, v fa.ReservationActInput) (fa.ReservationResult, bool, error) {
	t.Helper()
	if v.OperationID == "" {
		v.OperationID = onboardingID(t)
	}
	make := fa.NewReservationCancel
	if confirm {
		make = fa.NewReservationConfirm
	}
	c, err := make(v)
	if err != nil {
		t.Fatal("invalid fixture reservation act", err)
	}
	return s.ActOnReservation(t.Context(), fx.scope, fleetActor(fx), fx.member, c)
}

func blockAsset(t *testing.T, fx legalFixtureState, s *fleetpg.AssetStore, v fa.BlockCreateInput) (fa.BlockResult, error) {
	t.Helper()
	if v.OperationID == "" {
		v.OperationID = onboardingID(t)
	}
	c, err := fa.NewBlockCreate(v)
	if err != nil {
		t.Fatal("invalid fixture block", err)
	}
	result, _, err := s.CreateBlock(t.Context(), fx.scope, fleetActor(fx), fx.member, onboardingID(t), c)
	return result, err
}

func unblock(t *testing.T, fx legalFixtureState, s *fleetpg.AssetStore, asset, block, reason string) error {
	t.Helper()
	c, err := fa.NewBlockRemove(fa.BlockRemoveInput{OperationID: onboardingID(t), AssetID: asset, BlockID: block, Reason: reason})
	if err != nil {
		t.Fatal("invalid fixture removal", err)
	}
	_, _, err = s.RemoveBlock(t.Context(), fx.scope, fleetActor(fx), fx.member, c)
	return err
}

func storedReservation(t *testing.T, fx legalFixtureState, id string) (string, string, *int64) {
	t.Helper()
	var state, cause string
	var source *int64
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT state,end_cause,source_revision FROM org_asset_reservations WHERE id=$1`, id).Scan(&state, &cause, &source); err != nil {
		t.Fatal(err)
	}
	return state, cause, source
}

func reservationsOf(t *testing.T, fx legalFixtureState, s *fleetpg.AssetStore, asset string) map[string]fa.ReservationView {
	t.Helper()
	page, err := s.ListReservations(t.Context(), fx.scope, fleetActor(fx), fx.member, asset, fa.PeriodFilter{Limit: 100})
	if err != nil {
		t.Fatal("reservation list", err)
	}
	out := map[string]fa.ReservationView{}
	for _, v := range page.Reservations {
		out[v.ID] = v
	}
	return out
}

func conflictWindow(t *testing.T, err error, from time.Time, to *time.Time) {
	t.Helper()
	var conflict fa.ReservationConflict
	if !errors.As(err, &conflict) || !conflict.From.Equal(from) || (to == nil) != (conflict.To == nil) || to != nil && !conflict.To.Equal(*to) {
		t.Fatalf("conflict window: %v %+v", err, conflict)
	}
}

// B-BR-EMP-294/295/297/298 and AC2 (sequential): commands, receipts, replay,
// window conflicts that reveal only the window, blocks and their removal.
func TestFleetReservationCommandsConflictsAndReplay(t *testing.T) {
	fx, _, unit := lifecycleFixture(t)
	s := reservationStore(t, fx)
	org := fx.scope.OrgID().String()
	asset, _ := lifecycleAssetIn(t, fx, s, unit, fleet.Active)
	employee, _ := assignmentPerson(t, fx)
	now := exactNow(t, fx)
	from, to := now.Add(time.Hour), now.Add(3*time.Hour)
	input := fa.ReservationRequestInput{OperationID: onboardingID(t), AssetID: asset, Start: from, End: to, EmployeeID: employee, Purpose: "Synthetic delivery round"}
	first, replayed, err := reserve(t, fx, s, input)
	if err != nil || replayed || first.State != fleet.ReservationRequested || first.AssetID != asset {
		t.Fatal("request", first, err)
	}
	if again, replayed, err := reserve(t, fx, s, input); err != nil || !replayed || again != first {
		t.Fatal("replay returns the first result", again, err)
	}
	other := input
	other.Purpose = "Synthetic other intent"
	if _, _, err := reserve(t, fx, s, other); !errors.Is(err, fa.ErrOperationReused) {
		t.Fatal("same operation, other intent", err)
	}
	var dueAt time.Time
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT not_before FROM job_queue WHERE dedupe_key=$1`, fleetpg.ReservationOutcomeDedupe(fleet.ExpireReservation, fx.scope, first.ReservationID)).Scan(&dueAt); err != nil || !dueAt.Equal(from) {
		t.Fatal("expire job not queued at the start", dueAt, err)
	}
	confirmed, _, err := actOn(t, fx, s, true, fa.ReservationActInput{AssetID: asset, ReservationID: first.ReservationID})
	if err != nil || confirmed.State != fleet.ReservationConfirmed {
		t.Fatal("confirm", err)
	}
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT not_before FROM job_queue WHERE dedupe_key=$1`, fleetpg.ReservationOutcomeDedupe(fleet.MissReservation, fx.scope, first.ReservationID)).Scan(&dueAt); err != nil || !dueAt.Equal(from.Add(time.Hour)) {
		t.Fatal("no-show job not queued at the deadline", dueAt, err)
	}
	// AC2 sequential: overlapping windows refuse with only the window, and
	// leave no row, receipt or audit event.
	before := assignmentCount(t, fx, `SELECT count(*) FROM org_audit_events WHERE org_id=$1`, org)
	for label, confirm := range map[string]bool{"overlapping confirmation": true, "overlapping request": false} {
		_, _, err := reserve(t, fx, s, fa.ReservationRequestInput{AssetID: asset, Start: from.Add(30 * time.Minute), End: to.Add(time.Hour), Confirm: confirm})
		if !errors.Is(err, fa.ErrReservationConflict) {
			t.Fatal(label, err)
		}
		conflictWindow(t, err, from, &to)
	}
	if n := assignmentCount(t, fx, `SELECT count(*) FROM org_asset_reservations WHERE org_id=$1`, org); n != 1 || before != assignmentCount(t, fx, `SELECT count(*) FROM org_audit_events WHERE org_id=$1`, org) {
		t.Fatal("refused overlap left rows or audit events", n)
	}
	// REQUESTED windows may overlap until one is confirmed.
	later, laterEnd := now.Add(5*time.Hour), now.Add(6*time.Hour)
	a, _, errA := reserve(t, fx, s, fa.ReservationRequestInput{AssetID: asset, Start: later, End: laterEnd})
	b, _, errB := reserve(t, fx, s, fa.ReservationRequestInput{AssetID: asset, Start: later.Add(30 * time.Minute), End: laterEnd.Add(time.Hour)})
	if errA != nil || errB != nil {
		t.Fatal("overlapping requests", errA, errB)
	}
	if _, _, err := actOn(t, fx, s, true, fa.ReservationActInput{AssetID: asset, ReservationID: a.ReservationID}); err != nil {
		t.Fatal(err)
	}
	_, _, err = actOn(t, fx, s, true, fa.ReservationActInput{AssetID: asset, ReservationID: b.ReservationID})
	conflictWindow(t, err, later, &laterEnd)
	// Cancel: the requesting account needs no reason; an ended reservation is
	// refused and stays as it is.
	if v, _, err := actOn(t, fx, s, false, fa.ReservationActInput{AssetID: asset, ReservationID: a.ReservationID}); err != nil || v.State != fleet.ReservationCancelled {
		t.Fatal("cancel by the requester", err)
	}
	for _, confirm := range []bool{true, false} {
		if _, _, err := actOn(t, fx, s, confirm, fa.ReservationActInput{AssetID: asset, ReservationID: a.ReservationID, Reason: map[bool]string{false: "Synthetic again"}[confirm]}); !errors.Is(err, fleet.ErrReservationEnded) {
			t.Fatal("act on a cancelled reservation", confirm, err)
		}
	}
	// The released window can now be confirmed.
	if _, _, err := actOn(t, fx, s, true, fa.ReservationActInput{AssetID: asset, ReservationID: b.ReservationID}); err != nil {
		t.Fatal("released window", err)
	}
	// Blocks: a confirmation over a block reports the block's window only.
	block, err := blockAsset(t, fx, s, fa.BlockCreateInput{AssetID: asset, Reason: fleet.LentOutBlock, Note: "Synthetic loan"})
	if err != nil {
		t.Fatal("block", err)
	}
	var blockFrom time.Time
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT valid_from FROM org_asset_blocks WHERE id=$1`, block.BlockID).Scan(&blockFrom); err != nil {
		t.Fatal(err)
	}
	c, _, err := reserve(t, fx, s, fa.ReservationRequestInput{AssetID: asset, Start: now.Add(10 * time.Hour), End: now.Add(11 * time.Hour)})
	if !errors.Is(err, fa.ErrReservationConflict) {
		t.Fatal("request over an open-ended block", c, err)
	}
	conflictWindow(t, err, blockFrom, nil)
	if state, _, _ := storedReservation(t, fx, first.ReservationID); state != "CONFIRMED" {
		t.Fatal("a block changed a reservation", state)
	}
	if err := unblock(t, fx, s, asset, block.BlockID, "Synthetic asset returned"); err != nil {
		t.Fatal("unblock", err)
	}
	if err := unblock(t, fx, s, asset, block.BlockID, "Synthetic again"); !errors.Is(err, fleet.ErrBlockEnded) {
		t.Fatal("unblock a removed block", err)
	}
	if _, err := fa.NewBlockRemove(fa.BlockRemoveInput{OperationID: onboardingID(t), AssetID: asset, BlockID: block.BlockID}); !errors.Is(err, fleet.ErrInvalid) {
		t.Fatal("a block removed without a reason", err)
	}
	if _, _, err := reserve(t, fx, s, fa.ReservationRequestInput{AssetID: asset, Start: now.Add(10 * time.Hour), End: now.Add(11 * time.Hour), Confirm: true}); err != nil {
		t.Fatal("window after the removal", err)
	}
	if n := assignmentCount(t, fx, `SELECT count(*) FROM org_asset_reservation_commands WHERE org_id=$1`, org); n != 10 {
		t.Fatal("one receipt per committed command", n)
	}
	// Window in the past and a missing reservation.
	if _, _, err := reserve(t, fx, s, fa.ReservationRequestInput{AssetID: asset, Start: now.Add(-time.Minute), End: now.Add(time.Hour)}); !errors.Is(err, fleet.ErrInvalid) {
		t.Fatal("window not in the future", err)
	}
	if _, _, err := actOn(t, fx, s, true, fa.ReservationActInput{AssetID: asset, ReservationID: onboardingID(t)}); !errors.Is(err, fa.ErrNotFound) {
		t.Fatal("unknown reservation", err)
	}
}

// Negative references: another organization's asset or employee is not
// found; an asset that left service accepts no reservation and its open
// reservations are cancelled in the same transaction (fleet.md §6.1).
func TestFleetReservationReferencesAndAssetLeavingService(t *testing.T) {
	fx, _, unit := lifecycleFixture(t)
	s := reservationStore(t, fx)
	other := legalFixtureFor(t, fx.f, fx.onboarding, fx.actor, fx.flags)
	if _, _, err := other.onboarding.Repository.ActivateOrganization(t.Context(), other.scope, other.actor, other.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	var otherUnit string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT id::text FROM org_units WHERE org_id=$1`, other.scope.OrgID().String()).Scan(&otherUnit); err != nil {
		t.Fatal(err)
	}
	foreignAsset, _ := lifecycleAssetIn(t, other, s, otherUnit, fleet.Active)
	foreignEmployee, _ := assignmentPerson(t, other)
	asset, revision := lifecycleAssetIn(t, fx, s, unit, fleet.Active)
	now := exactNow(t, fx)
	window := func(hours int) fa.ReservationRequestInput {
		return fa.ReservationRequestInput{AssetID: asset, Start: now.Add(time.Duration(hours) * time.Hour), End: now.Add(time.Duration(hours+1) * time.Hour)}
	}
	foreign := window(1)
	foreign.AssetID = foreignAsset
	if _, _, err := reserve(t, fx, s, foreign); !errors.Is(err, fa.ErrNotFound) {
		t.Fatal("asset of another organization", err)
	}
	if _, err := blockAsset(t, fx, s, fa.BlockCreateInput{AssetID: foreignAsset, Reason: fleet.SafetyBlock}); !errors.Is(err, fa.ErrNotFound) {
		t.Fatal("block on another organization's asset", err)
	}
	withForeign := window(1)
	withForeign.EmployeeID = foreignEmployee
	if _, _, err := reserve(t, fx, s, withForeign); !errors.Is(err, fa.ErrNotFound) {
		t.Fatal("employee of another organization", err)
	}
	inactiveEmployee := window(1)
	inactiveEmployee.EmployeeID = assignmentEmployeeIn(t, fx, "INACTIVE")
	if _, _, err := reserve(t, fx, s, inactiveEmployee); !errors.Is(err, fleet.ErrInvalid) {
		t.Fatal("employee not ACTIVE", err)
	}
	if n := assignmentCount(t, fx, `SELECT count(*) FROM org_asset_reservations`); n != 0 {
		t.Fatal("refused references left rows", n)
	}
	requested, _, err := reserve(t, fx, s, window(2))
	if err != nil {
		t.Fatal(err)
	}
	confirmedInput := window(4)
	confirmedInput.Confirm = true
	confirmed, _, err := reserve(t, fx, s, confirmedInput)
	if err != nil {
		t.Fatal(err)
	}
	soon := exactNow(t, fx).Add(300 * time.Millisecond)
	past, _, err := reserve(t, fx, s, fa.ReservationRequestInput{AssetID: asset, Start: soon, End: soon.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	waitDBClock(t, fx, soon)
	deactivated, _, err := lifecycleWrite(t, fx, s, lifecycleCommand(t, lifecycleInputFor(t, fx, fleet.Deactivate, asset, revision)))
	if err != nil {
		t.Fatal("deactivation with open reservations", err)
	}
	for _, id := range []string{requested.ReservationID, confirmed.ReservationID} {
		if state, cause, source := storedReservation(t, fx, id); state != "CANCELLED" || cause != "ASSET_LEFT_SERVICE" || source == nil || *source != deactivated.Revision {
			t.Fatal("reservation not cancelled with the asset leaving service", state, cause, source)
		}
	}
	// A reservation already past its start had ended by time: it is not
	// cancelled and reads EXPIRED.
	if state, _, _ := storedReservation(t, fx, past.ReservationID); state != "REQUESTED" || reservationsOf(t, fx, s, asset)[past.ReservationID].State != fleet.ReservationExpired {
		t.Fatal("expired reservation rewritten by the lifecycle change", state)
	}
	if _, _, err := reserve(t, fx, s, window(6)); !errors.Is(err, fleet.ErrInvalid) {
		t.Fatal("reservation over an INACTIVE asset", err)
	}
	if _, err := blockAsset(t, fx, s, fa.BlockCreateInput{AssetID: asset, Reason: fleet.AdministrativeBlock}); err != nil {
		t.Fatal("block on an INACTIVE asset", err)
	}
	for _, state := range []fleet.State{fleet.Draft, fleet.Disposed, fleet.Archived} {
		id, _ := lifecycleAssetIn(t, fx, s, unit, state)
		in := window(1)
		in.AssetID = id
		if _, _, err := reserve(t, fx, s, in); !errors.Is(err, fleet.ErrInvalid) {
			t.Fatal("reservation over an asset out of service", state, err)
		}
		if _, err := blockAsset(t, fx, s, fa.BlockCreateInput{AssetID: id, Reason: fleet.SafetyBlock}); !errors.Is(err, fleet.ErrInvalid) {
			t.Fatal("block on an asset out of service", state, err)
		}
	}
	// Disposal of an ACTIVE asset cancels its open reservations as well.
	disposable, disposableRevision := lifecycleAssetIn(t, fx, s, unit, fleet.Active)
	in := window(8)
	in.AssetID, in.Confirm = disposable, true
	open, _, err := reserve(t, fx, s, in)
	if err != nil {
		t.Fatal(err)
	}
	dispose := lifecycleInputFor(t, fx, fleet.Dispose, disposable, disposableRevision)
	dispose.At = lifecycleDBNow(t, fx)
	disposed, _, err := lifecycleWrite(t, fx, s, lifecycleCommand(t, dispose))
	if err != nil {
		t.Fatal("disposal with an open reservation", err)
	}
	if state, cause, source := storedReservation(t, fx, open.ReservationID); state != "CANCELLED" || cause != "ASSET_LEFT_SERVICE" || source == nil || *source != disposed.Revision {
		t.Fatal("disposal did not cancel the reservation", state, cause)
	}
	if n := assignmentCount(t, fx, `SELECT count(*) FROM org_asset_reservations WHERE org_id=$1`, other.scope.OrgID().String()); n != 0 {
		t.Fatal("foreign organization changed", n)
	}
}

// B-BR-EMP-296, AC3 and the job: with no job run, an unconfirmed reservation
// past its start reads EXPIRED and a confirmed one past its deadline reads
// NO_SHOW; the job settles each once (a second run changes nothing); any
// command on the asset settles due outcomes before it checks windows.
func TestFleetReservationTimeOutcomesAtReadTimeAndByJob(t *testing.T) {
	fx, _, unit := lifecycleFixture(t)
	s := reservationStore(t, fx)
	org := fx.scope.OrgID().String()
	asset, _ := lifecycleAssetIn(t, fx, s, unit, fleet.Active)
	soon := exactNow(t, fx).Add(400 * time.Millisecond)
	end := soon.Add(400 * time.Millisecond)
	requested, _, err := reserve(t, fx, s, fa.ReservationRequestInput{AssetID: asset, Start: soon, End: end.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	// The short window's deadline is its end (the earlier of start + 60 min).
	missed, _, err := reserve(t, fx, s, fa.ReservationRequestInput{AssetID: asset, Start: soon, End: end, Confirm: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SettleReservation(t.Context(), fx.scope, requested.ReservationID, fleet.ExpireReservation); !errors.Is(err, fa.ErrNotDue) {
		t.Fatal("settlement before its instant", err)
	}
	waitDBClock(t, fx, end)
	// Job stopped: the stored states are unchanged, the reads are not.
	views := reservationsOf(t, fx, s, asset)
	if v := views[requested.ReservationID]; v.State != fleet.ReservationExpired || v.EndCause != fa.EndedByTime || v.EndedAt == nil || !v.EndedAt.Equal(soon) {
		t.Fatal("AC3 EXPIRED at read time", v)
	}
	if v := views[missed.ReservationID]; v.State != fleet.ReservationNoShow || v.EndedAt == nil || !v.EndedAt.Equal(end) {
		t.Fatal("NO_SHOW at read time", v)
	}
	for _, id := range []string{requested.ReservationID, missed.ReservationID} {
		if state, _, _ := storedReservation(t, fx, id); state != "REQUESTED" && state != "CONFIRMED" {
			t.Fatal("stored state changed without a job", state)
		}
	}
	view, err := s.ReadAvailability(t.Context(), fx.scope, fleetActor(fx), fx.member, asset)
	if err != nil || view.Assessment.Availability != fleet.Available {
		t.Fatal("a no-show still reserves the asset", view, err)
	}
	// The job: run the queued outcome twice through the handler.
	handler := fleetpg.ReservationOutcomeHandler{Outcomes: s}
	job := func(command fleet.ReservationCommand, id string) jobs.Job {
		t.Helper()
		var v jobs.Job
		if err := fx.f.Admin.QueryRow(t.Context(), `SELECT id::text,kind,payload,dedupe_key,max_attempts FROM job_queue WHERE dedupe_key=$1`, fleetpg.ReservationOutcomeDedupe(command, fx.scope, id)).Scan(&v.ID, &v.Kind, &v.Payload, &v.DedupeKey, &v.MaxAttempts); err != nil {
			t.Fatal("queued outcome", err)
		}
		return v
	}
	audits := func(action string) int {
		return assignmentCount(t, fx, `SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND action=$2`, org, action)
	}
	miss := job(fleet.MissReservation, missed.ReservationID)
	for run := 0; run < 2; run++ {
		if err := handler.Handle(t.Context(), miss); err != nil {
			t.Fatal("job run", run, err)
		}
	}
	if state, cause, _ := storedReservation(t, fx, missed.ReservationID); state != "NO_SHOW" || cause != "TIME" || audits("fleet.reservation.missed") != 1 {
		t.Fatal("job run twice", state, cause, audits("fleet.reservation.missed"))
	}
	var actor string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT actor_id::text FROM org_audit_events WHERE org_id=$1 AND action='fleet.reservation.missed'`, org).Scan(&actor); err != nil || actor != fx.actor.AccountID {
		t.Fatal("settlement audited with the requesting account", actor, err)
	}
	// A later command on the asset settles the remaining due outcome first.
	if _, err := blockAsset(t, fx, s, fa.BlockCreateInput{AssetID: asset, Reason: fleet.SafetyBlock}); err != nil {
		t.Fatal(err)
	}
	if state, _, _ := storedReservation(t, fx, requested.ReservationID); state != "REQUESTED" {
		t.Fatal("a block settled outcomes (only reservation commands do)", state)
	}
	if _, _, err := reserve(t, fx, s, fa.ReservationRequestInput{AssetID: asset, Start: exactNow(t, fx).Add(48 * time.Hour), End: exactNow(t, fx).Add(49 * time.Hour)}); !errors.Is(err, fa.ErrReservationConflict) {
		t.Fatal("request over the open block", err)
	}
	if state, _, _ := storedReservation(t, fx, requested.ReservationID); state != "REQUESTED" {
		t.Fatal("a refused command kept its settlement", state)
	}
	expire := job(fleet.ExpireReservation, requested.ReservationID)
	if err := handler.Handle(t.Context(), expire); err != nil {
		t.Fatal(err)
	}
	if err := handler.Handle(t.Context(), expire); err != nil {
		t.Fatal(err)
	}
	if state, _, _ := storedReservation(t, fx, requested.ReservationID); state != "EXPIRED" || audits("fleet.reservation.expired") != 1 {
		t.Fatal("expire job", state)
	}
	// A malformed or foreign job never runs.
	bad := miss
	bad.DedupeKey = fleetpg.ReservationOutcomeDedupe(fleet.ExpireReservation, fx.scope, missed.ReservationID)
	if err := handler.Handle(t.Context(), bad); err == nil {
		t.Fatal("job with a mismatched de-duplication key ran")
	}
	payload, _ := json.Marshal(map[string]any{"version": 1, "org_id": org, "reservation_id": missed.ReservationID, "outcome": "FULFIL"})
	bad.Payload, bad.DedupeKey = payload, "x"
	if err := handler.Handle(t.Context(), bad); err == nil {
		t.Fatal("job for an unsourced outcome ran")
	}
	if err := (fleetpg.ReservationOutcomeHandler{}).Handle(t.Context(), miss); !errors.Is(err, fa.ErrUnavailable) {
		t.Fatal("handler without a store", err)
	}
}

// AC2 under concurrency, both orderings, with barriers: two overlapping
// confirmations wait behind a held organization root lock; the first queued
// commits and the second receives RESERVATION_CONFLICT revealing only the
// winner's window. Concurrent confirm and cancel end in one consistent state.
func TestFleetReservationConcurrentConfirmationsAndCancel(t *testing.T) {
	fx, _, unit := lifecycleFixture(t)
	s := reservationStore(t, fx)
	org := fx.scope.OrgID().String()
	race := func(first, second func() error) (error, error) {
		t.Helper()
		ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
		defer cancel()
		holder, err := fx.f.Runtime.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer holder.Rollback(ctx)
		var holderPID int
		if err := holder.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&holderPID); err != nil {
			t.Fatal(err)
		}
		if _, err := holder.Exec(ctx, `SELECT set_config('anpfuel.org_id',$1,true)`, org); err != nil {
			t.Fatal(err)
		}
		if _, err := holder.Exec(ctx, `SELECT 1 FROM organizations WHERE org_id=$1 FOR UPDATE`, org); err != nil {
			t.Fatal(err)
		}
		waitBlocked := func(n int) {
			ticker := time.NewTicker(5 * time.Millisecond)
			defer ticker.Stop()
			for {
				var blocked int
				if err := fx.f.Admin.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND usename=$1 AND wait_event_type='Lock' AND pg_blocking_pids(pid)<>'{}'`, fx.f.RuntimeRole).Scan(&blocked); err != nil {
					t.Fatal(err)
				}
				if blocked >= n {
					return
				}
				select {
				case <-ticker.C:
				case <-ctx.Done():
					t.Fatal("no lock wait observed")
				}
			}
		}
		results := [2]chan error{make(chan error, 1), make(chan error, 1)}
		go func() { results[0] <- first() }()
		waitBlocked(1)
		go func() { results[1] <- second() }()
		waitBlocked(2)
		if err := holder.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		return <-results[0], <-results[1]
	}
	for _, order := range []string{"A then B", "B then A"} {
		asset, _ := lifecycleAssetIn(t, fx, s, unit, fleet.Active)
		now := exactNow(t, fx)
		aFrom, aTo := now.Add(time.Hour), now.Add(3*time.Hour)
		bFrom, bTo := now.Add(2*time.Hour), now.Add(4*time.Hour)
		a, _, errA := reserve(t, fx, s, fa.ReservationRequestInput{AssetID: asset, Start: aFrom, End: aTo})
		b, _, errB := reserve(t, fx, s, fa.ReservationRequestInput{AssetID: asset, Start: bFrom, End: bTo})
		if errA != nil || errB != nil {
			t.Fatal(errA, errB)
		}
		confirmA := func() error {
			_, _, err := actOn(t, fx, s, true, fa.ReservationActInput{AssetID: asset, ReservationID: a.ReservationID})
			return err
		}
		confirmB := func() error {
			_, _, err := actOn(t, fx, s, true, fa.ReservationActInput{AssetID: asset, ReservationID: b.ReservationID})
			return err
		}
		if order == "A then B" {
			first, second := race(confirmA, confirmB)
			if first != nil {
				t.Fatal(order, "first queued confirmation lost", first)
			}
			conflictWindow(t, second, aFrom, &aTo)
		} else {
			first, second := race(confirmB, confirmA)
			if first != nil {
				t.Fatal(order, "first queued confirmation lost", first)
			}
			conflictWindow(t, second, bFrom, &bTo)
		}
		if n := assignmentCount(t, fx, `SELECT count(*) FROM org_asset_reservations WHERE asset_id=$1 AND state='CONFIRMED'`, asset); n != 1 {
			t.Fatal(order, "overlapping confirmations left", n)
		}
	}
	// Concurrent confirm and cancel of one REQUESTED reservation.
	for _, confirmFirst := range []bool{true, false} {
		asset, _ := lifecycleAssetIn(t, fx, s, unit, fleet.Active)
		now := exactNow(t, fx)
		r, _, err := reserve(t, fx, s, fa.ReservationRequestInput{AssetID: asset, Start: now.Add(time.Hour), End: now.Add(2 * time.Hour)})
		if err != nil {
			t.Fatal(err)
		}
		confirm := func() error {
			_, _, err := actOn(t, fx, s, true, fa.ReservationActInput{AssetID: asset, ReservationID: r.ReservationID})
			return err
		}
		cancelling := func() error {
			_, _, err := actOn(t, fx, s, false, fa.ReservationActInput{AssetID: asset, ReservationID: r.ReservationID, Reason: "Synthetic concurrent cancel"})
			return err
		}
		if confirmFirst {
			first, second := race(confirm, cancelling)
			if first != nil || second != nil {
				t.Fatal("confirm then cancel", first, second)
			}
		} else {
			first, second := race(cancelling, confirm)
			if first != nil || !errors.Is(second, fleet.ErrReservationEnded) {
				t.Fatal("cancel then confirm", first, second)
			}
		}
		if state, cause, _ := storedReservation(t, fx, r.ReservationID); state != "CANCELLED" || cause != "CANCELLED" {
			t.Fatal("concurrent confirm and cancel final state", confirmFirst, state)
		}
	}
}

// AC1/AC4/AC5 through the availability read and the start-check port on the
// borrowed intake transaction (B-BR-EMP-299/300).
func TestFleetAvailabilityReadAndStartCheckPort(t *testing.T) {
	fx, _, unit := lifecycleFixture(t)
	s := reservationStore(t, fx)
	asset, _ := lifecycleAssetIn(t, fx, s, unit, fleet.Active)
	reserved, reservedMember := assignmentPerson(t, fx)
	_, otherMember := assignmentPerson(t, fx)
	// The other member holds an ACTIVE assignment of the asset.
	other, _ := assignmentPerson(t, fx)
	_ = other
	var otherEmployee string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT employee_id::text FROM org_memberships WHERE id=$1`, otherMember).Scan(&otherEmployee); err != nil {
		t.Fatal(err)
	}
	if _, _, err := assignmentWrite(t, fx, s, assignmentCreate(t, fa.AssignmentCreateInput{AssetID: asset, Kind: fleet.AdditionalAssignment, EmployeeIDs: []string{otherEmployee}})); err != nil {
		t.Fatal(err)
	}
	decide := func(member string) fleet.StartDecision {
		t.Helper()
		var decision fleet.StartDecision
		if err := database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
			var err error
			decision, err = s.StartDecisionDBTX(t.Context(), syncapp.Borrow(tx), fx.scope, asset, member, "", exactNow(t, fx))
			return err
		}); err != nil {
			t.Fatal("start check", err)
		}
		return decision
	}
	read := func() fa.AvailabilityView {
		t.Helper()
		v, err := s.ReadAvailability(t.Context(), fx.scope, fleetActor(fx), fx.member, asset)
		if err != nil {
			t.Fatal("availability read", err)
		}
		return v
	}
	if v := read(); v.Assessment.Availability != fleet.Available || len(v.Assessment.Causes) != 0 || len(v.MissingSources) != 2 || v.MissingSources[0] != fa.ActiveUseSource || v.MissingSources[1] != fa.MaintenanceHoldSource {
		t.Fatal("available with the two missing sources", v)
	}
	if d := decide(otherMember); !d.Admitted {
		t.Fatal("assigned member on an available asset", d)
	}
	if d := decide(reservedMember); d.Admitted || d.Code != fleet.AssetUnavailable {
		t.Fatal("unassigned member admitted", d)
	}
	soon := exactNow(t, fx).Add(300 * time.Millisecond)
	r, _, err := reserve(t, fx, s, fa.ReservationRequestInput{AssetID: asset, Start: soon, End: soon.Add(2 * time.Hour), EmployeeID: reserved, Confirm: true})
	if err != nil {
		t.Fatal(err)
	}
	waitDBClock(t, fx, soon)
	if v := read(); v.Assessment.Availability != fleet.Reserved || len(v.Assessment.Causes) != 1 || v.Assessment.Causes[0].ID != r.ReservationID || v.Assessment.Causes[0].Kind != fleet.ReservationCause {
		t.Fatal("RESERVED with its cause", v)
	}
	// AC4: someone outside the reservation is held; the reserved employee
	// starts normally even without an assignment.
	if d := decide(otherMember); d.Admitted || d.Code != fleet.AssetUnavailable {
		t.Fatal("AC4 start by someone else", d)
	}
	if d := decide(reservedMember); !d.Admitted {
		t.Fatal("reserved employee held", d)
	}
	// AC5 and row 4: a block is BLOCKED first, RESERVED still listed.
	block, err := blockAsset(t, fx, s, fa.BlockCreateInput{AssetID: asset, Reason: fleet.SafetyBlock})
	if err != nil {
		t.Fatal(err)
	}
	if v := read(); v.Assessment.Availability != fleet.Blocked || len(v.Assessment.Causes) != 2 || v.Assessment.Causes[0].ID != block.BlockID || v.Assessment.Causes[1].Kind != fleet.ReservationCause {
		t.Fatal("BLOCKED with every cause", v)
	}
	if d := decide(reservedMember); d.Admitted || d.Code != fleet.AssetUnavailable {
		t.Fatal("next start after a block", d)
	}
	if err := unblock(t, fx, s, asset, block.BlockID, "Synthetic safety check passed"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := actOn(t, fx, s, false, fa.ReservationActInput{AssetID: asset, ReservationID: r.ReservationID, Reason: "Synthetic plan change"}); err != nil {
		t.Fatal(err)
	}
	if v := read(); v.Assessment.Availability != fleet.Available {
		t.Fatal("cancelled reservation released the window", v)
	}
	// Row 1: a deactivated asset is UNAVAILABLE whatever else applies.
	var revision int64
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT revision FROM org_assets WHERE id=$1`, asset).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	if _, _, err := lifecycleWrite(t, fx, s, lifecycleCommand(t, lifecycleInputFor(t, fx, fleet.Deactivate, asset, revision))); err != nil {
		t.Fatal(err)
	}
	if v := read(); v.Assessment.Availability != fleet.Unavailable || v.Assessment.Causes[0].Kind != fleet.LifecycleCause || v.Assessment.Causes[0].Lifecycle != fleet.Inactive {
		t.Fatal("UNAVAILABLE", v)
	}
	if d := decide(otherMember); d.Admitted || d.Code != fleet.AssetUnavailable {
		t.Fatal("start on an inactive asset", d)
	}
	// Fail closed without the explicit ports.
	bare := *s
	bare.ActiveUse = nil
	if _, err := bare.ReadAvailability(t.Context(), fx.scope, fleetActor(fx), fx.member, asset); !errors.Is(err, fa.ErrUnavailable) {
		t.Fatal("availability without the active-use port", err)
	}
}
