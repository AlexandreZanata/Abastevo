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
	fuel "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/domain"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	syncpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/adapters"
	review "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain"
	trippg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

// tripStateOn is the EMP-TRIP-002 trip fixture (an ACTIVE tanked vehicle
// with an odometer, the ADMIN member assigned to it and the real start and
// finish owners) built on a given fixture database.
func tripStateOn(t *testing.T, f *orgtest.Fixture) tripState {
	t.Helper()
	_, _, onboarding, actor, flags := onboardingOnFixture(t, f)
	fx, _, meters, installed, _, revision := lifecycleIntakeFixtureOn(t, syncFixtureOn(t, devicesFixtureOn(t, legalFixtureFor(t, f, onboarding, actor, flags))))
	assets := assignmentStore(t, fx.legalFixtureState)
	meters.Authority = &orgpg.MeterAuthority{Memberships: fx.legalFixtureState.store.Memberships, Roles: fx.legalFixtureState.store.Roles, Capabilities: fx.flags, Devices: fx.devices, Assignee: &orgpg.ResourceAssignee{Pool: fx.f.Runtime, Assignments: assets}}
	handler, err := fleetpg.NewMeterIntakeHandler(meters.Authority, meters)
	if err != nil {
		t.Fatal(err)
	}
	var unit string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT id::text FROM org_units WHERE org_id=$1`, fx.scope.OrgID().String()).Scan(&unit); err != nil {
		t.Fatal(err)
	}
	x := tripState{selfIntakeState: selfIntakeState{syncFixtureState: fx, assets: assets, meters: meters, installed: installed, handler: handler, intake: syncpg.NewStore(meterReviewIntakeConfig(fx)), revision: revision, unit: unit}}
	tripGrants(t, x.legalFixtureState)
	meterAuthority, ok := x.meters.Authority.(*orgpg.MeterAuthority)
	if !ok {
		t.Fatal("fixture meter authority is not the real one")
	}
	x.authority = (*orgpg.TripAuthority)(meterAuthority)
	x.store = &trippg.Store{Allocations: x.assets, NewID: func() (string, error) { return onboardingID(t), nil }, PreLifecycleImage: !lifecycleImage(t, x.legalFixtureState),
		PreHandoverImage: !handoverImage(t, x.f)}
	x.assets.OpenWork, x.assets.ActiveUse, x.assets.MaintenanceHolds = trippg.OpenWork{}, trippg.ActiveUse{PreLifecycleImage: x.store.PreLifecycleImage}, fleetpg.NoMaintenanceHolds{}
	x.assets.ChecklistBlocks = checklistBlockSource(t, x.legalFixtureState)
	x.start, x.finish = x.handlers(t, x.meters, x.store)
	x.trips = syncpg.NewStore(tripIntakeConfig(x.syncFixtureState))
	input := fleetCreateInput()
	input.Profile.Kind, input.Profile.UsageBasis, input.UnitID = fleet.Vehicle, fleet.Distance, x.unit
	tank, err := fleet.NewTank(fuel.Catalog{}, fleet.Main, "100000", []string{"DIESEL_S10"})
	if err != nil {
		t.Fatal(err)
	}
	input.Profile.Tanks = []fleet.Tank{tank}
	created, _, err := fleetWrite(t, x.legalFixtureState, x.assets, fleetCommand(t, input))
	if err != nil {
		t.Fatal(err)
	}
	x.vehicle, _, err = meterInstall(t, x.legalFixtureState, x.meters, meterInstallCommand(t, meterInstallInput(t, created.AssetID, lifecycleDBNow(t, x.legalFixtureState).Add(-6*time.Hour))))
	if err != nil {
		t.Fatal(err)
	}
	active, _, err := fleetWrite(t, x.legalFixtureState, x.assets, fleetCommand(t, fa.AssetWriteInput{Kind: fa.ActivateAsset, AssetID: created.AssetID, ExpectedRevision: x.vehicle.AssetRevision}))
	if err != nil || active.State != fleet.Active {
		t.Fatal("tanked vehicle not active", err)
	}
	x.vehicleAt = active.Revision
	x.assign(t, x.person.employee, x.vehicle.AssetID, fleet.PrimaryAssignment, nil, nil)
	return x
}

// A populated head before 000226 (a completed DRAFT trip with its receipts)
// upgrades through an ordinary non-superuser migrator: an injected failure
// changes no schema or data, recovery applies once, replay is a no-op, the
// previous embedded image stays compatible, the new history is owned by the
// ordinary forced-RLS migrator and the pre-existing trip is then submitted
// and decided (EMP-TRIP-004, EDR-221).
func TestTripReviewPopulatedUpgradeFailureRecoveryAndPreviousImage(t *testing.T) {
	const name = "000226_organization_trip_reviews.sql"
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
		t.Fatal("previous image does not reach the 000224 head")
	}
	f := orgtest.NewWithMigrations(t, previous)
	base := tripStateOn(t, f)
	x := tripReviewState{tripState: base, intake: base.trips}
	at := x.declaredNow(t)
	trip, _ := x.completed(t, x.syncFixtureState, at)
	snapshot := func() string {
		t.Helper()
		var value string
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_trips x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,trip_id)FROM org_trip_finishes x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_sync_operations x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_sync_acknowledgements x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_sync_effects x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_audit_events x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_memberships x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_assets x))::text`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	before := snapshot()
	migrator := f.RuntimeRole + "_trip_review_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE,CREATE ON SCHEMA public TO "+role+";GRANT SELECT,INSERT ON schema_migrations TO "+role+
		";GRANT REFERENCES ON org_trips,org_memberships,org_employee_identities TO "+role); err != nil {
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
		t.Fatal("injected trip review migration failure not reached", err)
	}
	var clean bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('org_trip_review_entries') IS NULL AND to_regprocedure('org_trip_review_entry_source()') IS NULL
 AND to_regprocedure('org_trip_review_published()') IS NULL AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version='000226')`).Scan(&clean); err != nil || !clean || before != snapshot() {
		t.Fatal("failed trip review migration changed the schema or data", err)
	}
	current := fstest.MapFS{}
	for path, data := range previous {
		current[path] = data
	}
	current[name] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 1 || applied[0] != "000226" {
		t.Fatal("ordinary trip review migration recovery", applied, err)
	}
	if before != snapshot() {
		t.Fatal("trip review upgrade rewrote existing state")
	}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 0 {
		t.Fatal("trip review migration no-op replay", err)
	}
	if err := migrate.Check(t.Context(), f.Admin, previous); err != nil {
		t.Fatal("previous embedded image incompatible", err)
	}
	if err := f.Admin.QueryRow(t.Context(), `SELECT relrowsecurity AND relforcerowsecurity AND relowner=(SELECT oid FROM pg_roles WHERE rolname=$1 AND NOT rolsuper AND NOT rolbypassrls) FROM pg_class WHERE oid='org_trip_review_entries'::regclass`, migrator).Scan(&clean); err != nil || !clean {
		t.Fatal("review history not owned by the ordinary forced-RLS migrator", err)
	}
	if _, err := migrate.Apply(t.Context(), f.AdminDSN, migrations.Files); err != nil {
		t.Fatal("remaining embedded chain after the 226 rehearsal", err)
	}
	// The pre-existing trip is a draft that its author now submits and an
	// independent reviewer decides.
	tripReviewGrants(t, f)
	submit, err := trippg.NewSubmitHandler(x.authority, x.meters, x.store, x.store)
	if err != nil {
		t.Fatal(err)
	}
	x.submit, x.intake, x.reviews = submit, syncpg.NewStore(tripReviewIntakeConfig(x.syncFixtureState)), tripReviewStoreOf(t, x.legalFixtureState)
	if entries, _ := x.history(t, x.scope, trip); entries != "" {
		t.Fatal("the upgrade invented review history", entries)
	}
	x.submitted(t, x.syncFixtureState, trip, at.Add(2*time.Hour))
	if result, _, err := x.decide(x.scope, x.owner(), trip, 1, review.Approve, "", onboardingID(t)); err != nil || result.State != review.Approved {
		t.Fatal("upgraded trip decision", err)
	}
	if entries, audits := x.history(t, x.scope, trip); entries != "SUBMIT:SUBMITTED:1,OPEN:IN_REVIEW:1,APPROVE:APPROVED:1" || audits != 1 {
		t.Fatal("upgraded trip history", entries, audits)
	}
}
