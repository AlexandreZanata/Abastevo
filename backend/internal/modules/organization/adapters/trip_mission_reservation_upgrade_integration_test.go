//go:build integration

package adapters_test

import (
	"bytes"
	"context"
	"io/fs"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	fa "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

const missionReservationLinkMigration = "000249_organization_trip_reservation_links.sql"

// A populated head before 000249 (the 000244 head: a completed and an
// in-progress trip through the real signed intake, an ACCEPTED mission, a
// held ASSET_BUSY start, an asset reservation confirmed before the upgrade)
// upgrades to 000249 by an ordinary non-superuser migrator: an injected
// failure leaves schema, functions, constraints and data untouched, recovery
// applies once, replay is a no-op, the previous embedded image stays
// compatible, every redefined function keeps its earlier lines, earlier
// receipts replay, a pre-upgrade trip completes without touching a mission,
// and the reservation-to-trip link then works on the upgraded schema
// (EMP-TRIP-023).
func TestTripMissionReservationPopulatedUpgradeFailureRecoveryAndPreviousImage(t *testing.T) {
	previous := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() != "schema_min_supported" && (entry.Name() >= missionReservationLinkMigration || !strings.HasSuffix(entry.Name(), ".sql")) {
			continue
		}
		raw, err := fs.ReadFile(migrations.Files, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		previous[entry.Name()] = &fstest.MapFile{Data: raw}
	}
	for _, head := range []string{"000243_organization_trip_handovers.sql", "000244_organization_expenses.sql", missionVehicleMigration} {
		if _, ok := previous[head]; !ok {
			t.Fatal("previous image does not contain", head)
		}
	}
	f := orgtest.NewWithMigrations(t, previous)
	_, _, onboarding, actor, flags := onboardingOnFixture(t, f)
	legal := legalFixtureFor(t, f, onboarding, actor, flags)
	x := tripFixtureOn(t, selfIntakeFixtureOn(t, syncFixtureOn(t, devicesFixtureOn(t, legal))))
	d := x.syncFixtureState
	first := x.startOp(t, d, x.declaredNow(t), nil)
	ack, err := x.process(t, d, first)
	x.accepted(t, first, ack, err, 1, 1)
	end := x.finishOp(t, d, tripOf(first), x.declaredNow(t).Add(time.Microsecond), nil)
	ack, err = x.process(t, d, end)
	x.accepted(t, end, ack, err, 2, 1)
	ongoing := x.startOp(t, d, x.declaredNow(t), nil)
	ongoingAck, err := x.process(t, d, ongoing)
	x.accepted(t, ongoing, ongoingAck, err, 1, 1)
	busy := x.startOp(t, d, x.declaredNow(t), nil)
	busyAck, err := x.process(t, d, busy)
	x.held(t, busy, busyAck, err, "ASSET_BUSY")
	m := missionFixtureOn(t, x)
	org := x.scope.OrgID().String()
	start := exactNow(t, m.legalFixtureState).Truncate(time.Second).Add(2 * time.Hour)
	accepted := m.missionAt(t, d, m.vehicle.AssetID, start, start.Add(time.Hour))
	before244, _, err := reserve(t, m.legalFixtureState, m.assets, fa.ReservationRequestInput{AssetID: m.vehicle.AssetID, Start: start.Add(3 * time.Hour), End: start.Add(4 * time.Hour), Confirm: true})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := func() string {
		t.Helper()
		var value string
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id)FROM organizations x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_assets x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_trips x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,trip_id)FROM org_trip_finishes x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_trip_missions x),
(SELECT jsonb_agg(to_jsonb(x)-'trip_id' ORDER BY org_id,mission_id,revision)FROM org_trip_mission_transitions x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_trip_mission_commands x),
(SELECT jsonb_agg(to_jsonb(x)-'mission_id'-'trip_id'-'fulfilled_at' ORDER BY org_id,id)FROM org_asset_reservations x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,review_item_id)FROM org_trip_held_starts x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_sync_acknowledgements x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_audit_events x))::text`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	replaced := []string{"org_asset_reservation_guard()", "org_trip_held_source()", "org_trip_mission_complete()", "org_trip_mission_guard()", "org_trip_mission_next(text,text)", "org_trip_mission_transition_source()"}
	functions := func() string {
		t.Helper()
		var v string
		if err := f.Admin.QueryRow(t.Context(), `SELECT string_agg(pg_get_functiondef(to_regprocedure(p)),E'\n' ORDER BY p) FROM unnest($1::text[]) p`, replaced).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	constraints := func() string {
		t.Helper()
		var v string
		if err := f.Admin.QueryRow(t.Context(), `SELECT string_agg(conname||'='||pg_get_constraintdef(oid),E'\n' ORDER BY conname) FROM pg_constraint
WHERE conrelid IN('org_trip_mission_transitions'::regclass,'org_trip_mission_commands'::regclass,'org_asset_reservations'::regclass)`).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	before, beforeFunctions, beforeConstraints := snapshot(), functions(), constraints()
	migrator := f.RuntimeRole + "_link_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	grants := "CREATE ROLE " + role + " NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE,CREATE ON SCHEMA public TO " + role + ";GRANT SELECT,INSERT ON schema_migrations TO " + role +
		";GRANT REFERENCES ON org_trips,org_trip_missions,org_sync_operations TO " + role + ";GRANT TRIGGER ON org_trips TO " + role +
		";ALTER TABLE org_asset_reservations OWNER TO " + role + ";ALTER TABLE org_trip_mission_transitions OWNER TO " + role + ";ALTER TABLE org_trip_mission_commands OWNER TO " + role
	for _, p := range replaced {
		grants += ";ALTER FUNCTION " + p + " OWNER TO " + role
	}
	if _, err := f.Admin.Exec(t.Context(), grants); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := f.Admin.Exec(ctx, "REASSIGN OWNED BY "+role+" TO CURRENT_USER;DROP OWNED BY "+role+";DROP ROLE "+role); err != nil {
			t.Error(err)
		}
	})
	location, err := url.Parse(f.AdminDSN)
	if err != nil {
		t.Fatal(err)
	}
	query := location.Query()
	query.Set("options", "-c role="+migrator)
	location.RawQuery = strings.ReplaceAll(query.Encode(), "+", "%20")
	raw, err := fs.ReadFile(migrations.Files, missionReservationLinkMigration)
	if err != nil {
		t.Fatal(err)
	}
	version := strings.SplitN(missionReservationLinkMigration, "_", 2)[0]
	broken := fstest.MapFS{missionReservationLinkMigration: &fstest.MapFile{Data: append(append([]byte{}, raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(t.Context(), location.String(), broken); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
		t.Fatal("injected link migration failure not reached", err)
	}
	var clean bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('org_trip_mission_links') IS NULL AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)
 AND NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='org_asset_reservations' AND column_name IN('mission_id','trip_id','fulfilled_at'))`, version).Scan(&clean); err != nil || !clean ||
		before != snapshot() || beforeFunctions != functions() || beforeConstraints != constraints() {
		t.Fatal("failed link migration changed the original schema, functions, constraints or data", err)
	}
	current := fstest.MapFS{}
	for path, data := range previous {
		current[path] = data
	}
	current[missionReservationLinkMigration] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 1 || applied[0] != version {
		t.Fatal("ordinary link migration recovery", applied, err)
	}
	if before != snapshot() {
		t.Fatal("link upgrade rewrote existing state")
	}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 0 {
		t.Fatal("link migration no-op replay", err)
	}
	if err := migrate.Check(t.Context(), f.Admin, previous); err != nil {
		t.Fatal("previous embedded image incompatible", err)
	}
	if err := f.Admin.QueryRow(t.Context(), `SELECT NOT rolsuper AND NOT rolbypassrls AND NOT rolcanlogin AND(SELECT relrowsecurity AND relforcerowsecurity AND relowner=(SELECT oid FROM pg_roles WHERE rolname=$1)
 FROM pg_class WHERE oid='org_trip_mission_links'::regclass)FROM pg_roles WHERE rolname=$1`, migrator).Scan(&clean); err != nil || !clean {
		t.Fatal("the link table is not owned by the ordinary forced-RLS migrator", err)
	}
	// Every redefined function keeps the lines of its latest earlier body.
	var lines bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT g LIKE '%reservation already ended%' AND g LIKE '%org_asset_reservations_block_overlap%' AND g LIKE '%reservation fulfilment source unavailable%'
 AND h LIKE '%ACCESS_ENDED_BEFORE_RECEIPT%' AND h LIKE '%org_trip_responsible%' AND h LIKE '%''ORPHAN_REFERENCE''%'
 AND n LIKE '%''VEHICLE''%' AND n LIKE '%''RESERVE''%' AND n LIKE '%''START''%'
 AND s LIKE '%mission response source unavailable%' AND s LIKE '%mission trip source unavailable%'
 AND c LIKE '%org_trip_mission_vehicles%' AND c LIKE '%org_trip_mission_links%' AND k LIKE '%org_trip_mission_plans%' AND k LIKE '%org_asset_reservations%'
 FROM(SELECT pg_get_functiondef('org_asset_reservation_guard()'::regprocedure) g,pg_get_functiondef('org_trip_held_source()'::regprocedure) h,
 pg_get_functiondef('org_trip_mission_next(text,text)'::regprocedure) n,pg_get_functiondef('org_trip_mission_transition_source()'::regprocedure) s,
 pg_get_functiondef('org_trip_mission_complete()'::regprocedure) c,pg_get_functiondef('org_trip_mission_guard()'::regprocedure) k) x`).Scan(&lines); err != nil || !lines {
		t.Fatal("an upgraded function lost a line or misses its EMP-TRIP-023 branch", err)
	}
	// Earlier receipts replay byte for byte.
	for op, want := range map[*protocol.Operation]protocol.Acknowledgement{&ongoing: ongoingAck, &busy: busyAck} {
		again, err := x.process(t, d, *op)
		if err != nil || !bytes.Equal(again.CanonicalBytes(), want.CanonicalBytes()) {
			t.Fatal("a pre-upgrade receipt does not replay", err)
		}
	}
	tripMissionLinkGrant(t, f)
	m.assets.PreMissionLinkImage = false
	m.missions.Reservations = m.assets
	// The pre-upgrade trip completes without a mission; the pre-upgrade
	// reservation keeps no mission.
	closing := x.finishOp(t, d, tripOf(ongoing), x.declaredNow(t), nil)
	ack, err = x.process(t, d, closing)
	x.accepted(t, closing, ack, err, 2, 1)
	if r := m.reservationLink(t, before244.ReservationID); r.state != "CONFIRMED" || r.mission != "" || r.trip != "" {
		t.Fatal("the upgrade or a completion touched a pre-upgrade reservation", r)
	}
	// The pre-upgrade accepted mission now reserves, is executed by a linked
	// start that consumes the reservation, and completes with its trip.
	reserved, _, err := m.reserveMission(t, accepted, 3, "", "Reserva após a atualização")
	if err != nil {
		t.Fatal("reservation request after the upgrade", err)
	}
	m.confirmReservation(t, m.vehicle.AssetID, reserved.ReservationID)
	// The window starts in two hours: a start declared now is outside the
	// early tolerance and links the mission without consuming it.
	op := m.missionStartOp(t, d, accepted, m.declaredNow(t))
	ack, err = m.process(t, d, op)
	m.accepted(t, op, ack, err, 1, 1)
	if m.linkedMission(t, tripOf(op)) != accepted || m.head(t, accepted).state != "IN_PROGRESS" {
		t.Fatal("linked start on the upgraded schema")
	}
	if r := m.reservationLink(t, reserved.ReservationID); r.state != "CONFIRMED" || r.trip != "" {
		t.Fatal("a start outside the early tolerance consumed the reservation", r)
	}
	finish := m.finishOp(t, d, tripOf(op), m.declaredNow(t).Add(time.Second), nil)
	ack, err = m.process(t, d, finish)
	m.accepted(t, finish, ack, err, 2, 1)
	if h := m.head(t, accepted); h.state != "COMPLETED" {
		t.Fatal("mission completion on the upgraded schema", h)
	}
	if n := m.countWhere(t, `SELECT count(*) FROM org_trip_mission_links WHERE org_id=$1`, org); n != 1 {
		t.Fatal("links after the upgrade", n)
	}
	if _, err := migrate.Apply(t.Context(), f.AdminDSN, migrations.Files); err != nil {
		t.Fatal("remaining embedded chain after the 249 rehearsal", err)
	}
}
