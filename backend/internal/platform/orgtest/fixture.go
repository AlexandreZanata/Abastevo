//go:build integration

// Package orgtest provides disposable PostGIS and two synthetic organizations
// for private enterprise adapter tests. It requires an explicit test DSN.
package orgtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

// Organization contains the shared scope and deliberately colliding record keys.
type Organization struct {
	Key      string `json:"key"`
	ID       string `json:"id"`
	ParentID string `json:"parent_id"`
	RecordID string `json:"record_id"`
	Number   string `json:"number"`
}

// Scope returns this fixture organization's validated scope.
func (org Organization) Scope(t *testing.T) kernel.OrgScope {
	t.Helper()
	id, err := kernel.ParseOrgID(org.ID)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(id)
	if err != nil {
		t.Fatal(err)
	}
	return scope
}

// Fixture owns a fresh migrated database and a separate ordinary runtime login.
// AdminDSN is only for rehearsing migrations against this disposable database.
type Fixture struct {
	AdminDSN      string
	Admin         *pgxpool.Pool
	Runtime       *pgxpool.Pool
	RuntimeRole   string
	Organizations []Organization
	runtimeConfig *pgxpool.Config
}

// New creates and cleans up only its own uniquely named database and login.
// The supplied DSN must permit CREATE DATABASE/ROLE on disposable PostGIS.
func New(t *testing.T) *Fixture {
	t.Helper()
	return NewWithMigrations(t, dbmigrations.Files)
}

// NewWithMigrations rehearses an upgrade from a caller-supplied previous chain.
func NewWithMigrations(t *testing.T, files fs.FS) *Fixture {
	t.Helper()
	dsn := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("orgtest requires an explicit disposable ANPFUEL_TEST_DATABASE_URL")
	}
	location, err := url.Parse(dsn)
	if err != nil || (location.Scheme != "postgres" && location.Scheme != "postgresql") {
		t.Fatal("orgtest requires a PostgreSQL URL")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	control, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal("orgtest cannot connect to disposable PostGIS")
	}
	suffix := randomHex(t, 8)
	databaseName := "emp_isolation_" + suffix
	f := &Fixture{RuntimeRole: "emp_runtime_" + suffix}
	t.Cleanup(func() {
		if f.Admin != nil {
			f.Admin.Close()
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		defer control.Close(cleanupCtx)
		if _, err := control.Exec(cleanupCtx, "DROP DATABASE IF EXISTS "+pgx.Identifier{databaseName}.Sanitize()+" WITH (FORCE)"); err != nil {
			t.Errorf("orgtest database cleanup failed: %v", err)
		}
		if _, err := control.Exec(cleanupCtx, "DROP ROLE IF EXISTS "+pgx.Identifier{f.RuntimeRole}.Sanitize()); err != nil {
			t.Errorf("orgtest role cleanup failed: %v", err)
		}
	})
	if _, err := control.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{databaseName}.Sanitize()); err != nil {
		t.Fatalf("orgtest create database: %v", err)
	}
	password := randomHex(t, 24)
	// Both interpolated values are generated here. The identifier is quoted
	// and the password alphabet is restricted to hexadecimal.
	if _, err := control.Exec(ctx, "CREATE ROLE "+pgx.Identifier{f.RuntimeRole}.Sanitize()+
		" LOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE NOINHERIT PASSWORD '"+password+"'"); err != nil {
		t.Fatal("orgtest create runtime login failed")
	}
	location.Path = "/" + databaseName
	f.AdminDSN = location.String()
	if _, err := migrate.Apply(ctx, f.AdminDSN, files); err != nil {
		t.Fatalf("orgtest migrate fresh database: %v", err)
	}
	f.Admin, err = pgxpool.New(ctx, f.AdminDSN)
	if err != nil {
		t.Fatal("orgtest open migration pool failed")
	}
	var postgis string
	if err := f.Admin.QueryRow(ctx, "SELECT PostGIS_version()").Scan(&postgis); err != nil || postgis == "" {
		t.Fatal("orgtest requires actual PostGIS")
	}
	t.Logf("PostGIS %s", postgis)
	f.runtimeConfig, err = pgxpool.ParseConfig(f.AdminDSN)
	if err != nil {
		t.Fatal("orgtest parse runtime config failed")
	}
	f.runtimeConfig.ConnConfig.User = f.RuntimeRole
	f.runtimeConfig.ConnConfig.Password = password
	f.runtimeConfig.MinConns = 0
	f.Runtime = f.RuntimePool(t, 1)

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("orgtest repository location unavailable")
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "../../../../contracts/testdata/enterprise/fixture-two-orgs-v1.json"))
	if err != nil {
		t.Fatalf("orgtest read shared organizations: %v", err)
	}
	var data struct {
		Version       int            `json:"schema_version"`
		Organizations []Organization `json:"organizations"`
	}
	if err := json.Unmarshal(raw, &data); err != nil || data.Version != 1 || len(data.Organizations) != 2 {
		t.Fatal("orgtest requires the version 1 two-organization fixture")
	}
	f.Organizations = data.Organizations
	var roots bool
	if err := f.Admin.QueryRow(ctx, `SELECT to_regclass('public.organizations') IS NOT NULL`).Scan(&roots); err != nil {
		t.Fatal(err)
	}
	for _, org := range f.Organizations {
		org.Scope(t)
		if roots {
			// Only an ownership identity for synthetic foundation fixtures, not
			// invented organization metadata, memberships or activation.
			if _, err := f.Admin.Exec(ctx, `INSERT INTO organizations(org_id) VALUES($1) ON CONFLICT DO NOTHING`, org.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
	return f
}

// RuntimePool opens an additional ordinary-login pool with a bounded size.
func (f *Fixture) RuntimePool(t *testing.T, maxConns int32) *pgxpool.Pool {
	t.Helper()
	config := f.runtimeConfig.Copy()
	config.MaxConns = maxConns
	pool, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatal("orgtest open ordinary runtime pool failed")
	}
	t.Cleanup(pool.Close)
	return pool
}

func randomHex(t *testing.T, size int) string {
	t.Helper()
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		t.Fatal("orgtest random identifier unavailable")
	}
	return hex.EncodeToString(data)
}

// GrantTeamReductions provisions only this disposable runtime role's current
// employee/membership reduction projection. Previous-chain rehearsals keep
// their original grants when the owning migration has not been installed.
func (f *Fixture) GrantTeamReductions(t *testing.T) {
	t.Helper()
	var installed bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('public.org_team_links') IS NOT NULL`).Scan(&installed); err != nil {
		t.Fatal(err)
	}
	if installed {
		if _, err := f.Admin.Exec(t.Context(), "GRANT SELECT,UPDATE ON org_teams,org_team_links TO "+pgx.Identifier{f.RuntimeRole}.Sanitize()); err != nil {
			t.Fatal(err)
		}
	}
}

// GrantSettingsLifecycle enables only this fixture's atomic onboarding and
// designation-reduction projections; previous migration chains stay unchanged.
func (f *Fixture) GrantSettingsLifecycle(t *testing.T) {
	t.Helper()
	var installed bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('public.org_settings_versions') IS NOT NULL`).Scan(&installed); err != nil {
		t.Fatal(err)
	}
	if installed {
		r := pgx.Identifier{f.RuntimeRole}.Sanitize()
		if _, err := f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_settings_versions,org_settings_designations,org_settings_notices TO "+r+"; GRANT SELECT,INSERT,UPDATE ON org_settings_heads TO "+r); err != nil {
			t.Fatal(err)
		}
	}
}

// GrantOwnershipLifecycle enables only this disposable runtime's ownership
// receipts and reduction projections when their additive tables exist. Prior-chain upgrade
// fixtures keep their original grants; this is not runtime provisioning.
func (f *Fixture) GrantOwnershipLifecycle(t *testing.T) {
	t.Helper()
	var transfers, notices, commands bool
	if e := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('public.org_ownership_transfers') IS NOT NULL,to_regclass('public.org_ownership_notices') IS NOT NULL,to_regclass('public.org_ownership_commands') IS NOT NULL`).Scan(&transfers, &notices, &commands); e != nil {
		t.Fatal(e)
	}
	role := pgx.Identifier{f.RuntimeRole}.Sanitize()
	if transfers {
		if _, e := f.Admin.Exec(t.Context(), "GRANT SELECT,UPDATE ON org_ownership_transfers TO "+role); e != nil {
			t.Fatal(e)
		}
	}
	if notices {
		if _, e := f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_ownership_notices TO "+role); e != nil {
			t.Fatal(e)
		}
	}
	if commands {
		if _, e := f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_ownership_commands TO "+role); e != nil {
			t.Fatal(e)
		}
	}
}
