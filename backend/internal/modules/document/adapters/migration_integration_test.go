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
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
)

// documentMigration is the only source of the version used below; the
// coordinator may renumber it at integration without editing assertions.
const documentMigration = "000205_org_documents.sql"

func TestDocumentMigrationPopulatedUpgradeFailureRecoveryAndPreviousCompatibility(t *testing.T) {
	version := strings.SplitN(documentMigration, "_", 2)[0]
	previous := fstest.MapFS{}
	entries, err := fs.ReadDir(dbmigrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range entries {
		if entry.Name() == documentMigration {
			found = true
		}
		if entry.Name() != "schema_min_supported" && entry.Name() >= documentMigration {
			continue
		}
		raw, err := fs.ReadFile(dbmigrations.Files, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		previous[entry.Name()] = &fstest.MapFile{Data: raw}
	}
	if !found {
		t.Fatal("document migration absent from the embedded chain")
	}
	f := orgtest.NewWithMigrations(t, previous)
	ctx := t.Context()
	org := f.Organizations[0].ID
	// Populate the previous head with a scoped audit event and its private
	// sensitive-access shape (EMP-PRIV-005) that the upgrade must preserve.
	prior := newID(t)
	tx, err := f.Admin.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `SELECT set_config('anpfuel.org_id',$1,true)`, org); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO org_audit_events(org_id,operation_id,actor_id,action,target_type,target_id,request_hash) VALUES($1,$2,$3,'synthetic.previous.recorded','synthetic',$4,repeat('a',64))`, org, prior, newID(t), newID(t)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	raw, err := fs.ReadFile(dbmigrations.Files, documentMigration)
	if err != nil {
		t.Fatal(err)
	}
	current := fstest.MapFS{}
	for name, file := range previous {
		current[name] = file
	}
	current[documentMigration] = &fstest.MapFile{Data: raw}
	broken := fstest.MapFS{documentMigration: &fstest.MapFile{Data: append(append([]byte(nil), raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(ctx, f.AdminDSN, broken); err == nil {
		t.Fatal("broken document migration succeeded")
	}
	var clean bool
	if err := f.Admin.QueryRow(ctx, `SELECT to_regclass('org_documents') IS NULL
		AND to_regprocedure('org_document_guard()') IS NULL
		AND to_regprocedure('org_document_transition_allowed(text,text,text)') IS NULL
		AND NOT EXISTS (SELECT 1 FROM schema_migrations WHERE version=$1)`, version).Scan(&clean); err != nil || !clean {
		t.Fatal("failed document migration left schema or ledger state", err)
	}
	applied, err := migrate.Apply(ctx, f.AdminDSN, current)
	if err != nil || len(applied) != 1 || applied[0] != version {
		t.Fatalf("document migration retry failed: %v, %v", applied, err)
	}
	if err := migrate.Check(ctx, f.Admin, previous); err != nil {
		t.Fatalf("previous chain became incompatible: %v", err)
	}
	if err := migrate.Check(ctx, f.Admin, current); err != nil {
		t.Fatal(err)
	}
	if applied, err := migrate.Apply(ctx, f.AdminDSN, current); err != nil || len(applied) != 0 {
		t.Fatalf("document migration repeated work: %v, %v", applied, err)
	}
	var kept int
	if err := f.Admin.QueryRow(ctx, `SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND operation_id=$2 AND action='synthetic.previous.recorded'`, org, prior).Scan(&kept); err != nil || kept != 1 {
		t.Fatal("upgrade changed previous audit data", err)
	}
	floor, err := fs.ReadFile(dbmigrations.Files, "schema_min_supported")
	if err != nil || string(floor) != "000132\n" {
		t.Fatal("document migration changed compatibility floor")
	}
	// The upgraded schema serves a new declaration through the ordinary login.
	role := pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, err := f.Admin.Exec(ctx, "GRANT USAGE ON SCHEMA public TO "+role+"; GRANT SELECT, INSERT ON org_audit_events TO "+role+"; GRANT SELECT, INSERT, UPDATE ON org_documents TO "+role+"; GRANT SELECT, INSERT ON job_queue TO "+role); err != nil {
		t.Fatal(err)
	}
	catalog, err := audit.NewCatalog(adapters.AuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	scope := f.Organizations[0].Scope(t)
	actor := app.Actor{AccountID: newID(t), MembershipID: newID(t), Channel: "web"}
	authority := &roleAuthority{members: map[string]map[string]roleState{org: {actor.MembershipID: {account: actor.AccountID, role: "DRIVER"}}}}
	store := &adapters.Store{Pool: f.Runtime, Authority: authority, Audit: auditpg.NewStore(f.Runtime, catalog)}
	view, replayed, err := store.Create(ctx, scope, actor, declare(t, syntheticJPEG(t)))
	if err != nil || replayed || view.Statuses.Storage != "UPLOADING" {
		t.Fatal("upgraded schema refused a new declaration", err)
	}
}
