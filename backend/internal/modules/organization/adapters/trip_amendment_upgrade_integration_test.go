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
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

// A populated head before 000229 (an approved measured trip, a returned and
// a submitted trip with their history) upgrades through an ordinary
// non-superuser migrator: an injected failure changes no schema, function or
// row, recovery applies once, replay is a no-op, the previous embedded image
// stays compatible, the new histories are owned by the ordinary forced-RLS
// migrator, and afterwards the pre-existing trips are resubmitted, amended
// and held exactly as new ones (EMP-TRIP-015, EDR-234..237).
func TestTripAmendmentPopulatedUpgradeFailureRecoveryAndPreviousImage(t *testing.T) {
	const name = "000229_organization_trip_amendments.sql"
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
	if _, ok := previous["000228_organization_trip_period_lock.sql"]; !ok {
		t.Fatal("previous image does not reach the 000228 head")
	}
	f := orgtest.NewWithMigrations(t, previous)
	base := tripStateOn(t, f)
	tripReviewGrantsBefore229(t, f)
	periodGrant(t, f)
	x := tripAmendState{tripPeriodState: tripPeriodState{tripReviewState: tripReviewState{tripState: base}, periods: periodStoreFor(t, base.legalFixtureState)}}
	submit, err := trippg.NewSubmitHandler(x.authority, x.meters, x.store, x.store)
	if err != nil {
		t.Fatal(err)
	}
	x.submit, x.reviews = submit, tripReviewStoreOf(t, x.legalFixtureState)
	x.intake = syncpg.NewStore(tripReviewIntakeConfig(x.syncFixtureState))
	x.manager = x.reviewerFor(t, "FLEET_MANAGER", "ORG", "")
	d, at := x.assignedDriver(t)
	owner := x.owner()
	approved := x.measuredTrip(t, d, at)
	x.approve(t, owner, approved, 1)
	returned, _ := x.completed(t, d, at.Add(4*time.Hour))
	x.submitted(t, d, returned, at.Add(5*time.Hour+30*time.Minute))
	if _, _, err := x.decideBy(t, owner, returned, 1, review.Return, "Informe o motivo da parada"); err != nil {
		t.Fatal(err)
	}
	submitted, _ := x.completed(t, d, at.Add(6*time.Hour))
	x.submitted(t, d, submitted, at.Add(7*time.Hour+30*time.Minute))
	shape := func() string {
		t.Helper()
		var value string
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array(to_regclass('org_trip_amendments') IS NULL,to_regclass('org_trip_held_submissions') IS NULL,
 (SELECT prosrc FROM pg_proc WHERE oid='org_trip_review_entry_source()'::regprocedure),(SELECT prosrc FROM pg_proc WHERE oid='org_trip_review_published()'::regprocedure),
 (SELECT prosrc FROM pg_proc WHERE oid='org_trip_event_source()'::regprocedure),(SELECT prosrc FROM pg_proc WHERE oid='org_trip_held_event_source()'::regprocedure),
 (SELECT prosrc FROM pg_proc WHERE oid='org_sync_review_source_valid(uuid,uuid,text,bigint)'::regprocedure),
 (SELECT jsonb_agg(pg_get_constraintdef(oid) ORDER BY conname) FROM pg_constraint WHERE conrelid='org_trip_review_entries'::regclass))::text`).Scan(&value); err != nil {
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
	migrator := f.RuntimeRole + "_trip_amend_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE,CREATE ON SCHEMA public TO "+role+";GRANT SELECT,INSERT ON schema_migrations TO "+role+
		";GRANT REFERENCES ON org_trips,org_memberships,org_employee_identities,org_meter_readings,org_sync_review_items TO "+role+
		";ALTER TABLE org_trip_review_entries OWNER TO "+role+";ALTER FUNCTION org_trip_review_entry_source() OWNER TO "+role+";ALTER FUNCTION org_trip_review_published() OWNER TO "+role+
		";ALTER FUNCTION org_trip_event_source() OWNER TO "+role+";ALTER FUNCTION org_trip_held_event_source() OWNER TO "+role+
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
		t.Fatal("injected trip amendment migration failure not reached", err)
	}
	var clean bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version='000229')`).Scan(&clean); err != nil || !clean || shape() != beforeShape || snapshot() != before {
		t.Fatal("failed trip amendment migration changed the schema or data", err)
	}
	current := fstest.MapFS{}
	for path, data := range previous {
		current[path] = data
	}
	current[name] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 1 || applied[0] != "000229" {
		t.Fatal("ordinary trip amendment migration recovery", applied, err)
	}
	if shape() == beforeShape || snapshot() != before {
		t.Fatal("trip amendment upgrade did not install its guards or rewrote existing state")
	}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 0 {
		t.Fatal("trip amendment migration no-op replay", err)
	}
	if err := migrate.Check(t.Context(), f.Admin, previous); err != nil {
		t.Fatal("previous embedded image incompatible", err)
	}
	if err := f.Admin.QueryRow(t.Context(), `SELECT bool_and(relrowsecurity AND relforcerowsecurity AND relowner=(SELECT oid FROM pg_roles WHERE rolname=$1 AND NOT rolsuper AND NOT rolbypassrls))
 FROM pg_class WHERE oid IN('org_trip_amendments'::regclass,'org_trip_held_submissions'::regclass)`, migrator).Scan(&clean); err != nil || !clean {
		t.Fatal("new histories not owned by the ordinary forced-RLS migrator", err)
	}
	if _, err := migrate.Apply(t.Context(), f.AdminDSN, migrations.Files); err != nil {
		t.Fatal("remaining embedded chain after the 229 rehearsal", err)
	}
	// The pre-existing trips under the upgraded schema and owners.
	tripReviewGrants(t, f)
	amendReviews(x.tripReviewState)
	x.events = &trippg.EventStore{NewID: func() (string, error) { return onboardingID(t), nil }}
	if x.event, err = trippg.NewEventHandler(x.authority, x.meters, x.store, x.events); err != nil {
		t.Fatal(err)
	}
	x.intake = syncpg.NewStore(tripAmendIntakeConfig(x.syncFixtureState))
	resubmit := x.submitOp(t, d, returned, 1, at.Add(8*time.Hour), nil)
	if ack, err := x.process(t, d, resubmit); err != nil || ack.Result() != protocol.Accepted || *ack.Snapshot().EntityRevision != 2 {
		t.Fatal("the pre-existing returned trip was not resubmitted", err)
	}
	x.heldEvent(t, d, x.eventOp(t, d, submitted, "STOP", nil, at.Add(6*time.Hour+30*time.Minute)), tripapp.StaleRevision)
	if r, _, err := x.propose(x.reviews, x.manager, approved, 1, nil, odometer(184_600_000), "Leitura final digitada errada", onboardingID(t)); err != nil || r.Revision != 2 {
		t.Fatal("the pre-existing approved trip was not amended", err)
	}
	x.approve(t, owner, approved, 2)
	if view, err := x.read(owner, approved); err != nil || view.EffectiveRevision != 2 || view.EffectiveUsage.Delta != 280_000 {
		t.Fatal("the upgraded amendment is not in effect", err)
	}
}

// tripReviewGrantsBefore229 grants the EMP-TRIP-004/016 runtime privileges
// on a schema that does not have the 229 tables yet.
func tripReviewGrantsBefore229(t *testing.T, f *orgtest.Fixture) {
	t.Helper()
	role := pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_trip_review_entries TO "+role+
		";GRANT SELECT ON org_trips,org_trip_finishes,org_team_links,org_memberships,org_employee_identities,org_audit_events,org_sync_operations,org_sync_acknowledgements,org_sync_effects,org_device_enrollments TO "+role+
		";GRANT SELECT,INSERT,UPDATE ON identity_rate_windows TO "+role+";GRANT SELECT ON org_finance_periods,org_finance_period_transitions TO "+role); err != nil {
		t.Fatal(err)
	}
}
