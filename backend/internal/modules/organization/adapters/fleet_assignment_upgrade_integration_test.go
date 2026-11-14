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

// The one file-name constant of EMP-FLEET-006; the coordinator may renumber it.
const assignmentMigration = "000210_organization_asset_assignments.sql"

// Populated previous head → 210: a failed attempt changes nothing; an ordinary
// NOSUPERUSER/NOBYPASSRLS migrator recovers; nothing is invented for existing
// assets or employees; the previous image stays compatible and the installed
// guards and consumers work on the upgraded schema.
func TestFleetAssignmentPopulatedUpgradeIsAdditiveAndRecoverable(t *testing.T) {
	version := strings.SplitN(assignmentMigration, "_", 2)[0]
	previous := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() != "schema_min_supported" && (entry.Name() >= assignmentMigration || !strings.HasSuffix(entry.Name(), ".sql")) {
			continue
		}
		raw, err := fs.ReadFile(migrations.Files, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		previous[entry.Name()] = &fstest.MapFile{Data: raw}
	}
	if _, ok := previous["000208_role_structural_scopes.sql"]; !ok {
		t.Fatal("previous image lacks the structural-scope schema")
	}
	f := orgtest.NewWithMigrations(t, previous)
	_, _, onboarding, actor, flags := onboardingOnFixture(t, f)
	fx := legalFixtureFor(t, f, onboarding, actor, flags)
	if _, _, err := fx.onboarding.Repository.ActivateOrganization(t.Context(), fx.scope, fx.actor, fx.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	org := fx.scope.OrgID().String()
	s := lifecycleStore(t, fx)
	var unit string
	if err := f.Admin.QueryRow(t.Context(), `SELECT id::text FROM org_units WHERE org_id=$1`, org).Scan(&unit); err != nil {
		t.Fatal(err)
	}
	active, activeRevision := lifecycleAssetIn(t, fx, s, unit, fleet.Active)
	disposed, _ := lifecycleAssetIn(t, fx, s, unit, fleet.Disposed)
	draft, _ := lifecycleAssetIn(t, fx, s, unit, fleet.Draft)
	employee, member := assignmentPerson(t, fx)
	terminated := assignmentEmployeeIn(t, fx, "TERMINATED")
	snapshot := func() string {
		t.Helper()
		var value string
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array(
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_assets x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_employee_identities x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_memberships x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,operation_id) FROM org_asset_commands x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,asset_id,revision) FROM org_asset_lifecycle_events x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,asset_id,valid_from) FROM org_asset_unit_periods x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,operation_id) FROM org_audit_events x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id) FROM organizations x))::text`).Scan(&value); err != nil {
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
	migrator := f.RuntimeRole + "_assignment_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	// The ordinary migrator only references the parents, may add triggers to
	// the two source tables and owns the one function it replaces.
	if _, err := f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE,CREATE ON SCHEMA public TO "+role+
		";GRANT SELECT,INSERT ON schema_migrations TO "+role+";GRANT REFERENCES ON organizations,org_audit_events,org_assets,org_employee_identities TO "+role+
		";GRANT TRIGGER ON org_assets,org_employee_identities TO "+role+";ALTER FUNCTION org_asset_current_usage(uuid,uuid) OWNER TO "+role); err != nil {
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
	raw, err := fs.ReadFile(migrations.Files, assignmentMigration)
	if err != nil {
		t.Fatal("assignment migration missing", err)
	}
	clean := func(label string) {
		t.Helper()
		var ok bool
		if err := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('org_asset_assignments') IS NULL AND to_regclass('org_asset_assignment_commands') IS NULL
 AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)
 AND NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgname IN('org_asset_assignment_end_employee','org_asset_assignment_end_disposal'))
 AND to_regprocedure('org_asset_assignment_ended(timestamptz,timestamptz,timestamptz)') IS NULL`, version).Scan(&ok); err != nil || !ok || before != snapshot() || beforeUsage != usage() {
			t.Fatal(label, "changed the previous schema, consumers, usage source or data", err)
		}
	}
	broken := fstest.MapFS{assignmentMigration: &fstest.MapFile{Data: append(append([]byte{}, raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(t.Context(), location.String(), broken); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
		t.Fatal("injected assignment migration failure not reached", err)
	}
	clean("failed migration")
	// Previous image still disposes and terminates normally (no consumer).
	current := fstest.MapFS{}
	for key, data := range previous {
		current[key] = data
	}
	current[assignmentMigration] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 1 || applied[0] != version {
		t.Fatal("ordinary assignment migration recovery", err)
	}
	if before != snapshot() {
		t.Fatal("assignment migration rewrote existing assets, employees, memberships or history")
	}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 0 {
		t.Fatal("assignment migration no-op replay", err)
	}
	if err := migrate.Check(t.Context(), f.Admin, previous); err != nil {
		t.Fatal("previous embedded image incompatible", err)
	}
	var exact bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT NOT EXISTS(SELECT 1 FROM org_asset_assignments) AND NOT EXISTS(SELECT 1 FROM org_asset_assignment_commands)
 AND(SELECT bool_and(relrowsecurity AND relforcerowsecurity AND relowner=(SELECT oid FROM pg_roles WHERE rolname=$1)) FROM pg_class WHERE oid IN('org_asset_assignments'::regclass,'org_asset_assignment_commands'::regclass))
 AND NOT EXISTS(SELECT 1 FROM pg_constraint WHERE connamespace='public'::regnamespace AND NOT convalidated)
 AND(SELECT count(*) FROM pg_trigger WHERE tgname IN('org_asset_assignment_end_employee','org_asset_assignment_end_disposal','org_asset_assignment_write','org_asset_assignments_immutable','org_asset_assignment_replaced','org_asset_assignment_commands_immutable'))=6
 AND EXISTS(SELECT 1 FROM pg_constraint WHERE conname='org_asset_assignments_primary_no_overlap' AND contype='x')
 AND position('org_asset_assignments_asset_fkey' IN pg_get_functiondef('org_asset_current_usage(uuid,uuid)'::regprocedure))>0
 AND position('org_document_links_asset_fkey' IN pg_get_functiondef('org_asset_current_usage(uuid,uuid)'::regprocedure))>0`, migrator).Scan(&exact); err != nil || !exact {
		t.Fatal("assignment migration invented data, weakened RLS or missed a guard", err)
	}
	// Upgraded behaviour with the ordinary runtime grants.
	f.GrantAssignmentReductions(t)
	s = assignmentStore(t, fx)
	created, _, err := assignmentWrite(t, fx, s, assignmentCreate(t, fa.AssignmentCreateInput{AssetID: active, Kind: fleet.PrimaryAssignment, EmployeeIDs: []string{employee}}))
	if err != nil {
		t.Fatal("assignment on the upgraded schema", err)
	}
	if !assignedAt(t, fx, s, member, active, lifecycleDBNow(t, fx)) {
		t.Fatal("upgraded port")
	}
	for label, asset := range map[string]string{"disposed": disposed, "draft": draft} {
		if _, _, err := assignmentWrite(t, fx, s, assignmentCreate(t, fa.AssignmentCreateInput{AssetID: asset, Kind: fleet.AdditionalAssignment, EmployeeIDs: []string{employee}})); !errors.Is(err, fleet.ErrInvalid) {
			t.Fatal("upgraded asset admission", label, err)
		}
	}
	if _, _, err := assignmentWrite(t, fx, s, assignmentCreate(t, fa.AssignmentCreateInput{AssetID: active, Kind: fleet.AdditionalAssignment, EmployeeIDs: []string{terminated}})); !errors.Is(err, fleet.ErrInvalid) {
		t.Fatal("upgraded employee admission", err)
	}
	dispose := lifecycleInputFor(t, fx, fleet.Dispose, active, activeRevision)
	dispose.At = lifecycleDBNow(t, fx)
	if _, _, err := lifecycleWrite(t, fx, s, lifecycleCommand(t, dispose)); err != nil {
		t.Fatal("upgraded disposal", err)
	}
	if row := assignmentRowsOf(t, fx, active)[created.AssignmentIDs[0]]; row.cause != "ASSET_DISPOSED" || row.to == nil || !row.to.Equal(dispose.At.UTC().Truncate(time.Microsecond)) {
		t.Fatal("upgraded disposal consumer", row)
	}
	if _, err := f.Admin.Exec(t.Context(), "GRANT DELETE ON org_asset_assignments TO "+pgx.Identifier{f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	allocationWant(t, "upgraded delete", allocationExec(t, fx, `DELETE FROM org_asset_assignments WHERE org_id=$1`, org), "55000")
	if err := pgx.BeginTxFunc(t.Context(), f.Runtime, pgx.TxOptions{}, func(tx pgx.Tx) error {
		var n int
		if err := tx.QueryRow(t.Context(), `SELECT count(*) FROM org_asset_assignments`).Scan(&n); err != nil || n != 0 {
			t.Fatal("assignments visible without organization setting", n, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
