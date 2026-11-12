//go:build integration

package adapters_test

import (
	"context"
	"io/fs"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgaccess"
	accesspg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	org "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRoleReceiptsOrdinaryPopulatedUpgradeFailureAndRecovery(t *testing.T) {
	const name = "000186_role_command_receipts.sql"
	previous := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != "schema_min_supported" && entry.Name() >= name {
			continue
		}
		raw, err := fs.ReadFile(migrations.Files, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		previous[entry.Name()] = &fstest.MapFile{Data: raw}
	}
	f := orgtest.NewWithMigrations(t, previous)
	f.GrantTeamReductions(t)
	f.GrantSettingsLifecycle(t)
	f.GrantOwnershipLifecycle(t)
	runtime := pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, err = f.Admin.Exec(t.Context(), "GRANT USAGE ON SCHEMA public TO "+runtime+";GRANT SELECT,INSERT,UPDATE ON organizations,org_employee_identities,org_memberships,org_role_assignments,org_audit_events TO "+runtime+";GRANT SELECT,UPDATE ON accounts TO "+runtime+";GRANT SELECT,INSERT ON payment_billing_exits TO "+runtime); err != nil {
		t.Fatal(err)
	}
	a, b := scope(t, 301), scope(t, 302)
	for _, n := range []int{301, 302} {
		s := scope(t, n)
		root, err := org.NewPending(s, org.Profile{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = orgpg.NewStore(f.Runtime).Create(t.Context(), s, root); err != nil {
			t.Fatal(err)
		}
	}
	activate(t, f, a, 601, 701, "OWNER")
	activate(t, f, b, 602, 702, "OWNER")
	catalog, err := audit.NewCatalog(accesspg.RoleAuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	events := auditpg.NewStore(f.Runtime, catalog)
	for i, s := range []struct {
		member, assignment, operation int
	}{{601, 701, 8101}, {602, 702, 8102}} {
		scope := a
		if i == 1 {
			scope = b
		}
		_, err = events.RecordChange(t.Context(), scope, audit.Event{OperationID: key(s.operation), ActorID: key(s.member + 100), Action: "auth.assignment.created", TargetType: "assignment", TargetID: key(s.assignment), ReasonCode: "granted", RequestHash: strings.Repeat("a", 64)}, func(context.Context, pgx.Tx) error { return nil })
		if err != nil {
			t.Fatal("populate legacy role audit", err)
		}
	}
	snapshot := func() string {
		t.Helper()
		var value string
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id) FROM organizations x),(SELECT jsonb_agg(to_jsonb(x) ORDER BY id) FROM accounts x),(SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_employee_identities x),(SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_memberships x),(SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_role_assignments x),(SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,operation_id) FROM org_audit_events x))::text`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	before := snapshot()
	migrator := f.RuntimeRole + "_role_receipts_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	if _, err = f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE,CREATE ON SCHEMA public TO "+role+";GRANT SELECT,INSERT ON schema_migrations TO "+role+";GRANT REFERENCES ON organizations,accounts,org_audit_events TO "+role); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := f.Admin.Exec(ctx, "REASSIGN OWNED BY "+role+" TO CURRENT_USER;DROP OWNED BY "+role+";DROP ROLE "+role); err != nil {
			t.Error(err)
		}
	})
	location, err := url.Parse(f.AdminDSN)
	if err != nil {
		t.Fatal(err)
	}
	params := location.Query()
	params.Set("options", "-c role="+migrator)
	location.RawQuery = strings.ReplaceAll(params.Encode(), "+", "%20")
	raw, err := fs.ReadFile(migrations.Files, name)
	if err != nil {
		t.Fatal(err)
	}
	broken := fstest.MapFS{name: &fstest.MapFile{Data: append(append([]byte(nil), raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err = migrate.Apply(t.Context(), location.String(), broken); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
		t.Fatal("injected migration186 failure not reached", err)
	}
	var clean bool
	if err = f.Admin.QueryRow(t.Context(), `SELECT to_regclass('public.org_role_commands') IS NULL AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version='000186')`).Scan(&clean); err != nil || !clean || snapshot() != before {
		t.Fatal("failed receipt migration left table, ledger, or source changes", err)
	}
	current := fstest.MapFS{}
	for path, file := range previous {
		current[path] = file
	}
	current[name] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 1 || applied[0] != "000186" {
		t.Fatal("migration186 recovery failed", applied, err)
	}
	if snapshot() != before {
		t.Fatal("receipt migration rewrote legacy role history")
	}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 0 {
		t.Fatal("repeated migration186 not a no-op", applied, err)
	}
	if err = migrate.Check(t.Context(), f.Admin, previous); err != nil {
		t.Fatal("prior image compatibility", err)
	}
	var count int
	if err = f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_role_commands`).Scan(&count); err != nil || count != 0 {
		t.Fatal("migration fabricated historical acknowledgements", count, err)
	}
	if err = f.Admin.QueryRow(t.Context(), `SELECT NOT rolsuper AND NOT rolbypassrls AND NOT rolcanlogin AND(SELECT relrowsecurity AND relforcerowsecurity AND relowner=(SELECT oid FROM pg_roles WHERE rolname=$1) FROM pg_class WHERE oid='org_role_commands'::regclass) FROM pg_roles WHERE rolname=$1`, migrator).Scan(&clean); err != nil || !clean {
		t.Fatal("receipt table lacks ordinary forced-RLS ownership", err)
	}
	// Explicit scoped storage probes use retained synthetic audit references.
	// This is test data after upgrade; the migration itself fabricated no receipt.
	owner, err := pgxpool.New(t.Context(), location.String())
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	// The application intentionally refuses table-owning runtime roles. These
	// direct database probes test forced RLS on the ordinary migration owner.
	ownerTx := func(orgID string, work func(pgx.Tx) error) error {
		tx, err := owner.Begin(t.Context())
		if err != nil {
			return err
		}
		defer tx.Rollback(t.Context())
		if _, err = tx.Exec(t.Context(), `SELECT set_config('anpfuel.org_id',$1,true)`, orgID); err != nil {
			return err
		}
		if err = work(tx); err != nil {
			return err
		}
		return tx.Commit(t.Context())
	}
	receipt := q.InsertRoleCommandParams{OrgID: a.OrgID().String(), OperationID: key(8101), ActorID: key(701), Kind: "ASSIGN", RequestHash: strings.Repeat("a", 64), Result: []byte(`{"synthetic_storage_probe":true}`)}
	if err = ownerTx(a.OrgID().String(), func(tx pgx.Tx) error { return q.New(tx).InsertRoleCommand(t.Context(), receipt) }); err != nil {
		t.Fatal("ordinary owner could not insert own scoped receipt", err)
	}
	if err = owner.QueryRow(t.Context(), `SELECT count(*) FROM org_role_commands`).Scan(&count); err != nil || count != 0 {
		t.Fatal("table owner bypassed RLS without scope", count, err)
	}
	if err = ownerTx(b.OrgID().String(), func(tx pgx.Tx) error {
		return tx.QueryRow(t.Context(), `SELECT count(*) FROM org_role_commands`).Scan(&count)
	}); err != nil || count != 0 {
		t.Fatal("foreign receipt visible", count, err)
	}
	foreign := receipt
	foreign.OrgID = b.OrgID().String()
	foreign.OperationID = key(8102)
	foreign.ActorID = key(702)
	err = ownerTx(a.OrgID().String(), func(tx pgx.Tx) error { return q.New(tx).InsertRoleCommand(t.Context(), foreign) })
	code(t, err, "42501")
	for _, sql := range []string{`UPDATE org_role_commands SET result='{}'::jsonb`, `DELETE FROM org_role_commands`, `TRUNCATE org_role_commands`} {
		err = ownerTx(a.OrgID().String(), func(tx pgx.Tx) error { _, err := tx.Exec(t.Context(), sql); return err })
		code(t, err, "55000")
	}
	missing := receipt
	missing.OperationID = key(8199)
	err = ownerTx(a.OrgID().String(), func(tx pgx.Tx) error { return q.New(tx).InsertRoleCommand(t.Context(), missing) })
	code(t, err, "23503")
	if err = f.Admin.QueryRow(t.Context(), `SELECT count(*)=1 AND bool_and(result='{"synthetic_storage_probe":true}'::jsonb) FROM org_role_commands`).Scan(&clean); err != nil || !clean || snapshot() != before {
		t.Fatal("receipt guards modified legacy sources or retained failed writes", err)
	}
}
