//go:build integration

package adapters

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	cloud "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/driver/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/driver/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func movementClock(boot string, elapsed, utc int64) domain.Clock {
	return domain.Clock{BootID: boot, ElapsedMillis: elapsed, UTCMillis: utc}
}

func movementPolicy() domain.ClockPolicy {
	return domain.ClockPolicy{MaximumCheckpointGapMillis: 60_000, WallToleranceMillis: 1_000}
}

func startCommand(t *testing.T, vehicle string) app.ApplyShift {
	t.Helper()
	shift := newDriverID(t)
	return app.ApplyShift{
		ShiftID: shift,
		Create:  &app.ShiftStart{ID: shift, Origin: "android", Vehicle: vehicle, Timezone: "America/Cuiaba"},
		Command: domain.ShiftCommand{ID: newDriverID(t), Action: domain.ActionStart, ExpectedVersion: 0, Clock: movementClock("boot-a", 1_000, 1_000_000)},
		Policy:  movementPolicy(),
	}
}

func nextCommand(id, action string, version int64, clock domain.Clock) domain.ShiftCommand {
	return domain.ShiftCommand{ID: id, Action: action, ExpectedVersion: version, Clock: clock}
}

func TestDriverShiftPersistsReplayRestartAndClose(t *testing.T) {
	ctx := context.Background()
	p, dsn := freshDriverDB(t, migrations.Files)
	owner := seedDriverOwner(t, p)
	car := seedDriverVehicle(t, p, owner)
	s := New(p, cloud.RequireDriverOwnedVehicle)
	start := startCommand(t, car)
	opened, e := s.ApplyShift(ctx, owner, start)
	if e != nil {
		t.Fatal(e)
	}
	if opened.Replayed || opened.Shift.State != domain.ShiftRunning || opened.Shift.Version != 1 {
		t.Fatalf("open %+v", opened)
	}
	pause := app.ApplyShift{ShiftID: start.ShiftID, Command: nextCommand(newDriverID(t), domain.ActionPause, 1, movementClock("boot-a", 11_000, 1_010_000)), Policy: movementPolicy()}
	paused, e := s.ApplyShift(ctx, owner, pause)
	if e != nil || paused.Shift.State != domain.ShiftPaused || paused.Shift.ActiveMillis != 10_000 {
		t.Fatalf("pause %+v %v", paused, e)
	}
	cold, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer cold.Close()
	restarted := New(cold, cloud.RequireDriverOwnedVehicle)
	current, e := restarted.CurrentShift(ctx, owner)
	if e != nil || current.Version != 2 || current.State != domain.ShiftPaused || current.ActiveMillis != 10_000 {
		t.Fatalf("restart %+v %v", current, e)
	}
	replay, e := restarted.ApplyShift(ctx, owner, pause)
	if e != nil || !replay.Replayed || replay.Receipt.AcceptedVersion != 2 {
		t.Fatalf("lost response replay %+v %v", replay, e)
	}
	resume := app.ApplyShift{ShiftID: start.ShiftID, Command: nextCommand(newDriverID(t), domain.ActionResume, 2, movementClock("boot-a", 31_000, 1_030_000)), Policy: movementPolicy()}
	ended := app.ApplyShift{ShiftID: start.ShiftID, Command: nextCommand(newDriverID(t), domain.ActionEnd, 3, movementClock("boot-a", 41_000, 1_040_000)), Policy: movementPolicy()}
	resumed, e := restarted.ApplyShift(ctx, owner, resume)
	if e != nil || resumed.Shift.Version != 3 {
		t.Fatal("resume", resumed, e)
	}
	endedShift, e := restarted.ApplyShift(ctx, owner, ended)
	if e != nil || endedShift.Shift.State != domain.ShiftClosed || endedShift.Shift.Version != 4 || endedShift.Shift.ActiveMillis != 20_000 {
		t.Fatalf("end %+v %v", endedShift, e)
	}
	if _, e = restarted.CurrentShift(ctx, owner); !errors.Is(e, app.ErrNotFound) {
		t.Fatal("closed shift remained current", e)
	}
	closed, e := restarted.GetShift(ctx, owner, start.ShiftID)
	if e != nil || closed.State != domain.ShiftClosed || closed.ActiveMillis != 20_000 {
		t.Fatal("closed history", closed, e)
	}
	if _, e = restarted.GetShift(ctx, seedDriverOwner(t, p), start.ShiftID); !errors.Is(e, app.ErrNotFound) {
		t.Fatal("owner switch read", e)
	}
	foreignStart := startCommand(t, seedDriverVehicle(t, p, seedDriverOwner(t, p)))
	if _, e = restarted.ApplyShift(ctx, owner, foreignStart); !errors.Is(e, app.ErrDenied) {
		t.Fatal("foreign vehicle start", e)
	}
	stale := app.ApplyShift{ShiftID: start.ShiftID, Command: nextCommand(newDriverID(t), domain.ActionPause, 3, movementClock("boot-a", 42_000, 1_041_000)), Policy: movementPolicy()}
	if _, e = restarted.ApplyShift(ctx, owner, stale); !errors.Is(e, domain.ErrConflict) {
		t.Fatal("stale pause after close", e)
	}
}

func TestDriverShiftGapPersistenceAndReview(t *testing.T) {
	ctx := context.Background()
	p, dsn := freshDriverDB(t, migrations.Files)
	owner := seedDriverOwner(t, p)
	car := seedDriverVehicle(t, p, owner)
	s := New(p, cloud.RequireDriverOwnedVehicle)
	start := startCommand(t, car)
	if _, e := s.ApplyShift(ctx, owner, start); e != nil {
		t.Fatal(e)
	}
	checkpoint := app.ApplyShift{ShiftID: start.ShiftID, Command: nextCommand(newDriverID(t), domain.ActionCheckpoint, 1, movementClock("boot-b", 2_000, 1_001_000)), Policy: movementPolicy()}
	gap, e := s.ApplyShift(ctx, owner, checkpoint)
	if e != nil || gap.Shift.State != domain.ShiftRecoveryRequired || len(gap.Shift.Gaps) != 1 || gap.Shift.Gaps[0].Reason != domain.GapBootChanged {
		t.Fatalf("gap %+v %v", gap, e)
	}
	cold, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer cold.Close()
	current, e := New(cold, cloud.RequireDriverOwnedVehicle).CurrentShift(ctx, owner)
	if e != nil || current.State != domain.ShiftRecoveryRequired || len(current.Gaps) != 1 || current.CompleteWorkSeconds() != nil {
		t.Fatalf("gap restart %+v %v", current, e)
	}
	work := int64(500)
	review := app.ApplyShift{ShiftID: start.ShiftID, Command: domain.ShiftCommand{ID: newDriverID(t), Action: domain.ActionReviewGap, ExpectedVersion: 2, Clock: movementClock("boot-b", 3_000, 1_002_000), GapID: current.Gaps[0].ID, ReviewedWorkMillis: &work}, Policy: movementPolicy()}
	reviewed, e := New(cold, cloud.RequireDriverOwnedVehicle).ApplyShift(ctx, owner, review)
	if e != nil || reviewed.Shift.State != domain.ShiftPaused || reviewed.Shift.ActiveMillis != 500 || !reviewed.Shift.Gaps[0].Reviewed {
		t.Fatalf("review %+v %v", reviewed, e)
	}
	var dbReviewed bool
	var dbWork int64
	if e = p.QueryRow(ctx, "SELECT reviewed,reviewed_work_millis FROM driver_shift_gaps WHERE account_id=$1 AND shift_id=$2 AND id=$3", owner, start.ShiftID, current.Gaps[0].ID).Scan(&dbReviewed, &dbWork); e != nil || !dbReviewed || dbWork != 500 {
		t.Fatal("gap review not persisted", dbReviewed, dbWork, e)
	}
	again := review
	again.Command.ID = newDriverID(t)
	again.Command.ExpectedVersion = reviewed.Shift.Version
	if _, e = New(cold, cloud.RequireDriverOwnedVehicle).ApplyShift(ctx, owner, again); !errors.Is(e, domain.ErrConflict) {
		t.Fatal("gap reviewed twice", e)
	}
}

func TestDriverShiftConcurrentDevicesAndRollback(t *testing.T) {
	ctx := context.Background()
	p, _ := freshDriverDB(t, migrations.Files)
	owner := seedDriverOwner(t, p)
	carA, carB := seedDriverVehicle(t, p, owner), seedDriverVehicle(t, p, owner)
	s := New(p, cloud.RequireDriverOwnedVehicle)
	first, second := startCommand(t, carA), startCommand(t, carB)
	gate := make(chan struct{})
	type result struct {
		out domain.ShiftTransition
		err error
	}
	results := make(chan result, 2)
	for _, in := range []app.ApplyShift{first, second} {
		in := in
		go func() { <-gate; out, e := s.ApplyShift(ctx, owner, in); results <- result{out, e} }()
	}
	close(gate)
	success, conflicts := 0, 0
	for i := 0; i < 2; i++ {
		r := <-results
		switch {
		case r.err == nil:
			success++
		case errors.Is(r.err, domain.ErrConflict):
			conflicts++
		default:
			t.Fatal(r.err)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatal("two-device active shift", success, conflicts)
	}
	var active int
	if e := p.QueryRow(ctx, "SELECT count(*) FROM driver_shifts WHERE account_id=$1 AND state<>'CLOSED'", owner).Scan(&active); e != nil || active != 1 {
		t.Fatal("active shift count", active, e)
	}

	owner2 := seedDriverOwner(t, p)
	car2 := seedDriverVehicle(t, p, owner2)
	start2 := startCommand(t, car2)
	results = make(chan result, 2)
	gate = make(chan struct{})
	for i := 0; i < 2; i++ {
		go func() {
			<-gate
			out, e := New(p, cloud.RequireDriverOwnedVehicle).ApplyShift(ctx, owner2, start2)
			results <- result{out, e}
		}()
	}
	close(gate)
	replayed := 0
	for i := 0; i < 2; i++ {
		r := <-results
		if r.err != nil {
			t.Fatal(r.err)
		}
		if r.out.Replayed {
			replayed++
		}
	}
	if replayed != 1 {
		t.Fatal("identical concurrent start did not converge", replayed)
	}
	var commands int
	if e := p.QueryRow(ctx, "SELECT count(*) FROM driver_shift_commands WHERE account_id=$1 AND shift_id=$2", owner2, start2.ShiftID).Scan(&commands); e != nil || commands != 1 {
		t.Fatal("duplicate receipt", commands, e)
	}

	// A stale pause and a valid pause race; exactly one may advance the version.
	pauseA := app.ApplyShift{ShiftID: start2.ShiftID, Command: nextCommand(newDriverID(t), domain.ActionPause, 1, movementClock("boot-a", 11_000, 1_010_000)), Policy: movementPolicy()}
	pauseB := app.ApplyShift{ShiftID: start2.ShiftID, Command: nextCommand(newDriverID(t), domain.ActionPause, 1, movementClock("boot-a", 12_000, 1_011_000)), Policy: movementPolicy()}
	results = make(chan result, 2)
	gate = make(chan struct{})
	for _, in := range []app.ApplyShift{pauseA, pauseB} {
		in := in
		go func() {
			<-gate
			out, e := New(p, cloud.RequireDriverOwnedVehicle).ApplyShift(ctx, owner2, in)
			results <- result{out, e}
		}()
	}
	close(gate)
	ok, conflict := 0, 0
	for i := 0; i < 2; i++ {
		r := <-results
		if r.err == nil {
			ok++
		} else if errors.Is(r.err, domain.ErrConflict) {
			conflict++
		} else {
			t.Fatal(r.err)
		}
	}
	if ok != 1 || conflict != 1 {
		t.Fatal("stale pause race", ok, conflict)
	}

	// Denial on the final authorization must roll back the complete command.
	owner3 := seedDriverOwner(t, p)
	car3 := seedDriverVehicle(t, p, owner3)
	var calls atomic.Int32
	denyLast := func(ctx context.Context, tx pgx.Tx, a, v string) (bool, error) {
		if calls.Add(1) >= 3 {
			return false, nil
		}
		return cloud.RequireDriverOwnedVehicle(ctx, tx, a, v)
	}
	start3 := startCommand(t, car3)
	if _, e := New(p, denyLast).ApplyShift(ctx, owner3, start3); !errors.Is(e, app.ErrDenied) {
		t.Fatal("final owner proof denial", e)
	}
	var shifts, receipts, gaps int
	if e := p.QueryRow(ctx, "SELECT count(*) FROM driver_shifts WHERE account_id=$1", owner3).Scan(&shifts); e != nil {
		t.Fatal(e)
	}
	if e := p.QueryRow(ctx, "SELECT count(*) FROM driver_shift_commands WHERE account_id=$1", owner3).Scan(&receipts); e != nil {
		t.Fatal(e)
	}
	if e := p.QueryRow(ctx, "SELECT count(*) FROM driver_shift_gaps WHERE account_id=$1", owner3).Scan(&gaps); e != nil {
		t.Fatal(e)
	}
	if shifts != 0 || receipts != 0 || gaps != 0 {
		t.Fatal("denied command left partial state", shifts, receipts, gaps)
	}
}

func distanceRecord(t *testing.T, vehicle string, basis string, quality string, from, to time.Time) domain.DistanceRecord {
	t.Helper()
	r := domain.DistanceRecord{
		ID: newDriverID(t), Origin: "android", VehicleID: vehicle, Segment: "segment-a", Basis: basis,
		From: from, To: to, Timezone: "America/Cuiaba", Complete: true, Quality: quality,
	}
	if basis == "GPS_ESTIMATE" {
		v := int32(2)
		r.QualityVersion = &v
	}
	return r
}

func TestDriverDistancePersistenceOverlapAndReplay(t *testing.T) {
	ctx := context.Background()
	p, dsn := freshDriverDB(t, migrations.Files)
	owner := seedDriverOwner(t, p)
	car := seedDriverVehicle(t, p, owner)
	start, end := int64(10_000_000), int64(10_500_000)
	from := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)
	odo := distanceRecord(t, car, "ODOMETER", "MEASURED", from, to)
	metres := int64(500_000)
	odo.Metres = &metres
	odo.StartOdometerMetres = &start
	odo.EndOdometerMetres = &end
	operation := newDriverID(t)
	s := New(p, cloud.RequireDriverOwnedVehicle)
	receipt, e := s.SaveDistance(ctx, owner, app.SaveDistance{OperationID: operation, Record: odo})
	if e != nil || receipt.Replayed || receipt.ContentHash == "" {
		t.Fatal("save", receipt, e)
	}
	cold, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer cold.Close()
	got, e := New(cold, cloud.RequireDriverOwnedVehicle).GetDistance(ctx, owner, odo.ID)
	if e != nil || got.Metres == nil || *got.Metres != 500_000 || *got.StartOdometerMetres != start || *got.EndOdometerMetres != end || !got.Complete {
		t.Fatalf("restart %+v %v", got, e)
	}
	if replay, e := New(cold, cloud.RequireDriverOwnedVehicle).SaveDistance(ctx, owner, app.SaveDistance{OperationID: operation, Record: odo}); e != nil || !replay.Replayed {
		t.Fatal("retry", replay, e)
	}
	sameID := odo
	sameID.Segment = "segment-b"
	if _, e = New(cold, cloud.RequireDriverOwnedVehicle).SaveDistance(ctx, owner, app.SaveDistance{OperationID: newDriverID(t), Record: odo}); e != nil {
		t.Fatal("same body new operation", e)
	}
	if _, e = New(cold, cloud.RequireDriverOwnedVehicle).SaveDistance(ctx, owner, app.SaveDistance{OperationID: newDriverID(t), Record: sameID}); !errors.Is(e, domain.ErrConflict) {
		t.Fatal("changed body same ID", e)
	}
	overlap := distanceRecord(t, car, "ODOMETER", "MEASURED", from.Add(30*time.Minute), to.Add(30*time.Minute))
	overlap.Metres = &metres
	overlap.StartOdometerMetres = &start
	overlap.EndOdometerMetres = &end
	if _, e = New(cold, cloud.RequireDriverOwnedVehicle).SaveDistance(ctx, owner, app.SaveDistance{OperationID: newDriverID(t), Record: overlap}); !errors.Is(e, domain.ErrConflict) {
		t.Fatal("overlapping odometer", e)
	}
	adjacent := distanceRecord(t, car, "ODOMETER", "MEASURED", to, to.Add(time.Hour))
	adjacentEnd := end + 100_000
	adjacentMetres := int64(100_000)
	adjacent.Metres = &adjacentMetres
	adjacent.StartOdometerMetres = &end
	adjacent.EndOdometerMetres = &adjacentEnd
	if _, e = New(cold, cloud.RequireDriverOwnedVehicle).SaveDistance(ctx, owner, app.SaveDistance{OperationID: newDriverID(t), Record: adjacent}); e != nil {
		t.Fatal("adjacent window", e)
	}
	gps := distanceRecord(t, car, "GPS_ESTIMATE", "ESTIMATED", from, to)
	gps.Complete = false
	gps.HasGap = true
	gps.Metres = new(int64)
	*gps.Metres = 490_000
	gps.QualityNote = "gps-gap"
	if _, e = New(cold, cloud.RequireDriverOwnedVehicle).SaveDistance(ctx, owner, app.SaveDistance{OperationID: newDriverID(t), Record: gps}); e != nil {
		t.Fatal("different basis same window", e)
	}
	list, e := New(cold, cloud.RequireDriverOwnedVehicle).ListDistances(ctx, owner, car, from, to.Add(time.Hour), 10)
	if e != nil || len(list) != 3 || list[2].Basis != "ODOMETER" || !list[2].From.Equal(to) {
		t.Fatalf("list %+v %v", list, e)
	}
	seenBasis := map[string]bool{}
	for _, record := range list {
		seenBasis[record.Basis] = true
	}
	if !seenBasis["ODOMETER"] || !seenBasis["GPS_ESTIMATE"] {
		t.Fatal("list lost a basis", seenBasis)
	}
	foreignOwner := seedDriverOwner(t, p)
	if _, e = New(cold, cloud.RequireDriverOwnedVehicle).SaveDistance(ctx, foreignOwner, app.SaveDistance{OperationID: newDriverID(t), Record: odo}); !errors.Is(e, app.ErrDenied) {
		t.Fatal("foreign owner", e)
	}
	foreignCar := seedDriverVehicle(t, p, foreignOwner)
	foreign := distanceRecord(t, foreignCar, "MANUAL_TRIP", "MEASURED", from, to)
	foreign.Metres = &metres
	if _, e = New(cold, cloud.RequireDriverOwnedVehicle).SaveDistance(ctx, owner, app.SaveDistance{OperationID: newDriverID(t), Record: foreign}); !errors.Is(e, app.ErrDenied) {
		t.Fatal("foreign vehicle", e)
	}
	var forbidden int
	if e = p.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_name='driver_distance_observations' AND column_name IN ('latitude','longitude','passenger','raw_trace','token','notes')`).Scan(&forbidden); e != nil || forbidden != 0 {
		t.Fatal("forbidden distance fields", forbidden, e)
	}
	if _, e = p.Exec(ctx, "UPDATE accounts SET status='deleted' WHERE id=$1", owner); e != nil {
		t.Fatal(e)
	}
	for _, table := range []string{"driver_shifts", "driver_shift_gaps", "driver_shift_commands", "driver_distance_observations", "driver_distance_commands"} {
		if driverCount(t, p, table) != 0 {
			t.Fatal("soft deletion left movement data", table)
		}
	}
}

func TestDriverMovementSchemaAndUpgrade(t *testing.T) {
	ctx := context.Background()
	p, dsn := freshDriverDB(t, migrationCopy(t, false))
	for _, table := range []string{"driver_shifts", "driver_shift_gaps", "driver_shift_commands", "driver_distance_observations", "driver_distance_commands", "driver_cost_profiles", "driver_cost_profile_items", "driver_maintenance_schedules", "driver_reconciliation_decisions"} {
		var exists bool
		if e := p.QueryRow(ctx, "SELECT to_regclass($1) IS NULL", table).Scan(&exists); e != nil || !exists {
			t.Fatal("private table absent before upgrade", table, e)
		}
	}
	if _, e := migrate.Apply(ctx, dsn, migrations.Files); e != nil {
		t.Fatal(e)
	}
	for _, table := range []string{"driver_shifts", "driver_shift_gaps", "driver_shift_commands", "driver_distance_observations", "driver_distance_commands", "driver_cost_profiles", "driver_cost_profile_items", "driver_cost_commands", "driver_cost_owner_state", "driver_cost_tombstones", "driver_maintenance_schedules", "driver_maintenance_commands", "driver_maintenance_owner_state", "driver_maintenance_tombstones", "driver_reconciliation_decisions", "driver_reconciliation_records", "driver_reconciliation_commands"} {
		var exists bool
		if e := p.QueryRow(ctx, "SELECT to_regclass($1) IS NOT NULL", table).Scan(&exists); e != nil || !exists {
			t.Fatal("private table absent after upgrade", table, e)
		}
	}
	var index bool
	if e := p.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_indexes WHERE indexname='driver_one_nonclosed_shift')").Scan(&index); e != nil || !index {
		t.Fatal("active shift index", e)
	}
	var triggers int
	if e := p.QueryRow(ctx, "SELECT count(*) FROM pg_trigger WHERE tgname IN ('driver_erase_deleted_owner','driver_erase_deleted_owner_movement','driver_erase_deleted_owner_cost','driver_erase_deleted_owner_reconciliation')").Scan(&triggers); e != nil || triggers != 4 {
		t.Fatal("erasure triggers", triggers, e)
	}
}
