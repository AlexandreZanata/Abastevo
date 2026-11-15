//go:build integration

package adapters_test

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	fleetpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/adapters"
	fa "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	fuel "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	syncpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/adapters"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	trippg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/adapters"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	tripdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

// tripState is the SELF intake fixture plus an ACTIVE tanked vehicle with an
// odometer and the real trip.start/trip.finish owners (EMP-TRIP-002).
type tripState struct {
	selfIntakeState
	authority     *orgpg.TripAuthority
	store         *trippg.Store
	start, finish syncapp.Handler
	trips         *syncpg.Store
	vehicle       fa.MeterInstallResult
	vehicleAt     int64
}

func tripGrants(t *testing.T, fx legalFixtureState) {
	t.Helper()
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_trips,org_trip_finishes,org_trip_held_starts,org_trip_held_finishes,org_sync_review_items TO "+role+
		";GRANT UPDATE(state) ON org_trips TO "+role+";GRANT SELECT ON org_employee_identities,org_asset_unit_periods,org_asset_cost_centre_periods,org_meters,org_meter_readings,org_units,org_meter_segments TO "+role+
		";GRANT SELECT,INSERT,UPDATE ON org_asset_reservations,org_asset_blocks TO "+role+";GRANT SELECT,INSERT ON org_asset_reservation_commands TO "+role+";GRANT SELECT,INSERT ON job_queue TO "+role); err != nil {
		t.Fatal(err)
	}
	tripClosureGrant(t, fx.f)
}

// tripClosureGrant grants the EMP-TRIP-014 closures once the 000235 table
// exists: the completion, logbook and review guards read them, the closure
// owner inserts them (an upgrade rehearsal may stop at an earlier image).
func tripClosureGrant(t *testing.T, f *orgtest.Fixture) {
	t.Helper()
	var installed bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('public.org_trip_closures') IS NOT NULL`).Scan(&installed); err != nil {
		t.Fatal(err)
	}
	if installed {
		if _, err := f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_trip_closures TO "+pgx.Identifier{f.RuntimeRole}.Sanitize()); err != nil {
			t.Fatal(err)
		}
	}
}

func tripIntakeConfig(fx syncFixtureState) syncpg.Config {
	config := meterReviewIntakeConfig(fx)
	for _, kind := range []string{tripapp.StartOperationType, tripapp.FinishOperationType} {
		config.NoAsyncPlans[syncapp.OperationKey{Type: kind, Schema: 1}] = map[string]syncapp.FollowupMode{protocol.Accepted: syncapp.NoAsyncFollowup, protocol.Rejected: syncapp.NoAsyncFollowup, protocol.AcceptedForReview: syncapp.AtomicReview}
	}
	return config
}

func (x *tripState) handlers(t *testing.T, readings tripapp.Readings, store tripapp.Store) (syncapp.Handler, syncapp.Handler) {
	t.Helper()
	start, err := trippg.NewStartHandler(x.authority, readings, x.assets, store)
	if err != nil {
		t.Fatal(err)
	}
	finish, err := trippg.NewFinishHandler(x.authority, readings, store)
	if err != nil {
		t.Fatal(err)
	}
	return start, finish
}

func tripFixture(t *testing.T) tripState {
	t.Helper()
	return tripFixtureOn(t, selfIntakeFixture(t))
}

// tripFixtureOn builds the trip fixture on a given SELF intake fixture.
func tripFixtureOn(t *testing.T, base selfIntakeState) tripState {
	t.Helper()
	x := tripState{selfIntakeState: base}
	tripGrants(t, x.legalFixtureState)
	meterAuthority, ok := x.meters.Authority.(*orgpg.MeterAuthority)
	if !ok {
		t.Fatal("fixture meter authority is not the real one")
	}
	x.authority = (*orgpg.TripAuthority)(meterAuthority)
	x.store = &trippg.Store{Allocations: x.assets, NewID: func() (string, error) { return onboardingID(t), nil }}
	x.assets.OpenWork, x.assets.ActiveUse, x.assets.MaintenanceHolds = trippg.OpenWork{}, trippg.ActiveUse{}, fleetpg.NoMaintenanceHolds{}
	x.start, x.finish = x.handlers(t, x.meters, x.store)
	x.trips = syncpg.NewStore(tripIntakeConfig(x.syncFixtureState))
	input := fleetCreateInput()
	input.Profile.Kind, input.Profile.UsageBasis, input.UnitID = fleet.Vehicle, fleet.Distance, x.unit
	tank, err := fleet.NewTank(fuel.Catalog{}, fleet.Main, "100000", []string{"DIESEL_S10"})
	if err != nil {
		t.Fatal(err)
	}
	input.Profile.Tanks = []fleet.Tank{tank}
	created, _, err := fleetWrite(t, x.legalFixtureState, x.assets, fleetCommand(t, input))
	if err != nil {
		t.Fatal(err)
	}
	x.vehicle, _, err = meterInstall(t, x.legalFixtureState, x.meters, meterInstallCommand(t, meterInstallInput(t, created.AssetID, lifecycleDBNow(t, x.legalFixtureState).Add(-6*time.Hour))))
	if err != nil {
		t.Fatal(err)
	}
	active, _, err := fleetWrite(t, x.legalFixtureState, x.assets, fleetCommand(t, fa.AssetWriteInput{Kind: fa.ActivateAsset, AssetID: created.AssetID, ExpectedRevision: x.vehicle.AssetRevision}))
	if err != nil || active.State != fleet.Active {
		t.Fatal("tanked vehicle not active", err)
	}
	x.vehicleAt = active.Revision
	// FLEET-007 admits a start only for an assigned driver or the reserved one.
	x.assign(t, x.person.employee, x.vehicle.AssetID, fleet.PrimaryAssignment, nil, nil)
	return x
}

func (x tripState) declaredNow(t *testing.T) time.Time {
	t.Helper()
	return lifecycleDBNow(t, x.legalFixtureState).Truncate(time.Microsecond).Add(123 * time.Nanosecond)
}

func (x tripState) reading(t *testing.T, value string) map[string]any {
	return map[string]any{"meter_id": x.vehicle.MeterID, "segment_id": x.vehicle.SegmentID, "reading_id": onboardingID(t), "kind": "ODOMETER", "value": value}
}

func (x tripState) operation(t *testing.T, d syncFixtureState, kind, trip string, declared time.Time, payload map[string]any, edit func(envelope, payload map[string]any)) protocol.Operation {
	t.Helper()
	return syncHTTPOperation(t, d.deviceFixtureState, d.source.EnrollmentID, func(raw map[string]any) {
		raw["type"] = kind
		raw["entity"] = map[string]any{"type": tripapp.EntityType, "id": trip, "expected_revision": nil}
		raw["occurred_at"] = orgtime.FormatInstant(declared)
		raw["entry_zone"] = "America/Cuiaba"
		raw["device_offset_seconds"] = -14400
		raw["payload"] = payload
		if edit != nil {
			edit(raw, payload)
		}
	})
}

// startOp is member d's trip.start of a new trip on the vehicle with the
// VAL-01 departure reading 184 320 000 m.
func (x tripState) startOp(t *testing.T, d syncFixtureState, declared time.Time, edit func(envelope, payload map[string]any)) protocol.Operation {
	t.Helper()
	payload := map[string]any{"asset_id": x.vehicle.AssetID, "driver_employee_id": d.person.employee, "reading": x.reading(t, "184320000"), "missing_reading_reason": nil, "run_clock": nil}
	return x.operation(t, d, tripapp.StartOperationType, onboardingID(t), declared, payload, edit)
}

// finishOp is member d's trip.finish of trip with 184 610 000 m.
func (x tripState) finishOp(t *testing.T, d syncFixtureState, trip string, declared time.Time, edit func(envelope, payload map[string]any)) protocol.Operation {
	t.Helper()
	payload := map[string]any{"reading": x.reading(t, "184610000"), "missing_reading_reason": nil, "run_clock": map[string]any{"boot_id": "boot-a1", "elapsed_millis": "10800000"}}
	return x.operation(t, d, tripapp.FinishOperationType, trip, declared, payload, edit)
}

func tripOf(op protocol.Operation) string { return op.Snapshot().Entity.ID }

func (x tripState) process(t *testing.T, d syncFixtureState, op protocol.Operation) (protocol.Acknowledgement, error) {
	t.Helper()
	handler := x.start
	if op.Snapshot().Type == tripapp.FinishOperationType {
		handler = x.finish
	}
	return x.trips.Process(t.Context(), d.source, op, protocol.DefaultLimits(), handler)
}

type tripCounts struct{ trips, finishes, heldStarts, heldFinishes, items, readings, effects, claims, receipts, audits int }

func (x tripState) count(t *testing.T, op protocol.Operation) tripCounts {
	t.Helper()
	var c tripCounts
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_trips WHERE org_id=$1 AND operation_id=$2),(SELECT count(*) FROM org_trip_finishes WHERE org_id=$1 AND operation_id=$2),
(SELECT count(*) FROM org_trip_held_starts WHERE org_id=$1 AND operation_id=$2),(SELECT count(*) FROM org_trip_held_finishes WHERE org_id=$1 AND operation_id=$2),
(SELECT count(*) FROM org_sync_review_items WHERE org_id=$1 AND operation_id=$2),(SELECT count(*) FROM org_meter_readings WHERE org_id=$1 AND operation_id=$2),
(SELECT count(*) FROM org_sync_effects WHERE org_id=$1 AND operation_id=$2),(SELECT count(*) FROM org_sync_operations WHERE org_id=$1 AND operation_id=$2),
(SELECT count(*) FROM org_sync_acknowledgements WHERE org_id=$1 AND operation_id=$2),(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND operation_id=$2)`,
		x.scope.OrgID().String(), op.ID()).Scan(&c.trips, &c.finishes, &c.heldStarts, &c.heldFinishes, &c.items, &c.readings, &c.effects, &c.claims, &c.receipts, &c.audits); err != nil {
		t.Fatal(err)
	}
	return c
}

func (x tripState) accepted(t *testing.T, op protocol.Operation, ack protocol.Acknowledgement, err error, revision int64, readings int) {
	t.Helper()
	if err != nil || ack.Result() != protocol.Accepted || ack.Snapshot().EntityRevision == nil || *ack.Snapshot().EntityRevision != revision || len(ack.Warnings()) != 0 {
		t.Fatal("legitimate trip event was not accepted", err, ack.Result(), ack.Code())
	}
	want := tripCounts{readings: readings, effects: 1, claims: 1, receipts: 1, audits: 1}
	if revision == 1 {
		want.trips = 1
	} else {
		want.finishes = 1
	}
	if c := x.count(t, op); c != want {
		t.Fatalf("accepted trip event not stored exactly once: %+v", c)
	}
}

func (x tripState) held(t *testing.T, op protocol.Operation, ack protocol.Acknowledgement, err error, code string) {
	t.Helper()
	if err != nil || ack.Result() != protocol.AcceptedForReview || ack.Code() != code || ack.Snapshot().ReviewItemID == "" || ack.Snapshot().EntityRevision != nil {
		t.Fatal("expected a held review receipt", code, err, ack.Result(), ack.Code())
	}
	want := tripCounts{items: 1, claims: 1, receipts: 1, audits: 1}
	if op.Snapshot().Type == tripapp.StartOperationType {
		want.heldStarts = 1
	} else {
		want.heldFinishes = 1
	}
	if c := x.count(t, op); c != want {
		t.Fatalf("held trip fact not kept exactly once: %+v", c)
	}
}

func (x tripState) nothing(t *testing.T, op protocol.Operation) {
	t.Helper()
	if c := x.count(t, op); c != (tripCounts{}) {
		t.Fatalf("refused trip event left state: %+v", c)
	}
}

func (x tripState) malformed(t *testing.T, op protocol.Operation, ack protocol.Acknowledgement, err error) {
	t.Helper()
	if err != nil || ack.Result() != protocol.Rejected || ack.Code() != "MALFORMED" {
		t.Fatal("expected a final MALFORMED receipt", err, ack.Result(), ack.Code())
	}
	if c := x.count(t, op); c != (tripCounts{claims: 1, receipts: 1, audits: 1}) {
		t.Fatalf("malformed trip event stored a fact: %+v", c)
	}
}

func (x tripState) refused(t *testing.T, op protocol.Operation, ack protocol.Acknowledgement, err error, want orgapi.Code) {
	t.Helper()
	if code, _ := orgapi.Registered(err); !errors.Is(err, syncapp.ErrSourceDenied) || code != want || len(ack.CanonicalBytes()) != 0 {
		t.Fatalf("expected %s refusal, got %v", want, err)
	}
	x.nothing(t, op)
}

func (x tripState) pending(t *testing.T, op protocol.Operation, ack protocol.Acknowledgement, err error) {
	t.Helper()
	if err != nil || ack.Result() != protocol.Retry || ack.Code() != "DEPENDENCY_PENDING" {
		t.Fatal("expected RETRY/DEPENDENCY_PENDING", err, ack.Result(), ack.Code())
	}
	x.nothing(t, op)
}

func (x tripState) replays(t *testing.T, d syncFixtureState, op protocol.Operation, ack protocol.Acknowledgement) {
	t.Helper()
	for range 2 {
		replay, err := x.process(t, d, op)
		if err != nil || !bytes.Equal(replay.CanonicalBytes(), ack.CanonicalBytes()) {
			t.Fatal("resend did not return the stored ACK", err)
		}
	}
}

func (x tripState) openWork(t *testing.T) []fa.OpenWorkKind {
	t.Helper()
	var kinds []fa.OpenWorkKind
	if err := database.WithOrgTx(t.Context(), x.f.Runtime, x.scope, func(tx pgx.Tx) error {
		var err error
		kinds, err = trippg.OpenWork{}.OpenAssetWorkTx(t.Context(), tx, x.scope, x.vehicle.AssetID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return kinds
}

func (x tripState) activeAt(t *testing.T, at time.Time) []string {
	t.Helper()
	var ids []string
	if err := database.WithOrgTx(t.Context(), x.f.Runtime, x.scope, func(tx pgx.Tx) error {
		var err error
		ids, err = trippg.ActiveTripsOfAssetAtDBTX(t.Context(), syncapp.Borrow(tx), x.scope, x.vehicle.AssetID, at)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return ids
}

// BUC-EMP-157 / VAL-01: start and finish through the signed intake, each
// with its reading in one transaction, exact replay and digest reuse.
func TestTripIntakeStartsAndFinishesTheVAL01TripAndReplays(t *testing.T) {
	x := tripFixture(t)
	d := x.syncFixtureState
	started := x.declaredNow(t)
	op := x.startOp(t, d, started, nil)
	ack, err := x.process(t, d, op)
	x.accepted(t, op, ack, err, 1, 1)
	org, trip := x.scope.OrgID().String(), tripOf(op)
	var exact bool
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT t.state='IN_PROGRESS' AND t.asset_id=$3 AND t.driver_employee_id=$4 AND t.author_employee_id=$4 AND t.author_membership_id=$5
 AND t.enrollment_id=$6 AND t.payload_digest=$7 AND t.unit_id=$8 AND t.cost_centre_id IS NULL AND t.occurred_submicro=123 AND t.entry_zone='America/Cuiaba'
 AND t.device_offset_seconds=-14400 AND t.boot_id IS NULL AND t.missing_reading_reason IS NULL AND t.reporting_source='unit'
 AND t.reporting_zone=(SELECT time_zone FROM org_units WHERE org_id=t.org_id AND id=t.unit_id) AND t.reporting_day=(t.occurred_at AT TIME ZONE t.reporting_zone)::date
 AND EXISTS(SELECT 1 FROM org_meter_readings r WHERE r.org_id=t.org_id AND r.id=t.reading_id AND r.origin='TRIP_START' AND r.operation_id=t.operation_id AND r.value=184320000 AND r.state='VALID' AND r.method='TYPED')
 AND t.received_at<=(SELECT received_at FROM org_sync_acknowledgements a WHERE a.org_id=t.org_id AND a.operation_id=t.operation_id)
 FROM org_trips t WHERE t.org_id=$1 AND t.id=$2`, org, trip, x.vehicle.AssetID, d.person.employee, d.source.MembershipID, d.source.EnrollmentID, op.PayloadDigest(), x.unit).Scan(&exact); err != nil || !exact {
		t.Fatal("stored trip is not the exact start fact", err)
	}
	x.replays(t, d, op, ack)
	x.accepted(t, op, ack, nil, 1, 1)
	if kinds := x.openWork(t); len(kinds) != 1 || kinds[0] != fa.OpenTrip {
		t.Fatal("an in-progress trip is not open work", kinds)
	}
	if ids := x.activeAt(t, started); len(ids) != 1 || ids[0] != trip {
		t.Fatal("the active trip at its start instant", ids)
	}
	if ids := x.activeAt(t, started.Add(-time.Nanosecond)); len(ids) != 0 {
		t.Fatal("a trip is active before it started", ids)
	}
	reused := x.startOp(t, d, started, func(raw, _ map[string]any) {
		raw["operation_id"], raw["entity"] = op.ID(), map[string]any{"type": tripapp.EntityType, "id": trip, "expected_revision": nil}
	})
	refusal, err := x.process(t, d, reused)
	if err != nil || refusal.Result() != protocol.Rejected || refusal.Code() != "OPERATION_ID_REUSED" {
		t.Fatal("mismatched digest under the same operation was not refused", err, refusal.Code())
	}
	x.accepted(t, op, ack, nil, 1, 1)
	// Finish: 290 000 m of meter evidence, COMPLETED, revision 2.
	finished := started.Add(3 * time.Hour)
	end := x.finishOp(t, d, trip, finished, nil)
	endAck, err := x.process(t, d, end)
	x.accepted(t, end, endAck, err, 2, 1)
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT t.state='COMPLETED' AND f.author_membership_id=$3 AND f.boot_id='boot-a1' AND f.elapsed_millis=10800000
 AND f.end_day=(f.occurred_at AT TIME ZONE t.reporting_zone)::date AND r2.value-r1.value=290000 AND r2.origin='TRIP_END' AND r2.operation_id=f.operation_id
 FROM org_trips t JOIN org_trip_finishes f ON(f.org_id,f.trip_id)=(t.org_id,t.id) JOIN org_meter_readings r1 ON(r1.org_id,r1.id)=(t.org_id,t.reading_id)
 JOIN org_meter_readings r2 ON(r2.org_id,r2.id)=(f.org_id,f.reading_id) WHERE t.org_id=$1 AND t.id=$2`, org, trip, d.source.MembershipID).Scan(&exact); err != nil || !exact {
		t.Fatal("finish did not complete the trip with its reading evidence", err)
	}
	x.replays(t, d, end, endAck)
	x.replays(t, d, op, ack)
	if kinds := x.openWork(t); len(kinds) != 0 {
		t.Fatal("a completed trip is still open work", kinds)
	}
	if ids := x.activeAt(t, finished); len(ids) != 0 {
		t.Fatal("a trip is active at its finish instant", ids)
	}
	// Records are read back through the store as exact values.
	var record tripapp.Record
	if err := database.WithOrgTx(t.Context(), x.f.Runtime, x.scope, func(tx pgx.Tx) error {
		var err error
		record, err = x.store.ReadTripTx(t.Context(), syncapp.Borrow(tx), x.scope, trip)
		return err
	}); err != nil || record.Finish == nil || !record.Declared.OccurredAt.Equal(started) || !record.Finish.Declared.OccurredAt.Equal(finished) || record.Finish.Clock == nil || record.ReadingValue == nil || *record.ReadingValue != 184320000 {
		t.Fatal("stored trip record", err)
	}
}

// barrierHandler holds the winner's write admission inside its intake
// transaction (after the source and claim locks) until the loser's intake is
// observed waiting on a lock; then both proceed (B-BR-EMP-317).
type barrierHandler struct {
	syncapp.Handler
	winner          string
	inside, proceed chan struct{}
	once            *sync.Once
}

func (h barrierHandler) ReviewProof() syncapp.ReviewProof {
	return h.Handler.(syncapp.ReviewProofProvider).ReviewProof()
}

func (h barrierHandler) Admit(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, mode syncapp.Mode) error {
	if source.MembershipID == h.winner && mode == syncapp.Write {
		h.once.Do(func() {
			close(h.inside)
			<-h.proceed
		})
	}
	return h.Handler.Admit(ctx, db, source, op, mode)
}

// BUC-EMP-158 / VAL-07: two devices start the same vehicle; whichever takes
// the asset lock first is the active trip, the other is held ASSET_BUSY, in
// both orders, and the held receipt replays byte for byte.
func TestTripIntakeConcurrentStartsKeepOneActiveTripAndHoldTheOther(t *testing.T) {
	for _, order := range []string{"first-wins", "second-wins"} {
		t.Run(order, func(t *testing.T) {
			x := tripFixture(t)
			d1, d2 := x.driver(t), x.driver(t)
			var declared time.Time
			for _, d := range []syncFixtureState{d1, d2} {
				_, row := x.assign(t, d.person.employee, x.vehicle.AssetID, fleet.AdditionalAssignment, nil, nil)
				if row.from.After(declared) {
					declared = row.from
				}
			}
			op1 := x.startOp(t, d1, declared, nil)
			op2 := x.startOp(t, d2, declared.Add(time.Microsecond), nil)
			config := tripIntakeConfig(x.syncFixtureState)
			config.Pool = x.f.RuntimePool(t, 4)
			store := syncpg.NewStore(config)
			winner, loser := d1, d2
			winnerOp, loserOp := op1, op2
			if order == "second-wins" {
				winner, loser, winnerOp, loserOp = d2, d1, op2, op1
			}
			start := barrierHandler{Handler: x.start, winner: winner.source.MembershipID, inside: make(chan struct{}), proceed: make(chan struct{}), once: &sync.Once{}}
			acks := map[string]protocol.Acknowledgement{}
			errs := map[string]error{}
			var mu sync.Mutex
			var done sync.WaitGroup
			run := func(d syncFixtureState, op protocol.Operation) {
				done.Go(func() {
					ack, err := store.Process(context.Background(), d.source, op, protocol.DefaultLimits(), start)
					mu.Lock()
					acks[d.source.MembershipID], errs[d.source.MembershipID] = ack, err
					mu.Unlock()
				})
			}
			run(winner, winnerOp)
			select {
			case <-start.inside:
			case <-time.After(30 * time.Second):
				close(start.proceed)
				done.Wait()
				t.Fatal("the winning start never reached its admission", errs)
			}
			run(loser, loserOp)
			// Proof of contention: the loser's intake waits on a lock held by
			// the winner's open transaction before the winner continues.
			waited := false
			for deadline := time.Now().Add(10 * time.Second); !waited && time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
				var waiting int
				if err := x.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock'`).Scan(&waiting); err != nil {
					t.Fatal(err)
				}
				waited = waiting > 0
			}
			close(start.proceed)
			done.Wait()
			if !waited {
				t.Fatal("the losing start never waited on the winner's lock")
			}
			opOf := map[string]protocol.Operation{winner.source.MembershipID: winnerOp, loser.source.MembershipID: loserOp}
			w, l := winner.source.MembershipID, loser.source.MembershipID
			x.accepted(t, opOf[w], acks[w], errs[w], 1, 1)
			x.held(t, opOf[l], acks[l], errs[l], tripapp.AssetBusy)
			var active int
			if err := x.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_trips WHERE org_id=$1 AND asset_id=$2 AND state='IN_PROGRESS'`, x.scope.OrgID().String(), x.vehicle.AssetID).Scan(&active); err != nil || active != 1 {
				t.Fatal("two trips of one asset count as in progress", active, err)
			}
			if ids := x.activeAt(t, declared.Add(time.Hour)); len(ids) != 1 || ids[0] != tripOf(opOf[w]) {
				t.Fatal("the held start became active use", ids)
			}
			// The held start, its reading values and its review item are kept;
			// a replay returns the held ACK byte for byte.
			var kept bool
			if err := x.f.Admin.QueryRow(t.Context(), `SELECT h.reading_value=184320000 AND h.reading_id IS NOT NULL AND i.code='ASSET_BUSY' AND i.operation_type='trip.start'
 AND NOT EXISTS(SELECT 1 FROM org_meter_readings r WHERE r.org_id=h.org_id AND r.id=h.reading_id) AND a.review_item_id=i.id AND a.review_proof_version=1
 FROM org_trip_held_starts h JOIN org_sync_review_items i ON(i.org_id,i.id)=(h.org_id,h.review_item_id) JOIN org_sync_acknowledgements a ON(a.org_id,a.operation_id)=(i.org_id,i.operation_id)
 WHERE h.org_id=$1 AND h.operation_id=$2`, x.scope.OrgID().String(), opOf[l].ID()).Scan(&kept); err != nil || !kept {
				t.Fatal("held start not kept with its review item", err)
			}
			x.replays(t, loser, opOf[l], acks[l])
			x.replays(t, winner, opOf[w], acks[w])
		})
	}
}

// Concurrent identical starts leave one trip (12-way ledger contention).
func TestTripIntakeConcurrentDuplicatesLeaveOneTrip(t *testing.T) {
	x := tripFixture(t)
	d := x.syncFixtureState
	config := tripIntakeConfig(x.syncFixtureState)
	config.Pool = x.f.RuntimePool(t, 12)
	store := syncpg.NewStore(config)
	op := x.startOp(t, d, x.declaredNow(t), nil)
	var wg sync.WaitGroup
	acks := make([]protocol.Acknowledgement, 12)
	errs := make([]error, 12)
	for i := range 12 {
		wg.Go(func() {
			acks[i], errs[i] = store.Process(context.Background(), d.source, op, protocol.DefaultLimits(), x.start)
		})
	}
	wg.Wait()
	var original []byte
	for i := range 12 {
		if errs[i] != nil {
			if !errors.Is(errs[i], syncapp.ErrUnavailable) || errors.Is(errs[i], syncapp.ErrSourceDenied) {
				t.Fatal("concurrent duplicate failed outside RETRY", errs[i])
			}
			continue
		}
		if acks[i].Result() != protocol.Accepted || original != nil && !bytes.Equal(original, acks[i].CanonicalBytes()) {
			t.Fatal("concurrent duplicates produced different results")
		}
		original = acks[i].CanonicalBytes()
	}
	if original == nil {
		t.Fatal("no concurrent duplicate was accepted")
	}
	x.accepted(t, op, acks[0], nil, 1, 1)
}

// BUC-EMP-159: a second completion of a completed trip is kept for review.
func TestTripIntakeKeepsASecondFinishAsTripAlreadyClosed(t *testing.T) {
	x := tripFixture(t)
	d := x.syncFixtureState
	started := x.declaredNow(t)
	op := x.startOp(t, d, started, nil)
	ack, err := x.process(t, d, op)
	x.accepted(t, op, ack, err, 1, 1)
	first := x.finishOp(t, d, tripOf(op), started.Add(time.Hour), nil)
	firstAck, err := x.process(t, d, first)
	x.accepted(t, first, firstAck, err, 2, 1)
	second := x.finishOp(t, d, tripOf(op), started.Add(2*time.Hour), func(_, p map[string]any) { p["reading"], p["missing_reading_reason"] = nil, "Segundo aparelho" })
	secondAck, err := x.process(t, d, second)
	x.held(t, second, secondAck, err, tripapp.TripAlreadyClosed)
	x.replays(t, d, second, secondAck)
	x.replays(t, d, first, firstAck)
	var finishes int
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_trip_finishes WHERE org_id=$1 AND trip_id=$2`, x.scope.OrgID().String(), tripOf(op)).Scan(&finishes); err != nil || finishes != 1 {
		t.Fatal("a completed trip received a second completion", finishes, err)
	}
}

// B-BR-EMP-312..316 negatives: nothing is stored and nothing is dropped.
func TestTripIntakeRefusalsAndHoldsAtReceipt(t *testing.T) {
	x := tripFixture(t)
	d := x.syncFixtureState
	started := x.declaredNow(t)
	op := x.startOp(t, d, started, nil)
	ack, err := x.process(t, d, op)
	x.accepted(t, op, ack, err, 1, 1)
	trip := tripOf(op)
	t.Run("finish-before-start-malformed", func(t *testing.T) {
		before := x.finishOp(t, d, trip, started.Add(-time.Nanosecond), nil)
		ack, err := x.process(t, d, before)
		x.malformed(t, before, ack, err)
	})
	t.Run("finish-of-unknown-trip-pending", func(t *testing.T) {
		unknown := x.finishOp(t, d, onboardingID(t), started.Add(time.Hour), nil)
		ack, err := x.process(t, d, unknown)
		x.pending(t, unknown, ack, err)
	})
	t.Run("trip-identity-reused-malformed", func(t *testing.T) {
		again := x.startOp(t, d, started.Add(time.Minute), func(raw, _ map[string]any) {
			raw["entity"] = map[string]any{"type": tripapp.EntityType, "id": trip, "expected_revision": nil}
		})
		ack, err := x.process(t, d, again)
		x.malformed(t, again, ack, err)
	})
	t.Run("reading-and-reason-malformed", func(t *testing.T) {
		both := x.startOp(t, d, started.Add(time.Minute), func(_, p map[string]any) { p["missing_reading_reason"] = "Painel apagado" })
		ack, err := x.process(t, d, both)
		x.malformed(t, both, ack, err)
	})
	t.Run("unknown-meter-pending", func(t *testing.T) {
		unknown := x.startOp(t, d, started.Add(time.Minute), func(_, p map[string]any) { p["reading"].(map[string]any)["meter_id"] = onboardingID(t) })
		ack, err := x.process(t, d, unknown)
		x.pending(t, unknown, ack, err)
	})
	t.Run("foreign-asset-not-found", func(t *testing.T) {
		foreign := foreignAsset(t, fuelingState{selfIntakeState: x.selfIntakeState})
		op := x.startOp(t, d, started, func(_, p map[string]any) {
			p["asset_id"], p["reading"], p["missing_reading_reason"] = foreign, nil, "Ativo de outra organização"
		})
		ack, err := x.process(t, d, op)
		x.refused(t, op, ack, err, orgapi.NotFound)
	})
	t.Run("busy-asset-held", func(t *testing.T) {
		busy := x.startOp(t, d, started.Add(time.Minute), nil)
		ack, err := x.process(t, d, busy)
		x.held(t, busy, ack, err, tripapp.AssetBusy)
		finish := x.finishOp(t, d, tripOf(busy), started.Add(time.Hour), nil)
		pending, err := x.process(t, d, finish)
		x.pending(t, finish, pending, err)
	})
	t.Run("self-driver-never-assigned-forbidden", func(t *testing.T) {
		driver := x.driver(t)
		op := x.startOp(t, driver, started, nil)
		ack, err := x.process(t, driver, op)
		x.refused(t, op, ack, err, orgapi.Forbidden)
	})
	t.Run("another-member-finish-forbidden", func(t *testing.T) {
		driver := x.driver(t)
		x.assign(t, driver.person.employee, x.vehicle.AssetID, fleet.AdditionalAssignment, nil, nil)
		other := x.finishOp(t, driver, trip, started.Add(time.Hour), nil)
		ack, err := x.process(t, driver, other)
		x.refused(t, other, ack, err, orgapi.Forbidden)
	})
}

// A start declared before the member's assignment is a changed right: the
// start is kept for review with PERMISSION_CHANGED (EDR-118 continuity).
func TestTripIntakeHoldsAStartWhoseRightChanged(t *testing.T) {
	x := tripFixture(t)
	d := x.driver(t)
	_, row := x.assign(t, d.person.employee, x.vehicle.AssetID, fleet.AdditionalAssignment, nil, nil)
	before := x.startOp(t, d, row.from.Add(-time.Nanosecond), nil)
	ack, err := x.process(t, d, before)
	x.held(t, before, ack, err, tripapp.PermissionChanged)
	op := x.startOp(t, d, row.from, nil)
	ack, err = x.process(t, d, op)
	x.accepted(t, op, ack, err, 1, 1)
}

// An asset taken out of service at receipt keeps the start for review; while
// a trip is in progress FLEET-003 refuses deactivation with TRIP open work.
func TestTripOpenWorkAndUnavailableAsset(t *testing.T) {
	x := tripFixture(t)
	d := x.syncFixtureState
	started := x.declaredNow(t)
	op := x.startOp(t, d, started, nil)
	ack, err := x.process(t, d, op)
	x.accepted(t, op, ack, err, 1, 1)
	deactivate := lifecycleInputFor(t, x.legalFixtureState, fleet.Deactivate, x.vehicle.AssetID, x.vehicleAt)
	var open *fa.OpenOperationsError
	if _, _, err := lifecycleWrite(t, x.legalFixtureState, x.assets, lifecycleCommand(t, deactivate)); !errors.As(err, &open) || len(open.Kinds()) != 1 || open.Kinds()[0] != fa.OpenTrip {
		t.Fatal("deactivation of an asset with a trip in progress was not refused", err)
	}
	end := x.finishOp(t, d, tripOf(op), started.Add(time.Hour), nil)
	endAck, err := x.process(t, d, end)
	x.accepted(t, end, endAck, err, 2, 1)
	deactivate = lifecycleInputFor(t, x.legalFixtureState, fleet.Deactivate, x.vehicle.AssetID, x.vehicleAt)
	if _, _, err := lifecycleWrite(t, x.legalFixtureState, x.assets, lifecycleCommand(t, deactivate)); err != nil {
		t.Fatal("deactivation after completion", err)
	}
	late := x.startOp(t, d, started.Add(2*time.Hour), nil)
	lateAck, err := x.process(t, d, late)
	x.held(t, late, lateAck, err, tripapp.AssetUnavailable)
}

type tripFailingStore struct{ *trippg.Store }

func (s tripFailingStore) InsertTripTx(context.Context, syncapp.DBTX, kernel.OrgScope, syncapp.Source, tripapp.StartCommand, tripapp.StartFacts, tripdomain.Trip) error {
	return errors.New("injected trip failure after its reading")
}

// The reading and the trip commit together or not at all.
func TestTripIntakeRollsBackTheReadingAndTheTripTogether(t *testing.T) {
	x := tripFixture(t)
	d := x.syncFixtureState
	op := x.startOp(t, d, x.declaredNow(t), nil)
	failing, _ := x.handlers(t, x.meters, tripFailingStore{x.store})
	ack, err := x.trips.Process(t.Context(), d.source, op, protocol.DefaultLimits(), failing)
	if !errors.Is(err, syncapp.ErrUnavailable) || len(ack.CanonicalBytes()) != 0 {
		t.Fatal("injected failure was not a retry", err)
	}
	x.nothing(t, op)
	ack, err = x.process(t, d, op)
	x.accepted(t, op, ack, err, 1, 1)
}

// Forced RLS, the guarded completion update and insert-only history.
func TestTripSchemaIsForcedRLSGuardedAndInsertOnly(t *testing.T) {
	x := tripFixture(t)
	d := x.syncFixtureState
	op := x.startOp(t, d, x.declaredNow(t), nil)
	ack, err := x.process(t, d, op)
	x.accepted(t, op, ack, err, 1, 1)
	role := pgx.Identifier{x.f.RuntimeRole}.Sanitize()
	if _, err := x.f.Admin.Exec(t.Context(), "GRANT UPDATE,DELETE,TRUNCATE ON org_trips,org_trip_finishes,org_trip_held_starts,org_trip_held_finishes TO "+role); err != nil {
		t.Fatal(err)
	}
	var visible int
	if err := x.f.Runtime.QueryRow(t.Context(), `SELECT count(*) FROM org_trips`).Scan(&visible); err != nil || visible != 0 {
		t.Fatal("trips visible without an organization setting", visible, err)
	}
	for _, statement := range []string{
		`UPDATE org_trips SET entry_zone='UTC' WHERE id=$1`,
		`UPDATE org_trips SET state='COMPLETED' WHERE id=$1`,
		`DELETE FROM org_trips WHERE id=$1`,
		`UPDATE org_trip_held_starts SET entry_zone='UTC' WHERE trip_id=$1`,
		`DELETE FROM org_trip_finishes WHERE trip_id=$1`,
	} {
		err := database.WithOrgTx(t.Context(), x.f.Runtime, x.scope, func(tx pgx.Tx) error {
			_, err := tx.Exec(t.Context(), statement, tripOf(op))
			return err
		})
		if err == nil {
			t.Fatal("trip history was rewritten", statement)
		}
	}
	var unique bool
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT indexdef LIKE '%UNIQUE%' AND indexdef LIKE '%(org_id, asset_id)%' AND indexdef LIKE '%IN_PROGRESS%' FROM pg_indexes WHERE indexname='org_trips_one_active_asset'`).Scan(&unique); err != nil || !unique {
		t.Fatal("one active trip per asset is not a database invariant", err)
	}
	var forced bool
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT bool_and(relrowsecurity AND relforcerowsecurity) FROM pg_class WHERE relname IN('org_trips','org_trip_finishes','org_trip_held_starts','org_trip_held_finishes')`).Scan(&forced); err != nil || !forced {
		t.Fatal("trip tables are not forced RLS", err)
	}
}

// FLEET-007 AC4 through the trip intake: a start inside a confirmed
// reservation of another employee is held ASSET_UNAVAILABLE; the reserved
// driver's start is accepted; availability reads IN_USE from the trip source.
func TestTripIntakeHoldsAStartInsideAnotherEmployeesReservation(t *testing.T) {
	x := tripFixture(t)
	d := x.driver(t)
	_, row := x.assign(t, d.person.employee, x.vehicle.AssetID, fleet.AdditionalAssignment, nil, nil)
	from := row.from.Add(time.Second)
	if _, _, err := reserve(t, x.legalFixtureState, x.assets, fa.ReservationRequestInput{AssetID: x.vehicle.AssetID, Start: from, End: from.Add(2 * time.Hour), EmployeeID: x.person.employee, Confirm: true}); err != nil {
		t.Fatal("reservation fixture", err)
	}
	waitDBClock(t, x.legalFixtureState, from.Add(time.Millisecond))
	outsider := x.startOp(t, d, from.Add(time.Millisecond), nil)
	ack, err := x.process(t, d, outsider)
	x.held(t, outsider, ack, err, tripapp.AssetUnavailable)
	reserved := x.startOp(t, x.syncFixtureState, from.Add(2*time.Millisecond), nil)
	ack, err = x.process(t, x.syncFixtureState, reserved)
	x.accepted(t, reserved, ack, err, 1, 1)
	view, err := x.assets.ReadAvailability(t.Context(), x.scope, fleetActor(x.legalFixtureState), x.member, x.vehicle.AssetID)
	if err != nil || view.Assessment.Availability != fleet.InUse || len(view.MissingSources) != 1 || view.MissingSources[0] != fa.MaintenanceHoldSource {
		t.Fatal("the trip source does not answer active use", view, err)
	}
}

// FLEET-007 AC5 through the trip intake: a block added during a trip does
// not interrupt it (the finish is accepted) and holds the next start.
func TestTripIntakeBlockDuringATripAppliesToTheNextStart(t *testing.T) {
	x := tripFixture(t)
	d := x.syncFixtureState
	started := x.declaredNow(t)
	op := x.startOp(t, d, started, nil)
	ack, err := x.process(t, d, op)
	x.accepted(t, op, ack, err, 1, 1)
	if _, err := blockAsset(t, x.legalFixtureState, x.assets, fa.BlockCreateInput{AssetID: x.vehicle.AssetID, Reason: fleet.SafetyBlock}); err != nil {
		t.Fatal("block during the trip", err)
	}
	after := exactNow(t, x.legalFixtureState).Add(time.Second)
	waitDBClock(t, x.legalFixtureState, after)
	end := x.finishOp(t, d, tripOf(op), after, nil)
	endAck, err := x.process(t, d, end)
	x.accepted(t, end, endAck, err, 2, 1)
	next := x.startOp(t, d, after.Add(time.Millisecond), nil)
	nextAck, err := x.process(t, d, next)
	x.held(t, next, nextAck, err, tripapp.AssetUnavailable)
}
