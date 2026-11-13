//go:build integration

package adapters_test

import (
	"context"
	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
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

func TestAccountContextsOrdinaryPopulatedMigrationRollbackRecovery(t *testing.T) {
	const name = "000195_account_context_discovery.sql"
	previous := fstest.MapFS{}
	entries, e := fs.ReadDir(migrations.Files, ".")
	if e != nil {
		t.Fatal(e)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() != "schema_min_supported" && (entry.Name() >= name || !strings.HasSuffix(entry.Name(), ".sql")) {
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
	fx := legalFixtureFor(t, f, onboarding, actor, flags)
	snapshot := func() string {
		t.Helper()
		var out string
		if e := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_memberships x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_role_assignments x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id)FROM organizations x))::text`).Scan(&out); e != nil {
			t.Fatal(e)
		}
		return out
	}
	before := snapshot()
	migrator := f.RuntimeRole + "_context_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	if _, e = f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE,CREATE ON SCHEMA public TO "+role+";GRANT SELECT,INSERT ON schema_migrations TO "+role+";ALTER TABLE org_memberships OWNER TO "+role); e != nil {
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
	if _, e = migrate.Apply(t.Context(), location.String(), broken); e == nil || !strings.Contains(e.Error(), "SQLSTATE 22012") {
		t.Fatal("late context migration fault not reached", e)
	}
	var clean bool
	if e := f.Admin.QueryRow(t.Context(), `SELECT NOT EXISTS(SELECT 1 FROM pg_policy WHERE polname='org_memberships_account_context_read') AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version='000195') AND (SELECT relforcerowsecurity FROM pg_class WHERE oid='org_memberships'::regclass)`).Scan(&clean); e != nil || !clean || snapshot() != before {
		t.Fatal("failed context upgrade left partial authority", e)
	}
	current := fstest.MapFS{}
	for key, value := range previous {
		current[key] = value
	}
	current[name] = &fstest.MapFile{Data: raw}
	if applied, e := migrate.Apply(t.Context(), location.String(), current); e != nil || len(applied) != 1 || applied[0] != "000195" {
		t.Fatal("ordinary context migration recovery", applied, e)
	}
	if snapshot() != before {
		t.Fatal("context migration changed access history")
	}
	if applied, e := migrate.Apply(t.Context(), location.String(), current); e != nil || len(applied) != 0 {
		t.Fatal("repeat context migration", e)
	}
	if e := migrate.Check(t.Context(), f.Admin, previous); e != nil {
		t.Fatal("previous image additive compatibility", e)
	}
	page, e := contextStore(fx).ListAccountContexts(t.Context(), app.CompanyActor{AccountID: actor.AccountID, SessionID: actor.SessionID}, 100, "")
	if e != nil || len(page.Organizations) != 1 || page.Organizations[0].MembershipID != fx.member {
		t.Fatal("populated current discovery after upgrade", e)
	}
}
