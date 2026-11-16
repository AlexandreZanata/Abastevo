//go:build integration

package adapters_test

import (
	"context"
	"io/fs"
	"net/url"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	memberdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	syncpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/adapters"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	trippg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

const tripMembershipEndMigration = "000236_organization_trip_membership_end.sql"

// A populated head before 000236 (a member with a trip in progress)
// upgrades through an ordinary non-superuser migrator: an injected failure
// changes no function or row, recovery applies once, replay is a no-op, the
// previous embedded image stays compatible, and afterwards the pre-existing
// open trip is flagged by the offboarding of its member and its pre-end
// finish is held late (EMP-TRIP-018, EDR-260/261).
func TestTripMembershipEndPopulatedUpgradeFailureRecoveryAndPreviousImage(t *testing.T) {
	previous := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() != "schema_min_supported" && (entry.Name() >= tripMembershipEndMigration || !strings.HasSuffix(entry.Name(), ".sql")) {
			continue
		}
		raw, err := fs.ReadFile(migrations.Files, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		previous[entry.Name()] = &fstest.MapFile{Data: raw}
	}
	if _, ok := previous[tripClosureMigration]; !ok {
		t.Fatal("previous image does not reach the 000235 head")
	}
	f := orgtest.NewWithMigrations(t, previous)
	x := tripReviewState{tripState: tripStateOn(t, f)}
	tripReviewGrants(t, f)
	submit, err := trippg.NewSubmitHandler(x.authority, x.meters, x.store, x.store)
	if err != nil {
		t.Fatal(err)
	}
	x.submit, x.reviews = submit, tripReviewStoreOf(t, x.legalFixtureState)
	x.intake = syncpg.NewStore(tripReviewIntakeConfig(x.syncFixtureState))
	// The pre-end finish must be declared before the offboarding below, so
	// the running trip starts at the database's now (a completed trip of the
	// vehicle would hold that instant).
	d, at := x.assignedDriver(t)
	start := x.startOp(t, d, at, absentReading)
	ack, err := x.process(t, d, start)
	x.accepted(t, start, ack, err, 1, 0)
	running := tripOf(start)
	before := x.finishOp(t, d, running, at.Add(time.Microsecond), absentReading)
	shape := func() string {
		t.Helper()
		var value string
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT prosrc FROM pg_proc WHERE oid='org_offboarding_guard()'::regprocedure),
 (SELECT prosrc FROM pg_proc WHERE oid='org_trip_held_source()'::regprocedure))::text`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	snapshot := func() string {
		t.Helper()
		var value string
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_trips x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,trip_id)FROM org_trip_finishes x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_memberships x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_offboarding_commands x))::text`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	beforeShape, beforeData := shape(), snapshot()
	migrator := f.RuntimeRole + "_trip_end_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE,CREATE ON SCHEMA public TO "+role+";GRANT SELECT,INSERT ON schema_migrations TO "+role+
		";ALTER FUNCTION org_offboarding_guard() OWNER TO "+role+";ALTER FUNCTION org_trip_held_source() OWNER TO "+role); err != nil {
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
	raw, err := fs.ReadFile(migrations.Files, tripMembershipEndMigration)
	if err != nil {
		t.Fatal(err)
	}
	broken := fstest.MapFS{tripMembershipEndMigration: &fstest.MapFile{Data: append(append([]byte{}, raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(t.Context(), location.String(), broken); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
		t.Fatal("injected membership-end migration failure not reached", err)
	}
	var clean bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version='000236')`).Scan(&clean); err != nil || !clean || shape() != beforeShape || snapshot() != beforeData {
		t.Fatal("failed membership-end migration changed functions or data", err)
	}
	current := fstest.MapFS{}
	for path, data := range previous {
		current[path] = data
	}
	current[tripMembershipEndMigration] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 1 || applied[0] != "000236" {
		t.Fatal("ordinary membership-end migration recovery", applied, err)
	}
	if shape() == beforeShape || snapshot() != beforeData {
		t.Fatal("membership-end upgrade did not install its guards or rewrote existing state")
	}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 0 {
		t.Fatal("membership-end migration no-op replay", err)
	}
	if err := migrate.Check(t.Context(), f.Admin, previous); err != nil {
		t.Fatal("previous embedded image incompatible", err)
	}
	if _, err := migrate.Apply(t.Context(), f.AdminDSN, migrations.Files); err != nil {
		t.Fatal("remaining embedded chain after the 236 rehearsal", err)
	}
	// The pre-existing open trip under the upgraded guards.
	tripClosureGrant(t, f)
	closure := tripClosureState{tripAmendState: tripAmendState{tripPeriodState: tripPeriodState{tripReviewState: x}}}
	end := tripEndState{tripClosureState: closure, offboarding: tripEndOffboardingStore(t, closure, trippg.MembershipOpenWork{})}
	f.GrantLateIntake(t)
	config := tripReviewIntakeConfig(x.syncFixtureState)
	config.ConfirmLate = lateIntakeConfig(x.syncFixtureState).ConfirmLate
	end.late = syncpg.NewStore(config)
	v := end.review(t, end.offboarding, d.source.MembershipID)
	if !slices.Contains(keysOf(v.Items), "TRIP:"+running) {
		t.Fatal("the pre-existing open trip is not an open responsibility")
	}
	if out, err := end.offboard(t, end.offboarding, v, keysOf(v.Items)); err != nil || out.View.State.State != memberdomain.MembershipEnded {
		t.Fatal("the pre-existing member was not offboarded with its trip flag", err)
	}
	held, err := end.late.ProcessLate(context.Background(), d.source, before, protocol.DefaultLimits(), x.finish)
	if err != nil || held.Result() != protocol.AcceptedForReview || held.Code() != syncapp.LateHeldCode {
		t.Fatal("the pre-end finish of the pre-existing trip was not held late", err, held.Result(), held.Code())
	}
}
