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
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

const meterSegmentsMigration = "000251_organization_meter_segments.sql"

// The 000251 guards on real PostGIS: a meter and a segment change only by
// their one closure or removal, every closure is re-proved (instant, kind,
// value, previous reading, implied usage), the chain is whole at commit,
// history stays undeletable, the rollover cause needs its segment, and
// forced RLS hides and refuses another organization's segments.
func TestMeterSegmentSchemaGuardsTheChain(t *testing.T) {
	base := time.Now().UTC().Truncate(time.Second).Add(-10 * 24 * time.Hour)
	fx, store, installed := segmentFixture(t, 1_000_000, base, int64p(1_400_000))
	r1, _ := validationRecord(t, fx, store, installed, base.Add(24*time.Hour), 1_200_000, readingStateRow{state: "VALID"})
	low, _ := validationRecord(t, fx, store, installed, base.Add(48*time.Hour), 100, readingStateRow{state: "SUSPECT", flags: "POSSIBLE_ROLLOVER"})
	org, meterID, segment := fx.scope.OrgID().String(), installed.MeterID, installed.SegmentID
	var operation string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT created_operation_id::text FROM org_meters WHERE org_id=$1 AND id=$2`, org, meterID).Scan(&operation); err != nil {
		t.Fatal(err)
	}
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT UPDATE,DELETE,TRUNCATE ON org_meters,org_meter_segments TO "+role); err != nil {
		t.Fatal(err)
	}
	closeSQL := `UPDATE org_meter_segments SET closed_at=$3,closing_kind=$4,closing_value=$5,closing_operation_id=$6,closing_previous_reading_id=$7,closing_implied_usage=$8 WHERE org_id=$1 AND id=$2`
	at := base.Add(36 * time.Hour)
	for name, c := range map[string]struct {
		err  error
		code string
	}{
		"meter label rewritten":            {fleetSQL(t, fx.legalFixtureState, `UPDATE org_meters SET label='Changed' WHERE org_id=$1 AND id=$2`, org, meterID), "55000"},
		"meter removed with an open chain": {fleetSQL(t, fx.legalFixtureState, `UPDATE org_meters SET removed_at=$3,removal_operation_id=$4 WHERE org_id=$1 AND id=$2`, org, meterID, at, operation), "23514"},
		"segment reopened":                 {fleetSQL(t, fx.legalFixtureState, `UPDATE org_meter_segments SET opened_at=opened_at-interval '1 hour' WHERE org_id=$1 AND id=$2`, org, segment), "55000"},
		"rollover not at the modulus":      {fleetSQL(t, fx.legalFixtureState, closeSQL, org, segment, at, "ROLLOVER", 1_399_999, operation, r1, 199_999), "23514"},
		"reset with a wrong usage":         {fleetSQL(t, fx.legalFixtureState, closeSQL, org, segment, at, "RESET", 1_300_000, operation, r1, 1), "23514"},
		"reset from a stale reading":       {fleetSQL(t, fx.legalFixtureState, closeSQL, org, segment, at, "RESET", 1_300_000, operation, installed.OpeningReadingID, 300_000), "23514"},
		"reset left without successor":     {fleetSQL(t, fx.legalFixtureState, closeSQL, org, segment, at, "RESET", 1_300_000, operation, r1, 100_000), "23514"},
		"reset over a valid reading":       {fleetSQL(t, fx.legalFixtureState, closeSQL, org, segment, base.Add(12*time.Hour), "RESET", 1_300_000, operation, installed.OpeningReadingID, 300_000), "23514"},
		"reset closing reading":            {fleetSQL(t, fx.legalFixtureState, `UPDATE org_meter_segments SET closed_at=$3,closing_kind='RESET',closing_value=1300000,closing_reading_id=$4,closing_operation_id=$5,closing_previous_reading_id=$4,closing_implied_usage=100000 WHERE org_id=$1 AND id=$2`, org, segment, at, r1, operation), "23514"},
		"segment opened without closure":   {fleetSQL(t, fx.legalFixtureState, `INSERT INTO org_meter_segments(org_id,id,meter_id,kind,opened_at,opening_reading_id,opened_operation_id) VALUES($1,$2,$3,'RESET',$4,$5,$6)`, org, onboardingID(t), meterID, at, r1, operation), "23514"},
		"rollover without its segment":     {fleetSQL(t, fx.legalFixtureState, `INSERT INTO org_meter_reading_states(org_id,meter_id,reading_id,revision,state,cause,operation_id,actor_membership_id,reason) VALUES($1,$2,$3,2,'VALID','ROLLOVER',$4,$5,'Synthetic')`, org, meterID, low, onboardingID(t), fx.member), "23514"},
		"rollover flag confirmed":          {fleetSQL(t, fx.legalFixtureState, `INSERT INTO org_meter_reading_states(org_id,meter_id,reading_id,revision,state,cause,operation_id,actor_membership_id,reason) VALUES($1,$2,$3,2,'VALID','CONFIRMED',$4,$5,'Synthetic')`, org, meterID, low, onboardingID(t), fx.member), "23514"},
		"meter deleted":                    {fleetSQL(t, fx.legalFixtureState, `DELETE FROM org_meters WHERE org_id=$1`, org), "55000"},
		"segment deleted":                  {fleetSQL(t, fx.legalFixtureState, `DELETE FROM org_meter_segments WHERE org_id=$1`, org), "55000"},
		// Readings and closures reference segments, so PostgreSQL refuses
		// TRUNCATE (0A000) before the keep-history trigger, asserted below.
		"segments truncated": {fleetSQL(t, fx.legalFixtureState, `TRUNCATE org_meter_segments`), "0A000"},
	} {
		t.Run(name, func(t *testing.T) { fleetSQLCode(t, c.err, c.code) })
	}
	var kept bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*)=2 FROM pg_trigger WHERE tgname IN('org_meters_immutable','org_meter_segments_immutable') AND tgtype&32<>0 AND tgtype&8<>0 AND tgtype&16=0`).Scan(&kept); err != nil || !kept {
		t.Fatal("the keep-history triggers no longer cover DELETE and TRUNCATE only", err)
	}
	// A real reset closes once; its closure cannot be rewritten.
	result, _, err := change(t, fx, store, resetInput(t, installed, 1, at, 1_300_000, 0))
	if err != nil {
		t.Fatal("documented reset refused", err)
	}
	fleetSQLCode(t, fleetSQL(t, fx.legalFixtureState, `UPDATE org_meter_segments SET closing_value=1400000 WHERE org_id=$1 AND id=$2`, org, segment), "55000")
	fleetSQLCode(t, fleetSQL(t, fx.legalFixtureState, closeSQL, org, result.OpenedSegmentID, base.Add(30*time.Hour), "RESET", 1_000, operation, r1, 0), "23514")
	var n int
	if err := fx.f.Runtime.QueryRow(t.Context(), `SELECT count(*) FROM org_meter_segments`).Scan(&n); err != nil || n != 0 {
		t.Fatal("segments read without organization escaped forced RLS", err)
	}
	other, _, err := fx.onboarding.Create(t.Context(), fx.actor, onboardingCommand(t, onboardingID(t)))
	if err != nil {
		t.Fatal(err)
	}
	foreign := scopeFor(t, other.OrgID.String())
	err = database.WithOrgTx(t.Context(), fx.f.Runtime, foreign, func(tx pgx.Tx) error {
		if err := tx.QueryRow(t.Context(), `SELECT count(*) FROM org_meter_segments`).Scan(&n); err != nil || n != 0 {
			t.Fatal("foreign organization sees segments", err)
		}
		tag, err := tx.Exec(t.Context(), `UPDATE org_meter_segments SET closed_at=now() WHERE org_id=$1`, org)
		if err == nil && tag.RowsAffected() != 0 {
			t.Fatal("foreign organization closed a segment")
		}
		_, err = tx.Exec(t.Context(), `INSERT INTO org_meter_segments(org_id,id,meter_id,kind,opened_at,opening_reading_id,opened_operation_id) VALUES($1,$2,$3,'RESET',now(),$4,$5)`, org, onboardingID(t), meterID, r1, operation)
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "42501") && !strings.Contains(err.Error(), "row-level security") {
		t.Fatal("foreign organization wrote a segment", err)
	}
}

// A populated 000247 head (a meter with an installation segment, valid and
// suspect readings and their history) upgrades to 000251 by an ordinary
// non-superuser migrator: an injected failure leaves schema and data
// untouched, recovery applies once, replay is a no-op, the previous image
// stays compatible, nothing existing is rewritten, every legacy reading
// counts in its recorded segment, and the legacy meter can then be reset.
func TestMeterSegmentsPopulatedUpgradeFailureRecoveryAndPreviousImage(t *testing.T) {
	previous := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() != "schema_min_supported" && (entry.Name() >= meterSegmentsMigration || !strings.HasSuffix(entry.Name(), ".sql")) {
			continue
		}
		raw, err := fs.ReadFile(migrations.Files, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		previous[entry.Name()] = &fstest.MapFile{Data: raw}
	}
	if _, ok := previous["000247_organization_tracking_policy_commands.sql"]; !ok {
		t.Fatal("previous image does not contain the 000247 head")
	}
	f := orgtest.NewWithMigrations(t, previous)
	_, _, onboarding, actor, flags := onboardingOnFixture(t, f)
	legal := legalFixtureFor(t, f, onboarding, actor, flags)
	fx := syncFixtureOn(t, devicesFixtureOn(t, legal))
	base := time.Now().UTC().Truncate(time.Second).Add(-10 * 24 * time.Hour)
	assets := fleetStore(t, fx.legalFixtureState)
	asset, _, err := fleetWrite(t, fx.legalFixtureState, assets, fleetCommand(t, fleetCreateInput()))
	if err != nil {
		t.Fatal(err)
	}
	store := meterStore(t, fx.legalFixtureState)
	installed, _, err := meterInstall(t, fx.legalFixtureState, store, meterInstallCommand(t, func() app.MeterInstallInput {
		in := meterInstallInput(t, asset.AssetID, base)
		in.Value = odometerValue(t, 1_000_000)
		return in
	}()))
	if err != nil {
		t.Fatal(err)
	}
	segmentStore := store
	policy := meter.DefaultPlausibility()
	segmentStore.Plausibility = &policy
	segmentStore.Authority = &orgpg.MeterAuthority{Memberships: fx.legalFixtureState.store.Memberships, Roles: fx.legalFixtureState.store.Roles, Capabilities: fx.flags, Devices: fx.devices}
	r1, _ := validationRecord(t, fx, segmentStore, installed, base.Add(24*time.Hour), 1_200_000, readingStateRow{state: "VALID"})
	low, _ := validationRecord(t, fx, segmentStore, installed, base.Add(48*time.Hour), 5_000, readingStateRow{state: "SUSPECT", flags: "READING_BELOW_PREVIOUS"})
	snapshot := func() string {
		t.Helper()
		var value string
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x)-'removal_operation_id' ORDER BY org_id,id)FROM org_meters x),
(SELECT jsonb_agg(to_jsonb(x)-ARRAY['opened_operation_id','closing_kind','closing_value','closing_reading_id','closing_operation_id','closing_previous_reading_id','closing_implied_usage'] ORDER BY org_id,id)FROM org_meter_segments x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_meter_readings x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,reading_id,revision)FROM org_meter_reading_states x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_meter_commands x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_audit_events x))::text`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	before := snapshot()
	migrator := f.RuntimeRole + "_segments_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE,CREATE ON SCHEMA public TO "+role+";GRANT SELECT,INSERT ON schema_migrations TO "+role+
		";GRANT REFERENCES ON org_audit_events,org_meter_readings TO "+role+";ALTER TABLE org_meters OWNER TO "+role+";ALTER TABLE org_meter_segments OWNER TO "+role+";ALTER TABLE org_meter_reading_states OWNER TO "+role+
		";ALTER FUNCTION org_meter_flags_canonical(text[]) OWNER TO "+role+";ALTER FUNCTION org_meter_reading_state_guard() OWNER TO "+role); err != nil {
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
	raw, err := fs.ReadFile(migrations.Files, meterSegmentsMigration)
	if err != nil {
		t.Fatal(err)
	}
	guard := func() string {
		t.Helper()
		var body string
		if err := f.Admin.QueryRow(t.Context(), `SELECT pg_get_functiondef('org_meter_reading_state_guard()'::regprocedure)`).Scan(&body); err != nil {
			t.Fatal(err)
		}
		return body
	}
	oldGuard := guard()
	broken := fstest.MapFS{meterSegmentsMigration: &fstest.MapFile{Data: append(append([]byte{}, raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(t.Context(), location.String(), broken); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
		t.Fatal("injected segment migration failure not reached", err)
	}
	var clean bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT to_regprocedure('org_meter_reading_segment(uuid,uuid,uuid)') IS NULL AND to_regprocedure('org_meter_chain_complete()') IS NULL
 AND NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='org_meter_segments' AND column_name='closing_kind')
 AND EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='org_meters_immutable' AND tgtype&16<>0) AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version='000251')`).Scan(&clean); err != nil || !clean || guard() != oldGuard || before != snapshot() {
		t.Fatal("failed segment migration changed the original schema, guard or data", err)
	}
	current := fstest.MapFS{}
	for path, data := range previous {
		current[path] = data
	}
	current[meterSegmentsMigration] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 1 || applied[0] != "000251" {
		t.Fatal("ordinary segment migration recovery", applied, err)
	}
	if before != snapshot() {
		t.Fatal("segment upgrade rewrote existing evidence")
	}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 0 {
		t.Fatal("segment migration no-op replay", err)
	}
	if err := migrate.Check(t.Context(), f.Admin, previous); err != nil {
		t.Fatal("previous embedded image incompatible", err)
	}
	if err := f.Admin.QueryRow(t.Context(), `SELECT NOT rolsuper AND NOT rolbypassrls AND NOT rolcanlogin
 AND NOT EXISTS(SELECT 1 FROM pg_constraint WHERE connamespace='public'::regnamespace AND NOT convalidated)
 AND NOT EXISTS(SELECT 1 FROM org_meter_readings r WHERE org_meter_reading_segment(r.org_id,r.meter_id,r.id) IS DISTINCT FROM r.segment_id)
 AND NOT EXISTS(SELECT 1 FROM org_meter_segments WHERE closed_at IS NOT NULL OR closing_kind IS NOT NULL OR opened_operation_id IS NOT NULL)
 AND NOT EXISTS(SELECT 1 FROM org_meters WHERE removed_at IS NOT NULL OR removal_operation_id IS NOT NULL)
 FROM pg_roles WHERE rolname=$1`, migrator).Scan(&clean); err != nil || !clean {
		t.Fatal("legacy readings do not count in their recorded segment, or a closure was fabricated", err)
	}
	// The new guards and commands govern the legacy meter.
	role = pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "GRANT UPDATE(removed_at,removed_submicro,removal_operation_id) ON org_meters TO "+role+
		";GRANT UPDATE(closed_at,closed_submicro,closing_kind,closing_value,closing_reading_id,closing_operation_id,closing_previous_reading_id,closing_implied_usage) ON org_meter_segments TO "+role); err != nil {
		t.Fatal(err)
	}
	catalog, err := audit.NewCatalog(fleetpg.MeterAuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	segmentStore.Audit = auditpg.NewStore(fx.f.Runtime, catalog)
	segmentStore.Parents = correctionParents{}
	result, _, err := change(t, fx, segmentStore, resetInput(t, installed, 1, base.Add(36*time.Hour), 1_300_000, 0))
	if err != nil || result.PreviousReadingID != r1 || result.Implied == nil || *result.Implied != 100_000 || len(result.Reevaluated) != 1 || result.Reevaluated[0].ReadingID != low {
		t.Fatalf("legacy meter reset %+v %v", result, err)
	}
	if _, err := migrate.Apply(t.Context(), f.AdminDSN, migrations.Files); err != nil {
		t.Fatal("remaining embedded chain after the 251 rehearsal", err)
	}
}
