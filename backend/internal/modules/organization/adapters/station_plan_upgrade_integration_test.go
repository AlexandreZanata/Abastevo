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

func TestStationPlanUpgradePreservesEarlierStationAuthorityAndRecovers(t *testing.T) {
	const filename = "000284_organization_station_plan_meter.sql"
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
	station := agreementStationForOperator(t, f.Admin, fx.actor.AccountID, "EXEMPL01000170", "price.publish")
	read := func() string {
		t.Helper()
		var view string
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array(to_jsonb(g),to_jsonb(o),to_jsonb(s))::text
FROM representation_grants g JOIN station_operator_revisions o ON o.station_id=g.station_id AND o.valid_to IS NULL
JOIN directory_stations s ON s.id=g.station_id
WHERE g.station_id=$1 AND g.status='active'`, station).Scan(&view); err != nil {
			t.Fatal(err)
		}
		return view
	}
	before := read()
	raw, err := fs.ReadFile(migrations.Files, filename)
	if err != nil {
		t.Fatal(err)
	}
	files[filename] = &fstest.MapFile{Data: append(slices.Clone(raw), []byte("\nSELECT 1/0;\n")...)}
	if _, err := migrate.Apply(t.Context(), f.AdminDSN, files); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
		t.Fatalf("forced failure did not occur after the migration: %v", err)
	}
	var clean bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('public.org_billing_station_plan_reviews') IS NULL
 AND to_regclass('public.org_billing_station_plan_admissions') IS NULL`).Scan(&clean); err != nil || !clean || read() != before {
		t.Fatalf("failed migration changed the station's previous proof: %v", err)
	}
	files[filename] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(t.Context(), f.AdminDSN, files); err != nil || !slices.Equal(applied, []string{"000284"}) || read() != before {
		t.Fatalf("station meter recovery changed the earlier authority: %v / %v", applied, err)
	}
	var implicit bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT EXISTS (SELECT 1 FROM org_billing_station_plan_admissions)`).Scan(&implicit); err != nil || implicit {
		t.Fatalf("migration turned a pre-existing station grant into a company seat: %v", err)
	}
}
