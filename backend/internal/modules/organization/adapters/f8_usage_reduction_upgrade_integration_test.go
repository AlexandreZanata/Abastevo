//go:build integration

package adapters_test

import (
	"io/fs"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
)

func TestF8AssetUsageAndClosureReductionUpgradeRecoverWithExistingFleet(t *testing.T) {
	const usageFile = "000287_organization_held_fueling_asset_usage.sql"
	const reductionFile = "000288_organization_closure_reduction_scope.sql"
	files := fstest.MapFS{}
	names, err := fs.Glob(migrations.Files, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range append(names, "schema_min_supported") {
		if name != "schema_min_supported" && name >= usageFile {
			continue
		}
		raw, err := fs.ReadFile(migrations.Files, name)
		if err != nil {
			t.Fatal(err)
		}
		files[name] = &fstest.MapFile{Data: raw}
	}
	f := orgtest.NewWithMigrations(t, files)
	x := tripStateOn(t, f)
	ctx, org := t.Context(), x.scope.OrgID().String()
	asset := x.vehicle.AssetID
	rows := func() string {
		t.Helper()
		var value string
		if err := f.Admin.QueryRow(ctx, `SELECT jsonb_build_array(to_jsonb(o),to_jsonb(a),to_jsonb(m))::text
FROM organizations o JOIN org_assets a USING(org_id) JOIN org_memberships m USING(org_id)
WHERE (o.org_id,a.id,m.id)=($1,$2,$3)`, org, asset, x.member).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	definition := func(signature string) string {
		t.Helper()
		var value string
		if err := f.Admin.QueryRow(ctx, `SELECT pg_get_functiondef($1::regprocedure)`, signature).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	before := rows()
	for _, step := range []struct{ filename, version, function, witness string }{
		{usageFile, "000287", "org_asset_current_usage(uuid,uuid)", "org_held_fuelings_org_id_asset_id_fkey"},
		{reductionFile, "000288", "org_membership_access_policy(uuid,uuid,text)", "IF root_state='CLOSED' THEN"},
	} {
		original := definition(step.function)
		raw, err := fs.ReadFile(migrations.Files, step.filename)
		if err != nil {
			t.Fatal(err)
		}
		files[step.filename] = &fstest.MapFile{Data: append(slices.Clone(raw), []byte("\nSELECT 1/0;\n")...)}
		if _, err := migrate.Apply(ctx, f.AdminDSN, files); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
			t.Fatalf("%s did not reach the injected failure: %v", step.version, err)
		}
		if rows() != before || definition(step.function) != original {
			t.Fatalf("%s changed populated rows or its installed function after rollback", step.version)
		}
		files[step.filename] = &fstest.MapFile{Data: raw}
		if applied, err := migrate.Apply(ctx, f.AdminDSN, files); err != nil || !slices.Equal(applied, []string{step.version}) {
			t.Fatalf("%s did not recover: %v / %v", step.version, applied, err)
		}
		if rows() != before || !strings.Contains(definition(step.function), step.witness) {
			t.Fatalf("%s changed existing rows or missed its scoped function", step.version)
		}
	}
	if applied, err := migrate.Apply(ctx, f.AdminDSN, files); err != nil || len(applied) != 0 {
		t.Fatalf("migration retry reapplied work: %v / %v", applied, err)
	}
}
