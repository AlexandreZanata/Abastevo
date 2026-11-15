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
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

// A populated head before 000227 (organization, ACTIVE vehicle with an
// observed meter, a completed trip and a trip in progress through the real
// signed intake, a second unit) upgrades to 000227 by an ordinary
// non-superuser migrator: an injected failure leaves schema, functions and
// data untouched, recovery applies once, replay is a no-op, the previous
// embedded image stays compatible, the unit and asset usage sources keep
// every line and their answers, and missions then work on the upgraded
// schema (EMP-TRIP-008).
func TestTripMissionPopulatedUpgradeFailureRecoveryAndPreviousImage(t *testing.T) {
	previous := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() != "schema_min_supported" && (entry.Name() >= missionMigration || !strings.HasSuffix(entry.Name(), ".sql")) {
			continue
		}
		raw, err := fs.ReadFile(migrations.Files, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		previous[entry.Name()] = &fstest.MapFile{Data: raw}
	}
	if _, ok := previous["000225_organization_trip_events.sql"]; !ok {
		t.Fatal("previous image does not contain the 000225 head")
	}
	f := orgtest.NewWithMigrations(t, previous)
	_, _, onboarding, actor, flags := onboardingOnFixture(t, f)
	legal := legalFixtureFor(t, f, onboarding, actor, flags)
	x := tripFixtureOn(t, selfIntakeFixtureOn(t, syncFixtureOn(t, devicesFixtureOn(t, legal))))
	d := x.syncFixtureState
	completedAt := x.declaredNow(t)
	first := x.startOp(t, d, completedAt, nil)
	ack, err := x.process(t, d, first)
	x.accepted(t, first, ack, err, 1, 1)
	end := x.finishOp(t, d, tripOf(first), completedAt.Add(time.Microsecond), nil)
	ack, err = x.process(t, d, end)
	x.accepted(t, end, ack, err, 2, 1)
	ongoing := x.startOp(t, d, x.declaredNow(t), nil)
	ack, err = x.process(t, d, ongoing)
	x.accepted(t, ongoing, ack, err, 1, 1)
	org := x.scope.OrgID().String()
	snapshot := func() string {
		t.Helper()
		var value string
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id)FROM organizations x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_units x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_memberships x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_employee_identities x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_assets x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_meter_readings x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_trips x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,trip_id)FROM org_trip_finishes x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_sync_operations x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_sync_acknowledgements x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_audit_events x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY id)FROM accounts x))::text`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	usage := func() string {
		t.Helper()
		tx, err := f.Admin.Begin(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(context.Background()) }()
		if _, err := tx.Exec(t.Context(), `SELECT set_config('anpfuel.org_id',$1,true)`, org); err != nil {
			t.Fatal(err)
		}
		var asset, unit string
		if err := tx.QueryRow(t.Context(), `SELECT org_asset_current_usage($1,$2)::text,org_unit_current_usage($1,$3)::text`, org, x.vehicle.AssetID, x.unit).Scan(&asset, &unit); err != nil {
			t.Fatal("usage sources refused the populated state", err)
		}
		return asset + unit
	}
	functions := func() string {
		t.Helper()
		var v string
		if err := f.Admin.QueryRow(t.Context(), `SELECT pg_get_functiondef('org_asset_current_usage(uuid,uuid)'::regprocedure)||pg_get_functiondef('org_unit_current_usage(uuid,uuid)'::regprocedure)`).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	before, beforeUsage, beforeFunctions := snapshot(), usage(), functions()
	migrator := f.RuntimeRole + "_mission_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE,CREATE ON SCHEMA public TO "+role+";GRANT SELECT,INSERT ON schema_migrations TO "+role+
		";GRANT REFERENCES ON accounts,org_units,org_assets,org_employee_identities,org_memberships,org_sync_operations,org_device_enrollments TO "+role+
		";ALTER FUNCTION org_asset_current_usage(uuid,uuid) OWNER TO "+role+";ALTER FUNCTION org_unit_current_usage(uuid,uuid) OWNER TO "+role); err != nil {
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
	raw, err := fs.ReadFile(migrations.Files, missionMigration)
	if err != nil {
		t.Fatal(err)
	}
	version := strings.SplitN(missionMigration, "_", 2)[0]
	broken := fstest.MapFS{missionMigration: &fstest.MapFile{Data: append(append([]byte{}, raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(t.Context(), location.String(), broken); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
		t.Fatal("injected mission migration failure not reached", err)
	}
	var clean bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('org_trip_missions') IS NULL AND to_regclass('org_trip_mission_transitions') IS NULL AND to_regclass('org_trip_mission_commands') IS NULL
 AND to_regprocedure('org_trip_mission_next(text,text)') IS NULL AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, version).Scan(&clean); err != nil || !clean || before != snapshot() || beforeFunctions != functions() {
		t.Fatal("failed mission migration changed the original schema, functions or data", err)
	}
	current := fstest.MapFS{}
	for path, data := range previous {
		current[path] = data
	}
	current[missionMigration] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 1 || applied[0] != version {
		t.Fatal("ordinary mission migration recovery", applied, err)
	}
	if before != snapshot() || beforeUsage != usage() {
		t.Fatal("mission upgrade rewrote existing state or a usage answer")
	}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 0 {
		t.Fatal("mission migration no-op replay", err)
	}
	if err := migrate.Check(t.Context(), f.Admin, previous); err != nil {
		t.Fatal("previous embedded image incompatible", err)
	}
	if err := f.Admin.QueryRow(t.Context(), `SELECT NOT rolsuper AND NOT rolbypassrls AND NOT rolcanlogin AND(SELECT count(*)=3 AND bool_and(relrowsecurity AND relforcerowsecurity AND relowner=(SELECT oid FROM pg_roles WHERE rolname=$1))
 FROM pg_class WHERE oid=ANY(ARRAY['org_trip_missions'::regclass,'org_trip_mission_transitions'::regclass,'org_trip_mission_commands'::regclass]))FROM pg_roles WHERE rolname=$1`, migrator).Scan(&clean); err != nil || !clean {
		t.Fatal("new mission tables not owned by the ordinary forced-RLS migrator", err)
	}
	var lines bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT a LIKE '%org_trip_missions_asset_fkey%' AND a LIKE '%org_checklist_template_targets_asset_fkey%' AND a LIKE '%org_asset_implement_attachments_powered_fkey%'
 AND a LIKE '%org_trip_held_starts_asset_fkey%' AND u LIKE '%org_trip_missions_unit_fkey%' AND u LIKE '%org_trips_unit_fkey%' AND u LIKE '%org_fuelings_unit_fkey%'
 FROM(SELECT pg_get_functiondef('org_asset_current_usage(uuid,uuid)'::regprocedure) AS a,pg_get_functiondef('org_unit_current_usage(uuid,uuid)'::regprocedure) AS u) s`).Scan(&lines); err != nil || !lines {
		t.Fatal("upgraded usage sources lost a line or miss the mission keys", err)
	}
	// Missions work on the upgraded schema with the pre-existing records.
	m := missionFixtureOn(t, x)
	driver := m.driver(t)
	mission := m.create(t, m.unit, m.vehicle.AssetID).MissionID
	if _, err := m.assignTo(t, mission, 1, driver.person.employee); err != nil {
		t.Fatal(err)
	}
	accept := m.respondOp(t, driver, mission, 2, "ACCEPT", nil, nil)
	if ack, err := m.deliver(t, driver, accept); err != nil || ack.Result() != protocol.Accepted {
		t.Fatal("a response on the upgraded schema", err)
	}
	if usage() != beforeUsage {
		t.Fatal("a mission changed a usage answer")
	}
	if _, err := migrate.Apply(t.Context(), f.AdminDSN, migrations.Files); err != nil {
		t.Fatal("remaining embedded chain after the 227 rehearsal", err)
	}
}
