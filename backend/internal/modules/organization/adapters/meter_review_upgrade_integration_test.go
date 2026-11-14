//go:build integration

package adapters_test

import (
	"bytes"
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
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	identitypg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters"
	memberpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	syncpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/adapters"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// meterReviewLegacyFixture builds the actual schema198 source: a real enrolled
// device, ACTIVE organization and ordinary runtime grants, with no review proof.
func meterReviewLegacyFixture(t *testing.T, f *orgtest.Fixture) syncFixtureState {
	t.Helper()
	_, _, onboarding, actor, flags := onboardingOnFixture(t, f)
	legal := legalFixtureFor(t, f, onboarding, actor, flags)
	f.GrantDeviceCommands(t)
	if _, _, err := onboarding.Repository.ActivateOrganization(t.Context(), legal.scope, legal.actor, legal.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	catalog, err := audit.NewCatalog(memberpg.DeviceAuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	devices := &memberpg.DeviceStore{Pool: f.Runtime, Authority: &orgpg.DeviceAuthority{OffboardingAuthority: orgpg.OffboardingAuthority{MembershipAccessAuthority: orgpg.MembershipAccessAuthority{Memberships: legal.store.Memberships, Roles: legal.store.Roles, Capabilities: flags}}}, Audit: auditpg.NewStore(f.Runtime, catalog), Capabilities: flags, NewID: jobs.NewUUIDv4}
	accounts := &account.Service{Clock: onboardingClock{time.Now().UTC().Truncate(time.Second)}, Store: accountpg.NewPGStore(f.Admin), Hasher: accountd.SHA256Hasher{}, IDGen: jobs.NewUUIDv4, TokenGen: accountd.GenerateToken, KeyGen: accountd.GenerateAccountKey}
	fx := deviceFixtureState{legalFixtureState: legal, devices: devices, accounts: accounts, registrar: identitypg.NewRegistrar(f.Admin)}
	fx.person = deviceNewPerson(t, fx)
	matrixAssign(t, fx, matrixPerson{member: fx.person.member}, "ADMIN", "ORG", "ACTIVE")
	enrolled, err := devices.EnrollDevice(t.Context(), fx.scope, fx.person.actor, deviceEnrollCommand(t, "Synthetic schema198 review source"))
	if err != nil {
		t.Fatal(err)
	}
	enrollment, err := enrolled.Device.Enrollment.Snapshot(fx.scope)
	if err != nil {
		t.Fatal(err)
	}
	ac, err := audit.NewCatalog(syncpg.AuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	source := syncapp.Source{Scope: fx.scope, Origin: deviceTestOrigin, Actor: fx.person.actor, MembershipID: fx.person.member, EnrollmentID: enrollment.ID}
	config := syncpg.Config{Pool: f.Runtime, Audit: auditpg.NewStore(f.Runtime, ac), AuditCatalog: ac, ConfirmSource: func(ctx context.Context, tx pgx.Tx, s syncapp.Source) (time.Time, error) {
		_, at, err := devices.CheckCurrentDeviceTx(ctx, tx, s.Scope, s.Actor, s.MembershipID, s.EnrollmentID)
		if err != nil {
			return time.Time{}, syncapp.SourceDenied(err)
		}
		return at, nil
	}}
	r := pgx.Identifier{f.RuntimeRole}.Sanitize()
	// The synthetic custom producer's own held facts model a schema196-198
	// REVIEW owner that predates the typed core proof introduced by 199.
	if _, err := f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_sync_operations TO "+r+";GRANT UPDATE(operation_id) ON org_sync_operations TO "+r+";GRANT SELECT,INSERT ON org_sync_acknowledgements,org_sync_effects,org_audit_events TO "+r+
		";CREATE TABLE synthetic_legacy_review_facts(org_id uuid NOT NULL,operation_id uuid NOT NULL,review_id uuid NOT NULL,value text NOT NULL,PRIMARY KEY(org_id,operation_id));ALTER TABLE synthetic_legacy_review_facts ENABLE ROW LEVEL SECURITY;ALTER TABLE synthetic_legacy_review_facts FORCE ROW LEVEL SECURITY;CREATE POLICY scoped ON synthetic_legacy_review_facts USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);GRANT SELECT,INSERT ON synthetic_legacy_review_facts TO "+r); err != nil {
		t.Fatal(err)
	}
	return syncFixtureState{deviceFixtureState: fx, source: source, config: config, target: onboardingID(t)}
}

// storeLegacyReviewReceipt writes the schema196-198 receipt shape exactly as
// the earlier intake persisted it: original operation, audit and REVIEW ACK.
func storeLegacyReviewReceipt(t *testing.T, fx syncFixtureState, op protocol.Operation, reviewID string) protocol.Acknowledgement {
	t.Helper()
	v := op.Snapshot()
	var original protocol.Acknowledgement
	err := database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
		if _, err := fx.config.ConfirmSource(t.Context(), tx, fx.source); err != nil {
			return err
		}
		if _, err := tx.Exec(t.Context(), `INSERT INTO org_sync_operations(org_id,operation_id,owner_account_id,api_origin,membership_id,enrollment_id,operation_type,operation_schema,entity_type,entity_id,payload_digest)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, v.OrgID, op.ID(), v.OwnerAccountID, v.APIOrigin, v.MembershipID, v.EnrollmentID, v.Type, v.OperationSchema, v.Entity.Type, v.Entity.ID, op.PayloadDigest()); err != nil {
			return err
		}
		if _, err := tx.Exec(t.Context(), `INSERT INTO synthetic_legacy_review_facts VALUES($1,$2,$3,'Synthetic immutable legacy held fact')`, v.OrgID, op.ID(), reviewID); err != nil {
			return err
		}
		_, err := fx.config.Audit.RecordChangeTx(t.Context(), tx, fx.scope, audit.Event{OperationID: op.ID(), ActorID: fx.source.Actor.AccountID, Action: "offl.operation.review", TargetType: "offl.operation", TargetID: op.ID(), ReasonCode: "intake", RequestHash: strings.TrimPrefix(op.PayloadDigest(), "sha256:")}, func(ctx context.Context, tx pgx.Tx) error {
			var at time.Time
			if err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&at); err != nil {
				return err
			}
			var err error
			original, err = protocol.NewAcknowledgement(op, protocol.AcknowledgementSnapshot{Result: protocol.AcceptedForReview, Code: "STALE_REVISION", ReviewItemID: reviewID, ReceivedAt: orgtime.FormatInstant(at)}, protocol.DefaultLimits())
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `INSERT INTO org_sync_acknowledgements(org_id,operation_id,result,code,review_item_id,received_at,canonical_ack,audit_hash)VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, v.OrgID, op.ID(), protocol.AcceptedForReview, "STALE_REVISION", reviewID, at, original.CanonicalBytes(), strings.TrimPrefix(op.PayloadDigest(), "sha256:"))
			return err
		})
		return err
	})
	if err != nil {
		t.Fatal("actual schema198 source-backed legacy REVIEW receipt", err)
	}
	return original
}

func meterReviewSQLCode(t *testing.T, err error) string {
	t.Helper()
	var pe *pgconn.PgError
	if !errors.As(err, &pe) {
		t.Fatal("expected an actual PostgreSQL refusal", err)
	}
	return pe.Code
}

func TestMeterReviewPopulatedLegacyUpgradeFailureRecoveryKeepsUnprovenHistory(t *testing.T) {
	const name = "000199_organization_meter_reviews.sql"
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
	fx := meterReviewLegacyFixture(t, f)
	org := fx.scope.OrgID().String()
	// Populated schema198: a real legacy REVIEW receipt, an ordinary ACCEPT-free
	// history, plus an installed Meter with its original opening reading.
	legacy := syncOperation(t, fx, onboardingID(t), "Synthetic immutable legacy held fact", nil, nil, nil)
	legacyReview := onboardingID(t)
	original := storeLegacyReviewReceipt(t, fx, legacy, legacyReview)
	assets := fleetStore(t, fx.legalFixtureState)
	asset, _, err := fleetWrite(t, fx.legalFixtureState, assets, fleetCommand(t, fleetCreateInput()))
	if err != nil {
		t.Fatal(err)
	}
	meters := meterStore(t, fx.legalFixtureState)
	installed, _, err := meterInstall(t, fx.legalFixtureState, meters, meterInstallCommand(t, meterInstallInput(t, asset.AssetID, time.Now().UTC().Add(-2*time.Hour))))
	if err != nil {
		t.Fatal(err)
	}
	snapshot := func() string {
		t.Helper()
		var out string
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array(
 (SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_sync_operations x),
 (SELECT jsonb_agg(jsonb_build_array(org_id,operation_id,result,code,entity_revision,review_item_id,received_at,warnings,encode(canonical_ack,'hex'),audit_hash)ORDER BY org_id,operation_id)FROM org_sync_acknowledgements),
 (SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_sync_effects x),
 (SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_audit_events x),
 (SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM synthetic_legacy_review_facts x),
 (SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_assets x),
 (SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_meters x),
 (SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_meter_segments x),
 (SELECT jsonb_agg(to_jsonb(x)-'review_resolution_id' ORDER BY org_id,id)FROM org_meter_readings x),
 (SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_meter_commands x),
 (SELECT jsonb_agg(to_jsonb(x)ORDER BY id)FROM accounts x),
 (SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_memberships x),
 (SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_employee_identities x),
 (SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_device_enrollments x))::text`).Scan(&out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	readings := 0
	if err := f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_meter_readings WHERE org_id=$1`, org).Scan(&readings); err != nil || readings != 1 {
		t.Fatal("populated schema198 lacks the original opening reading", err)
	}
	migrator := f.RuntimeRole + "_review_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	// An ordinary migrator owns exactly what 199 alters or replaces and only
	// references the other parents; it is neither superuser nor BYPASSRLS.
	if _, err := f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE,CREATE ON SCHEMA public TO "+role+";GRANT SELECT,INSERT ON schema_migrations TO "+role+
		";GRANT REFERENCES ON org_employee_identities,org_assets,org_meters,org_meter_segments,org_units,org_memberships,org_audit_events TO "+role+
		";ALTER TABLE org_sync_operations OWNER TO "+role+";ALTER TABLE org_sync_acknowledgements OWNER TO "+role+";ALTER TABLE org_meter_readings OWNER TO "+role+
		";ALTER FUNCTION org_meter_observation_guard() OWNER TO "+role+";ALTER FUNCTION org_sync_effect_source() OWNER TO "+role+";ALTER FUNCTION org_asset_current_usage(uuid,uuid) OWNER TO "+role+";ALTER FUNCTION org_unit_current_usage(uuid,uuid) OWNER TO "+role); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := f.Admin.Exec(ctx, "REASSIGN OWNED BY "+role+" TO CURRENT_USER;DROP OWNED BY "+role+";DROP ROLE "+role); err != nil {
			t.Error(err)
		}
	})
	functions := func() string {
		t.Helper()
		var out string
		if err := f.Admin.QueryRow(t.Context(), `SELECT string_agg(pg_get_functiondef(oid),E'\n' ORDER BY proname) FROM pg_proc WHERE oid=ANY(ARRAY['org_meter_observation_guard()'::regprocedure,'org_sync_effect_source()'::regprocedure,'org_asset_current_usage(uuid,uuid)'::regprocedure,'org_unit_current_usage(uuid,uuid)'::regprocedure])`).Scan(&out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	before, beforeFunctions := snapshot(), functions()
	location, err := url.Parse(f.AdminDSN)
	if err != nil {
		t.Fatal(err)
	}
	parameters := location.Query()
	parameters.Set("options", "-c role="+migrator)
	location.RawQuery = strings.ReplaceAll(parameters.Encode(), "+", "%20")
	raw, err := fs.ReadFile(migrations.Files, name)
	if err != nil {
		t.Fatal(err)
	}
	broken := fstest.MapFS{name: &fstest.MapFile{Data: append(append([]byte{}, raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(t.Context(), location.String(), broken); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
		t.Fatal("injected review migration failure not reached", err)
	}
	var clean bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('org_sync_review_items') IS NULL AND to_regclass('org_meter_held_readings') IS NULL AND to_regclass('org_sync_review_resolutions') IS NULL AND to_regprocedure('org_sync_review_source_valid(uuid,uuid,text,bigint)') IS NULL
 AND NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND(table_name,column_name) IN(('org_sync_acknowledgements','review_proof_version'),('org_meter_readings','review_resolution_id')))
 AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version='000199')`).Scan(&clean); err != nil || !clean || before != snapshot() || beforeFunctions != functions() {
		t.Fatal("failed review migration changed schema198 schema, guards or history", err)
	}
	current := fstest.MapFS{}
	for key, data := range previous {
		current[key] = data
	}
	current[name] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 1 || applied[0] != "000199" {
		t.Fatal("ordinary review migration recovery", err)
	}
	if before != snapshot() {
		t.Fatal("review migration rewrote legacy receipts, readings or source history")
	}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 0 {
		t.Fatal("review migration no-op replay", err)
	}
	if err := migrate.Check(t.Context(), f.Admin, previous); err != nil {
		t.Fatal("previous embedded image incompatible", err)
	}
	// Legacy history stays explicitly UNPROVEN: exact bytes, NULL marker, no
	// fabricated item/fact/resolution/effect, and a validated MATCH SIMPLE FK.
	if err := f.Admin.QueryRow(t.Context(), `SELECT a.review_proof_version IS NULL AND a.review_item_id=$3::uuid AND a.canonical_ack=$4::bytea
 AND NOT EXISTS(SELECT 1 FROM org_sync_review_items) AND NOT EXISTS(SELECT 1 FROM org_meter_held_readings) AND NOT EXISTS(SELECT 1 FROM org_sync_review_resolutions) AND NOT EXISTS(SELECT 1 FROM org_sync_effects WHERE operation_id=$2)
 AND NOT EXISTS(SELECT 1 FROM org_meter_readings WHERE review_resolution_id IS NOT NULL)
 AND(SELECT convalidated AND condeferrable AND condeferred AND confmatchtype='s' FROM pg_constraint WHERE conname='org_sync_ack_review_proof_fkey' AND conrelid='org_sync_acknowledgements'::regclass)
 AND NOT EXISTS(SELECT 1 FROM pg_constraint WHERE connamespace='public'::regnamespace AND NOT convalidated)
 FROM org_sync_acknowledgements a WHERE a.org_id=$1 AND a.operation_id=$2`, org, legacy.ID(), legacyReview, original.CanonicalBytes()).Scan(&clean); err != nil || !clean {
		t.Fatal("legacy REVIEW ACK was rewritten, backfilled or accepted with an unvalidated reference", err)
	}
	if err := f.Admin.QueryRow(t.Context(), `SELECT NOT rolsuper AND NOT rolbypassrls AND NOT rolcanlogin AND(SELECT count(*)=3 AND bool_and(relrowsecurity AND relforcerowsecurity AND relowner=(SELECT oid FROM pg_roles WHERE rolname=$1)) FROM pg_class WHERE oid=ANY(ARRAY['org_sync_review_items'::regclass,'org_meter_held_readings'::regclass,'org_sync_review_resolutions'::regclass])) FROM pg_roles WHERE rolname=$1`, migrator).Scan(&clean); err != nil || !clean {
		t.Fatal("review tables not owned by the ordinary forced-RLS migrator", err)
	}
	f.GrantMeterUsage(t)
	f.GrantMeterReviews(t)
	reviews := meterReviewStore(t, fx)
	runtime := pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "GRANT UPDATE ON org_sync_acknowledgements TO "+runtime); err != nil {
		t.Fatal(err)
	}
	// No runtime path can promote the legacy marker into a current proof.
	exec := func(sql string, args ...any) error {
		return database.WithOrgTx(t.Context(), f.Runtime, fx.scope, func(tx pgx.Tx) error { _, err := tx.Exec(t.Context(), sql, args...); return err })
	}
	if code := meterReviewSQLCode(t, exec(`UPDATE org_sync_acknowledgements SET review_proof_version=1 WHERE org_id=$1 AND operation_id=$2`, org, legacy.ID())); code != "55000" {
		t.Fatal("legacy proof marker could be rewritten", code)
	}
	if code := meterReviewSQLCode(t, exec(`INSERT INTO org_sync_review_items(org_id,id,operation_id,operation_type,operation_schema,payload_digest,code,author_employee_id) SELECT o.org_id,$3::uuid,o.operation_id,o.operation_type,o.operation_schema,o.payload_digest,'STALE_REVISION',m.employee_id FROM org_sync_operations o JOIN org_memberships m ON(m.org_id,m.id)=(o.org_id,o.membership_id) WHERE o.org_id=$1 AND o.operation_id=$2`, org, legacy.ID(), legacyReview)); code != "23503" {
		t.Fatal("legacy REVIEW acquired a fabricated core review item", code)
	}
	if results, err := reviews.OwnMeterReviewResults(t.Context(), fx.source, []protocol.ReviewSelector{{OperationID: legacy.ID(), PayloadDigest: legacy.PayloadDigest()}}); !errors.Is(err, orgapi.NotFound) || len(results) != 0 {
		t.Fatal("unproven legacy receipt became a current author result source", err)
	}
	if before != snapshot() {
		t.Fatal("refused promotion attempts changed legacy history")
	}
	var usage string
	if err := pgx.BeginFunc(t.Context(), f.Admin, func(tx pgx.Tx) error {
		return tx.QueryRow(t.Context(), `SELECT org_unit_current_usage($1::uuid,u.id)::text FROM(SELECT set_config('anpfuel.org_id',$1::text,true)) s,org_units u WHERE u.org_id=$1::uuid LIMIT 1`, org).Scan(&usage)
	}); err != nil || usage == "" {
		t.Fatal("upgraded unit usage rejected the held-fact unit history reference", err)
	}
	// The upgraded schema still delivers the real typed Meter source end to end.
	_, ref, ack, err := storeMeterHeldReview(t, fx, reviews, meterObservedCommand(t, installed, time.Now().UTC().Add(-time.Hour), 7))
	if err != nil || ref.Revision != 1 || ack.Snapshot().Result != protocol.AcceptedForReview {
		t.Fatal("real typed Meter review unavailable after ordinary migration", err)
	}
	decision, err := app.NewMeterReviewDecision(app.MeterReviewDecisionInput{OperationID: onboardingID(t), ReviewItemID: ref.ReviewItemID, ExpectedRevision: 1, Decision: app.MeterReviewAccept})
	if err != nil {
		t.Fatal(err)
	}
	if result, replay, err := reviews.ResolveMeterReview(t.Context(), fx.scope, fleetActor(fx.legalFixtureState), fx.member, decision); err != nil || replay || result.Snapshot().Outcome != protocol.ReviewAccepted {
		t.Fatal("current reviewer cannot apply typed review on upgraded schema", err)
	}
	var after []byte
	if err := f.Admin.QueryRow(t.Context(), `SELECT canonical_ack FROM org_sync_acknowledgements WHERE org_id=$1 AND operation_id=$2 AND review_proof_version IS NULL`, org, legacy.ID()).Scan(&after); err != nil || !bytes.Equal(after, original.CanonicalBytes()) {
		t.Fatal("legacy canonical receipt changed after new reviewed history", err)
	}
	if err := f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_sync_acknowledgements WHERE org_id=$1 AND result='ACCEPTED_FOR_REVIEW' AND review_proof_version=1)=1 AND(SELECT count(*) FROM org_sync_acknowledgements WHERE org_id=$1 AND result='ACCEPTED_FOR_REVIEW' AND review_proof_version IS NULL)=1`, org).Scan(&clean); err != nil || !clean {
		t.Fatal("new REVIEW lacked version1 proof or legacy history was relabelled", err)
	}
}
