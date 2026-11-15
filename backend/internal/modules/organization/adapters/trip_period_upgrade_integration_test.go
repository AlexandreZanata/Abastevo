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
	syncpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/adapters"
	review "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain"
	period "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain/period"
	trippg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/adapters"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

// A populated head before 000228 (a submitted trip of a locked month, an
// approved trip and the period history) upgrades through an ordinary
// non-superuser migrator: an injected failure keeps the 226 guard body and
// every row, recovery applies once, replay is a no-op, the previous embedded
// image stays compatible, and afterwards the database itself refuses a
// direct opening in the locked month while a reopening admits the decision
// (EMP-TRIP-016, EDR-233).
func TestTripPeriodPopulatedUpgradeFailureRecoveryAndPreviousImage(t *testing.T) {
	const name = "000228_organization_trip_period_lock.sql"
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
	if _, ok := previous["000226_organization_trip_reviews.sql"]; !ok {
		t.Fatal("previous image does not reach the 000226 head")
	}
	f := orgtest.NewWithMigrations(t, previous)
	base := tripStateOn(t, f)
	tripReviewGrantsBefore229(t, f)
	periodGrant(t, f)
	x := tripPeriodState{tripReviewState: tripReviewState{tripState: base}, periods: periodStoreFor(t, base.legalFixtureState)}
	submit, err := trippg.NewSubmitHandler(x.authority, x.meters, x.store, x.store)
	if err != nil {
		t.Fatal(err)
	}
	x.submit, x.intake, x.reviews = submit, syncpg.NewStore(tripReviewIntakeConfig(x.syncFixtureState)), tripReviewStoreOf(t, x.legalFixtureState)
	x.finance = x.reviewerFor(t, "FINANCE", "ORG", "")
	d, at := x.assignedDriver(t)
	approved := x.submittedTrip(t, d, at)
	pending := x.submittedTrip(t, d, at.Add(2*time.Hour))
	owner := x.owner()
	if _, _, err := x.decideBy(t, owner, approved, 1, review.Approve, ""); err != nil {
		t.Fatal(err)
	}
	month := x.month(t, pending)
	locked := x.lock(t, month, 1)
	reviewer := x.reviewerFor(t, "SUPERVISOR", "ORG", "")
	guarded := func() bool {
		t.Helper()
		var has bool
		if err := f.Admin.QueryRow(t.Context(), `SELECT prosrc LIKE '%trip reporting period locked%' FROM pg_proc WHERE oid='org_trip_review_entry_source()'::regprocedure`).Scan(&has); err != nil {
			t.Fatal(err)
		}
		return has
	}
	if guarded() {
		t.Fatal("the previous image already guards the period")
	}
	snapshot := func() string {
		t.Helper()
		var value string
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_trips x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,trip_id)FROM org_trip_finishes x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,trip_id,sequence)FROM org_trip_review_entries x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,period_month,revision)FROM org_finance_period_transitions x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,period_month)FROM org_finance_periods x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_audit_events x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_sync_acknowledgements x))::text`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	before := snapshot()
	migrator := f.RuntimeRole + "_trip_period_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE,CREATE ON SCHEMA public TO "+role+";GRANT SELECT,INSERT ON schema_migrations TO "+role+
		";ALTER FUNCTION org_trip_review_entry_source() OWNER TO "+role); err != nil {
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
		t.Fatal("injected trip period migration failure not reached", err)
	}
	var clean bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version='000228')`).Scan(&clean); err != nil || !clean || guarded() || before != snapshot() {
		t.Fatal("failed trip period migration changed the schema or data", err)
	}
	current := fstest.MapFS{}
	for path, data := range previous {
		current[path] = data
	}
	current[name] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 1 || applied[0] != "000228" {
		t.Fatal("ordinary trip period migration recovery", applied, err)
	}
	if !guarded() || before != snapshot() {
		t.Fatal("trip period upgrade did not install the guard or rewrote existing state")
	}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 0 {
		t.Fatal("trip period migration no-op replay", err)
	}
	if err := migrate.Check(t.Context(), f.Admin, previous); err != nil {
		t.Fatal("previous embedded image incompatible", err)
	}
	if _, err := migrate.Apply(t.Context(), f.AdminDSN, migrations.Files); err != nil {
		t.Fatal("remaining embedded chain after the 228 rehearsal", err)
	}
	tripReviewGrants(t, f)
	// The pre-existing submitted trip of the locked month: the handler gate
	// and the database guard both refuse; the approved trip stays approved.
	if _, _, err := x.decideBy(t, owner, pending, 1, review.Approve, ""); !errors.Is(err, tripapp.ErrPeriodLocked) {
		t.Fatal("upgraded locked month admitted a decision", err)
	}
	open := `INSERT INTO org_trip_review_entries(org_id,trip_id,sequence,kind,revision,state,reason,actor_membership_id,actor_employee_id,operation_id) VALUES($1,$2,2,'OPEN',1,'IN_REVIEW','',$3,$4,$5)`
	if err := unitSchemaExec(t, x.legalFixtureState, open, x.scope.OrgID().String(), pending, reviewer.member, reviewer.employee, onboardingID(t)); err == nil || !strings.Contains(err.Error(), "trip reporting period locked") {
		t.Fatal("the upgraded database admitted a direct opening in the locked month", err)
	}
	if entries, audits := x.history(t, x.scope, approved); entries != "SUBMIT:SUBMITTED:1,OPEN:IN_REVIEW:1,APPROVE:APPROVED:1" || audits != 1 {
		t.Fatal("the upgrade changed the approved trip", entries, audits)
	}
	x.reopen(t, month, locked.Revision, "Revisão após a atualização")
	if result, _, err := x.decideBy(t, owner, pending, 1, review.Return, "Informe o motivo da parada"); err != nil || result.State != review.Returned {
		t.Fatal("upgraded reopened month refused the decision", err)
	}
	if head, err := x.periods.ReadPeriod(t.Context(), x.scope, periodOwner(x.legalFixtureState), x.member, mustMonth(t, month)); err != nil || head.State != period.Reopened || len(head.History) != 2 {
		t.Fatal("period history after the upgrade", err)
	}
}
