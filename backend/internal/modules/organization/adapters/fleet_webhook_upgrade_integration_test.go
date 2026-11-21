//go:build integration

package adapters_test

import (
	"io/fs"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
)

func TestFleetWebhookUpgradePreservesEarlierLifecycleAndRecovers(t *testing.T) {
	const filename = "000285_organization_fleet_webhooks.sql"
	files := fstest.MapFS{}
	for _, pattern := range []string{"*.sql", "schema_min_supported"} {
		names, err := fs.Glob(migrations.Files, pattern)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range names {
			if name != "schema_min_supported" && name >= filename {
				continue
			}
			raw, err := fs.ReadFile(migrations.Files, name)
			if err != nil {
				t.Fatal(err)
			}
			files[name] = &fstest.MapFile{Data: raw}
		}
	}
	f := orgtest.NewWithMigrations(t, files)
	_, _, onboarding, actor, flags := onboardingOnFixture(t, f)
	fx := legalFixtureFor(t, f, onboarding, actor, flags)
	assets := lifecycleStore(t, fx)
	if _, _, err := fx.onboarding.Repository.ActivateOrganization(t.Context(), fx.scope, fx.actor, fx.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	var unit string
	org := fx.scope.OrgID().String()
	if err := f.Admin.QueryRow(t.Context(), `SELECT id::text FROM org_units WHERE org_id=$1`, org).Scan(&unit); err != nil {
		t.Fatal(err)
	}
	asset, _ := lifecycleAssetIn(t, fx, assets, unit, fleet.Inactive)
	read := func() string {
		t.Helper()
		var state string
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array(to_jsonb(a),to_jsonb(l))::text
FROM org_assets a JOIN org_asset_lifecycle_events l ON (l.org_id,l.asset_id)=(a.org_id,a.id)
WHERE a.org_id=$1 AND a.id=$2 AND l.command='DEACTIVATE'`, org, asset).Scan(&state); err != nil {
			t.Fatal(err)
		}
		return state
	}
	before := read()
	raw, err := fs.ReadFile(migrations.Files, filename)
	if err != nil {
		t.Fatal(err)
	}
	files[filename] = &fstest.MapFile{Data: append(slices.Clone(raw), []byte("\nSELECT 1/0;\n")...)}
	if _, err := migrate.Apply(t.Context(), f.AdminDSN, files); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
		t.Fatalf("migration did not reach the injected failure: %v", err)
	}
	var clean bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('public.org_webhook_deliveries') IS NULL
AND to_regclass('public.org_webhook_endpoints') IS NULL`).Scan(&clean); err != nil || !clean || read() != before {
		t.Fatalf("failed migration altered the prior lifecycle: %v", err)
	}
	files[filename] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(t.Context(), f.AdminDSN, files); err != nil || !slices.Equal(applied, []string{"000285"}) || read() != before {
		t.Fatalf("webhook upgrade altered the prior lifecycle: %v / %v", applied, err)
	}
	if err := f.Admin.QueryRow(t.Context(), `SELECT NOT EXISTS (SELECT 1 FROM org_webhook_deliveries)`).Scan(&clean); err != nil || !clean {
		t.Fatalf("upgrade scheduled unreviewed historical deliveries: %v", err)
	}
}
