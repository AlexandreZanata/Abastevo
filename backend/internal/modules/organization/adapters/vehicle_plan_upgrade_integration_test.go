//go:build integration

package adapters_test

import (
	"bytes"
	"io/fs"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
)

func TestVehiclePlanUpgradeRecoversAndPreservesExistingFuelingReceipt(t *testing.T) {
	const selection = "000281_organization_vehicle_plan_selection.sql"
	const offline = "000282_organization_vehicle_plan_offline.sql"
	prior := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range entries {
		name := item.Name()
		if item.IsDir() || name != "schema_min_supported" && (name >= selection || !strings.HasSuffix(name, ".sql")) {
			continue
		}
		data, err := fs.ReadFile(migrations.Files, name)
		if err != nil {
			t.Fatal(err)
		}
		prior[name] = &fstest.MapFile{Data: data}
	}
	f := orgtest.NewWithMigrations(t, prior)
	_, _, onboarding, actor, flags := onboardingOnFixture(t, f)
	x := fuelingFixtureOn(t, selfIntakeFixtureOn(t, syncFixtureOn(t, devicesFixtureOn(t, legalFixtureFor(t, f, onboarding, actor, flags)))), nil)
	op := x.op(t, x.syncFixtureState, x.declaredNow(t), nil)
	ack, err := x.process(t, x.syncFixtureState, op)
	if err != nil || ack.Result() != protocol.Accepted {
		t.Fatalf("prior image did not retain the exact signed fueling: %v", err)
	}

	selectionSQL, err := fs.ReadFile(migrations.Files, selection)
	if err != nil {
		t.Fatal(err)
	}
	offlineSQL, err := fs.ReadFile(migrations.Files, offline)
	if err != nil {
		t.Fatal(err)
	}
	prior[selection] = &fstest.MapFile{Data: selectionSQL}
	prior[offline] = &fstest.MapFile{Data: append(slices.Clone(offlineSQL), []byte("\nSELECT 1/0;\n")...)}
	if _, err := migrate.Apply(t.Context(), f.AdminDSN, prior); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
		t.Fatalf("forced offline migration failure did not roll back: %v", err)
	}
	var clean bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('public.org_billing_vehicle_plan_selections') IS NOT NULL
 AND to_regprocedure('public.org_sync_vehicle_plan_archive_at(uuid,uuid,timestamptz,timestamptz)') IS NULL
 AND NOT EXISTS (SELECT 1 FROM information_schema.columns
  WHERE table_name='org_sync_state_intakes' AND column_name='vehicle_plan_selection_id')`).Scan(&clean); err != nil || !clean {
		t.Fatalf("failed migration left partial offline schema: %v", err)
	}
	prior[offline] = &fstest.MapFile{Data: offlineSQL}
	if applied, err := migrate.Apply(t.Context(), f.AdminDSN, prior); err != nil || !slices.Equal(applied, []string{"000282"}) {
		t.Fatalf("vehicle plan schema recovery failed: %v / %v", applied, err)
	}
	retry, err := x.process(t, x.syncFixtureState, op)
	if err != nil || !bytes.Equal(retry.CanonicalBytes(), ack.CanonicalBytes()) {
		t.Fatalf("vehicle plan migration changed the retained receipt: %v", err)
	}
	x.accepted(t, op, retry, nil, 0, 1)
}
