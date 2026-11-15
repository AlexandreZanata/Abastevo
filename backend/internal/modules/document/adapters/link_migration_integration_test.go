//go:build integration

package adapters_test

import (
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/domain"
	memberpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
)

// linkMigration is the only source of the version used below; the
// coordinator may renumber it at integration without editing assertions.
const linkMigration = "000209_org_document_links.sql"

func TestDocumentLinkMigrationPopulatedUpgradeFailureRecoveryAndPreviousCompatibility(t *testing.T) {
	version := strings.SplitN(linkMigration, "_", 2)[0]
	previous := fstest.MapFS{}
	entries, err := fs.ReadDir(dbmigrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range entries {
		if entry.Name() == linkMigration {
			found = true
		}
		if entry.Name() != "schema_min_supported" && entry.Name() >= linkMigration {
			continue
		}
		raw, err := fs.ReadFile(dbmigrations.Files, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		previous[entry.Name()] = &fstest.MapFile{Data: raw}
	}
	if !found {
		t.Fatal("document link migration absent from the embedded chain")
	}
	f := orgtest.NewWithMigrations(t, previous)
	ctx := t.Context()
	org := f.Organizations[0].ID
	scope := f.Organizations[0].Scope(t)
	role := pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, err := f.Admin.Exec(ctx, "GRANT USAGE ON SCHEMA public TO "+role+"; GRANT SELECT, INSERT ON org_audit_events TO "+role+"; GRANT SELECT, INSERT, UPDATE ON org_documents TO "+role+"; GRANT SELECT, INSERT ON job_queue TO "+role); err != nil {
		t.Fatal(err)
	}
	catalog, err := audit.NewCatalog(adapters.AuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	// Populate the previous head with a pending document and its creation
	// event through the ordinary login; the upgrade must preserve both.
	owner := app.Actor{AccountID: newID(t), MembershipID: newID(t), Channel: "web"}
	authority := &roleAuthority{members: map[string]map[string]roleState{org: {owner.MembershipID: {account: owner.AccountID, role: "OWNER"}}}}
	store := &adapters.Store{Pool: f.Runtime, Authority: authority, Audit: auditpg.NewStore(f.Runtime, catalog), Targets: app.Targets{domain.Employee: memberpg.EmployeeDocumentTargets{}}}
	document, _, err := store.Create(ctx, scope, owner, declare(t, syntheticJPEG(t)))
	if err != nil {
		t.Fatal(err)
	}
	snapshot := func() string {
		t.Helper()
		var out string
		if err := f.Admin.QueryRow(ctx, `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_documents x),
			(SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,operation_id) FROM org_audit_events x))::text`).Scan(&out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	usage := func() string {
		t.Helper()
		var out string
		if err := f.Admin.QueryRow(ctx, `SELECT pg_get_functiondef('org_asset_current_usage(uuid,uuid)'::regprocedure)`).Scan(&out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	before, beforeUsage := snapshot(), usage()
	raw, err := fs.ReadFile(dbmigrations.Files, linkMigration)
	if err != nil {
		t.Fatal(err)
	}
	current := fstest.MapFS{}
	for name, file := range previous {
		current[name] = file
	}
	current[linkMigration] = &fstest.MapFile{Data: raw}
	broken := fstest.MapFS{linkMigration: &fstest.MapFile{Data: append(append([]byte(nil), raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(ctx, f.AdminDSN, broken); err == nil {
		t.Fatal("broken document link migration succeeded")
	}
	var clean bool
	if err := f.Admin.QueryRow(ctx, `SELECT to_regclass('org_document_links') IS NULL
		AND to_regprocedure('org_document_link_guard()') IS NULL
		AND to_regprocedure('org_document_link_keep()') IS NULL
		AND NOT EXISTS (SELECT 1 FROM schema_migrations WHERE version=$1)`, version).Scan(&clean); err != nil || !clean || usage() != beforeUsage || snapshot() != before {
		t.Fatal("failed document link migration left schema, ledger, usage source or data changes", err)
	}
	applied, err := migrate.Apply(ctx, f.AdminDSN, current)
	if err != nil || len(applied) != 1 || applied[0] != version {
		t.Fatalf("document link migration retry failed: %v, %v", applied, err)
	}
	if err := migrate.Check(ctx, f.Admin, previous); err != nil {
		t.Fatalf("previous chain became incompatible: %v", err)
	}
	if err := migrate.Check(ctx, f.Admin, current); err != nil {
		t.Fatal(err)
	}
	if applied, err := migrate.Apply(ctx, f.AdminDSN, current); err != nil || len(applied) != 0 {
		t.Fatalf("document link migration repeated work: %v, %v", applied, err)
	}
	if snapshot() != before {
		t.Fatal("upgrade changed previous document or audit data")
	}
	// The asset usage source keeps refusing unknown references and lists
	// exactly the new same-organization asset reference (EDR-162).
	if after := usage(); after == beforeUsage || !strings.Contains(after, "org_document_links_asset_fkey") || !strings.Contains(after, "asset usage source unavailable") {
		t.Fatal("asset usage source does not recognise the link reference")
	}
	floor, err := fs.ReadFile(dbmigrations.Files, "schema_min_supported")
	if err != nil || string(floor) != "000132\n" {
		t.Fatal("document link migration changed compatibility floor")
	}
	// Only the target parent is seeded below the owning register's guards
	// (its organization root has no snapshot in this harness); the link itself
	// goes through the ordinary login, every guard and the foreign keys.
	employee := newID(t)
	tx, err := f.Admin.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `SET LOCAL session_replication_role = replica`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO org_employee_identities(org_id,id,state) VALUES($1,$2,'ACTIVE')`, org, employee); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	// The current store also writes the EMP-CHECK-003 typed reference
	// (000230), so the rehearsal reaches the embedded head before linking.
	if _, err := migrate.Apply(ctx, f.AdminDSN, dbmigrations.Files); err != nil {
		t.Fatal("remaining embedded chain after the 209 rehearsal", err)
	}
	if _, err := f.Admin.Exec(ctx, "GRANT SELECT, INSERT ON org_document_links TO "+role+"; GRANT SELECT(org_id,id) ON org_employee_identities TO "+role); err != nil {
		t.Fatal(err)
	}
	view, replayed, err := store.Link(ctx, scope, owner, app.Link{OperationID: newID(t), DocumentID: document.ID, Target: app.TargetRef{Kind: domain.Employee, ID: employee}})
	if err != nil || replayed || view.DocumentStorage != domain.Uploading || view.Target.ID != employee {
		t.Fatal("upgraded schema refused a link", err)
	}
}
