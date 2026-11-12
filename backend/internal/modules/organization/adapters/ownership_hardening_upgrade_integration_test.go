//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"io/fs"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	accesspg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

func TestOwnershipHardeningOrdinaryPopulatedUpgradeFailureAndRecovery(t *testing.T) {
	const name = "000187_ownership_transfer_hardening.sql"
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
	fx := legalFixtureFor(t, f, onboarding, actor, flags)
	if _, e := f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON org_ownership_transfers TO "+pgx.Identifier{f.RuntimeRole}.Sanitize()); e != nil {
		t.Fatal(e)
	}
	if e := unitSchemaExec(t, fx, `UPDATE organizations SET state='ACTIVE',revision=revision+1 WHERE org_id=$1`, fx.scope.OrgID().String()); e != nil {
		t.Fatal(e)
	}
	account := onboardingID(t)
	if _, e := f.Admin.Exec(t.Context(), `INSERT INTO accounts(id,alias,status) VALUES($1,$2,'active')`, account, "synthetic-ownership-upgrade-"+account); e != nil {
		t.Fatal(e)
	}
	_, target := teamDriver(t, fx, account)
	transfer, operation := onboardingID(t), onboardingID(t)
	if e := unitSchemaExec(t, fx, `INSERT INTO org_ownership_transfers(org_id,transfer_id,initiator_membership_id,target_membership_id,step_down_to_admin,state,created_at,expires_at,operation_id) VALUES($1,$2,$3,$4,false,'PENDING','2000-01-01T00:00:00Z','2000-01-08T00:00:00Z',$5)`, fx.scope.OrgID().String(), transfer, fx.member, target, operation); e != nil {
		t.Fatal(e)
	}
	snapshot := func() string {
		t.Helper()
		var value string
		// Only historical columns are compared: additive cause fields deliberately
		// remain empty for legacy rows whose source was never recorded.
		if e := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id) FROM organizations x),(SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_memberships x),(SELECT jsonb_agg(jsonb_build_array(org_id,transfer_id,initiator_membership_id,target_membership_id,step_down_to_admin,state,expires_at,created_at,accepted_at,cancelled_at,declined_at,operation_id) ORDER BY org_id,transfer_id) FROM org_ownership_transfers),(SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,operation_id) FROM org_audit_events x))::text`).Scan(&value); e != nil {
			t.Fatal(e)
		}
		return value
	}
	guard := func() string {
		t.Helper()
		var value string
		if e := f.Admin.QueryRow(t.Context(), `SELECT pg_get_functiondef('org_ownership_transfer_guard()'::regprocedure)`).Scan(&value); e != nil {
			t.Fatal(e)
		}
		return value
	}
	before, oldGuard := snapshot(), guard()
	migrator := f.RuntimeRole + "_ownership_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	if _, e := f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE,CREATE ON SCHEMA public TO "+role+";GRANT SELECT,INSERT ON schema_migrations TO "+role+";GRANT REFERENCES ON accounts,org_audit_events TO "+role+";ALTER TABLE org_ownership_transfers OWNER TO "+role+";ALTER FUNCTION org_ownership_transfer_guard() OWNER TO "+role+";ALTER FUNCTION org_ownership_transfer_consume_membership() OWNER TO "+role+";ALTER FUNCTION org_ownership_transfer_consume_organization() OWNER TO "+role); e != nil {
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
	query := location.Query()
	query.Set("options", "-c role="+migrator)
	location.RawQuery = strings.ReplaceAll(query.Encode(), "+", "%20")
	raw, e := fs.ReadFile(migrations.Files, name)
	if e != nil {
		t.Fatal(e)
	}
	broken := fstest.MapFS{name: &fstest.MapFile{Data: append(append([]byte(nil), raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, e := migrate.Apply(t.Context(), location.String(), broken); e == nil || !strings.Contains(e.Error(), "SQLSTATE 22012") {
		t.Fatal("injected upgrade failure not reached", e)
	}
	var clean bool
	if e := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('org_ownership_commands') IS NULL AND to_regclass('org_ownership_notices') IS NULL AND NOT EXISTS(SELECT 1 FROM pg_attribute WHERE attrelid='org_ownership_transfers'::regclass AND attname='reduction_cause' AND NOT attisdropped) AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version='000187')`).Scan(&clean); e != nil || !clean || snapshot() != before || guard() != oldGuard {
		t.Fatal("failed upgrade left partial schema, history, or ledger", e)
	}
	current := fstest.MapFS{}
	for k, v := range previous {
		current[k] = v
	}
	current[name] = &fstest.MapFile{Data: raw}
	if applied, e := migrate.Apply(t.Context(), location.String(), current); e != nil || len(applied) != 1 || applied[0] != "000187" {
		t.Fatal("upgrade recovery", applied, e)
	}
	if snapshot() != before {
		t.Fatal("upgrade rewrote historical rows")
	}
	if applied, e := migrate.Apply(t.Context(), location.String(), current); e != nil || len(applied) != 0 {
		t.Fatal("repeated upgrade", e)
	}
	if e := migrate.Check(t.Context(), f.Admin, previous); e != nil {
		t.Fatal("prior image compatibility", e)
	}
	if e := f.Admin.QueryRow(t.Context(), `SELECT NOT rolsuper AND NOT rolbypassrls AND NOT rolcanlogin AND(SELECT count(*)=3 AND bool_and(relrowsecurity AND relforcerowsecurity AND relowner=(SELECT oid FROM pg_roles WHERE rolname=$1)) FROM pg_class WHERE oid IN('org_ownership_transfers'::regclass,'org_ownership_commands'::regclass,'org_ownership_notices'::regclass)) FROM pg_roles WHERE rolname=$1`, migrator).Scan(&clean); e != nil || !clean {
		t.Fatal("ordinary forced RLS ownership", e)
	}
	f.GrantOwnershipLifecycle(t)
	catalog, e := audit.NewCatalog(orgpg.OwnershipTransferAuditDefinitions())
	if e != nil {
		t.Fatal(e)
	}
	store := orgpg.NewOwnershipTransferStore(f.Runtime, accesspg.NewStore(f.Runtime), auditpg.NewStore(f.Runtime, catalog))
	store.Memberships, store.Capabilities = fx.store.Memberships, fx.flags
	if _, e := f.Admin.Exec(t.Context(), `CREATE FUNCTION synthetic_ownership_expiry_failure() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='synthetic expiry notice storage outage';END$$;CREATE CONSTRAINT TRIGGER synthetic_ownership_expiry_failure AFTER INSERT ON org_ownership_notices DEFERRABLE INITIALLY DEFERRED FOR EACH ROW WHEN(NEW.kind='EXPIRED') EXECUTE FUNCTION synthetic_ownership_expiry_failure()`); e != nil {
		t.Fatal(e)
	}
	if result, e := store.GetCurrent(t.Context(), fx.scope, fx.actor, fx.member, transfer); !errors.Is(e, app.ErrUnavailable) || result != (d.OwnershipTransfer{}) {
		t.Fatal("uncommitted expiry returned private content", e)
	}
	if e := f.Admin.QueryRow(t.Context(), `SELECT state='PENDING' AND NOT EXISTS(SELECT 1 FROM org_ownership_notices WHERE org_id=t.org_id AND transfer_id=t.transfer_id) FROM org_ownership_transfers t WHERE t.org_id=$1 AND t.transfer_id=$2`, fx.scope.OrgID().String(), transfer).Scan(&clean); e != nil || !clean {
		t.Fatal("failed expiry left state or notice", e)
	}
	if _, e := f.Admin.Exec(t.Context(), `DROP TRIGGER synthetic_ownership_expiry_failure ON org_ownership_notices`); e != nil {
		t.Fatal(e)
	}
	if result, e := store.GetCurrent(t.Context(), fx.scope, fx.actor, fx.member, transfer); e != nil || result.State != d.OwnershipTransferExpired || result.CreatedAt.Year() != 2000 {
		t.Fatal("honest legacy expiry read recovery", e)
	}
	if e := f.Admin.QueryRow(t.Context(), `SELECT reduction_cause='' AND source_membership_id IS NULL AND source_membership_revision IS NULL AND(SELECT count(*) FROM org_ownership_notices WHERE org_id=t.org_id AND transfer_id=t.transfer_id)=1 FROM org_ownership_transfers t WHERE t.org_id=$1 AND t.transfer_id=$2`, fx.scope.OrgID().String(), transfer).Scan(&clean); e != nil || !clean {
		t.Fatal("upgrade invented source metadata or proposed delivery", e)
	}
	ownershipSchemaProposal(t, fx, target)
}
