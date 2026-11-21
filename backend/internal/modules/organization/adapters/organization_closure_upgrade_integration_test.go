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

func TestOrganizationClosureUpgradePreservesArchivedMembershipAndRecovers(t *testing.T) {
	const filename = "000286_organization_closure.sql"
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
	if _, _, err := fx.onboarding.Repository.ActivateOrganization(t.Context(), fx.scope, fx.actor, fx.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	org := fx.scope.OrgID().String()
	if err := unitSchemaExec(t, fx, `UPDATE organizations SET state='ARCHIVED',revision=revision+1 WHERE org_id=$1`, org); err != nil {
		t.Fatal(err)
	}
	read := func() string {
		t.Helper()
		var state string
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array(to_jsonb(o),to_jsonb(m))::text
FROM organizations o JOIN org_memberships m ON m.org_id=o.org_id
WHERE (o.org_id,m.id)=($1,$2)`, org, fx.member).Scan(&state); err != nil {
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
	if err := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('public.org_closure_requests') IS NULL
AND to_regclass('public.org_lifecycle_receipts') IS NULL`).Scan(&clean); err != nil || !clean || read() != before {
		t.Fatalf("failed migration altered the archived organization or membership: %v", err)
	}
	files[filename] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(t.Context(), f.AdminDSN, files); err != nil || !slices.Equal(applied, []string{"000286"}) || read() != before {
		t.Fatalf("closure upgrade altered the archived organization or membership: %v / %v", applied, err)
	}
	if err := f.Admin.QueryRow(t.Context(), `SELECT NOT EXISTS (SELECT 1 FROM org_closure_requests)`).Scan(&clean); err != nil || !clean {
		t.Fatalf("upgrade scheduled unrequested closures: %v", err)
	}
}
