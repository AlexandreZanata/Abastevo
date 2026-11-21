//go:build integration

package adapters_test

import (
	"errors"
	"io/fs"
	"maps"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestDataSourceReviewOrderMigrationRecoversWithCatalogIntact(t *testing.T) {
	const migration = "000291_enterprise_data_source_review_order.sql"
	version, _, _ := strings.Cut(migration, "_")
	entries, err := fs.ReadDir(dbmigrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	previous := fstest.MapFS{}
	for _, entry := range entries {
		if entry.Name() != "schema_min_supported" && entry.Name() >= migration {
			continue
		}
		raw, err := fs.ReadFile(dbmigrations.Files, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		previous[entry.Name()] = &fstest.MapFile{Data: raw}
	}
	f := orgtest.NewWithMigrations(t, previous)
	ctx := t.Context()
	if _, err := f.Admin.Exec(ctx, `INSERT INTO enterprise_data_source_reviews
(source_code,source_digest,revision,status,decision_ref,license_ref,attribution)
VALUES ('S-ESP-01','sha256:'||repeat('a',64),1,'APPROVED','synthetic-registry','synthetic-license','Synthetic registry'),
       ('S-ESP-02','sha256:'||repeat('b',64),1,'APPROVED','synthetic-price','synthetic-license','Synthetic survey')`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Admin.Exec(ctx, `INSERT INTO enterprise_regional_datasets
(dataset_key,revision,display_name,state_code,minimum_stations,method_version,snapshot_digest,official_revision,
 registry_digest,registry_review,price_digest,price_review)
VALUES ('previous-regional',1,'Previous synthetic catalog','MT',5,'abastevo-metrics-v1','sha256:'||repeat('c',64),$1,
 'sha256:'||repeat('a',64),1,'sha256:'||repeat('b',64),1)`, f.Organizations[0].RecordID); err != nil {
		t.Fatal(err)
	}
	snapshot := func() string {
		t.Helper()
		var value string
		if err := f.Admin.QueryRow(ctx, `SELECT jsonb_build_array(
 (SELECT jsonb_agg(to_jsonb(r) ORDER BY source_code,source_digest,revision) FROM enterprise_data_source_reviews r),
 (SELECT jsonb_agg(to_jsonb(d) ORDER BY dataset_key,revision) FROM enterprise_regional_datasets d))::text`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	before := snapshot()
	raw, err := fs.ReadFile(dbmigrations.Files, migration)
	if err != nil {
		t.Fatal(err)
	}
	broken := maps.Clone(previous)
	broken[migration] = &fstest.MapFile{Data: append(slices.Clone(raw), "\nSELECT 1/0;\n"...)}
	_, err = migrate.Apply(ctx, f.AdminDSN, broken)
	var failure *pgconn.PgError
	if !errors.As(err, &failure) || failure.Code != "22012" {
		t.Fatalf("expected injected migration failure: %v", err)
	}
	var clean bool
	if err := f.Admin.QueryRow(ctx, `SELECT to_regprocedure('enterprise_data_order_source_review()') IS NULL
 AND NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname='enterprise_data_source_review_order')
 AND NOT EXISTS (SELECT 1 FROM schema_migrations WHERE version=$1)`, version).Scan(&clean); err != nil || !clean || snapshot() != before {
		t.Fatalf("failed trigger upgrade changed catalog, schema or ledger: %v", err)
	}
	current := maps.Clone(previous)
	current[migration] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(ctx, f.AdminDSN, current); err != nil || !slices.Equal(applied, []string{version}) {
		t.Fatalf("retry did not apply source review ordering: %v / %v", applied, err)
	}
	if applied, err := migrate.Apply(ctx, f.AdminDSN, current); err != nil || len(applied) != 0 || snapshot() != before {
		t.Fatalf("retry changed preexisting source decisions or catalog entries: %v / %v", applied, err)
	}
	role, secret := f.ProvisionScriptRole(t)
	runtime := f.ScriptRuntimePool(t, role, secret, 1)
	if err := database.WithOrgBootstrapTx(ctx, runtime, func(tx pgx.Tx) error {
		catalog, err := adapters.OpenCatalog(ctx, tx)
		if err != nil {
			return err
		}
		_, err = catalog.ReadAdmitted(ctx, "previous-regional", 1)
		return err
	}); err != nil {
		t.Fatalf("ordinary runtime could not read the prior source decision: %v", err)
	}
}
