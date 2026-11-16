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
	syncpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	review "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain"
	trippg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/adapters"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	tripdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

const tripClosureMigration = "000235_organization_trip_closures.sql"

// A populated head before 000235 (a completed and submitted trip, a trip in
// progress) upgrades through an ordinary non-superuser migrator: an injected
// failure changes no schema, function or row, recovery applies once, replay
// is a no-op, the previous embedded image stays compatible, the closures are
// owned by the ordinary forced-RLS migrator, and afterwards the pre-existing
// running trip is closed, its driver's finish held, and the pre-existing
// submitted trip decided under the replaced guards (EMP-TRIP-014, EDR-258).
func TestTripClosurePopulatedUpgradeFailureRecoveryAndPreviousImage(t *testing.T) {
	previous := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() != "schema_min_supported" && (entry.Name() >= tripClosureMigration || !strings.HasSuffix(entry.Name(), ".sql")) {
			continue
		}
		raw, err := fs.ReadFile(migrations.Files, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		previous[entry.Name()] = &fstest.MapFile{Data: raw}
	}
	if _, ok := previous["000232_organization_trip_mission_vehicles.sql"]; !ok {
		t.Fatal("previous image does not reach the 000232 head")
	}
	f := orgtest.NewWithMigrations(t, previous)
	x := tripReviewState{tripState: tripStateOn(t, f)}
	tripReviewGrants(t, f)
	periodGrant(t, f)
	submit, err := trippg.NewSubmitHandler(x.authority, x.meters, x.store, x.store)
	if err != nil {
		t.Fatal(err)
	}
	x.submit, x.reviews = submit, tripReviewStoreOf(t, x.legalFixtureState)
	x.intake = syncpg.NewStore(tripReviewIntakeConfig(x.syncFixtureState))
	d, at := x.assignedDriver(t)
	submitted, _ := x.completed(t, d, at)
	x.submitted(t, d, submitted, at.Add(90*time.Minute))
	start := x.startOp(t, d, at.Add(2*time.Hour), absentReading)
	ack, err := x.process(t, d, start)
	x.accepted(t, start, ack, err, 1, 0)
	running := tripOf(start)
	shape := func() string {
		t.Helper()
		var value string
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array(to_regclass('org_trip_closures') IS NULL,
 (SELECT prosrc FROM pg_proc WHERE oid='org_trip_completion_guard()'::regprocedure),(SELECT prosrc FROM pg_proc WHERE oid='org_trip_event_source()'::regprocedure),
 (SELECT prosrc FROM pg_proc WHERE oid='org_trip_held_event_source()'::regprocedure),(SELECT prosrc FROM pg_proc WHERE oid='org_trip_review_entry_source()'::regprocedure),
 (SELECT jsonb_agg(tgname ORDER BY tgname) FROM pg_trigger WHERE tgrelid='org_trips'::regclass))::text`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	snapshot := func() string {
		t.Helper()
		var value string
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_trips x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,trip_id)FROM org_trip_finishes x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,trip_id,sequence)FROM org_trip_review_entries x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_audit_events x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_sync_acknowledgements x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_meter_readings x))::text`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	beforeShape, before := shape(), snapshot()
	migrator := f.RuntimeRole + "_trip_closure_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE,CREATE ON SCHEMA public TO "+role+";GRANT SELECT,INSERT ON schema_migrations TO "+role+
		";GRANT REFERENCES ON org_trips,org_memberships,org_employee_identities,org_audit_events TO "+role+
		";ALTER FUNCTION org_trip_completion_guard() OWNER TO "+role+";ALTER FUNCTION org_trip_revision(uuid,uuid) OWNER TO "+role+";ALTER FUNCTION org_trip_event_source() OWNER TO "+role+
		";ALTER FUNCTION org_trip_held_event_source() OWNER TO "+role+";ALTER FUNCTION org_trip_review_entry_source() OWNER TO "+role); err != nil {
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
	raw, err := fs.ReadFile(migrations.Files, tripClosureMigration)
	if err != nil {
		t.Fatal(err)
	}
	broken := fstest.MapFS{tripClosureMigration: &fstest.MapFile{Data: append(append([]byte{}, raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(t.Context(), location.String(), broken); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
		t.Fatal("injected trip closure migration failure not reached", err)
	}
	var clean bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version='000235')`).Scan(&clean); err != nil || !clean || shape() != beforeShape || snapshot() != before {
		t.Fatal("failed trip closure migration changed the schema or data", err)
	}
	current := fstest.MapFS{}
	for path, data := range previous {
		current[path] = data
	}
	current[tripClosureMigration] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 1 || applied[0] != "000235" {
		t.Fatal("ordinary trip closure migration recovery", applied, err)
	}
	if shape() == beforeShape || snapshot() != before {
		t.Fatal("trip closure upgrade did not install its guards or rewrote existing state")
	}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 0 {
		t.Fatal("trip closure migration no-op replay", err)
	}
	if err := migrate.Check(t.Context(), f.Admin, previous); err != nil {
		t.Fatal("previous embedded image incompatible", err)
	}
	if err := f.Admin.QueryRow(t.Context(), `SELECT relrowsecurity AND relforcerowsecurity AND relowner=(SELECT oid FROM pg_roles WHERE rolname=$1 AND NOT rolsuper AND NOT rolbypassrls)
 FROM pg_class WHERE oid='org_trip_closures'::regclass`, migrator).Scan(&clean); err != nil || !clean {
		t.Fatal("closures not owned by the ordinary forced-RLS migrator", err)
	}
	if _, err := migrate.Apply(t.Context(), f.AdminDSN, migrations.Files); err != nil {
		t.Fatal("remaining embedded chain after the 235 rehearsal", err)
	}
	// The pre-existing trips under the upgraded schema and guards.
	tripClosureGrant(t, f)
	tripHandoverGrant(t, f)
	closures := tripClosureState{tripAmendState: tripAmendState{tripPeriodState: tripPeriodState{tripReviewState: x}}, closures: tripClosureStoreOf(t, x.legalFixtureState, f.Runtime)}
	supervisor := x.reviewerFor(t, "SUPERVISOR", "ORG", "")
	result, _, err := closures.close(x.scope, supervisor, running, 1, closureReason, closureMissing, onboardingID(t))
	if err != nil || result.Operational != tripdomain.Completed || result.UsageReason != tripdomain.ReasonReadingAbsent {
		t.Fatal("the pre-existing running trip was not closed", err)
	}
	finish := x.finishOp(t, d, running, at.Add(3*time.Hour), absentReading)
	ack, err = x.process(t, d, finish)
	x.held(t, finish, ack, err, tripapp.TripAlreadyClosed)
	if _, _, err := x.decide(x.scope, x.owner(), submitted, 1, review.Approve, "", onboardingID(t)); err != nil {
		t.Fatal("the pre-existing submitted trip was not decided under the replaced guard", err)
	}
	if ack.Result() != protocol.AcceptedForReview {
		t.Fatal("held finish result", ack.Result())
	}
}
