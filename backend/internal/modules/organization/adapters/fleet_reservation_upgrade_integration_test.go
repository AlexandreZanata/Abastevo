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
	fa "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

// The one file-name constant of EMP-FLEET-007; the coordinator may renumber it.
const reservationMigration = "000217_organization_asset_reservations.sql"

// Populated previous head → 217: a failed attempt changes nothing; an ordinary
// NOSUPERUSER/NOBYPASSRLS migrator recovers; nothing is invented for existing
// assets, assignments or fuelings; the previous image stays compatible and
// the installed guards and the left-service consumer work after the upgrade.
func TestFleetReservationPopulatedUpgradeIsAdditiveAndRecoverable(t *testing.T) {
	version := strings.SplitN(reservationMigration, "_", 2)[0]
	previous := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() != "schema_min_supported" && (entry.Name() >= reservationMigration || !strings.HasSuffix(entry.Name(), ".sql")) {
			continue
		}
		raw, err := fs.ReadFile(migrations.Files, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		previous[entry.Name()] = &fstest.MapFile{Data: raw}
	}
	for _, required := range []string{assignmentMigration, "000215_organization_fuelings.sql"} {
		if _, ok := previous[required]; !ok {
			t.Fatal("previous image lacks", required)
		}
	}
	f := orgtest.NewWithMigrations(t, previous)
	_, _, onboarding, actor, flags := onboardingOnFixture(t, f)
	fx := legalFixtureFor(t, f, onboarding, actor, flags)
	if _, _, err := fx.onboarding.Repository.ActivateOrganization(t.Context(), fx.scope, fx.actor, fx.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	org := fx.scope.OrgID().String()
	s := assignmentStore(t, fx)
	var unit string
	if err := f.Admin.QueryRow(t.Context(), `SELECT id::text FROM org_units WHERE org_id=$1`, org).Scan(&unit); err != nil {
		t.Fatal(err)
	}
	active, activeRevision := lifecycleAssetIn(t, fx, s, unit, fleet.Active)
	inactive, _ := lifecycleAssetIn(t, fx, s, unit, fleet.Inactive)
	disposed, _ := lifecycleAssetIn(t, fx, s, unit, fleet.Disposed)
	employee, _ := assignmentPerson(t, fx)
	if _, _, err := assignmentWrite(t, fx, s, assignmentCreate(t, fa.AssignmentCreateInput{AssetID: active, Kind: fleet.PrimaryAssignment, EmployeeIDs: []string{employee}})); err != nil {
		t.Fatal(err)
	}
	snapshot := func() string {
		t.Helper()
		var value string
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array(
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_assets x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_employee_identities x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_asset_assignments x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,asset_id,revision) FROM org_asset_lifecycle_events x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,operation_id) FROM org_audit_events x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id) FROM organizations x),
 (SELECT count(*) FROM job_queue))::text`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	usage := func() string {
		t.Helper()
		var out string
		if err := f.Admin.QueryRow(t.Context(), `SELECT pg_get_functiondef('org_asset_current_usage(uuid,uuid)'::regprocedure)`).Scan(&out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	before, beforeUsage := snapshot(), usage()
	migrator := f.RuntimeRole + "_reservation_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	// The ordinary migrator only references the parents, may add a trigger to
	// the asset table and owns the one function it replaces.
	if _, err := f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE,CREATE ON SCHEMA public TO "+role+
		";GRANT SELECT,INSERT ON schema_migrations TO "+role+";GRANT REFERENCES ON organizations,org_audit_events,org_assets,org_employee_identities TO "+role+
		";GRANT TRIGGER ON org_assets TO "+role+";ALTER FUNCTION org_asset_current_usage(uuid,uuid) OWNER TO "+role); err != nil {
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
	raw, err := fs.ReadFile(migrations.Files, reservationMigration)
	if err != nil {
		t.Fatal("reservation migration missing", err)
	}
	clean := func(label string) {
		t.Helper()
		var ok bool
		if err := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('org_asset_reservations') IS NULL AND to_regclass('org_asset_blocks') IS NULL AND to_regclass('org_asset_reservation_commands') IS NULL
 AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)
 AND NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='org_asset_reservation_left_service')
 AND to_regprocedure('org_asset_reservation_deadline(timestamptz,timestamptz,integer)') IS NULL`, version).Scan(&ok); err != nil || !ok || before != snapshot() || beforeUsage != usage() {
			t.Fatal(label, "changed the previous schema, consumer, usage source or data", err)
		}
	}
	broken := fstest.MapFS{reservationMigration: &fstest.MapFile{Data: append(append([]byte{}, raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(t.Context(), location.String(), broken); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
		t.Fatal("injected reservation migration failure not reached", err)
	}
	clean("failed migration")
	current := fstest.MapFS{}
	for key, data := range previous {
		current[key] = data
	}
	current[reservationMigration] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 1 || applied[0] != version {
		t.Fatal("ordinary reservation migration recovery", err)
	}
	if before != snapshot() {
		t.Fatal("reservation migration rewrote existing assets, assignments, history or jobs")
	}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 0 {
		t.Fatal("reservation migration no-op replay", err)
	}
	if err := migrate.Check(t.Context(), f.Admin, previous); err != nil {
		t.Fatal("previous embedded image incompatible", err)
	}
	var exact bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT NOT EXISTS(SELECT 1 FROM org_asset_reservations) AND NOT EXISTS(SELECT 1 FROM org_asset_blocks) AND NOT EXISTS(SELECT 1 FROM org_asset_reservation_commands)
 AND(SELECT bool_and(relrowsecurity AND relforcerowsecurity AND relowner=(SELECT oid FROM pg_roles WHERE rolname=$1)) FROM pg_class WHERE oid IN('org_asset_reservations'::regclass,'org_asset_blocks'::regclass,'org_asset_reservation_commands'::regclass))
 AND NOT EXISTS(SELECT 1 FROM pg_constraint WHERE connamespace='public'::regnamespace AND NOT convalidated)
 AND(SELECT count(*) FROM pg_trigger WHERE tgname IN('org_asset_reservation_left_service','org_asset_reservation_write','org_asset_reservations_immutable','org_asset_block_write','org_asset_blocks_immutable','org_asset_reservation_commands_immutable'))=6
 AND EXISTS(SELECT 1 FROM pg_constraint WHERE conname='org_asset_reservations_confirmed_no_overlap' AND contype='x')
 AND position('org_asset_reservations_asset_fkey' IN pg_get_functiondef('org_asset_current_usage(uuid,uuid)'::regprocedure))>0
 AND position('org_asset_blocks_asset_fkey' IN pg_get_functiondef('org_asset_current_usage(uuid,uuid)'::regprocedure))>0
 AND position('org_fuelings_asset_fkey' IN pg_get_functiondef('org_asset_current_usage(uuid,uuid)'::regprocedure))>0
 AND position('org_asset_assignments_asset_fkey' IN pg_get_functiondef('org_asset_current_usage(uuid,uuid)'::regprocedure))>0`, migrator).Scan(&exact); err != nil || !exact {
		t.Fatal("reservation migration invented data, weakened RLS or missed a guard", err)
	}
	// The upgraded asset-use source still answers (no new usage counted).
	if err := allocationExec(t, fx, `SELECT org_asset_current_usage($1,$2)`, org, active); err != nil {
		t.Fatal("asset-use source after the upgrade", err)
	}
	// Upgraded behaviour with the ordinary runtime grants.
	f.GrantReservationReductions(t)
	s = reservationStore(t, fx)
	now := exactNow(t, fx)
	open, _, err := reserve(t, fx, s, fa.ReservationRequestInput{AssetID: active, Start: now.Add(time.Hour), End: now.Add(2 * time.Hour), EmployeeID: employee, Confirm: true})
	if err != nil {
		t.Fatal("reservation on the upgraded schema", err)
	}
	for label, asset := range map[string]string{"inactive": inactive, "disposed": disposed} {
		if _, _, err := reserve(t, fx, s, fa.ReservationRequestInput{AssetID: asset, Start: now.Add(time.Hour), End: now.Add(2 * time.Hour)}); !errors.Is(err, fleet.ErrInvalid) {
			t.Fatal("upgraded asset admission", label, err)
		}
	}
	// The lifecycle producer runs the left-service consumer with exactly the
	// documented reduction columns, not the Fleet reservation grants.
	if _, err := f.Admin.Exec(t.Context(), "REVOKE SELECT,INSERT,UPDATE ON org_asset_reservations FROM "+pgx.Identifier{f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	f.GrantReservationReductions(t)
	deactivated, _, err := lifecycleWrite(t, fx, s, lifecycleCommand(t, lifecycleInputFor(t, fx, fleet.Deactivate, active, activeRevision)))
	if err != nil {
		t.Fatal("upgraded deactivation", err)
	}
	if state, cause, source := storedReservation(t, fx, open.ReservationID); state != "CANCELLED" || cause != "ASSET_LEFT_SERVICE" || source == nil || *source != deactivated.Revision {
		t.Fatal("upgraded left-service consumer", state, cause)
	}
	if _, err := f.Admin.Exec(t.Context(), "GRANT DELETE ON org_asset_reservations TO "+pgx.Identifier{f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	allocationWant(t, "upgraded delete", allocationExec(t, fx, `DELETE FROM org_asset_reservations WHERE org_id=$1`, org), "55000")
}
