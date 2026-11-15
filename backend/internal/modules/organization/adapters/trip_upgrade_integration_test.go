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
	fa "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	fuel "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

// A populated 000215 head (organization, unit, cost centre, ACTIVE tanked
// vehicle with an observed meter and its unit period) upgrades to 000220 by
// an ordinary non-superuser migrator: an injected failure leaves schema and
// data untouched, recovery applies once, replay is a no-op, the previous
// embedded image stays compatible, the redefined usage sources still answer
// for the existing asset, unit and cost centre and the review-source hook
// keeps its meter branch (EMP-TRIP-002).
func TestTripPopulatedUpgradeFailureRecoveryAndPreviousImage(t *testing.T) {
	const name = "000220_organization_trips.sql"
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
	var unit string
	if err := f.Admin.QueryRow(t.Context(), `SELECT id::text FROM org_units WHERE org_id=$1`, org).Scan(&unit); err != nil {
		t.Fatal(err)
	}
	if _, _, err := fx.onboarding.Repository.ActivateOrganization(t.Context(), fx.scope, fx.actor, fx.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	assets := lifecycleStore(t, fx)
	meters := meterStore(t, fx)
	input := fleetCreateInput()
	input.Profile.Kind, input.Profile.UsageBasis, input.UnitID = fleet.Vehicle, fleet.Distance, unit
	tank, err := fleet.NewTank(fuel.Catalog{}, fleet.Main, "100000", []string{"DIESEL_S10"})
	if err != nil {
		t.Fatal(err)
	}
	input.Profile.Tanks = []fleet.Tank{tank}
	created, _, err := fleetWrite(t, fx, assets, fleetCommand(t, input))
	if err != nil {
		t.Fatal(err)
	}
	installed, _, err := meterInstall(t, fx, meters, meterInstallCommand(t, meterInstallInput(t, created.AssetID, time.Now().UTC().Add(-time.Hour))))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := fleetWrite(t, fx, assets, fleetCommand(t, fa.AssetWriteInput{Kind: fa.ActivateAsset, AssetID: created.AssetID, ExpectedRevision: installed.AssetRevision})); err != nil {
		t.Fatal(err)
	}
	centre := onboardingID(t)
	if err := pgx.BeginFunc(t.Context(), f.Admin, func(tx pgx.Tx) error {
		if _, err := tx.Exec(t.Context(), `SELECT set_config('anpfuel.org_id',$1,true)`, org); err != nil {
			return err
		}
		_, err := tx.Exec(t.Context(), `INSERT INTO org_cost_centres(org_id,id,name,code,state) VALUES($1,$2,'Synthetic centre','SYN-1','ACTIVE')`, org, centre)
		return err
	}); err != nil {
		t.Fatal("cost-centre fixture", err)
	}
	snapshot := func() string {
		t.Helper()
		var value string
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id)FROM organizations x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_units x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_cost_centres x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_memberships x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_employee_identities x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_assets x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,asset_id,position)FROM org_asset_tanks x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,asset_id,valid_from)FROM org_asset_unit_periods x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_meters x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_meter_readings x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_audit_events x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_sync_operations x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY id)FROM accounts x))::text`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	usage := func() (string, string, string) {
		t.Helper()
		tx, err := f.Admin.Begin(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(context.Background()) }()
		var asset, units, centres string
		if err := tx.QueryRow(t.Context(), `SELECT set_config('anpfuel.org_id',$1,true)`, org).Scan(new(string)); err != nil {
			t.Fatal(err)
		}
		if err := tx.QueryRow(t.Context(), `SELECT org_asset_current_usage($1,$2)::text,org_unit_current_usage($1,$3)::text,org_cost_centre_current_usage($1,$4)::text`, org, created.AssetID, unit, centre).Scan(&asset, &units, &centres); err != nil {
			t.Fatal("usage source refused the populated state", err)
		}
		return asset, units, centres
	}
	before := snapshot()
	beforeAsset, beforeUnit, beforeCentre := usage()
	migrator := f.RuntimeRole + "_trip_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE,CREATE ON SCHEMA public TO "+role+";GRANT SELECT,INSERT ON schema_migrations TO "+role+
		";GRANT REFERENCES ON organizations,org_sync_operations,org_assets,org_employee_identities,org_memberships,org_device_enrollments,org_meter_readings,org_meters,org_meter_segments,org_units,org_cost_centres,org_sync_review_items TO "+role+
		";ALTER FUNCTION org_asset_current_usage(uuid,uuid) OWNER TO "+role+";ALTER FUNCTION org_unit_current_usage(uuid,uuid) OWNER TO "+role+";ALTER FUNCTION org_cost_centre_current_usage(uuid,uuid) OWNER TO "+role+
		";ALTER FUNCTION org_sync_review_source_valid(uuid,uuid,text,bigint) OWNER TO "+role); err != nil {
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
	broken := fstest.MapFS{name: &fstest.MapFile{Data: append(append([]byte{}, raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(t.Context(), location.String(), broken); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
		t.Fatal("injected trip migration failure not reached", err)
	}
	var clean bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('org_trips') IS NULL AND to_regclass('org_trip_finishes') IS NULL AND to_regclass('org_trip_held_starts') IS NULL AND to_regclass('org_trip_held_finishes') IS NULL
 AND to_regprocedure('org_trip_source()') IS NULL AND to_regprocedure('org_trip_complete()') IS NULL AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version='000220')
 AND pg_get_functiondef('org_asset_current_usage(uuid,uuid)'::regprocedure) NOT LIKE '%org_trips%' AND pg_get_functiondef('org_unit_current_usage(uuid,uuid)'::regprocedure) NOT LIKE '%org_trips%'
 AND pg_get_functiondef('org_cost_centre_current_usage(uuid,uuid)'::regprocedure) NOT LIKE '%org_trips%'
 AND pg_get_functiondef('org_sync_review_source_valid(uuid,uuid,text,bigint)'::regprocedure) NOT LIKE '%trip.start%'`).Scan(&clean); err != nil || !clean || before != snapshot() {
		t.Fatal("failed trip migration changed the original schema or data", err)
	}
	current := fstest.MapFS{}
	for path, data := range previous {
		current[path] = data
	}
	current[name] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 1 || applied[0] != "000220" {
		t.Fatal("ordinary trip migration recovery", applied, err)
	}
	if before != snapshot() {
		t.Fatal("trip upgrade rewrote existing enterprise/personal state")
	}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 0 {
		t.Fatal("trip migration no-op replay", err)
	}
	if err := migrate.Check(t.Context(), f.Admin, previous); err != nil {
		t.Fatal("previous embedded image incompatible", err)
	}
	if err := f.Admin.QueryRow(t.Context(), `SELECT NOT rolsuper AND NOT rolbypassrls AND NOT rolcanlogin AND(SELECT count(*)=4 AND bool_and(relrowsecurity AND relforcerowsecurity AND relowner=(SELECT oid FROM pg_roles WHERE rolname=$1))FROM pg_class WHERE oid=ANY(ARRAY['org_trips'::regclass,'org_trip_finishes'::regclass,'org_trip_held_starts'::regclass,'org_trip_held_finishes'::regclass]))FROM pg_roles WHERE rolname=$1`, migrator).Scan(&clean); err != nil || !clean {
		t.Fatal("new trip tables not owned by the ordinary forced-RLS migrator", err)
	}
	if asset, units, centres := usage(); asset != beforeAsset || units != beforeUnit || centres != beforeCentre {
		t.Fatal("redefined usage sources changed the populated answers", asset, units, centres)
	}
	var listed bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT pg_get_functiondef('org_asset_current_usage(uuid,uuid)'::regprocedure) LIKE '%org_trips_asset_fkey%' AND pg_get_functiondef('org_asset_current_usage(uuid,uuid)'::regprocedure) LIKE '%org_trip_held_starts_asset_fkey%' AND pg_get_functiondef('org_asset_current_usage(uuid,uuid)'::regprocedure) LIKE '%org_fuelings_asset_fkey%'
 AND pg_get_functiondef('org_unit_current_usage(uuid,uuid)'::regprocedure) LIKE '%org_trips_unit_fkey%' AND pg_get_functiondef('org_unit_current_usage(uuid,uuid)'::regprocedure) LIKE '%org_fuelings_unit_fkey%'
 AND pg_get_functiondef('org_cost_centre_current_usage(uuid,uuid)'::regprocedure) LIKE '%org_trips_cost_centre_fkey%' AND pg_get_functiondef('org_cost_centre_current_usage(uuid,uuid)'::regprocedure) LIKE '%org_fuelings_cost_centre_fkey%'
 AND pg_get_functiondef('org_asset_current_usage(uuid,uuid)'::regprocedure) LIKE '%org_asset_reservations_asset_fkey%' AND pg_get_functiondef('org_asset_current_usage(uuid,uuid)'::regprocedure) LIKE '%org_asset_blocks_asset_fkey%'
 AND pg_get_functiondef('org_sync_review_source_valid(uuid,uuid,text,bigint)'::regprocedure) LIKE '%meter.reading.record%' AND pg_get_functiondef('org_sync_review_source_valid(uuid,uuid,text,bigint)'::regprocedure) LIKE '%org_meter_held_readings%'`).Scan(&listed); err != nil || !listed {
		t.Fatal("redefined sources lost an existing key or miss the trip key", err)
	}
	if _, err := migrate.Apply(t.Context(), f.AdminDSN, migrations.Files); err != nil {
		t.Fatal("remaining embedded chain after the 220 rehearsal", err)
	}
}
