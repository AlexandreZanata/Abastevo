//go:build integration

package adapters_test

import (
	"context"
	"io/fs"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

const trackingMigration = "000240_organization_tracking_intake.sql"

// A populated head before 000240 (a completed and an in-progress trip through
// the real signed intake, with their readings and receipts) upgrades to 000240
// by an ordinary non-superuser migrator: an injected failure leaves schema and
// data untouched, recovery applies once, replay is a no-op, the previous
// embedded image stays compatible, earlier receipts replay, and the in-progress
// trip is then tracked on the upgraded schema (EMP-GPS-004).
func TestTrackingPopulatedUpgradeFailureRecoveryAndPreviousImage(t *testing.T) {
	previous := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() != "schema_min_supported" && (entry.Name() >= trackingMigration || !strings.HasSuffix(entry.Name(), ".sql")) {
			continue
		}
		raw, err := fs.ReadFile(migrations.Files, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		previous[entry.Name()] = &fstest.MapFile{Data: raw}
	}
	if _, ok := previous[missionVehicleMigration]; !ok {
		t.Fatal("previous image does not contain", missionVehicleMigration)
	}
	f := orgtest.NewWithMigrations(t, previous)
	_, _, onboarding, actor, flags := onboardingOnFixture(t, f)
	legal := legalFixtureFor(t, f, onboarding, actor, flags)
	x := tripFixtureOn(t, selfIntakeFixtureOn(t, syncFixtureOn(t, devicesFixtureOn(t, legal))))
	d := x.syncFixtureState
	completedAt := x.declaredNow(t)
	done := x.startOp(t, d, completedAt, nil)
	ack, err := x.process(t, d, done)
	x.accepted(t, done, ack, err, 1, 1)
	end := x.finishOp(t, d, tripOf(done), completedAt.Add(time.Microsecond), nil)
	endAck, err := x.process(t, d, end)
	x.accepted(t, end, endAck, err, 2, 1)
	snapshot := func() string {
		t.Helper()
		var value string
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id)FROM organizations x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_trips x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,trip_id)FROM org_trip_finishes x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_meter_readings x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_sync_acknowledgements x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_audit_events x))::text`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	before := snapshot()
	migrator := f.RuntimeRole + "_tracking_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE,CREATE ON SCHEMA public TO "+role+";GRANT SELECT,INSERT ON schema_migrations TO "+role+
		";GRANT REFERENCES ON organizations,org_trips,org_memberships,org_sync_operations,org_device_enrollments TO "+role); err != nil {
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
	raw, err := fs.ReadFile(migrations.Files, trackingMigration)
	if err != nil {
		t.Fatal(err)
	}
	version := strings.SplitN(trackingMigration, "_", 2)[0]
	broken := fstest.MapFS{trackingMigration: &fstest.MapFile{Data: append(append([]byte{}, raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(t.Context(), location.String(), broken); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
		t.Fatal("injected tracking migration failure not reached", err)
	}
	var clean bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('org_tracking_sessions') IS NULL AND to_regclass('org_tracking_segments') IS NULL AND to_regclass('org_tracking_policies') IS NULL
 AND to_regprocedure('org_tracking_precision_at(uuid,uuid,timestamptz)') IS NULL AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, version).Scan(&clean); err != nil || !clean || before != snapshot() {
		t.Fatal("failed tracking migration changed the original schema or data", err)
	}
	current := fstest.MapFS{}
	for path, data := range previous {
		current[path] = data
	}
	current[trackingMigration] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 1 || applied[0] != version {
		t.Fatal("ordinary tracking migration recovery", applied, err)
	}
	if before != snapshot() {
		t.Fatal("tracking upgrade rewrote existing state")
	}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 0 {
		t.Fatal("tracking migration no-op replay", err)
	}
	if err := migrate.Check(t.Context(), f.Admin, previous); err != nil {
		t.Fatal("previous embedded image incompatible", err)
	}
	if err := f.Admin.QueryRow(t.Context(), `SELECT NOT rolsuper AND NOT rolbypassrls AND NOT rolcanlogin AND(SELECT count(*)=4 AND bool_and(relrowsecurity AND relforcerowsecurity AND relowner=(SELECT oid FROM pg_roles WHERE rolname=$1))
 FROM pg_class WHERE oid=ANY(ARRAY['org_tracking_policies'::regclass,'org_tracking_sessions'::regclass,'org_tracking_transitions'::regclass,'org_tracking_segments'::regclass]))FROM pg_roles WHERE rolname=$1`, migrator).Scan(&clean); err != nil || !clean {
		t.Fatal("the tracking tables are not owned by the ordinary forced-RLS migrator", err)
	}
	// Earlier receipts replay on the upgraded schema.
	if replay, err := x.process(t, d, done); err != nil || string(replay.CanonicalBytes()) != string(ack.CanonicalBytes()) {
		t.Fatal("a pre-upgrade start does not replay", err)
	}
	if replay, err := x.process(t, d, end); err != nil || string(replay.CanonicalBytes()) != string(endAck.CanonicalBytes()) {
		t.Fatal("a pre-upgrade finish does not replay", err)
	}
	// The pre-upgrade trip cannot gain a session (it ended); a new trip is
	// tracked on the upgraded schema.
	y := trackingFixtureOn(t, x)
	y.inForce(t)
	late := y.driverOp(t, d, "gps.session.start", onboardingID(t), x.declaredNow(t).Truncate(time.Microsecond), "PRECISE", func(_, p map[string]any) { p["trip_id"] = tripOf(done) })
	lateAck, err := y.process(t, d, late)
	y.rejected(t, late, lateAck, err, "MALFORMED")
	session := onboardingID(t)
	y.startTracking(t, d, session, 1, "PRECISE", 2)
	batch := y.batchOp(t, d, session, 1, []int{1, 2, 3}, nil)
	batchAck, err := y.process(t, d, batch)
	y.accepted(t, batch, batchAck, err, 1, trackingCounts{segments: 1})
	if _, err := migrate.Apply(t.Context(), f.AdminDSN, migrations.Files); err != nil {
		t.Fatal("remaining embedded chain after the 240 rehearsal", err)
	}
}
