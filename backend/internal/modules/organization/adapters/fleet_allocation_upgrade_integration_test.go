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
	fa "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

// The one file-name constant of EMP-FLEET-005; the coordinator may renumber it.
const allocationMigration = "000206_organization_asset_allocation_history.sql"

// Populated previous head → 206 (B-BR-EMP-225): a failed attempt and a refused
// inconsistent source change nothing; recovery moves every activated asset's
// single unit into exactly one period at its recorded activation instant,
// invents nothing for drafts or cost centres, replays as a no-op, keeps the
// previous image compatible and installs the new guards.
func TestFleetAllocationPopulatedUpgradeMovesTheSingleUnitExactly(t *testing.T) {
	version := strings.SplitN(allocationMigration, "_", 2)[0]
	previous := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() != "schema_min_supported" && (entry.Name() >= allocationMigration || !strings.HasSuffix(entry.Name(), ".sql")) {
			continue
		}
		raw, err := fs.ReadFile(migrations.Files, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		previous[entry.Name()] = &fstest.MapFile{Data: raw}
	}
	if _, ok := previous["000201_organization_asset_lifecycle.sql"]; !ok {
		t.Fatal("previous image lacks the lifecycle schema")
	}
	f := orgtest.NewWithMigrations(t, previous)
	_, _, onboarding, actor, flags := onboardingOnFixture(t, f)
	fx := legalFixtureFor(t, f, onboarding, actor, flags)
	if _, _, err := fx.onboarding.Repository.ActivateOrganization(t.Context(), fx.scope, fx.actor, fx.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	org := fx.scope.OrgID().String()
	s := lifecycleStore(t, fx)
	field, old, spare := allocationUnit(t, fx, "FIELD"), allocationUnit(t, fx, "OLD"), allocationUnit(t, fx, "SPARE")
	draftWithUnit, _ := lifecycleAssetIn(t, fx, s, field, fleet.Draft)
	draftWithout, _ := lifecycleAssetIn(t, fx, s, "", fleet.Draft)
	activeInput := fleetCreateInput()
	activeInput.UnitID = field
	created, _, err := fleetWrite(t, fx, s, fleetCommand(t, activeInput))
	if err != nil {
		t.Fatal(err)
	}
	active := created.AssetID
	activation := fleetCommand(t, fa.AssetWriteInput{Kind: fa.ActivateAsset, AssetID: active, ExpectedRevision: 1})
	activated, _, err := fleetWrite(t, fx, s, activation)
	if err != nil {
		t.Fatal("schema-previous activation", err)
	}
	inactive, _ := lifecycleAssetIn(t, fx, s, old, fleet.Inactive)
	archived, _ := lifecycleAssetIn(t, fx, s, field, fleet.Archived)
	// The out-of-service asset's unit is archived before the upgrade (allowed:
	// only ACTIVE assets count), so the move must not re-admit it.
	if err := allocationExec(t, fx, `UPDATE org_units SET state='ARCHIVED',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, old); err != nil {
		t.Fatal(err)
	}
	snapshot := func() string {
		t.Helper()
		var value string
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array(
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_assets x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,asset_id,started_at) FROM org_asset_unit_allocations x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,operation_id) FROM org_asset_commands x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,asset_id,revision) FROM org_asset_revisions x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,asset_id,revision) FROM org_asset_lifecycle_events x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,operation_id) FROM org_audit_events x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_units x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_cost_centres x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id) FROM organizations x))::text`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	functions := func() string {
		t.Helper()
		var out string
		if err := f.Admin.QueryRow(t.Context(), `SELECT string_agg(pg_get_functiondef(oid),E'\n' ORDER BY proname) FROM pg_proc WHERE oid=ANY(ARRAY['org_asset_current_usage(uuid,uuid)'::regprocedure,'org_unit_current_usage(uuid,uuid)'::regprocedure,'org_cost_centre_current_usage(uuid,uuid)'::regprocedure])`).Scan(&out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	var extensionBefore bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname='btree_gist')`).Scan(&extensionBefore); err != nil {
		t.Fatal(err)
	}
	before, beforeFunctions := snapshot(), functions()
	migrator := f.RuntimeRole + "_allocation_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	var dbName string
	if err := f.Admin.QueryRow(t.Context(), `SELECT current_database()`).Scan(&dbName); err != nil {
		t.Fatal(err)
	}
	// An ordinary migrator owns exactly what 206 alters, replaces or reads
	// across organizations and only references the other parents; it is
	// neither superuser nor BYPASSRLS. CREATE on the database admits the
	// trusted btree_gist extension, as for the production database owner.
	if _, err := f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE,CREATE ON SCHEMA public TO "+role+";GRANT CREATE ON DATABASE "+pgx.Identifier{dbName}.Sanitize()+" TO "+role+
		";GRANT SELECT,INSERT ON schema_migrations TO "+role+";GRANT REFERENCES ON org_units,org_cost_centres TO "+role+
		";ALTER TABLE org_assets OWNER TO "+role+";ALTER TABLE org_asset_unit_allocations OWNER TO "+role+
		";ALTER FUNCTION org_asset_current_usage(uuid,uuid) OWNER TO "+role+";ALTER FUNCTION org_unit_current_usage(uuid,uuid) OWNER TO "+role+";ALTER FUNCTION org_cost_centre_current_usage(uuid,uuid) OWNER TO "+role); err != nil {
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
	raw, err := fs.ReadFile(migrations.Files, allocationMigration)
	if err != nil {
		t.Fatal("allocation migration missing", err)
	}
	clean := func(label string) {
		t.Helper()
		var ok bool
		if err := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('org_asset_unit_periods') IS NULL AND to_regclass('org_asset_cost_centre_periods') IS NULL
 AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)
 AND EXISTS(SELECT 1 FROM pg_extension WHERE extname='btree_gist')=$2
 AND(SELECT bool_and(relrowsecurity AND relforcerowsecurity) FROM pg_class WHERE oid IN('org_assets'::regclass,'org_asset_unit_allocations'::regclass))
 AND NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='org_asset_initial_unit_period')`, version, extensionBefore).Scan(&ok); err != nil || !ok || before != snapshot() || beforeFunctions != functions() {
			t.Fatal(label, "changed the previous schema, guards, forced RLS or data", err)
		}
	}
	broken := fstest.MapFS{allocationMigration: &fstest.MapFile{Data: append(append([]byte{}, raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(t.Context(), location.String(), broken); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
		t.Fatal("injected allocation migration failure not reached", err)
	}
	clean("failed migration")
	current := fstest.MapFS{}
	for key, data := range previous {
		current[key] = data
	}
	current[allocationMigration] = &fstest.MapFile{Data: raw}
	// Refusal: an activated asset whose unit column no longer equals its
	// initial allocation (written around every guard by the disposable admin)
	// is never guessed; the whole migration fails unchanged.
	divert := func(unit string) {
		t.Helper()
		err := pgx.BeginTxFunc(t.Context(), f.Admin, pgx.TxOptions{}, func(tx pgx.Tx) error {
			if _, err := tx.Exec(t.Context(), `SET LOCAL session_replication_role=replica`); err != nil {
				return err
			}
			_, err := tx.Exec(t.Context(), `UPDATE org_assets SET unit_id=$3 WHERE org_id=$1 AND id=$2`, org, active, unit)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	divert(spare)
	diverted := snapshot()
	if _, err := migrate.Apply(t.Context(), location.String(), current); err == nil || !strings.Contains(err.Error(), "SQLSTATE 23514") {
		t.Fatal("inconsistent allocation source not refused", err)
	}
	before = diverted
	clean("refused migration")
	divert(field)
	before = snapshot()
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 1 || applied[0] != version {
		t.Fatal("ordinary allocation migration recovery", err)
	}
	if before != snapshot() {
		t.Fatal("allocation migration rewrote existing assets, allocations, receipts, history or structures")
	}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 0 {
		t.Fatal("allocation migration no-op replay", err)
	}
	if err := migrate.Check(t.Context(), f.Admin, previous); err != nil {
		t.Fatal("previous embedded image incompatible", err)
	}
	// Exactly one open period per activated asset, at its recorded activation
	// instant; none for drafts; no cost centre; forced RLS and validation.
	var exact bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_asset_unit_periods)=3
 AND NOT EXISTS(SELECT 1 FROM org_asset_cost_centre_periods)
 AND NOT EXISTS(SELECT 1 FROM org_asset_unit_periods WHERE asset_id IN($2,$3))
 AND(SELECT bool_and(p.unit_id=a.unit_id AND a.unit_id=x.unit_id AND p.valid_from=x.started_at AND p.recorded_at=x.started_at AND p.valid_to IS NULL)
  FROM org_asset_unit_periods p JOIN org_assets a ON(a.org_id,a.id)=(p.org_id,p.asset_id) JOIN org_asset_unit_allocations x ON(x.org_id,x.asset_id)=(p.org_id,p.asset_id))
 AND(SELECT array_agg(asset_id ORDER BY asset_id) FROM org_asset_unit_periods)=(SELECT array_agg(x ORDER BY x) FROM unnest(ARRAY[$4::uuid,$5::uuid,$6::uuid]) x)
 AND NOT EXISTS(SELECT 1 FROM pg_constraint WHERE connamespace='public'::regnamespace AND NOT convalidated)
 AND(SELECT bool_and(relrowsecurity AND relforcerowsecurity AND relowner=(SELECT oid FROM pg_roles WHERE rolname=$1)) FROM pg_class WHERE oid IN('org_asset_unit_periods'::regclass,'org_asset_cost_centre_periods'::regclass,'org_assets'::regclass,'org_asset_unit_allocations'::regclass))
 AND EXISTS(SELECT 1 FROM pg_extension WHERE extname='btree_gist')`, migrator, draftWithUnit, draftWithout, active, inactive, archived).Scan(&exact); err != nil || !exact {
		t.Fatal("allocation move invented, lost or misdated data, or weakened RLS", err)
	}
	f.GrantFleetUsage(t)
	f.GrantMeterUsage(t)
	s = lifecycleStore(t, fx)
	grantCostCentreTables(t, fx)
	runtime := pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "GRANT UPDATE,DELETE ON org_asset_unit_periods,org_asset_cost_centre_periods TO "+runtime); err != nil {
		t.Fatal(err)
	}
	// Original receipts replay; the moved periods answer the port exactly.
	if replay, replayed, err := fleetWrite(t, fx, s, activation); err != nil || !replayed || replay != activated {
		t.Fatal("pre-206 activation receipt changed", err)
	}
	for asset, unit := range map[string]string{active: field, inactive: old, archived: field} {
		start := allocationStart(t, fx, asset)
		if got, err := allocationAt(t, fx, s, asset, start); err != nil || got.UnitID != unit || got.CostCentreID != "" {
			t.Fatal("moved period answer", asset, err)
		}
		if _, err := allocationAt(t, fx, s, asset, start.Add(-time.Microsecond)); !errors.Is(err, fa.ErrUnallocated) {
			t.Fatal("moved period started before activation", asset, err)
		}
	}
	if _, err := allocationAt(t, fx, s, draftWithUnit, lifecycleDBNow(t, fx)); !errors.Is(err, fa.ErrUnallocated) {
		t.Fatal("draft proposal answered as an allocation", err)
	}
	// Previous image: its exact activation effects still open the first period.
	previousActivation := pre201Activate(t, fx, fleetCommand(t, fa.AssetWriteInput{Kind: fa.ActivateAsset, AssetID: draftWithUnit, ExpectedRevision: 1}), field)
	if previousActivation.State != fleet.Active {
		t.Fatal("previous image activation")
	}
	if rows := allocationRows(t, fx, "org_asset_unit_periods", "unit_id", draftWithUnit); len(rows) != 1 || rows[0].target != field || !rows[0].from.Equal(allocationStart(t, fx, draftWithUnit)) || rows[0].to != nil {
		t.Fatal("previous image activation did not open the first period", rows)
	}
	// Installed guards on the upgraded schema.
	allocationWant(t, "upgraded delete", allocationExec(t, fx, `DELETE FROM org_asset_unit_periods WHERE org_id=$1 AND asset_id=$2`, org, active), "55000")
	allocationWant(t, "upgraded close only period", allocationExec(t, fx, `UPDATE org_asset_unit_periods SET valid_to=clock_timestamp() WHERE org_id=$1 AND asset_id=$2`, org, active), "23514")
	allocationWant(t, "upgraded overlap", allocationExec(t, fx, `INSERT INTO org_asset_unit_periods(org_id,asset_id,unit_id,valid_from) VALUES($1,$2,$3,clock_timestamp())`, org, active, spare), "23P01")
	allocationWant(t, "upgraded archived unit", allocationReallocate(t, fx, "org_asset_unit_periods", "unit_id", inactive, old, lifecycleDBNow(t, fx).Truncate(time.Microsecond)), "23503")
	allocationWant(t, "upgraded unit archive guard", allocationExec(t, fx, `UPDATE org_units SET state='ARCHIVED',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, field), "P7703")
	if err := pgx.BeginTxFunc(t.Context(), f.Runtime, pgx.TxOptions{}, func(tx pgx.Tx) error {
		var n int
		if err := tx.QueryRow(t.Context(), `SELECT count(*) FROM org_asset_unit_periods`).Scan(&n); err != nil || n != 0 {
			t.Fatal("moved periods visible without organization setting", n, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
