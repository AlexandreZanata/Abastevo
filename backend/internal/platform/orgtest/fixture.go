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
	// Installed device projections run in every existing membership-ending
	// transaction. The ordinary fixture role needs the exact source effect
	// privileges even when the selected membership has no enrolled device.
	var devices bool
	if err := f.Admin.QueryRow(ctx, `SELECT to_regclass('public.org_device_enrollments') IS NOT NULL`).Scan(&devices); err != nil {
		t.Fatal(err)
	}
	if devices {
		if _, err := f.Admin.Exec(ctx, "GRANT SELECT,UPDATE ON org_device_enrollments TO "+pgx.Identifier{f.RuntimeRole}.Sanitize()); err != nil {
			t.Fatal(err)
		}
	}

	f.GrantFleetUsage(t)
	f.GrantMeterUsage(t)

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
	f.GrantInvitationLifecycle(t)
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

// GrantInvitationLifecycle is limited to this disposable fixture runtime. It
// preserves previous-chain upgrade fixtures and is not production provisioning.
func (f *Fixture) GrantInvitationLifecycle(t *testing.T) {
	t.Helper()
	var installed bool
	if e := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('public.org_invitations') IS NOT NULL`).Scan(&installed); e != nil {
		t.Fatal(e)
	}
	if installed {
		role := pgx.Identifier{f.RuntimeRole}.Sanitize()
		if _, e := f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON org_invitations TO "+role+";GRANT SELECT,INSERT ON org_invitation_commands,org_invitation_notices TO "+role); e != nil {
			t.Fatal(e)
		}
	}
	var redemption bool
	if e := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('public.org_invitation_acceptances') IS NOT NULL`).Scan(&redemption); e != nil {
		t.Fatal(e)
	}
	if redemption {
		role := pgx.Identifier{f.RuntimeRole}.Sanitize()
		if _, e := f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_invitation_locators,org_invitation_person_commands,org_invitation_acceptance_notices TO "+role+";GRANT SELECT,INSERT,UPDATE ON org_invitation_acceptances TO "+role); e != nil {
			t.Fatal(e)
		}
	}

}

// GrantMembershipAccessCommands enables only this disposable runtime's new
// manual lifecycle receipts/notices, never production role provisioning.
func (f *Fixture) GrantMembershipAccessCommands(t *testing.T) {
	t.Helper()
	var installed bool
	if e := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('public.org_membership_access_commands') IS NOT NULL`).Scan(&installed); e != nil {
		t.Fatal(e)
	}
	if installed {
		if _, e := f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_membership_access_commands,org_membership_access_notices TO "+pgx.Identifier{f.RuntimeRole}.Sanitize()); e != nil {
			t.Fatal(e)
		}
	}
}

// GrantOffboardingCommands is disposable test provisioning, never runtime grants.
func (f *Fixture) GrantOffboardingCommands(t *testing.T) {
	t.Helper()
	var installed bool
	if e := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('public.org_offboarding_commands') IS NOT NULL`).Scan(&installed); e != nil {
		t.Fatal(e)
	}
	if installed {
		if _, e := f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_offboarding_commands,org_offboarding_notices TO "+pgx.Identifier{f.RuntimeRole}.Sanitize()); e != nil {
			t.Fatal(e)
		}
	}
}

// GrantRehireCommands provisions only the disposable ordinary test role.
func (f *Fixture) GrantRehireCommands(t *testing.T) {
	t.Helper()
	var installed bool
	if e := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('public.org_employee_rehire_commands') IS NOT NULL`).Scan(&installed); e != nil {
		t.Fatal(e)
	}
	if installed {
		if _, e := f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_employee_rehire_commands TO "+pgx.Identifier{f.RuntimeRole}.Sanitize()); e != nil {
			t.Fatal(e)
		}
	}
}

// GrantEmployeeMergeCommands provisions only the disposable ordinary test role.
func (f *Fixture) GrantEmployeeMergeCommands(t *testing.T) {
	t.Helper()
	var installed bool
	if e := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('public.org_employee_merge_commands') IS NOT NULL`).Scan(&installed); e != nil {
		t.Fatal(e)
	}
	if installed {
		if _, e := f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_employee_merge_commands TO "+pgx.Identifier{f.RuntimeRole}.Sanitize()); e != nil {
			t.Fatal(e)
		}
	}
}

// GrantDeviceCommands provisions only the disposable ordinary test role. Source
// rows retain real locks/proof checks; no privileged verifier replaces them.
func (f *Fixture) GrantDeviceCommands(t *testing.T) {
	t.Helper()
	var installed bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('public.org_device_commands') IS NOT NULL`).Scan(&installed); err != nil {
		t.Fatal(err)
	}
	if installed {
		role := pgx.Identifier{f.RuntimeRole}.Sanitize()
		if _, err := f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_device_commands TO "+role+
			";GRANT SELECT,INSERT,UPDATE ON org_device_enrollments TO "+role+
			";GRANT SELECT,UPDATE ON accounts,account_session_families,identity_keys,identity_contributors,identity_challenges,account_contributor_bindings TO "+role+
			";GRANT SELECT ON payment_billing_exits TO "+role); err != nil {
			t.Fatal(err)
		}
	}
}

// GrantFleetUsage provisions only the columns read by the installed current
// unit-usage projection. Earlier embedded migration images remain unchanged.
func (f *Fixture) GrantFleetUsage(t *testing.T) {
	t.Helper()
	var installed bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('public.org_assets') IS NOT NULL AND to_regclass('public.org_asset_unit_allocations') IS NOT NULL`).Scan(&installed); err != nil {
		t.Fatal(err)
	}
	if installed {
		if _, err := f.Admin.Exec(t.Context(), "GRANT SELECT(org_id,id,state) ON org_assets TO "+pgx.Identifier{f.RuntimeRole}.Sanitize()+";GRANT SELECT(org_id,asset_id,unit_id,ended_at) ON org_asset_unit_allocations TO "+pgx.Identifier{f.RuntimeRole}.Sanitize()); err != nil {
			t.Fatal(err)
		}
	}
}

// GrantMeterUsage adds only the actual columns read by installed asset-use and
// meter-presence guards. It does not authorize reading observation evidence.
func (f *Fixture) GrantMeterUsage(t *testing.T) {
	t.Helper()
	var installed bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('public.org_meters') IS NOT NULL AND to_regclass('public.org_meter_readings') IS NOT NULL`).Scan(&installed); err != nil {
		t.Fatal(err)
	}
	if installed {
		role := pgx.Identifier{f.RuntimeRole}.Sanitize()
		if _, err := f.Admin.Exec(t.Context(), "GRANT SELECT(org_id,id,asset_id,installed_at,installed_submicro,removed_at) ON org_meters TO "+role+";GRANT SELECT(org_id,meter_id) ON org_meter_readings TO "+role); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('public.org_meter_held_readings') IS NOT NULL`).Scan(&installed); err != nil {
		t.Fatal(err)
	}
	if installed {
		// Asset usage counts held facts; the reading guard checks the reserved
		// held identity. Neither exposes any held value or observation.
		if _, err := f.Admin.Exec(t.Context(), "GRANT SELECT(org_id,asset_id,reading_id,operation_id) ON org_meter_held_readings TO "+pgx.Identifier{f.RuntimeRole}.Sanitize()); err != nil {
			t.Fatal(err)
		}
	}
}

// GrantMeterReviews enables current read projections only. The owning command
// fixture must explicitly grant inserts and the item-row lock for decisions.
func (f *Fixture) GrantMeterReviews(t *testing.T) {
	t.Helper()
	var installed bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('public.org_sync_review_items') IS NOT NULL AND to_regclass('public.org_meter_held_readings') IS NOT NULL AND to_regclass('public.org_sync_review_resolutions') IS NOT NULL`).Scan(&installed); err != nil {
		t.Fatal(err)
	}
	if installed {
		if _, err := f.Admin.Exec(t.Context(), "GRANT SELECT ON org_sync_review_items,org_meter_held_readings,org_sync_review_resolutions,org_sync_operations,org_sync_acknowledgements TO "+pgx.Identifier{f.RuntimeRole}.Sanitize()); err != nil {
			t.Fatal(err)
		}
	}
}
