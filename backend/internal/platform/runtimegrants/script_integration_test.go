//go:build integration

package runtimegrants_test

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/runtimegrants"
)

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("test location unavailable")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "..")
}

func modulePath(t *testing.T, backend string) string {
	t.Helper()
	file, err := os.Open(filepath.Join(backend, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if path, ok := strings.CutPrefix(scanner.Text(), "module "); ok {
			return strings.TrimSpace(path)
		}
	}
	t.Fatal("module path not found")
	return ""
}

// headScript derives the inventory of the production API and worker binaries
// against the freshly migrated head and renders it.
func headScript(t *testing.T, admin *pgxpool.Pool) (runtimegrants.Inventory, []byte) {
	t.Helper()
	catalog, err := runtimegrants.LoadCatalog(t.Context(), admin)
	if err != nil {
		t.Fatal(err)
	}
	backend := filepath.Join(repositoryRoot(t), "backend")
	packages, err := runtimegrants.Closure(runtimegrants.ProductionContext(), backend, modulePath(t, backend), "cmd/api", "cmd/worker")
	if err != nil {
		t.Fatal(err)
	}
	usage := runtimegrants.NewUsage()
	for _, pkg := range packages {
		if err := runtimegrants.ScanPackage(usage, pkg); err != nil {
			t.Fatal(err)
		}
	}
	inventory, err := runtimegrants.Build(catalog, usage, runtimegrants.ReviewedExceptions())
	if err != nil {
		t.Fatal(err)
	}
	return inventory, runtimegrants.Render(inventory)
}

func TestRuntimeScriptMatchesHead(t *testing.T) {
	f := orgtest.New(t)
	inventory, rendered := headScript(t, f.Admin)
	path := filepath.Join(repositoryRoot(t), runtimegrants.ScriptPath)
	if os.Getenv("ANPFUEL_RUNTIME_GRANTS_WRITE") == "1" {
		if err := os.WriteFile(path, rendered, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(current, rendered) {
		t.Fatalf("%s is stale for schema %s; regenerate it with ANPFUEL_RUNTIME_GRANTS_WRITE=1 (see the script header) and review the diff", runtimegrants.ScriptPath, inventory.SchemaVersion)
	}
	granted := 0
	for _, table := range inventory.Tables {
		if len(table.Privileges) > 0 {
			granted++
		}
	}
	t.Logf("schema %s: %d relations, %d granted, %d sequences, %d explicit functions, %d operator-only", inventory.SchemaVersion, len(inventory.Tables), granted, len(inventory.Sequences), len(inventory.Functions), len(inventory.OperatorOnly))
}

func sqlState(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

func TestRuntimeScriptProvisionsOrdinaryRoleAndRefusesUnsafeUse(t *testing.T) {
	f := orgtest.New(t)
	ctx := t.Context()
	role, secret := f.ProvisionScriptRole(t)
	// Idempotent: a second application converges to the same privileges.
	before := privileges(t, f.Admin, role)
	script, err := os.ReadFile(filepath.Join(repositoryRoot(t), runtimegrants.ScriptPath))
	if err != nil {
		t.Fatal(err)
	}
	again, err := runtimegrants.ForRole(script, role)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Admin.Exec(ctx, again); err != nil {
		t.Fatal("second application", err)
	}
	if after := privileges(t, f.Admin, role); after != before {
		t.Fatal("re-application changed the privilege set")
	}
	// A widened grant is converged away by the next application.
	name := pgx.Identifier{role}.Sanitize()
	if _, err := f.Admin.Exec(ctx, "GRANT TRUNCATE, DELETE ON org_audit_events TO "+name+"; GRANT UPDATE(checksum) ON schema_migrations TO "+name); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Admin.Exec(ctx, again); err != nil {
		t.Fatal("convergent application", err)
	}
	if after := privileges(t, f.Admin, role); after != before {
		t.Fatal("re-application kept a widened privilege")
	}
	var super, bypass, inherit, createRole, createDB, login bool
	if err := f.Admin.QueryRow(ctx, `SELECT rolsuper, rolbypassrls, rolinherit, rolcreaterole, rolcreatedb, rolcanlogin FROM pg_roles WHERE rolname=$1`, role).Scan(&super, &bypass, &inherit, &createRole, &createDB, &login); err != nil {
		t.Fatal(err)
	}
	if super || bypass || inherit || createRole || createDB || !login {
		t.Fatal("runtime role attributes are not ordinary")
	}
	var password string
	if err := f.Admin.QueryRow(ctx, `SELECT COALESCE(rolpassword,'') FROM pg_authid WHERE rolname=$1`, role).Scan(&password); err != nil || !strings.HasPrefix(password, "SCRAM-SHA-256$") {
		t.Fatal("test secret was not stored as a verifier", err)
	}

	pool := f.ScriptRuntimePool(t, role, secret, 2)
	scope := f.Organizations[0].Scope(t)
	if err := database.WithOrgTx(ctx, pool, scope, func(tx pgx.Tx) error { return nil }); err != nil {
		t.Fatal("the script role is refused by the organization role guard", err)
	}
	if err := database.WithOrgTx(ctx, f.Admin, scope, func(tx pgx.Tx) error { return nil }); !errors.Is(err, database.ErrUnsafeOrgRole) {
		t.Fatal("the migration owner (superuser) login must stay refused", err)
	}
	// Forced RLS: the other organization's root is invisible inside a scope.
	if err := database.WithOrgTx(ctx, pool, scope, func(tx pgx.Tx) error {
		var visible int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM organizations WHERE org_id=$1`, f.Organizations[1].ID).Scan(&visible); err != nil {
			return err
		}
		if visible != 0 {
			t.Fatal("another organization is visible under the runtime role")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	refused := func(label, sql string, args ...any) {
		t.Helper()
		_, err := pool.Exec(ctx, sql, args...)
		if sqlState(err) != "42501" {
			t.Fatalf("%s: want insufficient_privilege, got %v", label, err)
		}
	}
	refused("truncate append-only audit", `TRUNCATE org_audit_events`)
	refused("delete append-only audit", `DELETE FROM org_audit_events`)
	refused("update append-only audit", `UPDATE org_audit_events SET reason_code=reason_code`)
	refused("delete append-only intake", `DELETE FROM org_sync_operations`)
	refused("truncate accounts", `TRUNCATE accounts`)
	refused("write migration ledger", `INSERT INTO schema_migrations(version, checksum) VALUES('999999','x')`)
	refused("delete migration ledger", `DELETE FROM schema_migrations`)
	refused("operator-only projection rebuild", `SELECT org_meter_current_rebuild($1::uuid)`, f.Organizations[0].ID)
	refused("closed owner-count function", `SELECT org_pending_owner_count($1::uuid,$1::uuid)`, f.Organizations[0].ID)
	refused("ddl in public", `CREATE TABLE public.runtime_probe(id int)`)
	refused("ddl on existing table", `ALTER TABLE accounts ADD COLUMN runtime_probe int`)
	refused("role escalation", `SET ROLE `+pgx.Identifier{"pg_read_all_data"}.Sanitize())
	var definers int
	if err := f.Admin.QueryRow(ctx, `SELECT count(*) FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname='public' AND p.prosecdef AND has_function_privilege($1, p.oid, 'EXECUTE')`, role).Scan(&definers); err != nil || definers != 0 {
		t.Fatal("runtime role can execute a SECURITY DEFINER function", definers, err)
	}
	var columnGrants int
	if err := f.Admin.QueryRow(ctx, `SELECT count(*) FROM pg_attribute a WHERE a.attacl::text LIKE '%'||$1||'=%'`, role).Scan(&columnGrants); err != nil || columnGrants != 0 {
		t.Fatal("column-level grants survived convergence", columnGrants, err)
	}

	// Refusals of the script itself: another schema version, an owner role
	// and the migration owner as runtime role apply nothing.
	if _, err := f.Admin.Exec(ctx, strings.Replace(again, "IF head IS DISTINCT FROM '", "IF head IS DISTINCT FROM 'x", 1)); err == nil || !strings.Contains(err.Error(), "apply the script of the deployed release") {
		t.Fatal("version guard", err)
	}
	owner := "emp_owner_" + strings.TrimPrefix(role, "emp_script_")
	ownerName := pgx.Identifier{owner}.Sanitize()
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, _ = f.Admin.Exec(cleanup, "DROP OWNED BY "+ownerName+"; DROP ROLE IF EXISTS "+ownerName)
	})
	if _, err := f.Admin.Exec(ctx, "CREATE ROLE "+ownerName+" NOLOGIN; CREATE TABLE runtime_owned_probe(id int); ALTER TABLE runtime_owned_probe OWNER TO "+ownerName); err != nil {
		t.Fatal(err)
	}
	owned, err := runtimegrants.ForRole(script, owner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Admin.Exec(ctx, owned); err == nil || !strings.Contains(err.Error(), "must own nothing") {
		t.Fatal("an owning role was provisioned", err)
	}
	var self string
	if err := f.Admin.QueryRow(ctx, `SELECT current_user`).Scan(&self); err != nil {
		t.Fatal(err)
	}
	if selfSQL, err := runtimegrants.ForRole(script, self); err == nil {
		if _, err := f.Admin.Exec(ctx, selfSQL); err == nil || !strings.Contains(err.Error(), "cannot be the migration owner") {
			t.Fatal("the migration owner was provisioned as runtime role", err)
		}
	}
}

// privileges is the effective privilege fingerprint of role over schema public.
func privileges(t *testing.T, admin *pgxpool.Pool, role string) string {
	t.Helper()
	var out string
	err := admin.QueryRow(t.Context(), `SELECT string_agg(item, ',' ORDER BY item) FROM (
		SELECT c.relname||':'||p.privilege AS item FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
			CROSS JOIN unnest(ARRAY['SELECT','INSERT','UPDATE','DELETE','TRUNCATE','REFERENCES','TRIGGER']) p(privilege)
			WHERE n.nspname='public' AND c.relkind IN ('r','p','v','m') AND has_table_privilege($1, c.oid, p.privilege)
		UNION ALL SELECT c.relname||'.'||a.attname||':'||p.privilege FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
			JOIN pg_attribute a ON a.attrelid=c.oid AND a.attnum>0 AND NOT a.attisdropped
			CROSS JOIN unnest(ARRAY['SELECT','INSERT','UPDATE']) p(privilege)
			WHERE n.nspname='public' AND c.relkind IN ('r','p') AND has_column_privilege($1, c.oid, a.attnum, p.privilege)
			AND NOT has_table_privilege($1, c.oid, p.privilege)
		UNION ALL SELECT s.relname||':USAGE' FROM pg_class s JOIN pg_namespace n ON n.oid=s.relnamespace
			WHERE n.nspname='public' AND s.relkind='S' AND CASE WHEN s.relkind='S' THEN has_sequence_privilege($1, s.oid, 'USAGE') ELSE false END
		UNION ALL SELECT p.oid::regprocedure::text||':EXECUTE' FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace
			WHERE n.nspname='public' AND has_function_privilege($1, p.oid, 'EXECUTE')
	) items`, role).Scan(&out)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
