//go:build integration

package adapters_test

import (
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"

	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/domain"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestCatalogUpgradePreservesPilotStateAndKeepsPlanRevisionsImmutable(t *testing.T) {
	const migration = "000263_organization_plan_catalog.sql"
	ctx := t.Context()
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	old := fstest.MapFS{}
	for _, entry := range entries {
		if entry.Name() != "schema_min_supported" && entry.Name() >= migration {
			continue
		}
		bytes, err := fs.ReadFile(migrations.Files, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		old[entry.Name()] = &fstest.MapFile{Data: bytes}
	}
	fixture := orgtest.NewWithMigrations(t, old)
	org := fixture.Organizations[0].ID
	if _, err := fixture.Admin.Exec(ctx, `INSERT INTO org_capabilities(org_id, capability, enabled, revision)
VALUES($1, 'organizations', true, 1)`, org); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if _, err := migrate.Apply(ctx, fixture.AdminDSN, migrations.Files); err != nil {
			t.Fatalf("catalog migration attempt %d: %v", attempt+1, err)
		}
	}
	var enabled bool
	var revision int64
	if err := fixture.Admin.QueryRow(ctx, `SELECT enabled, revision FROM org_capabilities
WHERE org_id=$1 AND capability='organizations'`, org).Scan(&enabled, &revision); err != nil || !enabled || revision != 1 {
		t.Fatalf("pilot entitlement changed: enabled=%v revision=%d err=%v", enabled, revision, err)
	}
	var count int
	if err := fixture.Admin.QueryRow(ctx, "SELECT count(*) FROM enterprise_plan_catalog").Scan(&count); err != nil || count != 0 {
		t.Fatalf("migration installed unapproved plans: count=%d err=%v", count, err)
	}

	// Only the later billing composition will grant the runtime a catalog read.
	// This task exercises the adapter through a read-only login.
	role := pgx.Identifier{fixture.RuntimeRole}.Sanitize()
	if _, err := fixture.Admin.Exec(ctx, "GRANT SELECT ON enterprise_plan_catalog TO "+role); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.Admin.Exec(ctx, `INSERT INTO enterprise_plan_catalog
(product_code,plan_code,revision,display_name,capabilities,active_vehicles,storage_bytes)
VALUES ('fleet','synthetic',1,'Synthetic I',ARRAY['fleet'],2,0),
       ('fleet','synthetic',2,'Synthetic II',ARRAY['fleet','reports'],4,NULL),
       ('data','synthetic',1,'Synthetic Data',ARRAY['unregistered'],NULL,NULL)`); err != nil {
		t.Fatal(err)
	}
	known, err := capability.NewCatalog([]string{"fleet", "reports"})
	if err != nil {
		t.Fatal(err)
	}
	store := adapters.CatalogStore{DB: fixture.Runtime, Capabilities: known}
	key := domain.PlanKey{Product: "fleet", Plan: "synthetic", Revision: 1}
	before, err := store.GetPlan(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	n, err := before.Limit(domain.ActiveVehicles)
	if err != nil || n != 2 || before.Includes("reports") {
		t.Fatalf("wrong original facts: ceiling=%d err=%v", n, err)
	}
	zero, err := before.Limit(domain.StorageBytes)
	if err != nil || zero != 0 {
		t.Fatalf("lost explicit zero: value=%d err=%v", zero, err)
	}

	key.Revision = 2
	after, err := store.GetPlan(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	n, err = after.Limit(domain.ActiveVehicles)
	if err != nil || n != 4 || !after.Includes("reports") {
		t.Fatalf("wrong revised facts: ceiling=%d err=%v", n, err)
	}
	if _, err := after.Limit(domain.StorageBytes); !errors.Is(err, domain.ErrLimitUndefined) {
		t.Fatalf("absent ceiling was defaulted: %v", err)
	}

	key.Revision = 3
	if _, err := store.GetPlan(ctx, key); !errors.Is(err, domain.ErrPlanNotFound) {
		t.Fatalf("invalid revision was resolved: %v", err)
	}
	key.Product, key.Revision = "data", 1
	if _, err := store.GetPlan(ctx, key); !errors.Is(err, domain.ErrInvalidPlan) {
		t.Fatalf("unregistered stored capability conferred access: %v", err)
	}

	for _, query := range []string{
		"UPDATE enterprise_plan_catalog SET display_name='changed'",
		"DELETE FROM enterprise_plan_catalog",
		"TRUNCATE enterprise_plan_catalog CASCADE",
	} {
		_, err := fixture.Admin.Exec(ctx, query)
		var refusal *pgconn.PgError
		if !errors.As(err, &refusal) || refusal.Code != "55000" {
			t.Fatalf("catalog history mutation was accepted: %v", err)
		}
	}
	_, err = fixture.Runtime.Exec(ctx, `INSERT INTO enterprise_plan_catalog
(product_code,plan_code,revision,display_name) VALUES('fleet','synthetic',3,'Synthetic III')`)
	var refusal *pgconn.PgError
	if !errors.As(err, &refusal) || refusal.Code != "42501" {
		t.Fatalf("catalog read role could publish a plan: %v", err)
	}

	const insertVersion = `INSERT INTO enterprise_plan_catalog
(product_code,plan_code,revision,display_name,active_vehicles) VALUES('fleet','synthetic',3,'Synthetic III',$1)`
	if _, err := fixture.Admin.Exec(ctx, insertVersion, -1); !errors.As(err, &refusal) || refusal.Code != "23514" {
		t.Fatalf("database admitted a negative meter ceiling: %v", err)
	}

	race := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := fixture.Admin.Exec(ctx, insertVersion, 6)
			race <- err
		}()
	}
	var successes, conflicts int
	for range 2 {
		switch err := <-race; {
		case err == nil:
			successes++
		case errors.As(err, &refusal) && refusal.Code == "23505":
			conflicts++
		default:
			t.Fatalf("unexpected catalog publish result: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("catalog revision had %d inserts and %d conflicts", successes, conflicts)
	}

}
