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
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
)

// safetyMigration is the only source of the version used below; the
// coordinator may renumber it at integration without editing assertions.
const safetyMigration = "000212_org_document_upload_safety.sql"

// previousDocument writes a document exactly as the previous binary did: a
// creation event and the initial row, with no safety job, then the given
// storage steps through the previous guard.
func previousDocument(t *testing.T, f *orgtest.Fixture, scope kernel.OrgScope, steps ...string) string {
	t.Helper()
	id, operation := newID(t), newID(t)
	if err := database.WithOrgTx(t.Context(), f.Runtime, scope, func(tx pgx.Tx) error {
		org := scope.OrgID().String()
		if _, err := tx.Exec(t.Context(), `INSERT INTO org_audit_events(org_id,operation_id,actor_id,action,target_type,target_id,request_hash)
			VALUES($1,$2,$3,'document.upload.created','document',$4,repeat('a',64))`, org, operation, newID(t), id); err != nil {
			return err
		}
		if _, err := tx.Exec(t.Context(), `INSERT INTO org_documents(org_id,id,kind,data_mode,operation_id,created_by,original_sha256,original_bytes,original_type,
			storage_state,storage_revision,extraction_state,extraction_revision,structure_state,structure_revision,conference_state,conference_revision,
			reconciliation_state,fiscal_state,fiscal_revision) VALUES($1,$2,'RECEIPT','SYNTHETIC',$3,$4,repeat('b',64),1024,'image/jpeg','UPLOADING',1,
			'NONE',1,'NOT_APPLICABLE',1,'UNCONFERRED',1,'UNALLOCATED','NOT_CONSULTED',1)`, org, id, operation, newID(t)); err != nil {
			return err
		}
		for i, state := range steps {
			if _, err := tx.Exec(t.Context(), `UPDATE org_documents SET storage_state=$3,storage_revision=$4 WHERE org_id=$1 AND id=$2`, org, id, state, i+2); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestSafetyMigrationPopulatedUpgradeFailureRecoveryAndPreviousCompatibility(t *testing.T) {
	version := strings.SplitN(safetyMigration, "_", 2)[0]
	previous := fstest.MapFS{}
	entries, err := fs.ReadDir(dbmigrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range entries {
		if entry.Name() == safetyMigration {
			found = true
		}
		if entry.Name() != "schema_min_supported" && entry.Name() >= safetyMigration {
			continue
		}
		raw, err := fs.ReadFile(dbmigrations.Files, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		previous[entry.Name()] = &fstest.MapFile{Data: raw}
	}
	if !found {
		t.Fatal("upload safety migration absent from the embedded chain")
	}
	f := orgtest.NewWithMigrations(t, previous)
	ctx := t.Context()
	scopeA, scopeB := f.Organizations[0].Scope(t), f.Organizations[1].Scope(t)
	role := pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, err := f.Admin.Exec(ctx, "GRANT USAGE ON SCHEMA public TO "+role+"; GRANT SELECT, INSERT ON org_audit_events TO "+role+
		"; GRANT SELECT, INSERT, UPDATE ON org_documents TO "+role+"; GRANT SELECT, INSERT ON job_queue TO "+role); err != nil {
		t.Fatal(err)
	}
	// The previous head holds pending documents in two organizations, a
	// stored one and a FAILED one written directly without any reason.
	pendingA := previousDocument(t, f, scopeA)
	pendingB := previousDocument(t, f, scopeB)
	stored := previousDocument(t, f, scopeA, "RECEIVED", "STORED")
	legacyFailed := previousDocument(t, f, scopeA, "FAILED")
	snapshot := func() string {
		t.Helper()
		var out string
		if err := f.Admin.QueryRow(ctx, `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x)-'storage_reason' ORDER BY org_id,id) FROM org_documents x),
			(SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,operation_id) FROM org_audit_events x))::text`).Scan(&out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	guard := func() string {
		t.Helper()
		var out string
		if err := f.Admin.QueryRow(ctx, `SELECT pg_get_functiondef('org_document_guard()'::regprocedure)`).Scan(&out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	jobCount := func() int {
		t.Helper()
		var n int
		if err := f.Admin.QueryRow(ctx, `SELECT count(*) FROM job_queue`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	before, beforeGuard, beforeJobs := snapshot(), guard(), jobCount()
	raw, err := fs.ReadFile(dbmigrations.Files, safetyMigration)
	if err != nil {
		t.Fatal(err)
	}
	current := fstest.MapFS{}
	for name, file := range previous {
		current[name] = file
	}
	current[safetyMigration] = &fstest.MapFile{Data: raw}
	broken := fstest.MapFS{safetyMigration: &fstest.MapFile{Data: append(append([]byte(nil), raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(ctx, f.AdminDSN, broken); err == nil {
		t.Fatal("broken upload safety migration succeeded")
	}
	var clean bool
	if err := f.Admin.QueryRow(ctx, `SELECT NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='org_documents' AND column_name='storage_reason')
		AND to_regprocedure('org_document_reason_allowed(text,text,text)') IS NULL
		AND to_regprocedure('org_document_upload_abandoned(timestamptz)') IS NULL
		AND (SELECT relforcerowsecurity FROM pg_class WHERE relname='org_documents')
		AND NOT EXISTS (SELECT 1 FROM schema_migrations WHERE version=$1)`, version).Scan(&clean); err != nil || !clean ||
		guard() != beforeGuard || jobCount() != beforeJobs || snapshot() != before {
		t.Fatal("failed upload safety migration left schema, guard, jobs, ledger or data changes", err)
	}
	applied, err := migrate.Apply(ctx, f.AdminDSN, current)
	if err != nil || len(applied) != 1 || applied[0] != version {
		t.Fatalf("upload safety migration retry failed: %v, %v", applied, err)
	}
	if err := migrate.Check(ctx, f.Admin, previous); err != nil {
		t.Fatalf("previous chain became incompatible: %v", err)
	}
	if err := migrate.Check(ctx, f.Admin, current); err != nil {
		t.Fatal(err)
	}
	if applied, err := migrate.Apply(ctx, f.AdminDSN, current); err != nil || len(applied) != 0 {
		t.Fatalf("upload safety migration repeated work: %v, %v", applied, err)
	}
	if snapshot() != before {
		t.Fatal("upgrade changed previous document or audit data")
	}
	var reasons int
	var forced bool
	if err := f.Admin.QueryRow(ctx, `SELECT count(*) FILTER (WHERE storage_reason IS NOT NULL),(SELECT relforcerowsecurity FROM pg_class WHERE relname='org_documents') FROM org_documents`).Scan(&reasons, &forced); err != nil || reasons != 0 || !forced {
		t.Fatal("upgrade invented reasons or left row-level security lifted", reasons, err)
	}
	if after := guard(); after == beforeGuard || !strings.Contains(after, "org_document_reason_allowed") || !strings.Contains(after, "document identity is immutable") {
		t.Fatal("guard not extended from its latest body")
	}
	// Exactly the pending documents of every organization got their closure.
	rows, err := f.Admin.Query(ctx, `SELECT j.payload->>'document_id',j.dedupe_key,j.max_attempts,
		j.not_before=d.created_at+make_interval(secs=>$1) AND j.payload=jsonb_build_object('version',1,'org_id',d.org_id::text,'document_id',d.id::text)
		FROM job_queue j JOIN org_documents d ON d.id::text=j.payload->>'document_id' WHERE j.kind='org-document-safety' AND j.status='queued' AND j.attempts=0 ORDER BY 1`,
		(app.UploadWindow + app.UploadGrace).Seconds())
	if err != nil {
		t.Fatal(err)
	}
	backfilled := map[string]string{}
	for rows.Next() {
		var id, dedupe string
		var attempts int32
		var exact bool
		if err := rows.Scan(&id, &dedupe, &attempts, &exact); err != nil {
			t.Fatal(err)
		}
		if attempts != adapters.SafetyAttempts || !exact {
			t.Fatal("backfilled closure job budget, due time or payload", attempts, exact)
		}
		backfilled[id] = dedupe
	}
	if rows.Err() != nil || len(backfilled) != 2 || jobCount() != beforeJobs+2 ||
		backfilled[pendingA] != adapters.SafetyDedupe(adapters.ExpiryPurpose, scopeA, pendingA) || backfilled[pendingB] != adapters.SafetyDedupe(adapters.ExpiryPurpose, scopeB, pendingB) {
		t.Fatal("closure jobs not queued for exactly the pending documents", backfilled)
	}
	if _, ok := backfilled[stored]; ok {
		t.Fatal("settled document queued")
	}
	if _, ok := backfilled[legacyFailed]; ok {
		t.Fatal("settled document queued")
	}
	floor, err := fs.ReadFile(dbmigrations.Files, "schema_min_supported")
	if err != nil || string(floor) != "000132\n" {
		t.Fatal("upload safety migration changed compatibility floor")
	}
	// The upgraded schema runs the new flow through the ordinary login, and
	// the previous binary's single stored transaction is still admitted.
	catalog, err := audit.NewCatalog(adapters.AuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	owner := app.Actor{AccountID: newID(t), MembershipID: newID(t), Channel: "web"}
	authority := &roleAuthority{members: map[string]map[string]roleState{scopeA.OrgID().String(): {owner.MembershipID: {account: owner.AccountID, role: "OWNER"}}}}
	store := &adapters.Store{Pool: f.Runtime, Authority: authority, Audit: auditpg.NewStore(f.Runtime, catalog)}
	command := declare(t, syntheticJPEG(t))
	if _, _, err := store.Create(ctx, scopeA, owner, command); err != nil {
		t.Fatal(err)
	}
	if view, _, err := store.MarkReceived(ctx, scopeA, owner, command.DocumentID, newID(t)); err != nil || view.Statuses.Storage != domain.Received {
		t.Fatal("upgraded schema refused the received transition", err)
	}
	if outcome, err := store.Settle(ctx, scopeA, command.DocumentID, domain.Received, 2, app.UnsupportedContent, newID(t)); err != nil || outcome != (app.Outcome{State: domain.StorageFailed, Reason: app.UnsupportedContent}) {
		t.Fatal("upgraded schema refused a reasoned settlement", outcome, err)
	}
	previousStored := previousDocument(t, f, scopeA, "RECEIVED", "STORED")
	if subject, err := store.Subject(ctx, scopeA, previousStored); err != nil || subject.State != domain.Stored || subject.Reason != "" {
		t.Fatal("previous binary's stored transaction refused after the upgrade", err)
	}
	if subject, err := store.Subject(ctx, scopeA, legacyFailed); err != nil || subject.State != domain.StorageFailed || subject.Reason != "" {
		t.Fatal("legacy failed document not readable without a reason", err)
	}
}
