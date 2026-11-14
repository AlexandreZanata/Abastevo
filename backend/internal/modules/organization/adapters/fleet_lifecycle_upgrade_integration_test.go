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
	fleetpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/adapters"
	fa "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

// Populated schema200 → 201: additive columns, no invented data, a failed
// attempt changes nothing, recovery is exact and old receipts still replay.
func TestFleetLifecyclePopulatedOrdinaryUpgradeFailureRecoveryAndPreviousImage(t *testing.T) {
	const name = "000201_organization_asset_lifecycle.sql"
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
	if _, ok := previous["000200_invitation_erasure_corrections.sql"]; !ok {
		t.Fatal("previous image is not schema200")
	}
	f := orgtest.NewWithMigrations(t, previous)
	_, _, onboarding, actor, flags := onboardingOnFixture(t, f)
	fx := legalFixtureFor(t, f, onboarding, actor, flags)
	if _, _, err := fx.onboarding.Repository.ActivateOrganization(t.Context(), fx.scope, fx.actor, fx.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	var unit string
	if err := f.Admin.QueryRow(t.Context(), `SELECT id::text FROM org_units WHERE org_id=$1`, fx.scope.OrgID().String()).Scan(&unit); err != nil {
		t.Fatal(err)
	}
	assets := fleetStore(t, fx)
	draftCommand := fleetCommand(t, fleetCreateInput())
	draft, _, err := fleetWrite(t, fx, assets, draftCommand)
	if err != nil {
		t.Fatal(err)
	}
	activeInput := fleetCreateInput()
	activeInput.UnitID, activeInput.Profile.Plate = unit, "UPG2E01"
	active, _, err := fleetWrite(t, fx, assets, fleetCommand(t, activeInput))
	if err != nil {
		t.Fatal(err)
	}
	// Current code reads the 201 columns, so the schema200 activation effects
	// are reproduced through the actual 200 guards as the ordinary runtime role.
	activateCommand := fleetCommand(t, fa.AssetWriteInput{Kind: fa.ActivateAsset, AssetID: active.AssetID, ExpectedRevision: 1})
	activated := pre201Activate(t, fx, activateCommand, unit)
	snapshot := func() string {
		t.Helper()
		var value string
		// Only pre-201 columns: the added lifecycle columns are proved NULL below.
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array(
 (SELECT jsonb_agg(to_jsonb(x)-ARRAY['acquired_at','acquired_submicro','disposal_kind','disposed_at','disposed_submicro','disposal_reason'] ORDER BY org_id,id) FROM org_assets x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,operation_id) FROM org_asset_commands x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,asset_id,revision) FROM org_asset_revisions x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,asset_id,started_at) FROM org_asset_unit_allocations x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,operation_id) FROM org_audit_events x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_meter_readings x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_meters x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_units x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id) FROM organizations x))::text`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	functions := func() string {
		t.Helper()
		var out string
		if err := f.Admin.QueryRow(t.Context(), `SELECT string_agg(pg_get_functiondef(oid),E'\n' ORDER BY proname) FROM pg_proc WHERE oid=ANY(ARRAY['org_asset_guard()'::regprocedure,'org_meter_observation_guard()'::regprocedure,'org_meter_held_source()'::regprocedure])`).Scan(&out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	before, beforeFunctions := snapshot(), functions()
	migrator := f.RuntimeRole + "_lifecycle_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	// An ordinary migrator owns exactly what 201 alters or replaces and only
	// references the other parents; it is neither superuser nor BYPASSRLS.
	if _, err := f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE,CREATE ON SCHEMA public TO "+role+";GRANT SELECT,INSERT ON schema_migrations TO "+role+
		";GRANT REFERENCES ON org_asset_revisions,org_asset_commands,org_audit_events TO "+role+
		";ALTER TABLE org_assets OWNER TO "+role+";ALTER TABLE org_asset_commands OWNER TO "+role+
		";ALTER FUNCTION org_asset_guard() OWNER TO "+role+";ALTER FUNCTION org_meter_observation_guard() OWNER TO "+role+";ALTER FUNCTION org_meter_held_source() OWNER TO "+role); err != nil {
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
	query := location.Query()
	query.Set("options", "-c role="+migrator)
	location.RawQuery = strings.ReplaceAll(query.Encode(), "+", "%20")
	raw, err := fs.ReadFile(migrations.Files, name)
	if err != nil {
		t.Fatal("lifecycle migration missing", err)
	}
	broken := fstest.MapFS{name: &fstest.MapFile{Data: append(append([]byte{}, raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(t.Context(), location.String(), broken); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
		t.Fatal("injected lifecycle migration failure not reached", err)
	}
	var clean bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('org_asset_lifecycle_events') IS NULL
 AND NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='org_assets' AND column_name IN('acquired_at','acquired_submicro','disposal_kind','disposed_at','disposed_submicro','disposal_reason'))
 AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version='000201')`).Scan(&clean); err != nil || !clean || before != snapshot() || beforeFunctions != functions() {
		t.Fatal("failed lifecycle migration changed schema200 schema, guards or data", err)
	}
	current := fstest.MapFS{}
	for key, data := range previous {
		current[key] = data
	}
	current[name] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 1 || applied[0] != "000201" {
		t.Fatal("ordinary lifecycle migration recovery", err)
	}
	if before != snapshot() {
		t.Fatal("lifecycle migration rewrote existing assets, receipts, history or readings")
	}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 0 {
		t.Fatal("lifecycle migration no-op replay", err)
	}
	if err := migrate.Check(t.Context(), f.Admin, previous); err != nil {
		t.Fatal("previous embedded image incompatible", err)
	}
	// Additive only: every existing row has empty lifecycle data, no history
	// row is invented, all constraints are validated and RLS is forced.
	if err := f.Admin.QueryRow(t.Context(), `SELECT NOT EXISTS(SELECT 1 FROM org_assets WHERE acquired_at IS NOT NULL OR acquired_submicro<>0 OR disposal_kind IS NOT NULL OR disposed_at IS NOT NULL OR disposed_submicro<>0 OR disposal_reason IS NOT NULL)
 AND NOT EXISTS(SELECT 1 FROM org_asset_lifecycle_events) AND(SELECT count(*) FROM org_assets)=2
 AND NOT EXISTS(SELECT 1 FROM pg_constraint WHERE connamespace='public'::regnamespace AND NOT convalidated)
 AND(SELECT relrowsecurity AND relforcerowsecurity AND relowner=(SELECT oid FROM pg_roles WHERE rolname=$1) FROM pg_class WHERE oid='org_asset_lifecycle_events'::regclass)`, migrator).Scan(&clean); err != nil || !clean {
		t.Fatal("lifecycle migration invented data, skipped validation or weakened RLS", err)
	}
	f.GrantMeterUsage(t)
	s := lifecycleStore(t, fx)
	// Original pre-201 receipts replay byte-for-byte in result.
	if replay, replayed, err := fleetWrite(t, fx, s, draftCommand); err != nil || !replayed || replay != draft {
		t.Fatal("pre-201 create receipt changed", err)
	}
	if replay, replayed, err := fleetWrite(t, fx, s, activateCommand); err != nil || !replayed || replay != activated {
		t.Fatal("pre-201 activation receipt changed", err)
	}
	// Refusal checks on the upgraded schema.
	exec := func(sql string, args ...any) error {
		return database.WithOrgTx(t.Context(), f.Runtime, fx.scope, func(tx pgx.Tx) error { _, err := tx.Exec(t.Context(), sql, args...); return err })
	}
	org := fx.scope.OrgID().String()
	if code := lifecycleSQLCode(exec(`UPDATE org_assets SET state='DISPOSED',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, active.AssetID)); code != "23514" {
		t.Fatal("upgraded schema admitted a disposal without data", code)
	}
	if code := lifecycleSQLCode(exec(`UPDATE org_assets SET state='INACTIVE',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, active.AssetID)); code != "23514" {
		t.Fatal("upgraded schema admitted a transition without history", code)
	}
	if code := lifecycleSQLCode(exec(`UPDATE org_assets SET state='INACTIVE',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, draft.AssetID)); code != "P7903" {
		t.Fatal("upgraded schema admitted DRAFT to INACTIVE", code)
	}
	// The pre-existing ACTIVE asset now moves through the actual commands.
	revision := activated.Revision
	for _, command := range []fleet.LifecycleCommand{fleet.Deactivate, fleet.Dispose, fleet.RevertDisposal, fleet.Dispose, fleet.Archive} {
		result, _, err := lifecycleWrite(t, fx, s, lifecycleCommand(t, lifecycleInputFor(t, fx, command, active.AssetID, revision)))
		if err != nil {
			t.Fatal("upgraded lifecycle command", command, err)
		}
		revision = result.Revision
	}
	if code := lifecycleSQLCode(exec(`UPDATE org_assets SET revision=revision+1 WHERE org_id=$1 AND id=$2`, org, active.AssetID)); code != "P7903" {
		t.Fatal("upgraded ARCHIVED asset is not terminal", code)
	}
	role2 := pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "GRANT UPDATE,DELETE ON org_asset_lifecycle_events TO "+role2); err != nil {
		t.Fatal(err)
	}
	if code := lifecycleSQLCode(exec(`DELETE FROM org_asset_lifecycle_events WHERE org_id=$1`, org)); code != "55000" {
		t.Fatal("upgraded lifecycle history deletable", code)
	}
	reused := fleetCreateInput()
	reused.Profile.Plate = "upg-2e01"
	if _, _, err := fleetWrite(t, fx, s, fleetCommand(t, reused)); err != nil {
		t.Fatal("archived pre-201 plate not freed", err)
	}
	if err := f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_asset_lifecycle_events WHERE org_id=$1 AND asset_id=$2)=5`, org, active.AssetID).Scan(&clean); err != nil || !clean {
		t.Fatal("upgraded lifecycle history or readings incomplete", err)
	}
}

// pre201Activate writes exactly the FLEET-002 activation effects (state,
// initial allocation, revision snapshot, receipt and audit) on schema200.
func pre201Activate(t *testing.T, fx legalFixtureState, command fa.AssetWrite, unit string) fa.AssetResult {
	t.Helper()
	v, err := command.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := audit.NewCatalog(fleetpg.AssetAuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	digest := command.Digest(fx.scope)
	org := fx.scope.OrgID().String()
	err = database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
		_, err := auditpg.NewStore(fx.f.Runtime, catalog).RecordChangeTx(t.Context(), tx, fx.scope, audit.Event{OperationID: v.OperationID, ActorID: fx.actor.AccountID, Action: "fleet.asset.activated", TargetType: "asset", TargetID: v.AssetID, RequestHash: digest}, func(ctx context.Context, tx pgx.Tx) error {
			for _, statement := range []struct {
				sql  string
				args []any
			}{
				{`UPDATE org_assets SET state='ACTIVE',revision=2 WHERE org_id=$1 AND id=$2 AND revision=1`, []any{org, v.AssetID}},
				{`INSERT INTO org_asset_unit_allocations(org_id,asset_id,unit_id) VALUES($1,$2,$3)`, []any{org, v.AssetID, unit}},
				{`INSERT INTO org_asset_revisions(org_id,asset_id,revision,operation_id,snapshot) SELECT org_id,asset_id,2,$3,jsonb_set(snapshot,'{state}','"ACTIVE"') FROM org_asset_revisions WHERE org_id=$1 AND asset_id=$2 AND revision=1`, []any{org, v.AssetID, v.OperationID}},
				{`INSERT INTO org_asset_commands(org_id,operation_id,actor_id,asset_id,kind,request_hash,result_state,result_revision) VALUES($1,$2,$3,$4,'ACTIVATE',$5,'ACTIVE',2)`, []any{org, v.OperationID, fx.actor.AccountID, v.AssetID, digest}},
			} {
				if _, err := tx.Exec(ctx, statement.sql, statement.args...); err != nil {
					return err
				}
			}
			return nil
		})
		return err
	})
	if err != nil {
		t.Fatal("schema200 activation effects", err)
	}
	return fa.AssetResult{AssetID: v.AssetID, State: fleet.Active, Revision: 2}
}

// pre201InstallMeter writes exactly the METER-002 InstallMeter effects (meter,
// installation segment, observed WEB_SESSION opening reading, asset revision
// bump and snapshot, receipt and audit) on a pre-201 schema through its guards.
func pre201InstallMeter(t *testing.T, fx legalFixtureState, asset string, at time.Time) fa.MeterInstallResult {
	t.Helper()
	input := meterInstallInput(t, asset, at)
	command := meterInstallCommand(t, input)
	digest := command.Digest(fx.scope)
	meterID, segmentID := onboardingID(t), onboardingID(t)
	catalog, err := audit.NewCatalog(fleetpg.MeterAuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	org := fx.scope.OrgID().String()
	stamp, sub := at.UTC().Truncate(time.Microsecond), int16(at.Nanosecond()%1000)
	err = database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
		_, err := auditpg.NewStore(fx.f.Runtime, catalog).RecordChangeTx(t.Context(), tx, fx.scope, audit.Event{OperationID: input.OperationID, ActorID: fx.actor.AccountID, Action: "meter.installed", TargetType: "meter", TargetID: meterID, RequestHash: digest}, func(ctx context.Context, tx pgx.Tx) error {
			for _, statement := range []struct {
				sql  string
				args []any
			}{
				{`INSERT INTO org_meters(org_id,id,asset_id,kind,label,modulus,installed_at,installed_submicro,created_operation_id) VALUES($1,$2,$3,'ODOMETER',$4,NULL,$5,$6,$7)`, []any{org, meterID, asset, input.Label, stamp, sub, input.OperationID}},
				{`INSERT INTO org_meter_segments(org_id,id,meter_id,kind,opened_at,opened_submicro,opening_reading_id) VALUES($1,$2,$3,'INSTALLATION',$4,$5,$6)`, []any{org, segmentID, meterID, stamp, sub, input.OpeningReadingID}},
				{`INSERT INTO org_meter_readings(org_id,id,meter_id,segment_id,value,occurred_at,occurred_submicro,entry_zone,device_offset_seconds,author_membership_id,capture_kind,web_session_id,operation_id,request_hash,origin,method,state) VALUES($1,$2,$3,$4,0,$5,$6,$7,$8,$9,'WEB_SESSION',$10,$11,$12,'METER_INSTALLATION','TYPED','VALID')`, []any{org, input.OpeningReadingID, meterID, segmentID, stamp, sub, input.Declared.EntryZone, input.Declared.DeviceOffsetSeconds, fx.member, fx.actor.SessionID, input.OperationID, digest}},
				{`UPDATE org_assets SET revision=revision+1 WHERE org_id=$1 AND id=$2 AND revision=1`, []any{org, asset}},
				{`INSERT INTO org_asset_revisions(org_id,asset_id,revision,operation_id,snapshot) SELECT org_id,asset_id,2,$3,snapshot FROM org_asset_revisions WHERE org_id=$1 AND asset_id=$2 AND revision=1`, []any{org, asset, input.OperationID}},
				{`INSERT INTO org_meter_commands(org_id,operation_id,actor_id,asset_id,meter_id,segment_id,opening_reading_id,request_hash,asset_revision) VALUES($1,$2,$3,$4,$5,$6,$7,$8,2)`, []any{org, input.OperationID, fx.actor.AccountID, asset, meterID, segmentID, input.OpeningReadingID, digest}},
			} {
				if _, err := tx.Exec(ctx, statement.sql, statement.args...); err != nil {
					return err
				}
			}
			return nil
		})
		return err
	})
	if err != nil {
		t.Fatal("pre-201 meter installation effects", err)
	}
	return fa.MeterInstallResult{AssetID: asset, MeterID: meterID, SegmentID: segmentID, OpeningReadingID: input.OpeningReadingID, AssetRevision: 2}
}
