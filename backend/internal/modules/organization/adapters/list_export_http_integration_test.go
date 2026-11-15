//go:build integration

package adapters_test

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/adapters/lists"
	exportdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// listReaderGrant provisions the column privileges proposed for the export
// worker's list readers (EMP-RPT-002) on the disposable ordinary login.
func listReaderGrant(t *testing.T, fx deviceFixtureState) {
	t.Helper()
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	for _, grant := range []string{
		"GRANT SELECT(org_id,id,kind,category_id,name,fleet_number,plate,serial_number,usage_basis,state,revision,acquired_at,acquired_submicro,disposal_kind,disposed_at,disposed_submicro,disposal_reason) ON org_assets TO ",
		"GRANT SELECT(org_id,id,label) ON org_asset_categories TO ",
		"GRANT SELECT(org_id,asset_id,unit_id,valid_from,valid_to) ON org_asset_unit_periods TO ",
		"GRANT SELECT(org_id,asset_id,cost_centre_id,valid_from,valid_to) ON org_asset_cost_centre_periods TO ",
		"GRANT SELECT(org_id,id,asset_id,kind) ON org_meters TO ",
		"GRANT SELECT(org_id,id,meter_id,value,occurred_at,occurred_submicro,entry_zone,device_offset_seconds,received_at,origin,method,state,author_membership_id,capture_kind) ON org_meter_readings TO ",
		"GRANT SELECT(org_id,meter_id,reading_id,revision,state,recorded_at) ON org_meter_reading_states TO ",
		"GRANT SELECT(org_id,id,state,revision,merged_into) ON org_employee_identities TO ",
		"GRANT SELECT(org_id,employee_id,display_name,registration_number,job_title,home_unit_id,revision) ON org_employee_profiles TO ",
		"GRANT SELECT(org_id,employee_id,state) ON org_memberships TO ",
		"GRANT SELECT(org_id,display_name,time_zone) ON organizations TO ",
	} {
		if _, err := fx.f.Admin.Exec(t.Context(), grant+role); err != nil {
			t.Fatal(err)
		}
	}
}

type listDownload struct {
	read     map[string]any
	csv      [][]string
	rawCSV   []byte
	manifest exportdomain.ListManifest
	rawMan   []byte
}

// settle runs the production export build dispatcher until every named job
// is READY or terminal; retried attempts are due after the dispatcher's
// short retry delay.
func settle(t *testing.T, fx deviceFixtureState, dispatcher *jobs.Dispatcher, exports ...string) {
	t.Helper()
	for i := 0; i < 400; i++ {
		if _, err := dispatcher.RunOnce(t.Context()); err != nil {
			_ = err // a retried attempt is asserted on the job row
		}
		done := true
		for _, export := range exports {
			if state, _, _ := listJobState(t, fx, export); state == "QUEUED" || state == "RUNNING" {
				done = false
			}
		}
		if done {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("export jobs did not settle")
}

func listContent(t *testing.T, fx deviceFixtureState, router chi.Router, actor matrixPerson, base, export string) listDownload {
	t.Helper()
	read := matrixCall(t, fx, router, actor, base+"/"+export+"/read", map[string]any{}, 200, "", "", nil)
	if read["state"] != "READY" || read["media_type"] != exportdomain.ListMediaType {
		t.Fatal("list export not ready", read)
	}
	granted := matrixCall(t, fx, router, actor, base+"/"+export+"/access", map[string]any{}, 200, "", "", nil)
	content := matrixCall(t, fx, router, actor, base+"/"+export+"/content", map[string]any{"capability": granted["capability"]}, 200, "", "", nil)
	raw, err := base64.StdEncoding.DecodeString(fmt.Sprint(content["content_base64"]))
	if err != nil || content["sha256"] != read["sha256"] || content["media_type"] != exportdomain.ListMediaType {
		t.Fatal("content", err)
	}
	reader, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil || len(reader.File) != 2 || reader.File[1].Name != exportdomain.ListManifestName {
		t.Fatal("package", err)
	}
	out := listDownload{read: read}
	for _, entry := range reader.File {
		rc, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		if entry.Name == exportdomain.ListManifestName {
			out.rawMan = body
			if err := json.Unmarshal(body, &out.manifest); err != nil {
				t.Fatal(err)
			}
			continue
		}
		out.rawCSV = body
		if !bytes.HasPrefix(body, []byte(exportdomain.ByteOrderMark)) {
			t.Fatal("CSV without byte-order mark")
		}
		if out.csv, err = csv.NewReader(bytes.NewReader(body[len(exportdomain.ByteOrderMark):])).ReadAll(); err != nil {
			t.Fatal(err)
		}
	}
	if out.manifest.RowCount != int64(len(out.csv)-1) || read["row_count"] != float64(out.manifest.RowCount) || out.manifest.ExportID != export {
		t.Fatal("row count or identity", out.manifest.RowCount, read["row_count"])
	}
	return out
}

func listColumn(t *testing.T, d listDownload, name string) int {
	t.Helper()
	for i, header := range d.csv[0] {
		if header == name {
			return i
		}
	}
	t.Fatal("missing column", name)
	return -1
}

func listRow(t *testing.T, d listDownload, id string) []string {
	t.Helper()
	for _, row := range d.csv[1:] {
		if row[0] == id {
			return row
		}
	}
	t.Fatal("missing row", id)
	return nil
}

func inertCell(t *testing.T, cell, stored string) {
	t.Helper()
	if cell != exportdomain.InertText(stored) || exportdomain.RecoverText(cell) != stored || cell != "" && strings.ContainsRune("=+-@\t\r\n", rune(cell[0])) {
		t.Fatalf("cell %q is not the inert form of %q", cell, stored)
	}
}

func listJobState(t *testing.T, fx deviceFixtureState, export string) (string, string, bool) {
	t.Helper()
	var state, failure string
	var object bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT state,COALESCE(failure_code,''),object_sha256 IS NOT NULL FROM org_export_jobs WHERE id=$1`, export).Scan(&state, &failure, &object); err != nil {
		t.Fatal(err)
	}
	return state, failure, object
}

// B-BR-EMP-255..258, BUC-EMP-130/131 through the actual company HTTP entry,
// the actual export authority, the production list formats composed by the
// suite, the production queue handlers, real PostGIS and the disposable
// MinIO bucket.
func TestListExportsThroughActualCompanyHTTPAndWorker(t *testing.T) {
	fx := devicesFixture(t)
	router, dispatcher := auditorRouter(t, fx)
	listReaderGrant(t, fx)
	org := fx.scope.OrgID().String()
	base := "/v1/orgs/" + org
	exports := base + "/reports/exports"
	owner, manager, auditor, stale := matrixNewPerson(t, fx, router), matrixNewPerson(t, fx, router), matrixNewPerson(t, fx, router), matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, owner, "OWNER", "ORG", "ACTIVE")
	matrixAssign(t, fx, manager, "FLEET_MANAGER", "ORG", "ACTIVE")
	matrixAssign(t, fx, auditor, "AUDITOR", "ORG", "ACTIVE")
	matrixAssign(t, fx, stale, "FLEET_MANAGER", "ORG", "ACTIVE")
	other := legalFixtureFor(t, fx.f, fx.onboarding, fx.actor, fx.flags)
	if _, _, err := other.onboarding.Repository.ActivateOrganization(t.Context(), other.scope, other.actor, other.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	// A foreign asset in the other organization, written by the migration
	// owner with triggers suspended.
	foreignCategory, foreignAsset := onboardingID(t), onboardingID(t)
	ftx, err := fx.f.Admin.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`SELECT set_config('anpfuel.org_id',$1,true)`, []any{other.scope.OrgID().String()}},
		{`SET LOCAL session_replication_role = replica`, nil},
		{`INSERT INTO org_asset_categories(org_id,id,label) VALUES($1,$2,'FOREIGN listed category')`, []any{other.scope.OrgID().String(), foreignCategory}},
		{`INSERT INTO org_assets(org_id,id,kind,category_id,name,fleet_number,fleet_key,usage_basis,state,revision) VALUES($1,$2,'VEHICLE',$3,'FOREIGN listed asset','FRN-9','FRN9','DISTANCE','DRAFT',1)`, []any{other.scope.OrgID().String(), foreignAsset, foreignCategory}},
	} {
		if _, err := ftx.Exec(t.Context(), q.sql, q.args...); err != nil {
			_ = ftx.Rollback(t.Context())
			t.Fatal("foreign fixture", err)
		}
	}
	if err := ftx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}

	// Real source records created through the company entry, with text that
	// a spreadsheet would execute.
	vehicleName, serial := `=HYPERLINK("http://example.invalid","x")`, "@SUM(A1)"
	vehicle, _ := matrixCall(t, fx, router, owner, base+"/fleet/assets/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "kind": "VEHICLE", "name": vehicleName, "fleet_number": "+5511", "plate": "ABC1D23", "serial_number": serial, "usage_basis": "DISTANCE",
		"category": map[string]any{"new_label": "-Synthetic exported fleet"}}, 201, "", "", nil)["asset_id"].(string)
	machine, _ := matrixCall(t, fx, router, owner, base+"/fleet/assets/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "kind": "MACHINE", "name": "+Synthetic exported tractor", "fleet_number": "", "plate": "", "serial_number": "SER-2", "usage_basis": "HOURS",
		"category": map[string]any{"new_label": "Synthetic machines"}}, 201, "", "", nil)["asset_id"].(string)
	occurred := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)
	installed := matrixCall(t, fx, router, owner, base+"/fleet/assets/"+vehicle+"/meters/install", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1, "opening_reading_id": onboardingID(t), "kind": "ODOMETER",
		"label": "Synthetic exported odometer", "opening_value": "123456789", "method": "TYPED", "occurred_at": occurred.Format(time.RFC3339Nano), "entry_zone": "America/Sao_Paulo", "device_offset_seconds": -10800}, 201, "", "", nil)
	reading, _ := installed["opening_reading_id"].(string)
	if vehicle == "" || machine == "" || reading == "" {
		t.Fatal("source fixtures missing")
	}
	employee, _ := matrixCall(t, fx, router, owner, base+"/employees/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "display_name": "=CMD|' /C calc'!A0", "registration_number": "-0042",
		"work_email": "export.contact@example.invalid", "work_phone": "+5565999990001", "job_title": "@Lead", "home_unit_id": "", "allow_possible_duplicate": false}, 201, "", "", nil)["employee_id"].(string)
	if employee == "" {
		t.Fatal("employee fixture missing")
	}
	var session string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT COALESCE(web_session_id::text,enrollment_id::text) FROM org_meter_readings WHERE id=$1`, reading).Scan(&session); err != nil || session == "" {
		t.Fatal("capture locator", err)
	}
	request := func(actor matrixPerson, kind string, parameters map[string]any, want int, code, permission string) string {
		t.Helper()
		body := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "kind": kind, "parameters": parameters}
		out := matrixCall(t, fx, router, actor, exports+"/request", body, want, code, permission, nil)
		if want != 202 {
			return ""
		}
		job, _ := out["export"].(map[string]any)
		if job["state"] != "QUEUED" || job["kind"] != kind {
			t.Fatal("request", out)
		}
		return job["export_id"].(string)
	}

	// Refusals before anything is written.
	request(manager, "fleet.asset_list.xlsx", map[string]any{}, 422, "EXPORT_FORMAT_UNAVAILABLE", "")
	request(manager, "idn.employee_list.xlsx", map[string]any{}, 422, "EXPORT_FORMAT_UNAVAILABLE", "")
	request(manager, lists.AssetListKind, map[string]any{"state": "BOGUS"}, 422, "VALIDATION_FAILED", "")
	request(manager, lists.ReadingListKind, map[string]any{"occurred_from": "2026-02-01T00:00:00Z", "occurred_to": "2026-01-01T00:00:00Z"}, 422, "VALIDATION_FAILED", "")
	request(manager, lists.EmployeeListKind, map[string]any{"work_email": "x@example.invalid"}, 422, "VALIDATION_FAILED", "")

	assets := request(manager, lists.AssetListKind, map[string]any{}, 202, "", "")
	readings := request(manager, lists.ReadingListKind, map[string]any{"asset_id": vehicle}, 202, "", "")
	staff := request(auditor, lists.EmployeeListKind, map[string]any{}, 202, "", "")
	lost := request(stale, lists.AssetListKind, map[string]any{"kind": "VEHICLE"}, 202, "", "")
	// Stale requester scope: the role ends between request and worker.
	matrixAssign(t, fx, stale, "FLEET_MANAGER", "ORG", "NONE")
	settle(t, fx, dispatcher, assets, readings, staff, lost)
	if state, failure, object := listJobState(t, fx, lost); state != "FAILED" || failure != "EXPORT_RIGHTS_LOST" || object {
		t.Fatal("stale requester published", state, failure, object)
	}

	// Assets: exact, inert, scoped, deterministic order.
	a := listContent(t, fx, router, manager, exports, assets)
	if len(a.csv) != 3 || a.csv[1][0] > a.csv[2][0] || a.manifest.Kind != lists.AssetListKind || a.manifest.OrganizationID != org || a.manifest.ReviewBasis != exportdomain.NotApplicable {
		t.Fatal("asset list", a.csv, string(a.rawMan))
	}
	row := listRow(t, a, vehicle)
	inertCell(t, row[listColumn(t, a, "name")], vehicleName)
	inertCell(t, row[listColumn(t, a, "fleet_number")], "+5511")
	inertCell(t, row[listColumn(t, a, "serial_number")], serial)
	inertCell(t, row[listColumn(t, a, "category_label")], "-Synthetic exported fleet")
	var revision int64
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT revision FROM org_assets WHERE id=$1`, vehicle).Scan(&revision); err != nil || row[listColumn(t, a, "revision")] != fmt.Sprint(revision) || row[listColumn(t, a, "state")] != "DRAFT" {
		t.Fatal("asset revision or state", err, row)
	}
	inertCell(t, listRow(t, a, machine)[listColumn(t, a, "name")], "+Synthetic exported tractor")
	for _, forbidden := range []string{"FOREIGN", foreignAsset, other.scope.OrgID().String()} {
		if bytes.Contains(a.rawCSV, []byte(forbidden)) || bytes.Contains(a.rawMan, []byte(forbidden)) {
			t.Fatal("foreign row exported", forbidden)
		}
	}

	// Readings: exact base units and a negative offset left numeric; the
	// capture locator is omitted with its reason.
	r := listContent(t, fx, router, manager, exports, readings)
	if len(r.csv) != 2 || r.manifest.Filters["asset_id"] != vehicle || r.manifest.ReviewBasis != exportdomain.ApprovedOnly {
		t.Fatal("reading list", r.csv, string(r.rawMan))
	}
	got := listRow(t, r, reading)
	for column, want := range map[string]string{"asset_id": vehicle, "meter_kind": "ODOMETER", "value_m": "123456789", "value_s": "", "occurred_at": occurred.Format(time.RFC3339Nano), "entry_zone": "America/Sao_Paulo",
		"device_offset_s": "-10800", "origin": "METER_INSTALLATION", "method": "TYPED", "state": "VALID", "author_membership_id": owner.member} {
		if got[listColumn(t, r, column)] != want {
			t.Fatalf("reading %s = %q, want %q", column, got[listColumn(t, r, column)], want)
		}
	}
	if bytes.Contains(r.rawCSV, []byte(session)) || bytes.Contains(r.rawMan, []byte(session)) || len(r.manifest.OmittedColumns) != 1 || r.manifest.OmittedColumns[0].Reason != "CAPTURE_LOCATOR" {
		t.Fatal("capture locator exported or not reported")
	}

	// Employees: the AUDITOR exports the register; work contact is omitted
	// and reported, never written.
	e := listContent(t, fx, router, auditor, exports, staff)
	person := listRow(t, e, employee)
	inertCell(t, person[listColumn(t, e, "display_name")], "=CMD|' /C calc'!A0")
	inertCell(t, person[listColumn(t, e, "registration_number")], "-0042")
	inertCell(t, person[listColumn(t, e, "job_title")], "@Lead")
	for _, contact := range []string{"export.contact@example.invalid", "+5565999990001", "5565999990001"} {
		if bytes.Contains(e.rawCSV, []byte(contact)) || bytes.Contains(e.rawMan, []byte(contact)) {
			t.Fatal("work contact exported", contact)
		}
	}
	reasons := map[string]string{}
	for _, o := range e.manifest.OmittedColumns {
		reasons[o.Column] = o.Reason
	}
	if reasons["work_email"] != "PURPOSE_LIMITED" || reasons["work_phone"] != "PURPOSE_LIMITED" || len(reasons) != 2 {
		t.Fatal("omissions not reported", e.manifest.OmittedColumns)
	}
	// The AUDITOR's request and the other members' jobs stay private.
	matrixCall(t, fx, router, manager, exports+"/"+staff+"/read", map[string]any{}, 404, "NOT_FOUND", "", nil)

	// A read failure on every attempt never becomes READY.
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	// Revoking the table privilege also revokes its column privileges.
	if _, err := fx.f.Admin.Exec(t.Context(), "REVOKE SELECT ON org_meter_readings FROM "+role); err != nil {
		t.Fatal(err)
	}
	broken := request(manager, lists.ReadingListKind, map[string]any{}, 202, "", "")
	settle(t, fx, dispatcher, broken)
	if state, failure, object := listJobState(t, fx, broken); state != "FAILED" || failure != "EXPORT_ATTEMPTS_EXHAUSTED" || object {
		t.Fatal("an incomplete export settled otherwise", state, failure, object)
	}
	matrixCall(t, fx, router, manager, exports+"/"+broken+"/access", map[string]any{}, 409, "EXPORT_NOT_READY", "", nil)
}
