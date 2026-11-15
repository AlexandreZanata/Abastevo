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
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

const conflictMigration = "000237_organization_trip_conflict_resolutions.sql"

// A populated head before 000237 (a completed trip, a held start and a held
// finish through the real signed intake) upgrades to 000237 by an ordinary non-superuser migrator: an
// injected failure leaves schema, functions, constraints and data
// untouched, recovery applies once (the widened outcome check validated over
// the existing resolution table), replay is a no-op, the previous embedded
// image stays compatible, earlier receipts replay, and the held start is then
// resolved and accepted on the upgraded schema (EMP-TRIP-007, EDR-264).
func TestTripConflictPopulatedUpgradeFailureRecoveryAndPreviousImage(t *testing.T) {
	previous := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() != "schema_min_supported" && (entry.Name() >= conflictMigration || !strings.HasSuffix(entry.Name(), ".sql")) {
			continue
		}
		raw, err := fs.ReadFile(migrations.Files, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		previous[entry.Name()] = &fstest.MapFile{Data: raw}
	}
	if _, ok := previous["000232_organization_trip_mission_vehicles.sql"]; !ok {
		t.Fatal("previous image does not contain 000232")
	}
	f := orgtest.NewWithMigrations(t, previous)
	_, _, onboarding, actor, flags := onboardingOnFixture(t, f)
	legal := legalFixtureFor(t, f, onboarding, actor, flags)
	x := tripFixtureOn(t, selfIntakeFixtureOn(t, syncFixtureOn(t, devicesFixtureOn(t, legal))))
	d := x.syncFixtureState
	b := x.driver(t)
	x.assign(t, b.person.employee, x.vehicle.AssetID, fleet.AdditionalAssignment, nil, nil)
	at := x.declaredNow(t)
	running := x.startOp(t, d, at, absentReading)
	ack, err := x.process(t, d, running)
	x.accepted(t, running, ack, err, 1, 0)
	held := x.startOp(t, b, at.Add(2*time.Hour), nil)
	heldAck, err := x.process(t, b, held)
	x.held(t, held, heldAck, err, tripapp.AssetBusy)
	ack, err = x.process(t, d, x.finishOp(t, d, tripOf(running), at.Add(time.Hour), absentReading))
	if err != nil || ack.Result() != protocol.Accepted {
		t.Fatal("finish before the upgrade", err)
	}
	second := x.finishOp(t, d, tripOf(running), at.Add(3*time.Hour), absentReading)
	secondAck, err := x.process(t, d, second)
	x.held(t, second, secondAck, err, tripapp.TripAlreadyClosed)
	org := x.scope.OrgID().String()
	snapshot := func() string {
		t.Helper()
		var value string
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_trips x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,trip_id)FROM org_trip_finishes x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,review_item_id)FROM org_trip_held_starts x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,review_item_id)FROM org_trip_held_finishes x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_sync_review_items x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_sync_review_resolutions x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_meter_readings x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_sync_effects x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_sync_acknowledgements x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_audit_events x))::text`).Scan(&value); err != nil {
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
		if _, err := tx.Exec(t.Context(), `SELECT set_config('anpfuel.org_id',$1,true)`, org); err != nil {
			t.Fatal(err)
		}
		var a string
		if err := tx.QueryRow(t.Context(), `SELECT org_asset_current_usage($1,$2)::text||org_unit_current_usage($1,$3)::text`, org, x.vehicle.AssetID, x.unit).Scan(&a); err != nil {
			t.Fatal("usage refused the populated state", err)
		}
		return a
	}
	replaced := []string{"org_sync_review_resolution_source()", "org_sync_review_resolution_complete()", "org_meter_observation_guard()", "org_sync_effect_source()", "org_trip_source()", "org_trip_complete()"}
	functions := func() string {
		t.Helper()
		var v string
		if err := f.Admin.QueryRow(t.Context(), `SELECT string_agg(pg_get_functiondef(to_regprocedure(p)),E'\n' ORDER BY p) FROM unnest($1::text[]) p`, replaced).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	constraints := func() string {
		t.Helper()
		var v string
		if err := f.Admin.QueryRow(t.Context(), `SELECT string_agg(conname||'='||pg_get_constraintdef(oid),E'\n' ORDER BY conname) FROM pg_constraint WHERE conrelid='org_sync_review_resolutions'::regclass`).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	before, beforeUsage, beforeFunctions, beforeConstraints := snapshot(), usage(), functions(), constraints()
	migrator := f.RuntimeRole + "_conflict_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	grants := "CREATE ROLE " + role + " NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE,CREATE ON SCHEMA public TO " + role + ";GRANT SELECT,INSERT ON schema_migrations TO " + role +
		";ALTER TABLE org_sync_review_resolutions OWNER TO " + role
	for _, p := range replaced {
		grants += ";ALTER FUNCTION " + p + " OWNER TO " + role
	}
	if _, err := f.Admin.Exec(t.Context(), grants); err != nil {
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
	raw, err := fs.ReadFile(migrations.Files, conflictMigration)
	if err != nil {
		t.Fatal(err)
	}
	version := strings.SplitN(conflictMigration, "_", 2)[0]
	broken := fstest.MapFS{conflictMigration: &fstest.MapFile{Data: append(append([]byte{}, raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(t.Context(), location.String(), broken); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
		t.Fatal("injected conflict migration failure not reached", err)
	}
	var clean bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT to_regprocedure('org_trip_review_published(uuid,uuid,uuid)') IS NULL AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, version).Scan(&clean); err != nil || !clean ||
		before != snapshot() || beforeFunctions != functions() || beforeConstraints != constraints() {
		t.Fatal("failed conflict migration changed the original schema, functions, constraints or data", err)
	}
	current := fstest.MapFS{}
	for path, data := range previous {
		current[path] = data
	}
	current[conflictMigration] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 1 || applied[0] != version {
		t.Fatal("ordinary conflict migration recovery", applied, err)
	}
	if before != snapshot() || beforeUsage != usage() || beforeConstraints == constraints() || !strings.Contains(constraints(), "org_sync_review_resolutions_outcome=") {
		t.Fatal("conflict upgrade rewrote existing state or a usage answer, or kept the old outcome check")
	}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 0 {
		t.Fatal("conflict migration no-op replay", err)
	}
	if err := migrate.Check(t.Context(), f.Admin, previous); err != nil {
		t.Fatal("previous embedded image incompatible", err)
	}
	// Earlier receipts replay byte for byte on the upgraded schema.
	x.replays(t, b, held, heldAck)
	x.replays(t, d, second, secondAck)
	// The held start is resolved and accepted through the real store.
	r := tripReviewState{tripState: x}
	tripReviewGrants(t, f)
	c := conflictState{tripReviewState: r, conflicts: conflictStoreOf(t, r)}
	owner := c.owner()
	finishItem := secondAck.Snapshot().ReviewItemID
	if res, _, err := c.resolve(owner, finishItem, 1, tripapp.ConflictRefuse, "Segunda finalização", onboardingID(t)); err != nil || res.Snapshot().Outcome != protocol.ReviewRefused {
		t.Fatal("refusal on the upgraded schema", err)
	}
	if res, _, err := c.resolve(owner, heldAck.Snapshot().ReviewItemID, 1, tripapp.ConflictAccept, "", onboardingID(t)); err != nil || res.Snapshot().Outcome != protocol.ReviewAccepted {
		t.Fatal("acceptance on the upgraded schema", err)
	}
	if c.inProgress(t) != 1 || c.conflictCount(t, held) != (conflictCounts{resolutions: 1, audits: 1, trips: 1, effects: 1, readings: 1}) {
		t.Fatal("accepted start on the upgraded schema", c.conflictCount(t, held))
	}
}
