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
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

func TestCostCentrePopulatedOrdinaryUpgradeFailureRecoveryAndHistory(t *testing.T) {
	const name = "000175_cost_centres.sql"
	previous := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != "schema_min_supported" && entry.Name() >= name {
			continue
		}
		raw, err := fs.ReadFile(migrations.Files, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		previous[entry.Name()] = &fstest.MapFile{Data: raw}
	}
	f := orgtest.NewWithMigrations(t, previous)
	_, _, onboarding, actor, flags := onboardingOnFixture(t, f)
	fx := legalFixtureFor(t, f, onboarding, actor, flags)
	if _, _, err := fx.write(t, legalCommand(t, onboardingID(t), false, 0)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Admin.Exec(t.Context(), `INSERT INTO directory_stations(id,display_name) VALUES($1,'Prior synthetic public row')`, onboardingID(t)); err != nil {
		t.Fatal(err)
	}
	snapshot := func() string {
		t.Helper()
		var value string
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array(
 (SELECT jsonb_agg(to_jsonb(o) ORDER BY org_id) FROM organizations o),
 (SELECT jsonb_agg(to_jsonb(u) ORDER BY org_id,id) FROM org_units u),
 (SELECT jsonb_agg(to_jsonb(e) ORDER BY org_id,id) FROM org_establishments e),
 (SELECT jsonb_agg(to_jsonb(l) ORDER BY org_id,id) FROM org_legal_entities l),
 (SELECT jsonb_agg(to_jsonb(a) ORDER BY org_id,operation_id) FROM org_audit_events a),
 (SELECT jsonb_agg(to_jsonb(c) ORDER BY org_id,operation_id) FROM org_onboarding_commands c),
 (SELECT jsonb_agg(to_jsonb(m) ORDER BY org_id,id) FROM org_memberships m),
 (SELECT jsonb_agg(to_jsonb(r) ORDER BY org_id,id) FROM org_role_assignments r),
 (SELECT jsonb_agg(to_jsonb(d) ORDER BY id) FROM directory_stations d))::text`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	before := snapshot()
	migrator := f.RuntimeRole + "_cost_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE; GRANT USAGE,CREATE ON SCHEMA public TO "+role+"; GRANT SELECT,INSERT ON schema_migrations TO "+role+"; GRANT REFERENCES ON organizations,accounts,org_audit_events TO "+role); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := f.Admin.Exec(ctx, "REASSIGN OWNED BY "+role+" TO CURRENT_USER; DROP OWNED BY "+role+"; DROP ROLE "+role); err != nil {
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
	broken := fstest.MapFS{name: &fstest.MapFile{Data: append(append([]byte(nil), raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(t.Context(), location.String(), broken); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
		t.Fatal("did not reach injected cost-centre migration failure", err)
	}
	var clean bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('org_cost_centres') IS NULL AND to_regclass('org_cost_centre_codes') IS NULL AND to_regclass('org_cost_centre_commands') IS NULL AND to_regprocedure('org_cost_centre_current_usage(uuid,uuid)') IS NULL AND to_regprocedure('org_cost_centre_admit(uuid,uuid)') IS NULL AND to_regprocedure('org_cost_centre_guard()') IS NULL AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version='000175')`).Scan(&clean); err != nil || !clean {
		t.Fatal("incomplete cost-centre migration rollback", err)
	}
	if before != snapshot() {
		t.Fatal("failed migration changed prior rows")
	}
	current := fstest.MapFS{}
	for path, data := range previous {
		current[path] = data
	}
	current[name] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 1 || applied[0] != "000175" {
		t.Fatal("cost-centre migration recovery", applied, err)
	}
	if before != snapshot() {
		t.Fatal("upgrade changed prior rows")
	}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 0 {
		t.Fatal("cost-centre migration repeat", applied, err)
	}
	if err := migrate.Check(t.Context(), f.Admin, previous); err != nil {
		t.Fatal("previous schema compatibility failed", err)
	}
	if err := f.Admin.QueryRow(t.Context(), `SELECT NOT rolsuper AND NOT rolbypassrls AND NOT rolcanlogin AND(SELECT count(*)=3 AND bool_and(relrowsecurity AND relforcerowsecurity AND relowner=(SELECT oid FROM pg_roles WHERE rolname=$1)) FROM pg_class WHERE oid IN('org_cost_centres'::regclass,'org_cost_centre_codes'::regclass,'org_cost_centre_commands'::regclass)) FROM pg_roles WHERE rolname=$1`, migrator).Scan(&clean); err != nil || !clean {
		t.Fatal("ordinary cost-centre ownership/forced RLS missing", err)
	}
	grantCostCentreTables(t, fx)
	store := costCentreStore(t, fx)
	create := costCentreCommand(t, app.CostCentreWriteInput{Kind: app.CreateCostCentre, Profile: d.CostCentreProfile{Name: "Synthetic upgraded centre", Code: "OLD"}})
	first, replay, err := store.WriteCostCentre(t.Context(), fx.scope, fx.actor, fx.member, onboardingID(t), create)
	if err != nil || replay || first.Revision != 1 {
		t.Fatal("ordinary post-upgrade create failed", err)
	}
	change := costCentreCommand(t, app.CostCentreWriteInput{Kind: app.ChangeCostCentre, CentreID: first.CentreID, ExpectedRevision: 1, Profile: d.CostCentreProfile{Name: "Synthetic upgraded correction", Code: "NEW"}})
	if _, _, err := store.WriteCostCentre(t.Context(), fx.scope, fx.actor, fx.member, "", change); err != nil {
		t.Fatal("ordinary code correction failed", err)
	}
	centre, err := store.ReadCostCentreCode(t.Context(), fx.scope, fx.actor, fx.member, "old")
	if err != nil {
		t.Fatal(err)
	}
	value, err := centre.Snapshot(fx.scope)
	if err != nil || value.ID != first.CentreID || value.Code != "NEW" {
		t.Fatal("upgraded historical alias lost meaning", err)
	}
}
