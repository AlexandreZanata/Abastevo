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
	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	account "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	accountd "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	cloudpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/adapters"
	cloudapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/application"
	cloud "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/domain"
	identitypg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters"
	memberpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

func TestSyncIntakeOrdinaryPopulatedUpgradeRollbackRecovery(t *testing.T) {
	const name = "000196_organization_sync_intake.sql"
	previous := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() != "schema_min_supported" && (entry.Name() >= name || !strings.HasSuffix(entry.Name(), ".sql")) {
			continue
		}
		raw, err := fs.ReadFile(migrations.Files, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		previous[entry.Name()] = &fstest.MapFile{Data: raw}
	}
	f := orgtest.NewWithMigrations(t, previous)
	_, _, onboarding, actor, flags := onboardingOnFixture(t, f)
	legal := legalFixtureFor(t, f, onboarding, actor, flags)
	f.GrantDeviceCommands(t)
	if _, _, err := legal.onboarding.Repository.ActivateOrganization(t.Context(), legal.scope, actor, legal.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	catalog, err := audit.NewCatalog(memberpg.DeviceAuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	devices := &memberpg.DeviceStore{Pool: f.Runtime,
		Authority: &orgpg.DeviceAuthority{OffboardingAuthority: orgpg.OffboardingAuthority{MembershipAccessAuthority: orgpg.MembershipAccessAuthority{Memberships: legal.store.Memberships, Roles: legal.store.Roles, Capabilities: flags}}},
		Audit:     auditpg.NewStore(f.Runtime, catalog), Capabilities: flags, NewID: jobs.NewUUIDv4}
	accounts := &account.Service{Clock: onboardingClock{time.Now().UTC().Truncate(time.Second)}, Store: accountpg.NewPGStore(f.Admin), Hasher: accountd.SHA256Hasher{}, IDGen: jobs.NewUUIDv4, TokenGen: accountd.GenerateToken, KeyGen: accountd.GenerateAccountKey}
	fx := deviceFixtureState{legalFixtureState: legal, devices: devices, accounts: accounts, registrar: identitypg.NewRegistrar(f.Admin)}
	fx.person = deviceNewPerson(t, fx)
	if _, err := devices.EnrollDevice(t.Context(), fx.scope, fx.person.actor, deviceEnrollCommand(t, "Synthetic existing schema195 enrollment")); err != nil {
		t.Fatal(err)
	}
	snapshot := func() string {
		t.Helper()
		var value string
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array(
 (SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id)FROM organizations x),
 (SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_memberships x),
 (SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_role_assignments x),
 (SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_device_enrollments x),
 (SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_device_commands x),
 (SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_audit_events x),
 (SELECT jsonb_agg(to_jsonb(x)ORDER BY id)FROM accounts x),
 (SELECT jsonb_agg(to_jsonb(x)ORDER BY id)FROM account_session_families x))::text`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	before := snapshot()
	migrator := f.RuntimeRole + "_sync_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE,CREATE ON SCHEMA public TO "+role+";GRANT SELECT,INSERT ON schema_migrations TO "+role+";GRANT REFERENCES ON organizations,org_memberships,org_device_enrollments,org_audit_events TO "+role); err != nil {
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
	if _, err := migrate.Apply(t.Context(), location.String(), broken); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
		t.Fatal("late intake migration fault not reached", err)
	}
	var clean bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('org_sync_operations') IS NULL AND to_regclass('org_sync_acknowledgements') IS NULL AND to_regclass('org_sync_effects') IS NULL AND to_regprocedure('org_sync_keep_history()') IS NULL AND to_regprocedure('org_sync_operation_source()') IS NULL AND to_regprocedure('org_sync_ack_source()') IS NULL AND to_regprocedure('org_sync_effect_source()') IS NULL AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version='000196')`).Scan(&clean); err != nil || !clean || snapshot() != before {
		t.Fatal("failed intake upgrade left partial schema or changed existing source", err)
	}
	current := fstest.MapFS{}
	for key, value := range previous {
		current[key] = value
	}
	current[name] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 1 || applied[0] != "000196" {
		t.Fatal("ordinary intake migration recovery", applied, err)
	}
	if snapshot() != before {
		t.Fatal("intake migration rewrote populated identity or enterprise history")
	}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 0 {
		t.Fatal("repeat intake migration", err)
	}
	if err := migrate.Check(t.Context(), f.Admin, previous); err != nil {
		t.Fatal("previous embedded image rejected additive intake schema", err)
	}
	var valid bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT
 (SELECT count(*)=3 AND bool_and(relrowsecurity AND relforcerowsecurity)FROM pg_class WHERE oid IN('org_sync_operations'::regclass,'org_sync_acknowledgements'::regclass,'org_sync_effects'::regclass))
 AND NOT has_table_privilege($1,'org_sync_operations','SELECT')
 AND NOT has_table_privilege($1,'org_sync_acknowledgements','SELECT')
 AND NOT has_table_privilege($1,'org_sync_effects','SELECT')
 AND NOT EXISTS(SELECT 1 FROM org_sync_operations)
 AND NOT EXISTS(SELECT 1 FROM org_sync_acknowledgements)
 AND NOT EXISTS(SELECT 1 FROM org_sync_effects)
 AND NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname=$2 AND(rolsuper OR rolbypassrls OR rolcanlogin))`, f.RuntimeRole, migrator).Scan(&valid); err != nil || !valid {
		t.Fatal("intake migration fabricated records, grants or unsafe owner", err)
	}
	if _, err := devices.CheckDevice(t.Context(), fx.scope, fx.person.actor); err != nil {
		t.Fatal("previous signed current-device source stopped after upgrade", err)
	}
}

func TestSyncIntakePersonalErasurePreservesCompanyHistoryAndExportBoundary(t *testing.T) {
	fx := syncFixture(t)
	op := syncOperation(t, fx, onboardingID(t), "Synthetic private company fact", nil, nil, nil)
	ack, err := fx.store.Process(t.Context(), fx.source, op, protocol.DefaultLimits(), fx.handler)
	if err != nil || ack.Result() != protocol.Accepted {
		t.Fatal("company receipt before personal erasure", err)
	}
	snapshot := func() string {
		t.Helper()
		var value string
		if err := fx.f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array(
 (SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_sync_operations x),
 (SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_sync_acknowledgements x),
 (SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_sync_effects x),
 (SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_audit_events x),
 (SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_sync_test_records x))::text`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	before := snapshot()
	runtime := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT ON cloud_records TO "+runtime); err != nil {
		t.Fatal(err)
	}
	vehicle := onboardingID(t)
	if _, err := fx.f.Admin.Exec(t.Context(), `INSERT INTO cloud_records(account_id,dataset,id,payload)VALUES($1,'vehicles',$2,'{"name":"Synthetic personal vehicle"}')`, fx.person.auth.Account.ID, vehicle); err != nil {
		t.Fatal(err)
	}
	personal := cloudpg.NewStore(fx.f.Runtime)
	principal := cloudapp.Principal{AccountID: fx.person.auth.Account.ID, FamilyID: fx.person.auth.Session.FamilyID}
	rows, err := personal.List(t.Context(), principal, "vehicles", 100, "")
	if err != nil || len(rows) != 1 || rows[0].ID != vehicle {
		t.Fatal("personal export did not retain its own inventory", err)
	}
	for _, dataset := range []string{"org_sync_operations", "org_sync_acknowledgements", "org_sync_effects", "enterprise"} {
		if rows, err := personal.List(t.Context(), principal, dataset, 100, ""); !errors.Is(err, cloud.ErrInvalid) || len(rows) != 0 {
			t.Fatal("personal export admitted private company inventory")
		}
	}
	for _, dataset := range cloud.Datasets() {
		rows, err := personal.List(t.Context(), principal, dataset, 100, "")
		if err != nil {
			t.Fatal("personal dataset export", err)
		}
		for _, row := range rows {
			if row.ID == op.ID() || row.ID == fx.target || strings.Contains(string(row.Payload), "Synthetic private company fact") {
				t.Fatal("private company content entered personal export")
			}
		}
	}
	// Exercise the actual installed account-deletion projections using only the
	// exact grants already required by their existing erasure acceptance test.
	for _, grant := range []string{
		"GRANT SELECT,INSERT,UPDATE ON cloud_state,driver_earning_owner_state TO ",
		"GRANT SELECT,INSERT ON cloud_erased_ids,cloud_device_revocations,subscription_retired_tokens TO ",
		"GRANT SELECT,UPDATE ON cloud_records,cloud_changes,cloud_devices,entitlement_grants,contributor_github_links,staff_assignments,subscription_purchases,subscription_operations,subscription_verification_slots TO ",
		"GRANT SELECT,DELETE ON driver_earning_adjustments,driver_earning_revisions,driver_earnings,driver_earning_commands,driver_earning_tombstones,driver_distance_commands,driver_distance_observations,driver_shift_commands,driver_shift_gaps,driver_shifts,driver_cost_profile_items,driver_cost_commands,driver_cost_tombstones,driver_cost_profiles,driver_maintenance_commands,driver_maintenance_tombstones,driver_maintenance_schedules,driver_reconciliation_commands,driver_reconciliation_records,driver_reconciliation_decisions,driver_report_snapshots,contributor_claims,contributor_profile_edits,contributor_links,contributor_profiles,forum_comment_reactions,forum_post_reactions TO ",
	} {
		if _, err := fx.f.Admin.Exec(t.Context(), grant+runtime); err != nil {
			t.Fatal(err)
		}
	}
	eraser := fx.f.RuntimeRole + "_sync_eraser"
	role := pgx.Identifier{eraser}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE ON SCHEMA public TO "+role+";GRANT SELECT,UPDATE ON organizations,accounts,org_role_assignments,org_teams,org_team_links,org_ownership_transfers,org_device_enrollments TO "+role+";GRANT SELECT ON org_employee_identities,payment_billing_exits TO "+role+";GRANT SELECT,INSERT ON org_settings_versions,org_settings_designations,org_settings_notices,org_ownership_notices,org_invitation_notices,org_invitation_acceptance_notices TO "+role+";GRANT SELECT,INSERT,UPDATE ON org_settings_heads TO "+role+";GRANT SELECT,UPDATE ON org_invitations,org_invitation_acceptances TO "+role+";ALTER TABLE org_memberships OWNER TO "+role+";ALTER FUNCTION org_membership_account_erasure() OWNER TO "+role+";ALTER FUNCTION org_membership_check_account_exit(uuid) OWNER TO "+role); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := fx.f.Admin.Exec(ctx, "REASSIGN OWNED BY "+role+" TO CURRENT_USER;DROP OWNED BY "+role+";DROP ROLE "+role); err != nil {
			t.Error(err)
		}
	})
	caller := onboardingID(t)
	if err := pgx.BeginFunc(t.Context(), fx.f.Runtime, func(tx pgx.Tx) error {
		if _, err := tx.Exec(t.Context(), `SELECT set_config('anpfuel.org_id',$1,true)`, caller); err != nil {
			return err
		}
		if _, err := tx.Exec(t.Context(), `UPDATE accounts SET status='deleted' WHERE id=$1`, fx.person.auth.Account.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(t.Context(), `SET CONSTRAINTS ALL IMMEDIATE`); err != nil {
			return err
		}
		var restored string
		if err := tx.QueryRow(t.Context(), `SELECT current_setting('anpfuel.org_id',true)`).Scan(&restored); err != nil {
			return err
		}
		if restored != caller {
			t.Error("personal erasure leaked organization context")
		}
		return nil
	}); err != nil {
		t.Fatal("ordinary personal account deletion", err)
	}
	if snapshot() != before {
		t.Fatal("personal deletion rewrote private enterprise history")
	}
	var reduced bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT m.state='ENDED' AND m.account_id IS NULL AND m.account_tombstone IS NOT NULL AND d.state='REVOKED' AND d.revocation_class='ACCESS_ENDED' FROM org_memberships m JOIN org_device_enrollments d ON(d.org_id,d.membership_id)=(m.org_id,m.id) WHERE m.org_id=$1 AND m.id=$2 AND d.id=$3`, fx.scope.OrgID().String(), fx.person.member, fx.source.EnrollmentID).Scan(&reduced); err != nil || !reduced {
		t.Fatal("personal erasure failed to unlink current enterprise source", err)
	}
	if got, err := fx.store.Process(t.Context(), fx.source, op, protocol.DefaultLimits(), fx.handler); !errors.Is(err, syncapp.ErrSourceDenied) || len(got.CanonicalBytes()) != 0 {
		t.Fatal("unlinked source disclosed retained private receipt")
	}
	var cleared bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT NOT EXISTS(SELECT 1 FROM cloud_records WHERE account_id=$1 AND payload IS NOT NULL)`, fx.person.auth.Account.ID).Scan(&cleared); err != nil || !cleared {
		t.Fatal("personal erasure retained personal content", err)
	}
}
