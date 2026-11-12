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

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	org "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

func TestRoleUpgradeRecoveryCompatibilityAndOrdinaryOwner(t *testing.T) {
	const name = "000171_role_assignments.sql"
	previous := fstest.MapFS{}
	entries, err := fs.ReadDir(dbmigrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != "schema_min_supported" && entry.Name() >= name {
			continue
		}
		raw, err := fs.ReadFile(dbmigrations.Files, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		previous[entry.Name()] = &fstest.MapFile{Data: raw}
	}
	f := orgtest.NewWithMigrations(t, previous)
	a := scope(t, 301)
	runtimeRole := pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "GRANT USAGE ON SCHEMA public TO "+runtimeRole+"; GRANT SELECT,INSERT,UPDATE ON organizations,org_employee_identities,org_memberships TO "+runtimeRole+"; GRANT SELECT,UPDATE ON accounts TO "+runtimeRole+"; GRANT SELECT ON payment_billing_exits TO "+runtimeRole); err != nil {
		t.Fatal(err)
	}
	root, err := org.NewPending(a, org.Profile{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := orgpg.NewStore(f.Runtime).Create(t.Context(), a, root); err != nil {
		t.Fatal(err)
	}
	seed(t, f, a, 601)
	if _, err := f.Admin.Exec(t.Context(), `INSERT INTO directory_stations(id,display_name) VALUES($1,'Synthetic prior public row')`, key(901)); err != nil {
		t.Fatal(err)
	}
	snapshot := func() string {
		t.Helper()
		var result string
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(m)) FROM org_memberships m),(SELECT jsonb_agg(to_jsonb(e)) FROM org_employee_identities e),(SELECT jsonb_agg(to_jsonb(a)) FROM accounts a),(SELECT jsonb_agg(to_jsonb(d)) FROM directory_stations d))::text`).Scan(&result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	before := snapshot()
	migrator := f.RuntimeRole + "_role_migrator"
	r := pgx.Identifier{migrator}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "CREATE ROLE "+r+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE; GRANT USAGE,CREATE ON SCHEMA public TO "+r+"; GRANT SELECT,INSERT ON schema_migrations TO "+r+"; GRANT SELECT,UPDATE,REFERENCES ON organizations TO "+r+"; ALTER TABLE accounts OWNER TO "+r+"; ALTER TABLE payment_billing_exits OWNER TO "+r+"; ALTER TABLE org_memberships OWNER TO "+r+"; ALTER TABLE org_employee_identities OWNER TO "+r); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := f.Admin.Exec(ctx, "REASSIGN OWNED BY "+r+" TO CURRENT_USER; DROP OWNED BY "+r+"; DROP ROLE "+r); err != nil {
			t.Error(err)
		}
	})
	for _, signature := range []string{"org_membership_access_policy(uuid,uuid,text)", "org_membership_check_account_exit(uuid)", "org_membership_account_erasure()", "org_membership_billing_exit_guard()"} {
		if _, err := f.Admin.Exec(t.Context(), "ALTER FUNCTION public."+signature+" OWNER TO "+r); err != nil {
			t.Fatal(err)
		}
	}
	location, err := url.Parse(f.AdminDSN)
	if err != nil {
		t.Fatal(err)
	}
	query := location.Query()
	query.Set("options", "-c role="+migrator)
	location.RawQuery = strings.ReplaceAll(query.Encode(), "+", "%20")
	raw, err := fs.ReadFile(dbmigrations.Files, name)
	if err != nil {
		t.Fatal(err)
	}
	broken := fstest.MapFS{name: &fstest.MapFile{Data: append(append([]byte(nil), raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(t.Context(), location.String(), broken); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
		t.Fatal("did not reach injected failure", err)
	}
	var clean bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('org_role_assignments') IS NULL AND to_regprocedure('org_access_owner_coverage(uuid,uuid,uuid,uuid)') IS NULL AND NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgname IN ('org_account_access_reduction','org_membership_role_end')) AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version='000171')`).Scan(&clean); err != nil || !clean {
		t.Fatal("incomplete migration rollback", err)
	}
	if snapshot() != before {
		t.Fatal("failed migration changed previous rows")
	}
	current := fstest.MapFS{}
	for path, data := range previous {
		current[path] = data
	}
	current[name] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 1 || applied[0] != "000171" {
		t.Fatal("upgrade recovery", applied, err)
	}
	if snapshot() != before {
		t.Fatal("upgrade changed previous rows")
	}
	if err := migrate.Check(t.Context(), f.Admin, previous); err != nil {
		t.Fatal("old binary incompatible", err)
	}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 0 {
		t.Fatal("upgrade replay", applied, err)
	}
	var safe bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT NOT rolsuper AND NOT rolbypassrls AND NOT rolcanlogin AND (SELECT relrowsecurity AND relforcerowsecurity FROM pg_class WHERE oid='org_role_assignments'::regclass) FROM pg_roles WHERE rolname=$1`, migrator).Scan(&safe); err != nil || !safe {
		t.Fatal("missing ordinary ownership/forced RLS", err)
	}
	if _, err := f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON org_role_assignments TO "+runtimeRole); err != nil {
		t.Fatal(err)
	}
	if err := database.WithOrgTx(t.Context(), f.Runtime, a, func(tx pgx.Tx) error {
		if err := insert(t, tx, a, 701, 601, "OWNER", "ORG", a.OrgID().String(), time.Now().Add(-time.Minute), nil); err != nil {
			return err
		}
		_, err := tx.Exec(t.Context(), `UPDATE org_memberships SET state='ACTIVE',revision=revision+1 WHERE org_id=$1 AND id=$2`, a.OrgID().String(), key(601))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	_, err = f.Admin.Exec(t.Context(), `UPDATE accounts SET status='deleted' WHERE id=$1`, key(701))
	code(t, err, "P7501")
	activate(t, f, a, 602, 702, "OWNER")
	if _, err := f.Admin.Exec(t.Context(), `UPDATE accounts SET status='deleted' WHERE id=$1`, key(701)); err != nil {
		t.Fatal("ordinary definer erasure failed", err)
	}
	if err := database.WithOrgTx(t.Context(), f.Runtime, a, func(tx pgx.Tx) error {
		var ended bool
		err := tx.QueryRow(t.Context(), `SELECT m.state='ENDED' AND m.account_id IS NULL AND r.ended_at IS NOT NULL FROM org_memberships m JOIN org_role_assignments r ON (r.org_id,r.membership_id)=(m.org_id,m.id) WHERE m.org_id=$1 AND m.id=$2`, a.OrgID().String(), key(601)).Scan(&ended)
		if err == nil && !ended {
			t.Fatal("ordinary erasure left live access")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
