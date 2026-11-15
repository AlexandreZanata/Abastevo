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
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

// A populated 000224 head (organization, ACTIVE vehicle with an observed
// meter, a completed trip and a trip in progress, both through the real signed
// intake) upgrades to 000225 by an ordinary non-superuser migrator: an
// injected failure leaves schema and data untouched, recovery applies once,
// replay is a no-op, the previous embedded image stays compatible, the
// review-source hook keeps its meter, trip.start and trip.finish branches, and
// the pre-existing trips then take logbook events (EMP-TRIP-003).
func TestTripEventPopulatedUpgradeFailureRecoveryAndPreviousImage(t *testing.T) {
	const name = "000225_organization_trip_events.sql"
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
	if _, ok := previous["000224_organization_reference_feed.sql"]; !ok {
		t.Fatal("previous image does not end at the 000224 head")
	}
	f := orgtest.NewWithMigrations(t, previous)
	_, _, onboarding, actor, flags := onboardingOnFixture(t, f)
	legal := legalFixtureFor(t, f, onboarding, actor, flags)
	x := tripFixtureOn(t, selfIntakeFixtureOn(t, syncFixtureOn(t, devicesFixtureOn(t, legal))))
	d := x.syncFixtureState
	completedAt := x.declaredNow(t)
	first := x.startOp(t, d, completedAt, nil)
	ack, err := x.process(t, d, first)
	x.accepted(t, first, ack, err, 1, 1)
	firstFinish := completedAt.Add(time.Microsecond)
	end := x.finishOp(t, d, tripOf(first), firstFinish, nil)
	ack, err = x.process(t, d, end)
	x.accepted(t, end, ack, err, 2, 1)
	ongoingAt := x.declaredNow(t)
	ongoing := x.startOp(t, d, ongoingAt, nil)
	ack, err = x.process(t, d, ongoing)
	x.accepted(t, ongoing, ack, err, 1, 1)
	org := x.scope.OrgID().String()
	snapshot := func() string {
		t.Helper()
		var value string
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id)FROM organizations x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_units x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_memberships x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_employee_identities x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_assets x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_meters x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_meter_readings x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_trips x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,trip_id)FROM org_trip_finishes x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_sync_operations x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_sync_acknowledgements x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_sync_review_items x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_audit_events x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY id)FROM accounts x))::text`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	usage := func() string {
		t.Helper()
		tx, err := f.Admin.Begin(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(context.Background()) }()
		var asset string
		if err := tx.QueryRow(t.Context(), `SELECT set_config('anpfuel.org_id',$1,true)`, org).Scan(new(string)); err != nil {
			t.Fatal(err)
		}
		if err := tx.QueryRow(t.Context(), `SELECT org_asset_current_usage($1,$2)::text`, org, x.vehicle.AssetID).Scan(&asset); err != nil {
			t.Fatal("usage source refused the populated state", err)
		}
		return asset
	}
	before, beforeUsage := snapshot(), usage()
	migrator := f.RuntimeRole + "_event_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE,CREATE ON SCHEMA public TO "+role+";GRANT SELECT,INSERT ON schema_migrations TO "+role+
		";GRANT REFERENCES ON org_trips,org_sync_operations,org_memberships,org_employee_identities,org_device_enrollments,org_sync_review_items TO "+role+
		";ALTER FUNCTION org_sync_review_source_valid(uuid,uuid,text,bigint) OWNER TO "+role); err != nil {
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
		t.Fatal(err)
	}
	broken := fstest.MapFS{name: &fstest.MapFile{Data: append(append([]byte{}, raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(t.Context(), location.String(), broken); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
		t.Fatal("injected logbook migration failure not reached", err)
	}
	var clean bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('org_trip_events') IS NULL AND to_regclass('org_trip_held_events') IS NULL AND to_regprocedure('org_trip_event_source()') IS NULL
 AND to_regprocedure('org_trip_held_event_source()') IS NULL AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version='000225')
 AND pg_get_functiondef('org_sync_review_source_valid(uuid,uuid,text,bigint)'::regprocedure) NOT LIKE '%trip.event.record%'`).Scan(&clean); err != nil || !clean || before != snapshot() {
		t.Fatal("failed logbook migration changed the original schema or data", err)
	}
	current := fstest.MapFS{}
	for path, data := range previous {
		current[path] = data
	}
	current[name] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 1 || applied[0] != "000225" {
		t.Fatal("ordinary logbook migration recovery", applied, err)
	}
	if before != snapshot() || beforeUsage != usage() {
		t.Fatal("logbook upgrade rewrote existing state or the asset usage answer")
	}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 0 {
		t.Fatal("logbook migration no-op replay", err)
	}
	if err := migrate.Check(t.Context(), f.Admin, previous); err != nil {
		t.Fatal("previous embedded image incompatible", err)
	}
	if err := f.Admin.QueryRow(t.Context(), `SELECT NOT rolsuper AND NOT rolbypassrls AND NOT rolcanlogin AND(SELECT count(*)=2 AND bool_and(relrowsecurity AND relforcerowsecurity AND relowner=(SELECT oid FROM pg_roles WHERE rolname=$1))
 FROM pg_class WHERE oid=ANY(ARRAY['org_trip_events'::regclass,'org_trip_held_events'::regclass]))FROM pg_roles WHERE rolname=$1`, migrator).Scan(&clean); err != nil || !clean {
		t.Fatal("new logbook tables not owned by the ordinary forced-RLS migrator", err)
	}
	var hook bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT def LIKE '%meter.reading.record%' AND def LIKE '%org_meter_held_readings%' AND def LIKE '%org_trip_held_starts%' AND def LIKE '%org_trip_held_finishes%'
 AND def LIKE '%trip.event.record%' AND def LIKE '%org_trip_held_events%' FROM(SELECT pg_get_functiondef('org_sync_review_source_valid(uuid,uuid,text,bigint)'::regprocedure) AS def) h`).Scan(&hook); err != nil || !hook {
		t.Fatal("the review-source hook lost a branch or misses the logbook owner", err)
	}
	// The pre-existing trips take logbook events on the upgraded schema.
	e := eventFixtureOn(t, x)
	inProgress := e.eventOp(t, d, tripOf(ongoing), "ARRIVAL", "Cliente Norte", ongoingAt.Add(time.Minute), nil)
	ack, err = e.process(t, d, inProgress)
	e.accepted(t, inProgress, ack, err)
	late := e.eventOp(t, d, tripOf(first), "NOTE", "Entregue durante a viagem", firstFinish, nil)
	ack, err = e.process(t, d, late)
	e.accepted(t, late, ack, err)
	after := e.eventOp(t, d, tripOf(first), "STOP", nil, firstFinish.Add(time.Nanosecond), nil)
	ack, err = e.process(t, d, after)
	e.held(t, after, ack, err)
	if _, err := migrate.Apply(t.Context(), f.AdminDSN, migrations.Files); err != nil {
		t.Fatal("remaining embedded chain after the 225 rehearsal", err)
	}
}
