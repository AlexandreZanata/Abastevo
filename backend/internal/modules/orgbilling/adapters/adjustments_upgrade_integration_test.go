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

func TestAdjustmentMigrationRecoversWithExistingSubscription(t *testing.T) {
	checkBillingUpgradePreservesHistory(t, "000283_organization_billing_adjustments.sql",
		`to_regclass('public.org_billing_adjustment_postings') IS NULL
 AND to_regclass('public.org_billing_adjustment_approvals') IS NULL`)
}

func TestOpeningProofMigrationRecoversWithExistingSubscription(t *testing.T) {
	checkBillingUpgradePreservesHistory(t, "000296_organization_subscription_opening_proof.sql",
		`to_regprocedure('public.org_billing_require_opening_proof()') IS NULL`)
}

func TestReversalLocatorMigrationRecoversWithExistingSubscription(t *testing.T) {
	checkBillingUpgradePreservesHistory(t, "000295_organization_billing_reversal_lookup.sql",
		`NOT EXISTS (SELECT 1 FROM pg_policies WHERE policyname='org_billing_receipt_by_canonical_charge')`)
}

func TestSuspensionReviewMigrationRecoversWithExistingSubscription(t *testing.T) {
	checkBillingUpgradePreservesHistory(t, "000297_organization_subscription_suspension_approvals.sql",
		`to_regclass('public.org_billing_suspension_approvals') IS NULL`)
}

func checkBillingUpgradePreservesHistory(t *testing.T, filename, missingObject string) {
	t.Helper()
	version, _, _ := strings.Cut(filename, "_")
	const opening = "000296_organization_subscription_opening_proof.sql"
	fixtureHead := min(filename, opening)
	files := fstest.MapFS{}
	for _, pattern := range []string{"*.sql", "schema_min_supported"} {
		names, err := fs.Glob(migrations.Files, pattern)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range names {
			if name != "schema_min_supported" && name >= fixtureHead {
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
	ctx := t.Context()
	org := f.Organizations[0]
	if _, err := f.Admin.Exec(ctx, `INSERT INTO enterprise_plan_catalog(product_code,plan_code,revision,display_name,capabilities)
VALUES('fleet','upgrade-fixture',1,'Synthetic existing plan',ARRAY['organizations'])`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Admin.Exec(ctx, `INSERT INTO org_billing_subscription_events
(org_id,subscription_id,revision,product_code,plan_code,plan_revision,from_state,state,prior_state,event_kind,event_source,
 evidence_reference,occurred_at,term_start,term_end)
VALUES($1,$2,1,'fleet','upgrade-fixture',1,'','ACTIVE','','CONFIRM_SETTLEMENT','VERIFIED_BILLING',
 'synthetic-prior-term','2026-10-01T00:00Z','2026-10-01T00:00Z','2026-11-01T00:00Z')`, org.ID, org.RecordID); err != nil {
		t.Fatal(err)
	}
	read := func() string {
		t.Helper()
		var previous string
		if err := f.Admin.QueryRow(ctx, `SELECT to_jsonb(s)::text FROM org_billing_subscription_events s
 WHERE (org_id,subscription_id)=($1,$2)`, org.ID, org.RecordID).Scan(&previous); err != nil {
			t.Fatal(err)
		}
		return previous
	}
	prior := read()
	if filename > opening {
		source, err := fs.ReadFile(migrations.Files, opening)
		if err != nil {
			t.Fatal(err)
		}
		files[opening] = &fstest.MapFile{Data: source}
		if _, err := migrate.Apply(ctx, f.AdminDSN, files); err != nil || read() != prior {
			t.Fatalf("installation of opening proof changed legacy history: %v", err)
		}
	}
	raw, err := fs.ReadFile(migrations.Files, filename)
	if err != nil {
		t.Fatal(err)
	}
	files[filename] = &fstest.MapFile{Data: append(slices.Clone(raw), []byte("\nSELECT 1/0;\n")...)}
	if _, err := migrate.Apply(ctx, f.AdminDSN, files); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
		t.Fatalf("injected migration failure did not occur: %v", err)
	}
	var absent bool
	if err := f.Admin.QueryRow(ctx, "SELECT "+missingObject).Scan(&absent); err != nil || !absent || read() != prior {
		t.Fatalf("failed migration changed the prior image: %v", err)
	}
	files[filename] = &fstest.MapFile{Data: raw}
	if installed, err := migrate.Apply(ctx, f.AdminDSN, files); err != nil || !slices.Equal(installed, []string{version}) || read() != prior {
		t.Fatalf("migration recovery changed prior billing state: %v / %v", installed, err)
	}
}
