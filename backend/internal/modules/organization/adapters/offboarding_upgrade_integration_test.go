//go:build integration

package adapters_test

import (
	"context"
	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	memberpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
	"io/fs"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func TestOffboardingOrdinaryPopulatedUpgradeRollbackRecovery(t *testing.T) {
	const name = "000191_membership_offboarding.sql"
	previous := fstest.MapFS{}
	entries, e := fs.ReadDir(migrations.Files, ".")
	if e != nil {
		t.Fatal(e)
	}
	for _, entry := range entries {
		if entry.Name() != "schema_min_supported" && entry.Name() >= name {
			continue
		}
		raw, e := fs.ReadFile(migrations.Files, entry.Name())
		if e != nil {
			t.Fatal(e)
		}
		previous[entry.Name()] = &fstest.MapFile{Data: raw}
	}
	f := orgtest.NewWithMigrations(t, previous)
	_, _, onboarding, actor, flags := onboardingOnFixture(t, f)
	old := legalFixtureFor(t, f, onboarding, actor, flags)
	grantEmployeeTables(t, old)
	catalog, e := audit.NewCatalog(memberpg.InvitationAuditDefinitions())
	if e != nil {
		t.Fatal(e)
	}
	invfx := invitationFixtureState{legalFixtureState: old, invites: &memberpg.InvitationStore{Pool: f.Runtime, Memberships: old.store.Memberships, Authority: &orgpg.InvitationAuthority{Memberships: old.store.Memberships, Roles: old.store.Roles, Capabilities: flags, Settings: &orgpg.SettingsStore{Pool: f.Runtime}}, Audit: auditpg.NewStore(f.Runtime, catalog)}}
	fx := membershipAccessOn(t, invfx)
	target := accessMember(t, fx)
	snapshot := func() string {
		t.Helper()
		var out string
		if e := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_memberships x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_role_assignments x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_audit_events x))::text`).Scan(&out); e != nil {
			t.Fatal(e)
		}
		return out
	}
	before := snapshot()
	migrator := f.RuntimeRole + "_access_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	if _, e := f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE,CREATE ON SCHEMA public TO "+role+";GRANT SELECT,INSERT ON schema_migrations TO "+role+";GRANT REFERENCES ON accounts,org_memberships,org_audit_events TO "+role); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, e := f.Admin.Exec(ctx, "REASSIGN OWNED BY "+role+" TO CURRENT_USER;DROP OWNED BY "+role+";DROP ROLE "+role); e != nil {
			t.Error(e)
		}
	})
	location, e := url.Parse(f.AdminDSN)
	if e != nil {
		t.Fatal(e)
	}
	params := location.Query()
	params.Set("options", "-c role="+migrator)
	location.RawQuery = strings.ReplaceAll(params.Encode(), "+", "%20")
	raw, e := fs.ReadFile(migrations.Files, name)
	if e != nil {
		t.Fatal(e)
	}
	broken := fstest.MapFS{name: &fstest.MapFile{Data: append(append([]byte(nil), raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, e := migrate.Apply(t.Context(), location.String(), broken); e == nil || !strings.Contains(e.Error(), "SQLSTATE 22012") {
		t.Fatal("injected migration failure not reached", e)
	}
	var clean bool
	if e := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('org_offboarding_commands') IS NULL AND to_regclass('org_offboarding_notices') IS NULL AND to_regprocedure('org_offboarding_guard()') IS NULL AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version='000191')`).Scan(&clean); e != nil || !clean || snapshot() != before {
		t.Fatal("partial failed upgrade", e)
	}
	current := fstest.MapFS{}
	for key, value := range previous {
		current[key] = value
	}
	current[name] = &fstest.MapFile{Data: raw}
	if applied, e := migrate.Apply(t.Context(), location.String(), current); e != nil || len(applied) != 1 || applied[0] != "000191" {
		t.Fatal("upgrade recovery", applied, e)
	}
	if snapshot() != before {
		t.Fatal("historical source rewritten")
	}
	if applied, e := migrate.Apply(t.Context(), location.String(), current); e != nil || len(applied) != 0 {
		t.Fatal("repeat upgrade", e)
	}
	if e := migrate.Check(t.Context(), f.Admin, previous); e != nil {
		t.Fatal("previous image compatibility", e)
	}
	invitationCount(t, fx.invitationFixtureState, `SELECT count(*) FROM org_offboarding_commands`, 0)
	s := offboardingStore(t, fx)
	c := offboardingCommand(t, offboardingRead(t, fx, s, target))
	if _, e := s.CompleteOffboarding(t.Context(), fx.scope, accessActor(fx), fx.member, c); e != nil {
		t.Fatal(e)
	}
	tx, e := f.Admin.Begin(t.Context())
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(context.Background())
	if _, e := tx.Exec(t.Context(), "SET LOCAL ROLE "+role); e != nil {
		t.Fatal(e)
	}
	var n int
	if e := tx.QueryRow(t.Context(), `SELECT count(*) FROM org_offboarding_commands`).Scan(&n); e != nil || n != 0 {
		t.Fatal("owner bypassed RLS", e)
	}
	_ = tx.Rollback(t.Context())
	if _, e := f.Admin.Exec(t.Context(), "GRANT UPDATE,DELETE,TRUNCATE ON org_offboarding_commands,org_offboarding_notices TO "+pgx.Identifier{f.RuntimeRole}.Sanitize()); e != nil {
		t.Fatal(e)
	}
	for _, table := range []string{"org_offboarding_commands", "org_offboarding_notices"} {
		teamSQLCode(t, unitSchemaExec(t, old, "DELETE FROM "+table), "55000")
		teamSQLCode(t, unitSchemaExec(t, old, "UPDATE "+table+" SET operation_id=operation_id"), "55000")
	}
	teamSQLCode(t, unitSchemaExec(t, old, `TRUNCATE org_offboarding_commands,org_offboarding_notices`), "55000")
}
