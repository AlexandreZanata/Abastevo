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

// A populated 000215 head (organization, two assets with installed meters and
// their observed opening readings) upgrades to 000216 by an ordinary
// non-superuser migrator: an injected failure leaves schema, guard and data
// untouched, recovery applies once, replay is a no-op, the previous embedded
// image stays compatible, every existing reading keeps its recorded VALID
// state without fabricated history, and the new guards apply to it.
func TestMeterReadingStatesPopulatedUpgradeFailureRecoveryAndPreviousImage(t *testing.T) {
	const name = "000216_organization_meter_reading_states.sql"
	previous := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() != "schema_min_supported" && (entry.Name() >= name || !strings.HasSuffix(entry.Name(), ".sql")) {
			continue
		}
		raw, err := fs.ReadFile(migrations.Files, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		previous[entry.Name()] = &fstest.MapFile{Data: raw}
	}
	if _, ok := previous["000215_organization_fuelings.sql"]; !ok {
		t.Fatal("previous image does not end at the 000215 head")
	}
	f := orgtest.NewWithMigrations(t, previous)
	_, _, onboarding, actor, flags := onboardingOnFixture(t, f)
	fx := legalFixtureFor(t, f, onboarding, actor, flags)
	org := fx.scope.OrgID().String()
	assets := fleetStore(t, fx)
	meters := meterStore(t, fx)
	var openings []string
	for range 2 {
		created, _, err := fleetWrite(t, fx, assets, fleetCommand(t, fleetCreateInput()))
		if err != nil {
			t.Fatal(err)
		}
		installed, _, err := meterInstall(t, fx, meters, meterInstallCommand(t, meterInstallInput(t, created.AssetID, time.Now().UTC().Add(-time.Hour))))
		if err != nil {
			t.Fatal("unclassified installation must work on the 000215 schema", err)
		}
		openings = append(openings, installed.OpeningReadingID)
	}
	snapshot := func() string {
		t.Helper()
		var value string
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id)FROM organizations x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_memberships x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_assets x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_meters x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_meter_segments x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_meter_readings x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_meter_commands x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_audit_events x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY id)FROM accounts x))::text`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	before := snapshot()
	migrator := f.RuntimeRole + "_reading_states_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE,CREATE ON SCHEMA public TO "+role+";GRANT SELECT,INSERT ON schema_migrations TO "+role+
		";GRANT REFERENCES ON org_meter_readings,org_memberships TO "+role+";ALTER FUNCTION org_meter_observation_guard() OWNER TO "+role); err != nil {
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
	raw, err := fs.ReadFile(migrations.Files, name)
	if err != nil {
		t.Fatal(err)
	}
	guard := func() string {
		t.Helper()
		var body string
		if err := f.Admin.QueryRow(t.Context(), `SELECT pg_get_functiondef('org_meter_observation_guard()'::regprocedure)`).Scan(&body); err != nil {
			t.Fatal(err)
		}
		return body
	}
	oldGuard := guard()
	if !strings.Contains(oldGuard, "NEW.state='VALID';") {
		t.Fatal("previous observation guard is not the 000201 body")
	}
	broken := fstest.MapFS{name: &fstest.MapFile{Data: append(append([]byte{}, raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(t.Context(), location.String(), broken); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
		t.Fatal("injected reading-state migration failure not reached", err)
	}
	var clean bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('org_meter_reading_states') IS NULL AND to_regprocedure('org_meter_reading_state_guard()') IS NULL AND to_regprocedure('org_meter_flags_canonical(text[])') IS NULL
 AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version='000216')`).Scan(&clean); err != nil || !clean || guard() != oldGuard || before != snapshot() {
		t.Fatal("failed reading-state migration changed the original schema, guard or data", err)
	}
	current := fstest.MapFS{}
	for path, data := range previous {
		current[path] = data
	}
	current[name] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 1 || applied[0] != "000216" {
		t.Fatal("ordinary reading-state migration recovery", applied, err)
	}
	if before != snapshot() {
		t.Fatal("reading-state upgrade rewrote existing evidence")
	}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 0 {
		t.Fatal("reading-state migration no-op replay", err)
	}
	if err := migrate.Check(t.Context(), f.Admin, previous); err != nil {
		t.Fatal("previous embedded image incompatible", err)
	}
	newGuard := guard()
	if strings.Replace(oldGuard, "NEW.state='VALID';", "NEW.state IN('VALID','SUSPECT');", 1) != newGuard {
		t.Fatal("the observation guard changed beyond the reviewed-reading classification clause")
	}
	if err := f.Admin.QueryRow(t.Context(), `SELECT NOT rolsuper AND NOT rolbypassrls AND NOT rolcanlogin AND(SELECT relrowsecurity AND relforcerowsecurity AND relowner=(SELECT oid FROM pg_roles WHERE rolname=$1) FROM pg_class WHERE oid='org_meter_reading_states'::regclass)
 AND NOT EXISTS(SELECT 1 FROM org_meter_reading_states) AND(SELECT count(*)=2 AND bool_and(state='VALID') FROM org_meter_readings WHERE org_id=$2::uuid)
 AND NOT EXISTS(SELECT 1 FROM pg_constraint WHERE connamespace='public'::regnamespace AND NOT convalidated)
 FROM pg_roles WHERE rolname=$1`, migrator, org).Scan(&clean); err != nil || !clean {
		t.Fatal("upgraded history table not owned by the forced-RLS migrator, or legacy history fabricated", err)
	}
	// The new guard governs legacy readings from their recorded state.
	role = pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_meter_reading_states TO "+role); err != nil {
		t.Fatal(err)
	}
	var meterID string
	if err := f.Admin.QueryRow(t.Context(), `SELECT meter_id::text FROM org_meter_readings WHERE org_id=$1 AND id=$2`, org, openings[0]).Scan(&meterID); err != nil {
		t.Fatal(err)
	}
	insert := func(revision int, state, cause string) error {
		return fleetSQL(t, fx, `INSERT INTO org_meter_reading_states(org_id,meter_id,reading_id,revision,state,cause,operation_id,actor_membership_id,reason) VALUES($1,$2,$3,$4,$5,$6,$7,CASE WHEN $6 IN('VOIDED','CONFIRMED') THEN $8::uuid END,CASE WHEN $6 IN('VOIDED','CONFIRMED') THEN 'Synthetic legacy decision' END)`, org, meterID, openings[0], revision, state, cause, onboardingID(t), fx.member)
	}
	fleetSQLCode(t, insert(1, "SUSPECT", "INTAKE"), "23514")
	fleetSQLCode(t, insert(1, "VALID", "CONFIRMED"), "23514")
	if err := insert(1, "VOIDED", "VOIDED"); err != nil {
		t.Fatal("legacy VALID reading cannot follow the documented void transition", err)
	}
	if _, err := migrate.Apply(t.Context(), f.AdminDSN, migrations.Files); err != nil {
		t.Fatal("remaining embedded chain after the 216 rehearsal", err)
	}
}
