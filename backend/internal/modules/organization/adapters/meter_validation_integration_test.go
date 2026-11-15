//go:build integration

package adapters_test

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	fleetpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

// validationFixture installs an odometer whose opening reading has the given
// value at base, on the real signed-intake fixture, with validation on.
func validationFixture(t *testing.T, opening int64, base time.Time) (syncFixtureState, *fleetpg.MeterStore, app.MeterInstallResult) {
	t.Helper()
	fx := syncFixture(t)
	assets := fleetStore(t, fx.legalFixtureState)
	asset, _, err := fleetWrite(t, fx.legalFixtureState, assets, fleetCommand(t, fleetCreateInput()))
	if err != nil {
		t.Fatal(err)
	}
	store := meterStore(t, fx.legalFixtureState)
	store.Authority = &orgpg.MeterAuthority{Memberships: fx.legalFixtureState.store.Memberships, Roles: fx.legalFixtureState.store.Roles, Capabilities: fx.flags, Devices: fx.devices}
	input := meterInstallInput(t, asset.AssetID, base)
	if input.Value, err = meter.NewValue(meter.Odometer, opening, nil); err != nil {
		t.Fatal(err)
	}
	installed, _, err := meterInstall(t, fx.legalFixtureState, store, meterInstallCommand(t, input))
	if err != nil {
		t.Fatal(err)
	}
	policy := meter.DefaultPlausibility()
	store.Plausibility = &policy
	return fx, store, installed
}

type readingStateRow struct {
	revision    int
	state       string
	flags       string
	retroactive bool
	cause       string
	operation   string
}

func readingStates(t *testing.T, fx syncFixtureState, reading string) []readingStateRow {
	t.Helper()
	rows, err := fx.f.Admin.Query(t.Context(), `SELECT revision,state,array_to_string(flags,'+'),retroactive,cause,operation_id::text FROM org_meter_reading_states WHERE org_id=$1 AND reading_id=$2 ORDER BY revision`, fx.scope.OrgID().String(), reading)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []readingStateRow
	for rows.Next() {
		var r readingStateRow
		if err := rows.Scan(&r.revision, &r.state, &r.flags, &r.retroactive, &r.cause, &r.operation); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	if rows.Err() != nil {
		t.Fatal(rows.Err())
	}
	return out
}

func recordedState(t *testing.T, fx syncFixtureState, reading string) string {
	t.Helper()
	var state string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT state FROM org_meter_readings WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), reading).Scan(&state); err != nil {
		t.Fatal(err)
	}
	return state
}

func latestReadingID(t *testing.T, fx syncFixtureState, store *fleetpg.MeterStore, installed app.MeterInstallResult) string {
	t.Helper()
	view, err := store.ReadLatestMeter(t.Context(), fx.scope, fleetActor(fx.legalFixtureState), fx.member, installed.AssetID, installed.MeterID)
	if err != nil || view.Latest == nil || view.Latest.Snapshot().State != meter.Valid {
		t.Fatal("latest valid reading unavailable", err)
	}
	return view.Latest.Snapshot().Reference.ID()
}

// validationRecord records one standalone reading through the embedded port
// and checks its recorded classification against its revision 1.
func validationRecord(t *testing.T, fx syncFixtureState, store *fleetpg.MeterStore, installed app.MeterInstallResult, at time.Time, value int64, want readingStateRow) (string, app.MeterReadingCommand) {
	t.Helper()
	command := meterObservedCommand(t, installed, at, value)
	result, replay, err := meterRecord(t, fx, store, fx.source, command)
	if err != nil || replay {
		t.Fatal("validated reading refused instead of stored", err)
	}
	in, _ := command.Snapshot()
	if result.Reading.Snapshot().State != meter.State(want.state) || recordedState(t, fx, in.ReadingID) != want.state {
		t.Fatalf("recorded classification of %d: got %s", value, recordedState(t, fx, in.ReadingID))
	}
	want.revision, want.cause, want.operation = 1, "INTAKE", in.OperationID
	if rows := readingStates(t, fx, in.ReadingID); len(rows) < 1 || rows[0] != want {
		t.Fatalf("revision 1 of %d: got %+v want %+v", value, rows, want)
	}
	return in.ReadingID, command
}

// AC1..AC3 and the example-A variants of fleet §10.4 on real PostGIS: the
// retroactive reading is VALID with RETROACTIVE and keeps the interval sum,
// the typing error is SUSPECT and the current value does not move, and the
// too-fast reading is SUSPECT with IMPLAUSIBLE_RATE.
func TestMeterValidationExamplesThroughTheEmbeddedPort(t *testing.T) {
	base := time.Now().UTC().Truncate(time.Second).Add(-7 * 24 * time.Hour)
	fx, store, installed := validationFixture(t, 183_000_000, base)
	r1, _ := validationRecord(t, fx, store, installed, base.Add(47*time.Hour), 183_400_000, readingStateRow{state: "VALID"})
	r2, _ := validationRecord(t, fx, store, installed, base.Add(95*time.Hour), 183_932_000, readingStateRow{state: "VALID"})
	// Example A with R3 above its successor first, then the real R3 at the
	// same instant (received later, so ordered after the suspect variant).
	validationRecord(t, fx, store, installed, base.Add(75*time.Hour+30*time.Minute), 183_990_000, readingStateRow{state: "SUSPECT", flags: "READING_ABOVE_NEXT", retroactive: true})
	r3, _ := validationRecord(t, fx, store, installed, base.Add(75*time.Hour+30*time.Minute), 183_610_000, readingStateRow{state: "VALID", retroactive: true})
	if latestReadingID(t, fx, store, installed) != r2 {
		t.Fatal("retroactive reading moved the current value")
	}
	var r1v, r2v, r3v int64
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT(SELECT value FROM org_meter_readings WHERE id=$1),(SELECT value FROM org_meter_readings WHERE id=$2),(SELECT value FROM org_meter_readings WHERE id=$3)`, r1, r2, r3).Scan(&r1v, &r2v, &r3v); err != nil {
		t.Fatal(err)
	}
	if r3v-r1v != 210_000 || r2v-r3v != 322_000 || (r3v-r1v)+(r2v-r3v) != r2v-r1v || r2v-r1v != 532_000 {
		t.Fatal("example A intervals do not keep the total")
	}
	r4, c4 := validationRecord(t, fx, store, installed, base.Add(120*time.Hour), 148_190_000, readingStateRow{state: "SUSPECT", flags: "READING_BELOW_PREVIOUS"})
	if latestReadingID(t, fx, store, installed) != r2 {
		t.Fatal("suspect reading moved the current value")
	}
	// A resend replays the stored reading without a second revision.
	if result, replay, err := meterRecord(t, fx, store, fx.source, c4); err != nil || !replay || result.Reading.Snapshot().State != meter.Suspect || len(readingStates(t, fx, r4)) != 1 {
		t.Fatal("suspect replay changed the stored classification", err)
	}
	r5, c5 := validationRecord(t, fx, store, installed, base.Add(120*time.Hour), 184_190_000, readingStateRow{state: "VALID"})
	if latestReadingID(t, fx, store, installed) != r5 {
		t.Fatal("a valid reading at the same instant did not become current")
	}
	// R4 now also has a successor at its own instant 36 000 km higher: the
	// re-evaluation records that second conflict, and R4 stays SUSPECT.
	in5, _ := c5.Snapshot()
	if rows := readingStates(t, fx, r4); len(rows) != 2 || rows[1] != (readingStateRow{revision: 2, state: "SUSPECT", flags: "READING_BELOW_PREVIOUS+IMPLAUSIBLE_RATE", retroactive: true, cause: "REEVALUATION", operation: in5.OperationID}) {
		t.Fatalf("suspect neighbour not re-evaluated against its new successor: %+v", rows)
	}
	validationRecord(t, fx, store, installed, base.Add(144*time.Hour), 194_400_000, readingStateRow{state: "SUSPECT", flags: "IMPLAUSIBLE_RATE"})
	if latestReadingID(t, fx, store, installed) != r5 {
		t.Fatal("implausible reading moved the current value")
	}
	var history, intake int
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_meter_reading_states WHERE org_id=$1),(SELECT count(*) FROM org_meter_reading_states WHERE org_id=$1 AND cause='INTAKE')`, fx.scope.OrgID().String()).Scan(&history, &intake); err != nil || history != 8 || intake != 7 {
		t.Fatal("history is not one intake revision per classified reading plus the one re-evaluation (opening anchor excluded)", history, intake, err)
	}
}

// B-BR-EMP-285: a VALID reading that makes a suspect neighbour consistent
// promotes it in the same transaction, under the same operation; a rollback
// of the parent leaves the suspect reading untouched.
func TestMeterValidationReevaluatesSuspectNeighboursInTheSameTransaction(t *testing.T) {
	base := time.Now().UTC().Truncate(time.Second).Add(-24 * time.Hour)
	fx, store, installed := validationFixture(t, 0, base)
	q, _ := validationRecord(t, fx, store, installed, base.Add(2*time.Hour), 191_500, readingStateRow{state: "VALID"})
	s, _ := validationRecord(t, fx, store, installed, base.Add(time.Hour), 40_000, readingStateRow{state: "SUSPECT", flags: "IMPLAUSIBLE_RATE", retroactive: true})
	n := meterObservedCommand(t, installed, base.Add(90*time.Minute), 116_000)
	rollback := errors.New("synthetic parent rollback")
	if err := database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
		if _, _, err := store.RecordEmbedded(t.Context(), syncapp.Borrow(tx), fx.scope, app.MeterSource{Device: &fx.source}, fx.source.MembershipID, n); err != nil {
			return err
		}
		var promoted bool
		if err := tx.QueryRow(t.Context(), `SELECT EXISTS(SELECT 1 FROM org_meter_reading_states WHERE org_id=$1 AND reading_id=$2 AND cause='REEVALUATION')`, fx.scope.OrgID().String(), s).Scan(&promoted); err != nil || !promoted {
			t.Fatal("re-evaluation did not happen inside the reading transaction", err)
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	if rows := readingStates(t, fx, s); len(rows) != 1 || rows[0].state != "SUSPECT" {
		t.Fatal("rolled back re-evaluation survived", rows)
	}
	if _, replay, err := meterRecord(t, fx, store, fx.source, n); err != nil || replay {
		t.Fatal(err)
	}
	in, _ := n.Snapshot()
	rows := readingStates(t, fx, s)
	want := readingStateRow{revision: 2, state: "VALID", retroactive: true, cause: "REEVALUATION", operation: in.OperationID}
	if len(rows) != 2 || rows[1] != want || recordedState(t, fx, s) != "SUSPECT" {
		t.Fatalf("suspect neighbour not promoted by the consistent reading: %+v", rows)
	}
	if latestReadingID(t, fx, store, installed) != q {
		t.Fatal("re-evaluation changed the current value")
	}
}

// The signed intake answers ACCEPTED for a suspect reading, never holds or
// refuses it, keeps the current value, and replays the stored ACK byte for
// byte. (A future-declared signed reading is held by the METER-002 authority
// classifier before validation; FUTURE_INSTANT is proven on the embedded port.)
func TestMeterValidationSignedIntakeAcceptsSuspectAndReplaysTheAck(t *testing.T) {
	x := selfIntakeFixture(t)
	policy := meter.DefaultPlausibility()
	x.meters.Plausibility = &policy
	// Retained ADMIN coverage starts when the fixture assigned it, so every
	// declared instant stays after that start and before the server clock.
	now := lifecycleDBNow(t, x.legalFixtureState).Truncate(time.Microsecond)
	send := func(at time.Time, value int64, state, flags string) (protocol.Operation, protocol.Acknowledgement, string) {
		t.Helper()
		command := meterObservedCommand(t, x.installed, at, value)
		op := validationOperation(t, x.syncFixtureState, command)
		ack, err := x.process(t, x.syncFixtureState, op)
		x.accepted(t, op, ack, err)
		in, _ := command.Snapshot()
		if rows := readingStates(t, x.syncFixtureState, in.ReadingID); recordedState(t, x.syncFixtureState, in.ReadingID) != state || len(rows) != 1 || rows[0].state != state || rows[0].flags != flags {
			t.Fatalf("signed reading %d classified %+v", value, rows)
		}
		return op, ack, in.ReadingID
	}
	_, _, r1 := send(now.Add(-3*time.Microsecond), 100_000, "VALID", "")
	op2, ack2, r2 := send(now.Add(-2*time.Microsecond), 50_000, "SUSPECT", "READING_BELOW_PREVIOUS")
	x.replays(t, x.syncFixtureState, op2, ack2)
	if len(readingStates(t, x.syncFixtureState, r2)) != 1 {
		t.Fatal("replay re-classified the stored reading")
	}
	send(now.Add(-time.Microsecond), 102_000, "SUSPECT", "IMPLAUSIBLE_RATE")
	if latestReadingID(t, x.syncFixtureState, x.meters, x.installed) != r1 {
		t.Fatal("a suspect signed reading moved the current value")
	}
}

// validationOperation is meterReviewOperation with the command's own value.
func validationOperation(t *testing.T, fx syncFixtureState, command app.MeterReadingCommand) protocol.Operation {
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
		raw["payload"] = map[string]any{"asset_id": in.AssetID, "meter_id": in.MeterID, "segment_id": in.SegmentID, "kind": string(in.Value.Kind()), "value": strconv.FormatInt(in.Value.Count(), 10), "method": string(in.Method)}
	})
}

// The fueling's embedded reading gets exactly the same validation: an
// implausible reading is stored SUSPECT and the fueling is accepted.
func TestMeterValidationAppliesToTheFuelingEmbeddedReading(t *testing.T) {
	x := fuelingFixture(t)
	policy := meter.DefaultPlausibility()
	x.meters.Plausibility = &policy
	d := x.syncFixtureState
	op := x.op(t, d, x.declaredNow(t), nil)
	ack, err := x.process(t, d, op)
	x.accepted(t, op, ack, err, 0, 1)
	var reading string
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT reading_id::text FROM org_fuelings WHERE org_id=$1 AND operation_id=$2`, x.scope.OrgID().String(), op.ID()).Scan(&reading); err != nil {
		t.Fatal(err)
	}
	if rows := readingStates(t, d, reading); recordedState(t, d, reading) != "SUSPECT" || len(rows) != 1 || rows[0].flags != "IMPLAUSIBLE_RATE" || rows[0].operation != op.ID() {
		t.Fatalf("fueling reading not validated: %+v", rows)
	}
	replay, err := x.process(t, d, op)
	if err != nil || !bytes.Equal(replay.CanonicalBytes(), ack.CanonicalBytes()) {
		t.Fatal("suspect fueling reading broke the original ACK replay", err)
	}
	if latestReadingID(t, d, x.meters, x.vehicle) != x.vehicle.OpeningReadingID {
		t.Fatal("suspect fueling reading moved the current value")
	}
	plausible := x.op(t, d, x.declaredNow(t), func(_, p map[string]any) {
		p["reading"] = map[string]any{"meter_id": x.vehicle.MeterID, "segment_id": x.vehicle.SegmentID, "reading_id": onboardingID(t), "kind": "ODOMETER", "value": "1000"}
	})
	ack, err = x.process(t, d, plausible)
	x.accepted(t, plausible, ack, err, 0, 1)
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT reading_id::text FROM org_fuelings WHERE org_id=$1 AND operation_id=$2`, x.scope.OrgID().String(), plausible.ID()).Scan(&reading); err != nil {
		t.Fatal(err)
	}
	if recordedState(t, d, reading) != "VALID" || latestReadingID(t, d, x.meters, x.vehicle) != reading {
		t.Fatal("plausible fueling reading did not become current")
	}
}

// An accepted held reading is classified at its original receipt when the
// reviewer applies it.
func TestMeterValidationClassifiesAnAcceptedHeldReading(t *testing.T) {
	fx, store, installed := meterEvidenceFixture(t)
	policy := meter.DefaultPlausibility()
	store.Plausibility = &policy
	now := lifecycleDBNow(t, fx.legalFixtureState)
	validationRecord(t, fx, store, installed, now.Add(-2*time.Hour), 100_000, readingStateRow{state: "VALID"})
	reviews := meterReviewStore(t, fx)
	reviews.Plausibility = &policy
	command := meterObservedCommand(t, installed, now.Add(-time.Hour), 7)
	_, ref, ack, err := storeMeterHeldReview(t, fx, reviews, command)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := app.NewMeterReviewDecision(app.MeterReviewDecisionInput{OperationID: onboardingID(t), ReviewItemID: ref.ReviewItemID, ExpectedRevision: 1, Decision: app.MeterReviewAccept})
	if err != nil {
		t.Fatal(err)
	}
	resolution, _, err := reviews.ResolveMeterReview(t.Context(), fx.scope, fleetActor(fx.legalFixtureState), fx.member, decision)
	if err != nil {
		t.Fatal("accepted held reading refused for its classification", err)
	}
	in, _ := command.Snapshot()
	rows := readingStates(t, fx, in.ReadingID)
	if recordedState(t, fx, in.ReadingID) != "SUSPECT" || len(rows) != 1 || rows[0].flags != "READING_BELOW_PREVIOUS" || rows[0].operation != resolution.Snapshot().ResolutionID {
		t.Fatalf("reviewed reading not classified: %+v", rows)
	}
	var original bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT r.received_at=a.received_at AND a.canonical_ack=$3 FROM org_meter_readings r JOIN org_sync_acknowledgements a ON(a.org_id,a.operation_id)=(r.org_id,r.operation_id) WHERE r.org_id=$1 AND r.id=$2`, fx.scope.OrgID().String(), in.ReadingID, ack.CanonicalBytes()).Scan(&original); err != nil || !original {
		t.Fatal("reviewed reading lost its original receipt", err)
	}
}

// Two readings racing on one meter serialise on the root and meter locks in
// both orders; the later one is checked against the committed first one and
// the valid series never decreases.
func TestMeterValidationConcurrentPairOnOneMeterSerialisesInBothOrders(t *testing.T) {
	for _, firstHigh := range []bool{true, false} {
		t.Run(map[bool]string{true: "high-then-low", false: "low-then-high"}[firstHigh], func(t *testing.T) {
			base := time.Now().UTC().Truncate(time.Second).Add(-5 * time.Hour)
			fx, store, installed := validationFixture(t, 0, base)
			high := meterObservedCommand(t, installed, base.Add(time.Hour), 100_000)
			low := meterObservedCommand(t, installed, base.Add(2*time.Hour), 50_000)
			first, second := high, low
			if !firstHigh {
				first, second = low, high
			}
			holder := make(chan int, 1)
			release := make(chan struct{})
			done := make(chan error, 2)
			go func() {
				done <- database.WithOrgTx(context.Background(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
					if _, _, err := store.RecordEmbedded(context.Background(), syncapp.Borrow(tx), fx.scope, app.MeterSource{Device: &fx.source}, fx.source.MembershipID, first); err != nil {
						return err
					}
					var pid int
					if err := tx.QueryRow(context.Background(), `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
						return err
					}
					holder <- pid
					<-release
					return nil
				})
			}()
			var pid int
			select {
			case pid = <-holder:
			case err := <-done:
				t.Fatal("first reading failed before holding its locks", err)
			}
			go func() { _, _, err := meterRecord(t, fx, store, fx.source, second); done <- err }()
			deadline := time.Now().Add(30 * time.Second)
			for {
				var blocked bool
				if err := fx.f.Admin.QueryRow(t.Context(), `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND usename=$1 AND wait_event_type='Lock' AND $2=ANY(pg_blocking_pids(pid)))`, fx.f.RuntimeRole, pid).Scan(&blocked); err != nil {
					t.Fatal(err)
				}
				if blocked {
					break
				}
				if time.Now().After(deadline) {
					close(release)
					t.Fatal("second reading never waited behind the first")
				}
				time.Sleep(5 * time.Millisecond)
			}
			close(release)
			for range 2 {
				if err := <-done; err != nil {
					t.Fatal(err)
				}
			}
			hi, _ := high.Snapshot()
			lo, _ := low.Snapshot()
			if firstHigh {
				if recordedState(t, fx, hi.ReadingID) != "VALID" || recordedState(t, fx, lo.ReadingID) != "SUSPECT" || readingStates(t, fx, lo.ReadingID)[0].flags != "READING_BELOW_PREVIOUS" || latestReadingID(t, fx, store, installed) != hi.ReadingID {
					t.Fatal("later lower reading not checked against the committed higher one")
				}
			} else {
				if recordedState(t, fx, lo.ReadingID) != "VALID" || recordedState(t, fx, hi.ReadingID) != "SUSPECT" || readingStates(t, fx, hi.ReadingID)[0].flags != "READING_ABOVE_NEXT" || latestReadingID(t, fx, store, installed) != lo.ReadingID {
					t.Fatal("later retroactive higher reading not checked against the committed lower one")
				}
			}
		})
	}
}

// The configured rollback stores readings VALID without flags or history
// (EDR-185); the latest-valid query still answers.
func TestMeterValidationSwitchOffStoresValidWithoutHistory(t *testing.T) {
	base := time.Now().UTC().Truncate(time.Second).Add(-5 * time.Hour)
	fx, store, installed := validationFixture(t, 100_000, base)
	store.Plausibility = nil
	command := meterObservedCommand(t, installed, base.Add(time.Hour), 1)
	if _, _, err := meterRecord(t, fx, store, fx.source, command); err != nil {
		t.Fatal(err)
	}
	in, _ := command.Snapshot()
	if recordedState(t, fx, in.ReadingID) != "VALID" || len(readingStates(t, fx, in.ReadingID)) != 0 || latestReadingID(t, fx, store, installed) != in.ReadingID {
		t.Fatal("disabled validation flagged or historised a reading")
	}
}

// 000216 guards: forced RLS, insert-only history, the next revision only,
// revision 1 equal to the recorded classification and only the fleet §6.5
// transitions, whoever writes.
func TestMeterReadingStateSchemaGuardsTheHistory(t *testing.T) {
	base := time.Now().UTC().Truncate(time.Second).Add(-5 * time.Hour)
	fx, store, installed := validationFixture(t, 0, base)
	v, _ := validationRecord(t, fx, store, installed, base.Add(time.Hour), 100_000, readingStateRow{state: "VALID"})
	s, sc := validationRecord(t, fx, store, installed, base.Add(2*time.Hour), 50_000, readingStateRow{state: "SUSPECT", flags: "READING_BELOW_PREVIOUS"})
	f, _ := validationRecord(t, fx, store, installed, time.Now().UTC().Add(time.Hour), 120_000, readingStateRow{state: "SUSPECT", flags: "FUTURE_INSTANT"})
	sin, _ := sc.Snapshot()
	org, meterID := fx.scope.OrgID().String(), installed.MeterID
	insert := func(reading string, revision int, state, flags, cause, actor, reason, replacement string) error {
		return fleetSQL(t, fx.legalFixtureState, `INSERT INTO org_meter_reading_states(org_id,meter_id,reading_id,revision,state,flags,cause,operation_id,actor_membership_id,reason,superseded_by)
VALUES($1,$2,$3,$4,$5,COALESCE(string_to_array(NULLIF($6,''),'+'),'{}'),$7,$8,NULLIF($9,'')::uuid,NULLIF($10,''),NULLIF($11,'')::uuid)`, org, meterID, reading, revision, state, flags, cause, onboardingID(t), actor, reason, replacement)
	}
	for name, c := range map[string]struct {
		err  error
		code string
	}{
		"revision gap":                        {insert(s, 3, "VALID", "", "REEVALUATION", "", "", ""), "23514"},
		"intake twice":                        {insert(s, 2, "SUSPECT", "READING_BELOW_PREVIOUS", "INTAKE", "", "", ""), "23514"},
		"intake differs from recorded":        {insert(installed.OpeningReadingID, 1, "SUSPECT", "IMPLAUSIBLE_RATE", "INTAKE", "", "", ""), "23514"},
		"valid demoted":                       {insert(v, 2, "SUSPECT", "IMPLAUSIBLE_RATE", "REEVALUATION", "", "", ""), "23514"},
		"no-op reevaluation":                  {insert(s, 2, "SUSPECT", "READING_BELOW_PREVIOUS", "REEVALUATION", "", "", ""), "23514"},
		"reevaluation drops future":           {insert(f, 2, "VALID", "", "REEVALUATION", "", "", ""), "23514"},
		"monotonicity confirmed":              {insert(s, 2, "VALID", "", "CONFIRMED", fx.member, "Synthetic confirmation", ""), "23514"},
		"non-canonical flags":                 {insert(s, 2, "SUSPECT", "IMPLAUSIBLE_RATE+READING_BELOW_PREVIOUS", "REEVALUATION", "", "", ""), "23514"},
		"unknown flag":                        {insert(s, 2, "SUSPECT", "POSSIBLE_ROLLOVER", "REEVALUATION", "", "", ""), "23514"},
		"suspect without flags":               {insert(s, 2, "SUSPECT", "", "REEVALUATION", "", "", ""), "23514"},
		"person decision without reason":      {insert(s, 2, "VOIDED", "", "VOIDED", fx.member, "", ""), "23514"},
		"system cause with actor":             {insert(s, 2, "VALID", "", "REEVALUATION", fx.member, "Synthetic", ""), "23514"},
		"supersession from another operation": {insert(s, 2, "VOIDED", "", "SUPERSEDED", fx.member, "Synthetic correction", v), "23514"},
		"foreign reading":                     {insert(onboardingID(t), 1, "VALID", "", "INTAKE", "", "", ""), "23503"},
	} {
		t.Run(name, func(t *testing.T) { fleetSQLCode(t, c.err, c.code) })
	}
	if err := insert(s, 2, "VOIDED", "", "VOIDED", fx.member, "Synthetic void", ""); err != nil {
		t.Fatal("documented void transition refused", err)
	}
	fleetSQLCode(t, insert(s, 3, "VALID", "", "REEVALUATION", "", "", ""), "23514")
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT UPDATE,DELETE,TRUNCATE ON org_meter_reading_states TO "+role); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{`UPDATE org_meter_reading_states SET reason='x' WHERE org_id=$1`, `DELETE FROM org_meter_reading_states WHERE org_id=$1`} {
		fleetSQLCode(t, fleetSQL(t, fx.legalFixtureState, sql, org), "55000")
	}
	fleetSQLCode(t, fleetSQL(t, fx.legalFixtureState, `TRUNCATE org_meter_reading_states`), "55000")
	var n int
	if err := fx.f.Runtime.QueryRow(t.Context(), `SELECT count(*) FROM org_meter_reading_states`).Scan(&n); err != nil || n != 0 {
		t.Fatal("history read without organization escaped forced RLS", err)
	}
	other, _, err := fx.onboarding.Create(t.Context(), fx.actor, onboardingCommand(t, onboardingID(t)))
	if err != nil {
		t.Fatal(err)
	}
	foreign := scopeFor(t, other.OrgID.String())
	err = database.WithOrgTx(t.Context(), fx.f.Runtime, foreign, func(tx pgx.Tx) error {
		if err := tx.QueryRow(t.Context(), `SELECT count(*) FROM org_meter_reading_states`).Scan(&n); err != nil || n != 0 {
			t.Fatal("foreign organization sees reading history", err)
		}
		_, err := tx.Exec(t.Context(), `INSERT INTO org_meter_reading_states(org_id,meter_id,reading_id,revision,state,cause,operation_id) VALUES($1,$2,$3,2,'VALID','REEVALUATION',$4)`, org, meterID, sin.ReadingID, onboardingID(t))
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "42501") && !strings.Contains(err.Error(), "row-level security") {
		t.Fatal("foreign organization wrote reading history", err)
	}
}
