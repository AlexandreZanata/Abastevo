//go:build integration

package adapters_test

import (
	"io/fs"
	"maps"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
)

func TestFiscalReceiptMigrationPreservesHistoryAndRollsBackFailedUpgrade(t *testing.T) {
	const migration = "000290_organization_fiscal_consultation_receipts.sql"
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
	role := pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, err := f.Admin.Exec(ctx, "GRANT USAGE ON SCHEMA public TO "+role+"; GRANT SELECT, INSERT ON org_audit_events TO "+role+
		"; GRANT SELECT, INSERT, UPDATE ON org_documents TO "+role); err != nil {
		t.Fatal(err)
	}
	scope := f.Organizations[0].Scope(t)
	legacy := previousDocument(t, f, scope, "RECEIVED", "STORED")
	previousDocument(t, f, f.Organizations[1].Scope(t), "RECEIVED", "STORED")
	advanceLegacy := func(revision int, status string) error {
		return database.WithOrgTx(ctx, f.Runtime, scope, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `UPDATE org_documents SET fiscal_state=$3,fiscal_revision=$4,
 fiscal_consulted_at=statement_timestamp()-interval '1 hour'+make_interval(secs=>$4::bigint)
 WHERE org_id=$1 AND id=$2`, scope.OrgID().String(), legacy, status, revision)
			return err
		})
	}
	if err := advanceLegacy(2, "AUTHORIZED_AS_OF"); err != nil {
		t.Fatalf("previous schema rejected a legacy fiscal observation: %v", err)
	}
	snapshot := func() string {
		t.Helper()
		var result string
		if err := f.Admin.QueryRow(ctx, `SELECT jsonb_agg(to_jsonb(d) ORDER BY org_id,id)::text FROM org_documents d`).Scan(&result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	before := snapshot()
	raw, err := fs.ReadFile(dbmigrations.Files, migration)
	if err != nil {
		t.Fatal(err)
	}
	broken := maps.Clone(previous)
	broken[migration] = &fstest.MapFile{Data: append(slices.Clone(raw), "\nSELECT 1/0;\n"...)}
	if _, err := migrate.Apply(ctx, f.AdminDSN, broken); sqlState(err) != "22012" {
		t.Fatalf("expected the injected upgrade failure: %v", err)
	}
	var rolledBack bool
	if err := f.Admin.QueryRow(ctx, `SELECT to_regclass('public.org_document_fiscal_consultations') IS NULL
 AND to_regprocedure('org_document_require_fiscal_receipt()') IS NULL
 AND NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname='org_documents_require_fiscal_receipt')
 AND NOT EXISTS (SELECT 1 FROM schema_migrations WHERE version=$1)`, version).Scan(&rolledBack); err != nil || !rolledBack || snapshot() != before {
		t.Fatalf("failed fiscal migration changed the schema, ledger or documents: %v", err)
	}
	previous[migration] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(ctx, f.AdminDSN, previous); err != nil || !slices.Equal(applied, []string{version}) {
		t.Fatalf("fiscal upgrade retry: %v / %v", applied, err)
	}
	if applied, err := migrate.Apply(ctx, f.AdminDSN, previous); err != nil || len(applied) != 0 {
		t.Fatalf("fiscal upgrade was not idempotent: %v / %v", applied, err)
	}
	if err := advanceLegacy(3, "CANCELLED_AS_OF"); sqlState(err) != "42501" {
		t.Fatalf("upgraded schema allowed a legacy writer to advance without a receipt: %v", err)
	}
	var receipts int
	if err := f.Admin.QueryRow(ctx, `SELECT count(*) FROM org_document_fiscal_consultations`).Scan(&receipts); err != nil || receipts != 0 || snapshot() != before {
		t.Fatalf("upgrade changed historical documents or invented receipts: %d / %v", receipts, err)
	}

	const dataReview = "000291_enterprise_data_source_review_order.sql"
	const storageProof = "000292_organization_fiscal_consultation_storage_proof.sql"
	for _, name := range []string{dataReview, storageProof} {
		raw, err := fs.ReadFile(dbmigrations.Files, name)
		if err != nil {
			t.Fatal(err)
		}
		previous[name] = &fstest.MapFile{Data: raw}
	}
	projection := func() string {
		t.Helper()
		var definition string
		if err := f.Admin.QueryRow(ctx, `SELECT pg_get_functiondef('org_document_apply_fiscal_receipt()'::regprocedure)`).Scan(&definition); err != nil {
			t.Fatal(err)
		}
		return definition
	}
	priorProjection := projection()
	broken = maps.Clone(previous)
	broken[storageProof] = &fstest.MapFile{Data: append(slices.Clone(previous[storageProof].Data), "\nSELECT 1/0;\n"...)}
	if _, err := migrate.Apply(ctx, f.AdminDSN, broken); sqlState(err) != "22012" {
		t.Fatalf("expected the injected storage-proof upgrade failure: %v", err)
	}
	if err := f.Admin.QueryRow(ctx, `SELECT to_regclass('public.org_document_fiscal_stored_audit_idx') IS NULL
 AND NOT EXISTS (SELECT 1 FROM schema_migrations WHERE version='000292')`).Scan(&rolledBack); err != nil ||
		!rolledBack || projection() != priorProjection || snapshot() != before {
		t.Fatalf("failed storage-proof upgrade changed the schema or documents: %v", err)
	}
	if applied, err := migrate.Apply(ctx, f.AdminDSN, previous); err != nil || !slices.Equal(applied, []string{"000292"}) {
		t.Fatalf("storage-proof upgrade retry: %v / %v", applied, err)
	}
	if err := f.Admin.QueryRow(ctx, `SELECT to_regclass('public.org_document_fiscal_stored_audit_idx') IS NOT NULL
 AND NOT EXISTS (SELECT 1 FROM org_document_fiscal_consultations)`).Scan(&rolledBack); err != nil ||
		!rolledBack || projection() == priorProjection || snapshot() != before {
		t.Fatalf("storage-proof upgrade altered historical evidence or missed its guard: %v", err)
	}
}
