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

// The migration under test; the previous image is every file before it.
const incidentMigration = "000230_organization_incident_reports.sql"

// A populated head before 000230 (a completed and an in-progress trip with
// their receipts, a document with an ASSET link) upgrades through an ordinary
// non-superuser migrator: an injected failure changes no schema, function or
// data, recovery applies once, replay is a no-op, the previous embedded image
// stays compatible, the new tables are owned by the ordinary forced-RLS
// migrator, every usage-source line is kept with the new report keys, the old
// link survives the widened binding and the pre-existing trip then takes a
// report that appears on its timeline (EMP-CHECK-003, EDR-238..240).
func TestIncidentReportPopulatedUpgradeFailureRecoveryAndPreviousImage(t *testing.T) {
	previous := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() != "schema_min_supported" && (entry.Name() >= incidentMigration || !strings.HasSuffix(entry.Name(), ".sql")) {
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
	x := tripStateOn(t, f)
	d := x.syncFixtureState
	completedAt := x.declaredNow(t)
	first := x.startOp(t, d, completedAt, nil)
	ack, err := x.process(t, d, first)
	x.accepted(t, first, ack, err, 1, 1)
	end := x.finishOp(t, d, tripOf(first), completedAt.Add(time.Microsecond), nil)
	ack, err = x.process(t, d, end)
	x.accepted(t, end, ack, err, 2, 1)
	ongoingAt := x.declaredNow(t)
	ongoing := x.startOp(t, d, ongoingAt, nil)
	ack, err = x.process(t, d, ongoing)
	x.accepted(t, ongoing, ack, err, 1, 1)
	org := x.scope.OrgID().String()
	document, link := onboardingID(t), onboardingID(t)
	if err := pgx.BeginFunc(t.Context(), f.Admin, func(tx pgx.Tx) error {
		if _, err := tx.Exec(t.Context(), `SET LOCAL session_replication_role=replica`); err != nil {
			return err
		}
		if _, err := tx.Exec(t.Context(), `INSERT INTO org_documents(org_id,id,kind,data_mode,operation_id,created_by,original_sha256,original_bytes,original_type,storage_state,storage_revision,extraction_state,extraction_revision,
 structure_state,structure_revision,conference_state,conference_revision,reconciliation_state,fiscal_state,fiscal_revision)
VALUES($1,$2,'RECEIPT','SYNTHETIC',$3,$4,repeat('ab',32),1024,'image/jpeg','UPLOADING',1,'NONE',1,'NOT_APPLICABLE',1,'UNCONFERRED',1,'UNALLOCATED','NOT_CONSULTED',1)`, org, document, onboardingID(t), x.member); err != nil {
			return err
		}
		_, err := tx.Exec(t.Context(), `INSERT INTO org_document_links(org_id,document_id,target_kind,target_id,asset_id,operation_id,created_by) VALUES($1,$2,'ASSET',$3,$3,$4,$5)`, org, document, x.vehicle.AssetID, link, x.member)
		return err
	}); err != nil {
		t.Fatal("populated document link", err)
	}
	snapshot := func() string {
		t.Helper()
		var value string
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_trips x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,trip_id)FROM org_trip_finishes x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_sync_operations x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_sync_acknowledgements x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_sync_effects x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_audit_events x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_memberships x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_assets x),
(SELECT jsonb_agg(to_jsonb(x)-'incident_report_id' ORDER BY org_id,document_id)FROM org_document_links x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_documents x))::text`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	functions := func() string {
		t.Helper()
		var value string
		if err := f.Admin.QueryRow(t.Context(), `SELECT pg_get_functiondef('org_asset_current_usage(uuid,uuid)'::regprocedure)||pg_get_functiondef('org_unit_current_usage(uuid,uuid)'::regprocedure)
 ||pg_get_functiondef('org_cost_centre_current_usage(uuid,uuid)'::regprocedure)||pg_get_constraintdef((SELECT oid FROM pg_constraint WHERE conname='org_document_links_binding'))`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	before, beforeFunctions := snapshot(), functions()
	migrator := f.RuntimeRole + "_incident_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	// The production migration owner owns the link table and the three usage
	// sources it redefines and holds REFERENCES on the parents.
	if _, err := f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE,CREATE ON SCHEMA public TO "+role+";GRANT SELECT,INSERT ON schema_migrations TO "+role+
		";GRANT REFERENCES ON organizations,org_sync_operations,org_assets,org_units,org_cost_centres,org_trips,org_memberships,org_employee_identities,org_device_enrollments,org_audit_events TO "+role+
		";ALTER TABLE org_document_links OWNER TO "+role+";ALTER FUNCTION org_asset_current_usage(uuid,uuid) OWNER TO "+role+";ALTER FUNCTION org_unit_current_usage(uuid,uuid) OWNER TO "+role+
		";ALTER FUNCTION org_cost_centre_current_usage(uuid,uuid) OWNER TO "+role); err != nil {
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
	raw, err := fs.ReadFile(migrations.Files, incidentMigration)
	if err != nil {
		t.Fatal(err)
	}
	version := strings.SplitN(incidentMigration, "_", 2)[0]
	broken := fstest.MapFS{incidentMigration: &fstest.MapFile{Data: append(append([]byte{}, raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(t.Context(), location.String(), broken); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
		t.Fatal("injected incident migration failure not reached", err)
	}
	var clean bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('org_incident_reports') IS NULL AND to_regclass('org_incident_report_transitions') IS NULL
 AND to_regprocedure('org_incident_report_source()') IS NULL AND NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='org_document_links' AND column_name='incident_report_id')
 AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, version).Scan(&clean); err != nil || !clean || before != snapshot() || beforeFunctions != functions() {
		t.Fatal("failed incident migration changed the schema, a function or data", err)
	}
	current := fstest.MapFS{}
	for path, data := range previous {
		current[path] = data
	}
	current[incidentMigration] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 1 || applied[0] != version {
		t.Fatal("ordinary incident migration recovery", applied, err)
	}
	if before != snapshot() {
		t.Fatal("incident upgrade rewrote existing state")
	}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 0 {
		t.Fatal("incident migration no-op replay", err)
	}
	if err := migrate.Check(t.Context(), f.Admin, previous); err != nil {
		t.Fatal("previous embedded image incompatible", err)
	}
	if err := f.Admin.QueryRow(t.Context(), `SELECT count(*)=2 AND bool_and(relrowsecurity AND relforcerowsecurity AND relowner=(SELECT oid FROM pg_roles WHERE rolname=$1 AND NOT rolsuper AND NOT rolbypassrls))
 FROM pg_class WHERE oid IN('org_incident_reports'::regclass,'org_incident_report_transitions'::regclass)`, migrator).Scan(&clean); err != nil || !clean {
		t.Fatal("report tables not owned by the ordinary forced-RLS migrator", err)
	}
	// Every previous allow-list line is kept, with the report keys added.
	var kept bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT a LIKE '%org_trips_asset_fkey%' AND a LIKE '%org_trip_held_starts_asset_fkey%' AND a LIKE '%org_asset_implement_attachments_powered_fkey%'
 AND a LIKE '%org_checklist_template_targets_asset_fkey%' AND a LIKE '%org_document_links_asset_fkey%' AND a LIKE '%org_fuelings_asset_fkey%' AND a LIKE '%org_incident_reports_asset_fkey%'
 AND a LIKE '%org_trip_missions_asset_fkey%' AND u LIKE '%org_trip_missions_unit_fkey%'
 AND u LIKE '%org_trips_unit_fkey%' AND u LIKE '%org_fuelings_unit_fkey%' AND u LIKE '%org_incident_reports_unit_fkey%'
 AND c LIKE '%org_trips_cost_centre_fkey%' AND c LIKE '%org_fuelings_cost_centre_fkey%' AND c LIKE '%org_incident_reports_cost_centre_fkey%'
 AND b LIKE '%INCIDENT%incident_report_id%'
 FROM(SELECT pg_get_functiondef('org_asset_current_usage(uuid,uuid)'::regprocedure) a,pg_get_functiondef('org_unit_current_usage(uuid,uuid)'::regprocedure) u,
 pg_get_functiondef('org_cost_centre_current_usage(uuid,uuid)'::regprocedure) c,pg_get_constraintdef((SELECT oid FROM pg_constraint WHERE conname='org_document_links_binding')) b) s`).Scan(&kept); err != nil || !kept {
		t.Fatal("a usage source lost a line or misses the report key", err)
	}
	if _, err := migrate.Apply(t.Context(), f.AdminDSN, migrations.Files); err != nil {
		t.Fatal("remaining embedded chain after the 230 rehearsal", err)
	}
	// EMP-TRIP-011: the upgraded logbook and finish guards read the 000233
	// lifecycle rows, granted once the chain reaches them.
	tripGrants(t, x.legalFixtureState)
	// The pre-existing in-progress trip takes a report, read on its timeline;
	// the usage sources still answer with the new key and the old link.
	h := incidentFixtureOn(t, eventFixtureOn(t, x))
	report := h.reportOp(t, d, damagePayload(x.vehicle.AssetID, tripOf(ongoing)), ongoingAt.Add(time.Minute), nil)
	ack, err = h.process(t, d, report)
	h.reportAccepted(t, report, ack, err)
	timeline := h.timeline(t, tripOf(ongoing))
	if len(timeline) != 2 || timeline[1].FactType != "DAMAGE" || timeline[1].Item.ID != reportOf(report) {
		t.Fatal("upgraded trip timeline", len(timeline))
	}
	var usage bool
	if err := pgx.BeginFunc(t.Context(), f.Runtime, func(tx pgx.Tx) error {
		if _, err := tx.Exec(t.Context(), `SELECT set_config('anpfuel.org_id',$1,true)`, org); err != nil {
			return err
		}
		return tx.QueryRow(t.Context(), `SELECT org_asset_current_usage($1,$2) IS NOT NULL AND org_unit_current_usage($1,$3) IS NOT NULL AND(SELECT count(*) FROM org_document_links WHERE target_kind='ASSET')=1`,
			org, x.vehicle.AssetID, x.unit).Scan(&usage)
	}); err != nil || !usage {
		t.Fatal("upgraded usage sources or the old link", err)
	}
}
