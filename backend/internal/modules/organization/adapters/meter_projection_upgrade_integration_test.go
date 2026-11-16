//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"io/fs"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	fleetpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

const meterProjectionMigration = "000252_organization_meter_current_values.sql"

// A populated 000251 head (a meter with readings and history) upgrades to
// 000252 by an ordinary non-superuser migrator: an injected failure leaves
// schema and data untouched, recovery applies once, replay is a no-op, the
// previous image stays compatible and nothing existing is rewritten; the
// legacy meter has no projection until the operator rebuild (its usage is
// unavailable, never a number), after which the database maintains it.
func TestMeterProjectionPopulatedUpgradeFailureRecoveryAndRebuild(t *testing.T) {
	previous := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() != "schema_min_supported" && (entry.Name() >= meterProjectionMigration || !strings.HasSuffix(entry.Name(), ".sql")) {
			continue
		}
		raw, err := fs.ReadFile(migrations.Files, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		previous[entry.Name()] = &fstest.MapFile{Data: raw}
	}
	if _, ok := previous[meterSegmentsMigration]; !ok {
		t.Fatal("previous image does not contain the 000251 head")
	}
	f := orgtest.NewWithMigrations(t, previous)
	_, _, onboarding, actor, flags := onboardingOnFixture(t, f)
	fx := syncFixtureOn(t, devicesFixtureOn(t, legalFixtureFor(t, f, onboarding, actor, flags)))
	base := time.Now().UTC().Truncate(time.Second).Add(-10 * 24 * time.Hour)
	asset, _, err := fleetWrite(t, fx.legalFixtureState, fleetStore(t, fx.legalFixtureState), fleetCommand(t, fleetCreateInput()))
	if err != nil {
		t.Fatal(err)
	}
	store := meterStore(t, fx.legalFixtureState)
	store.Authority = &orgpg.MeterAuthority{Memberships: fx.legalFixtureState.store.Memberships, Roles: fx.legalFixtureState.store.Roles, Capabilities: fx.flags, Devices: fx.devices}
	installed, _, err := meterInstall(t, fx.legalFixtureState, store, meterInstallCommand(t, func() app.MeterInstallInput {
		in := meterInstallInput(t, asset.AssetID, base)
		in.Value = odometerValue(t, 1_000_000)
		return in
	}()))
	if err != nil {
		t.Fatal(err)
	}
	policy := meter.DefaultPlausibility()
	store.Plausibility = &policy
	r1, _ := validationRecord(t, fx, store, installed, base.Add(24*time.Hour), 1_200_000, readingStateRow{state: "VALID"})
	snapshot := func() string {
		t.Helper()
		var value string
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_meters x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_meter_segments x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_meter_readings x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,reading_id,revision)FROM org_meter_reading_states x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_audit_events x))::text`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	before := snapshot()
	migrator := f.RuntimeRole + "_projection_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE,CREATE ON SCHEMA public TO "+role+";GRANT SELECT,INSERT ON schema_migrations TO "+role+
		";GRANT SELECT,REFERENCES,TRIGGER ON org_meters,org_meter_segments,org_meter_readings TO "+role+";GRANT SELECT,TRIGGER ON org_meter_reading_states TO "+role+";GRANT SELECT,UPDATE ON organizations TO "+role); err != nil {
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
	raw, err := fs.ReadFile(migrations.Files, meterProjectionMigration)
	if err != nil {
		t.Fatal(err)
	}
	broken := fstest.MapFS{meterProjectionMigration: &fstest.MapFile{Data: append(append([]byte{}, raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(t.Context(), location.String(), broken); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
		t.Fatal("injected projection migration failure not reached", err)
	}
	var clean bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('org_meter_current_values') IS NULL AND to_regprocedure('org_meter_current_refresh(uuid,uuid)') IS NULL
 AND NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgname LIKE 'org_meter_current_%') AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version='000252')`).Scan(&clean); err != nil || !clean || before != snapshot() {
		t.Fatal("failed projection migration changed the original schema or data", err)
	}
	current := fstest.MapFS{}
	for path, data := range previous {
		current[path] = data
	}
	current[meterProjectionMigration] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 1 || applied[0] != "000252" {
		t.Fatal("ordinary projection migration recovery", applied, err)
	}
	if before != snapshot() {
		t.Fatal("projection upgrade rewrote existing evidence")
	}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 0 {
		t.Fatal("projection migration no-op replay", err)
	}
	if err := migrate.Check(t.Context(), f.Admin, previous); err != nil {
		t.Fatal("previous embedded image incompatible", err)
	}
	if err := f.Admin.QueryRow(t.Context(), `SELECT NOT rolsuper AND NOT rolbypassrls AND(SELECT relrowsecurity AND relforcerowsecurity AND relowner=(SELECT oid FROM pg_roles WHERE rolname=$1) FROM pg_class WHERE oid='org_meter_current_values'::regclass)
 AND NOT EXISTS(SELECT 1 FROM org_meter_current_values) AND(SELECT bool_and(prosecdef) FROM pg_proc WHERE proname IN('org_meter_current_refresh','org_meter_current_source','org_meter_current_rebuild'))
 FROM pg_roles WHERE rolname=$1`, migrator).Scan(&clean); err != nil || !clean {
		t.Fatal("projection not owned by the forced-RLS migrator, filled by the migration or written without definer functions", err)
	}
	rt := pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "GRANT SELECT ON org_meter_current_values TO "+rt+";GRANT EXECUTE ON FUNCTION org_meter_current_rebuild(uuid) TO "+rt); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UsageBetween(t.Context(), fx.scope, asset.AssetID, meter.Odometer, base, base.Add(24*time.Hour)); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("a legacy meter without its projection answered", err)
	}
	if n, err := fleetpg.RebuildMeterProjection(t.Context(), f.Runtime, fx.scope); err != nil || n != 1 {
		t.Fatal("rebuild of the legacy organization", n, err)
	}
	answer, err := store.UsageBetween(t.Context(), fx.scope, asset.AssetID, meter.Odometer, base, base.Add(24*time.Hour))
	if err != nil || !answer.Known() || answer.Usage != 200_000 || answer.End.ReadingID != r1 || answer.Revision != 4 {
		t.Fatalf("legacy usage after the rebuild %+v %v", answer, err)
	}
	r2, _ := validationRecord(t, fx, store, installed, base.Add(48*time.Hour), 1_300_000, readingStateRow{state: "VALID"})
	var maintained bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT reading_id=$2 AND value=1300000 AND revision=6 FROM org_meter_current_values WHERE org_id=$1`, fx.scope.OrgID().String(), r2).Scan(&maintained); err != nil || !maintained {
		t.Fatal("the database did not maintain the rebuilt projection", err)
	}
	if _, err := migrate.Apply(t.Context(), f.AdminDSN, migrations.Files); err != nil {
		t.Fatal("remaining embedded chain after the 252 rehearsal", err)
	}
}
