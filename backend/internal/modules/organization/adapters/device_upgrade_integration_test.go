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
	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/membership"
	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	account "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	accountd "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	identitypg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters"
	memberpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

func TestOrganizationDevicesOrdinaryPopulatedUpgradeRollbackRecovery(t *testing.T) {
	const name = "000194_organization_devices.sql"
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
	if _, _, e := old.onboarding.Repository.ActivateOrganization(t.Context(), old.scope, old.actor, old.member, onboardingID(t), 1); e != nil {
		t.Fatal(e)
	}
	accounts := &account.Service{Clock: onboardingClock{time.Now().UTC().Truncate(time.Second)}, Store: accountpg.NewPGStore(f.Admin), Hasher: accountd.SHA256Hasher{}, IDGen: jobs.NewUUIDv4, TokenGen: accountd.GenerateToken, KeyGen: accountd.GenerateAccountKey}
	fx := deviceFixtureState{legalFixtureState: old, accounts: accounts, registrar: identitypg.NewRegistrar(f.Admin)}
	fx.person = deviceNewPerson(t, fx)
	snapshot := func() string {
		t.Helper()
		var out string
		if e := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array(
  (SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_employee_identities x),
  (SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_memberships x),
  (SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_role_assignments x),
  (SELECT jsonb_agg(to_jsonb(x)ORDER BY id)FROM accounts x),
  (SELECT jsonb_agg(to_jsonb(x)ORDER BY id)FROM account_session_families x),
  (SELECT jsonb_agg(to_jsonb(x)ORDER BY id)FROM identity_keys x),
  (SELECT jsonb_agg(to_jsonb(x)ORDER BY account_id,contributor_id)FROM account_contributor_bindings x),
  (SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_audit_events x))::text`).Scan(&out); e != nil {
			t.Fatal(e)
		}
		return out
	}
	before := snapshot()
	migrator := f.RuntimeRole + "_device_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	if _, e := f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE,CREATE ON SCHEMA public TO "+role+";GRANT SELECT,INSERT ON schema_migrations TO "+role+";GRANT REFERENCES ON accounts,identity_keys,identity_contributors,org_memberships,org_audit_events TO "+role+";ALTER TABLE org_memberships OWNER TO "+role); e != nil {
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
		t.Fatal("injected device migration failure not reached", e)
	}
	var clean bool
	if e := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('org_device_enrollments') IS NULL AND to_regclass('org_device_commands') IS NULL AND to_regprocedure('org_device_enrollment_guard()') IS NULL AND to_regprocedure('org_device_command_guard()') IS NULL AND to_regprocedure('org_device_command_source_guard()') IS NULL AND NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='org_device_membership_ended') AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version='000194')`).Scan(&clean); e != nil || !clean || snapshot() != before {
		t.Fatal("partial failed device upgrade", e)
	}
	current := fstest.MapFS{}
	for k, v := range previous {
		current[k] = v
	}
	current[name] = &fstest.MapFile{Data: raw}
	if applied, e := migrate.Apply(t.Context(), location.String(), current); e != nil || len(applied) != 1 || applied[0] != "000194" {
		t.Fatal("device upgrade recovery", applied, e)
	}
	if snapshot() != before {
		t.Fatal("device migration fabricated or rewrote existing identity/access history")
	}
	if applied, e := migrate.Apply(t.Context(), location.String(), current); e != nil || len(applied) != 0 {
		t.Fatal("device repeat upgrade", e)
	}
	if e := migrate.Check(t.Context(), f.Admin, previous); e != nil {
		t.Fatal("previous image rejected additive device schema", e)
	}
	var devices int
	if e := f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_device_enrollments`).Scan(&devices); e != nil || devices != 0 {
		t.Fatal("device enrollment invented by migration", e)
	}
	// Upgrade does not fabricate runtime privileges for installed source effects.
	teamSQLCode(t, unitSchemaExec(t, old, `SELECT count(*) FROM org_device_enrollments`), "42501")
	f.GrantDeviceCommands(t)
	catalog, e := audit.NewCatalog(memberpg.DeviceAuditDefinitions())
	if e != nil {
		t.Fatal(e)
	}
	fx.devices = &memberpg.DeviceStore{Pool: f.Runtime, Authority: &orgpg.DeviceAuthority{OffboardingAuthority: orgpg.OffboardingAuthority{MembershipAccessAuthority: orgpg.MembershipAccessAuthority{Memberships: old.store.Memberships, Roles: old.store.Roles, Capabilities: flags}}}, Audit: auditpg.NewStore(f.Runtime, catalog), Capabilities: flags, NewID: jobs.NewUUIDv4}
	enrolled, e := fx.devices.EnrollDevice(t.Context(), old.scope, fx.person.actor, deviceEnrollCommand(t, "Synthetic upgrade device"))
	if e != nil {
		t.Fatal("ordinary upgraded enrollment", e)
	}
	state, e := enrolled.Device.Enrollment.Snapshot(old.scope)
	if e != nil {
		t.Fatal(e)
	}
	// The ordinary migration owner still obeys FORCE RLS after rows exist.
	tx, e := f.Admin.Begin(t.Context())
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(context.Background())
	if _, e := tx.Exec(t.Context(), "SET LOCAL ROLE "+role); e != nil {
		t.Fatal(e)
	}
	if e := tx.QueryRow(t.Context(), `SELECT count(*) FROM org_device_enrollments`).Scan(&devices); e != nil || devices != 0 {
		t.Fatal("device owner bypassed RLS", e)
	}
	if _, e := tx.Exec(t.Context(), `SELECT set_config('anpfuel.org_id',$1,true)`, old.scope.OrgID().String()); e != nil {
		t.Fatal(e)
	}
	if e := tx.QueryRow(t.Context(), `SELECT count(*) FROM org_device_enrollments`).Scan(&devices); e != nil || devices != 1 {
		t.Fatal("scoped migration owner lost row", e)
	}
	_ = tx.Rollback(t.Context())
	var receiptBefore, receiptAfter string
	if e := f.Admin.QueryRow(t.Context(), `SELECT result::text FROM org_device_commands WHERE org_id=$1 AND operation_id=$2`, old.scope.OrgID().String(), enrolled.OperationID).Scan(&receiptBefore); e != nil {
		t.Fatal(e)
	}
	// A pre-existing ordinary membership ending path consumes installed devices.
	if e := unitSchemaExec(t, old, `UPDATE org_memberships SET state='ENDED',revision=revision+1,end_reason='OFFBOARDED' WHERE org_id=$1 AND id=$2`, old.scope.OrgID().String(), fx.person.member); e != nil {
		t.Fatal("ordinary legacy ending did not revoke device", e)
	}
	var revokedState, sourceType, sourceID, class string
	var revision, sourceRevision int64
	if e := f.Admin.QueryRow(t.Context(), `SELECT state,revision,source_type,source_id::text,source_revision,revocation_class FROM org_device_enrollments WHERE org_id=$1 AND id=$2`, old.scope.OrgID().String(), state.ID).Scan(&revokedState, &revision, &sourceType, &sourceID, &sourceRevision, &class); e != nil || revokedState != "REVOKED" || revision != 2 || sourceType != "MEMBERSHIP" || sourceID != fx.person.member || sourceRevision != 3 || class != string(d.AccessEnded) {
		t.Fatal("device end source not retained", e)
	}
	if e := f.Admin.QueryRow(t.Context(), `SELECT result::text FROM org_device_commands WHERE org_id=$1 AND operation_id=$2`, old.scope.OrgID().String(), enrolled.OperationID).Scan(&receiptAfter); e != nil || receiptBefore != receiptAfter {
		t.Fatal("later membership ending rewrote original device receipt", e)
	}
	if _, e := accounts.ValidateAccess(t.Context(), fx.person.auth.Session.FamilyID, fx.person.auth.Session.AccessToken); e != nil {
		t.Fatal("organization device changed personal family", e)
	}
	if _, e := f.Admin.Exec(t.Context(), "GRANT UPDATE,DELETE,TRUNCATE ON org_device_enrollments,org_device_commands TO "+pgx.Identifier{f.RuntimeRole}.Sanitize()); e != nil {
		t.Fatal(e)
	}
	for _, query := range []string{`DELETE FROM org_device_enrollments`, `DELETE FROM org_device_commands`, `UPDATE org_device_commands SET operation_id=operation_id`, `TRUNCATE org_device_commands`, `TRUNCATE org_device_enrollments CASCADE`} {
		teamSQLCode(t, unitSchemaExec(t, old, query), "55000")
	}
	// No source setting exposes these records without a selected organization.
	if e := f.Runtime.QueryRow(t.Context(), `SELECT count(*) FROM org_device_enrollments`).Scan(&devices); e != nil || devices != 0 {
		t.Fatal("unscoped device visibility", e)
	}
}

// These probes use actual registered/bound keys and an ordinary scoped role.
// SQL source guards must reject missing/forged acknowledgements independently
// of application DTO validation and roll back the real audit and row together.
func TestOrganizationDeviceSchemaBindsActualCommandAtCommit(t *testing.T) {
	for _, kind := range []string{"missing_command", "null_result"} {
		t.Run(kind, func(t *testing.T) {
			fx := devicesFixture(t)
			operation, id := onboardingID(t), onboardingID(t)
			hash := strings.Repeat("e", 64)
			err := database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
				if _, e := tx.Exec(t.Context(), `INSERT INTO org_audit_events(org_id,operation_id,actor_id,action,target_type,target_id,reason_code,request_hash)VALUES($1,$2,$3,'idn.device.enrolled','device',$4,'enrolled',$5)`, fx.scope.OrgID().String(), operation, fx.person.auth.Account.ID, id, hash); e != nil {
					return e
				}
				if e := q.New(tx).InsertDevice(t.Context(), q.InsertDeviceParams{OrgID: fx.scope.OrgID().String(), ID: id, MembershipID: fx.person.member, KeyID: fx.person.signer.keyID, ContributorID: fx.person.signer.contributor, Fingerprint: fx.person.signer.fingerprint, Label: "Synthetic guarded source", EnrollmentLimit: 3, OperationID: operation}); e != nil {
					return e
				}
				if kind == "missing_command" {
					return nil
				}
				_, e := tx.Exec(t.Context(), `INSERT INTO org_device_commands(org_id,operation_id,actor_id,actor_key_id,device_id,kind,request_hash,reason,result)
    SELECT org_id,$2,$3,$4,id,'ENROLL',$5,'',jsonb_build_object('id',NULL,'membership_id',membership_id::text,'label',label,'state',state,'revision',revision,'revocation_class',revocation_class,'enrolled_at',enrolled_at,'last_sync_at',NULL,'revoked_at',NULL,'source_type','','source_id','','source_revision',0)
    FROM org_device_enrollments WHERE org_id=$1 AND id=$6`, fx.scope.OrgID().String(), operation, fx.person.auth.Account.ID, fx.person.signer.keyID, hash, id)
				return e
			})
			teamSQLCode(t, err, "P9005")
			var left int
			if e := fx.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_device_enrollments WHERE org_id=$1 AND id=$2)+(SELECT count(*) FROM org_device_commands WHERE org_id=$1 AND operation_id=$3)+(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND operation_id=$3)`, fx.scope.OrgID().String(), id, operation).Scan(&left); e != nil || left != 0 {
				t.Fatal("failed device source left an audit, enrollment or receipt", e)
			}
		})
	}
}
