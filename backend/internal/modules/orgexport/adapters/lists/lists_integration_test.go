//go:build integration

package lists_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/adapters/lists"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

// grantListReaders provisions only the disposable ordinary runtime login with
// the column privileges proposed for the export worker (EMP-RPT-002). Work
// contact columns and capture locators are deliberately not granted.
func grantListReaders(t *testing.T, f *orgtest.Fixture) {
	t.Helper()
	role := pgx.Identifier{f.RuntimeRole}.Sanitize()
	for _, grant := range []string{
		"GRANT SELECT(org_id,id,kind,category_id,name,fleet_number,plate,serial_number,usage_basis,state,revision,acquired_at,acquired_submicro,disposal_kind,disposed_at,disposed_submicro,disposal_reason) ON org_assets TO ",
		"GRANT SELECT(org_id,id,label) ON org_asset_categories TO ",
		"GRANT SELECT(org_id,asset_id,unit_id,valid_from,valid_to) ON org_asset_unit_periods TO ",
		"GRANT SELECT(org_id,asset_id,cost_centre_id,valid_from,valid_to) ON org_asset_cost_centre_periods TO ",
		"GRANT SELECT(org_id,id,asset_id,kind) ON org_meters TO ",
		"GRANT SELECT(org_id,id,meter_id,value,occurred_at,occurred_submicro,entry_zone,device_offset_seconds,received_at,origin,method,state,author_membership_id,capture_kind) ON org_meter_readings TO ",
		"GRANT SELECT(org_id,id,state,revision,merged_into) ON org_employee_identities TO ",
		"GRANT SELECT(org_id,employee_id,display_name,registration_number,job_title,home_unit_id,revision) ON org_employee_profiles TO ",
		"GRANT SELECT(org_id,id,employee_id,account_id,state) ON org_memberships TO ",
		"GRANT SELECT(org_id,display_name,time_zone) ON organizations TO ",
	} {
		if _, err := f.Admin.Exec(t.Context(), grant+role); err != nil {
			t.Fatal(err)
		}
	}
}

// requesterCheck stands in for the export authority's session-free re-check
// on the real database: the recorded membership must be ACTIVE for the
// account, and a loss can be forced. The real authority is exercised through
// the company HTTP entry in organization/adapters.
type requesterCheck struct {
	lost   atomic.Bool
	checks atomic.Int32
	kinds  atomic.Value
}

func (c *requesterCheck) CheckRequesterTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, r app.Requester, kind string, read permissions.Permission) error {
	c.checks.Add(1)
	c.kinds.Store(kind + "|" + string(read))
	if database.ValidateOrgTx(ctx, tx, scope) != nil || !app.ExportableRead(read) {
		return app.ErrUnavailable
	}
	if c.lost.Load() {
		return app.ErrRightsLost
	}
	var ok bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM org_memberships WHERE org_id=$1 AND id=$2 AND account_id=$3 AND state='ACTIVE')`, scope.OrgID().String(), r.MembershipID, r.AccountID).Scan(&ok); err != nil {
		return app.ErrUnavailable
	}
	if !ok {
		return app.ErrRightsLost
	}
	return nil
}

func adminExec(t *testing.T, f *orgtest.Fixture, scope kernel.OrgScope, statements ...[]any) {
	t.Helper()
	ctx := t.Context()
	tx, err := f.Admin.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	// Synthetic rows are written by the migration owner with triggers (and
	// so parent references) suspended; CHECK constraints still apply.
	if _, err := tx.Exec(ctx, `SELECT set_config('anpfuel.org_id',$1,true)`, scope.OrgID().String()); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `SET LOCAL session_replication_role = replica`); err != nil {
		t.Fatal(err)
	}
	for _, s := range statements {
		if _, err := tx.Exec(ctx, s[0].(string), s[1:]...); err != nil {
			t.Fatal("synthetic list fixture", s[0], err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

// ids are synthetic and ordered: p is the organization nibble, n the row.
func sid(p string, n int) string {
	return "7b0e2a54-" + p + "000-4000-8000-" + strings.Repeat("0", 10) + string(rune('0'+n/10)) + string(rune('0'+n%10))
}

func micro(at time.Time) (time.Time, int16) {
	return at.Truncate(time.Microsecond), int16(at.Nanosecond() % 1000)
}

type world struct {
	f          *orgtest.Fixture
	a, b       kernel.OrgScope
	requester  app.Requester
	check      *requesterCheck
	formats    map[string]app.Format
	assetRows  [][]string // default filter, by id
	readRows   [][]string // received at or before readingsAsOf, by occurrence then id
	employees  [][]string
	foreign    []string // values and identifiers of organization B
	contacts   []string
	acquiredNs string
}

var (
	readingsAsOf = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	lateReceipt  = time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
)

func seed(t *testing.T) *world {
	t.Helper()
	f := orgtest.New(t)
	grantListReaders(t, f)
	w := &world{f: f, a: f.Organizations[0].Scope(t), b: f.Organizations[1].Scope(t), check: &requesterCheck{}}
	w.formats = map[string]app.Format{}
	for _, format := range lists.New(f.Runtime, w.check) {
		w.formats[format.Kind()] = format
	}
	orgA, orgB := w.a.OrgID().String(), w.b.OrgID().String()
	adminExec(t, f, w.a, []any{`UPDATE organizations SET has_snapshot=true,display_name=$2,time_zone='America/Cuiaba',kind_tags=ARRAY['FARM'],state='ACTIVE',suspension_reason='' WHERE org_id=$1`, orgA, "Synthetic list organization"})
	// Requester of organization A.
	account, requesterEmployee, requesterMember := sid("c", 1), sid("a", 90), sid("a", 91)
	if _, err := f.Admin.Exec(t.Context(), `INSERT INTO accounts(id,alias,status) VALUES($1,'synlistrequester','active'),($2,'synlistmanaged','active')`, account, sid("c", 2)); err != nil {
		t.Fatal(err)
	}
	w.requester = app.Requester{AccountID: account, MembershipID: requesterMember}

	acquired := time.Date(2025, 3, 4, 5, 6, 7, 123456789, time.UTC)
	acquiredAt, acquiredSub := micro(acquired)
	disposed := time.Date(2026, 2, 3, 4, 5, 6, 1, time.UTC)
	disposedAt, disposedSub := micro(disposed)
	w.acquiredNs = acquired.Format(time.RFC3339Nano)
	unitOld, unitNow, centre := sid("a", 60), sid("a", 61), sid("a", 62)
	periodStart, periodOld := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	categoryA := sid("a", 50)
	assetInsert := `INSERT INTO org_assets(org_id,id,kind,category_id,name,fleet_number,plate,serial_number,fleet_key,plate_key,usage_basis,state,revision,acquired_at,acquired_submicro,disposal_kind,disposed_at,disposed_submicro,disposal_reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)`
	readingInsert := `INSERT INTO org_meter_readings(org_id,id,meter_id,segment_id,value,occurred_at,occurred_submicro,received_at,entry_zone,device_offset_seconds,author_membership_id,capture_kind,enrollment_id,web_session_id,operation_id,request_hash,origin,method,state) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)`
	hash := strings.Repeat("ab", 32)
	r1At, r1Sub := micro(time.Date(2026, 1, 1, 10, 0, 0, 1999, time.UTC))
	r2At, r2Sub := micro(time.Date(2026, 1, 2, 8, 0, 0, 0, time.UTC))
	r3At, r3Sub := micro(time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC))
	session, enrollment := sid("a", 80), sid("a", 81)
	adminExec(t, f, w.a,
		[]any{`INSERT INTO org_employee_identities(org_id,id,state) VALUES($1,$2,'ACTIVE')`, orgA, requesterEmployee},
		[]any{`INSERT INTO org_memberships(org_id,id,employee_id,account_id,access_model,state,started_at) VALUES($1,$2,$3,$4,'EXISTING_ACCOUNT','ACTIVE',clock_timestamp())`, orgA, requesterMember, requesterEmployee, account},
		[]any{`INSERT INTO org_asset_categories(org_id,id,label) VALUES($1,$2,'=Synthetic fleet, "quoted"')`, orgA, categoryA},
		[]any{assetInsert, orgA, sid("a", 1), "VEHICLE", categoryA, `=HYPERLINK("http://example.invalid","x")`, "+5511", "ABC1D23", "@SUM(A1)", "5511", "ABC1D23", "DISTANCE", "ACTIVE", 3, acquiredAt, acquiredSub, nil, nil, 0, nil},
		[]any{assetInsert, orgA, sid("a", 2), "MACHINE", categoryA, "-Synthetic tractor", "F-02", "", "", "F02", "", "HOURS", "DRAFT", 1, nil, 0, nil, nil, 0, nil},
		[]any{assetInsert, orgA, sid("a", 3), "IMPLEMENT", categoryA, "Synthetic archived implement", "ARC-1", "", "", "ARC1", "", "HOURS", "ARCHIVED", 5, nil, 0, "SALE", disposedAt, disposedSub, `'Sold, "as is"`},
		[]any{assetInsert, orgA, sid("a", 4), "VEHICLE", categoryA, "Synthetic inactive vehicle", "", "XYZ9K87", "", "", "XYZ9K87", "DISTANCE", "INACTIVE", 7, nil, 0, nil, nil, 0, nil},
		[]any{`INSERT INTO org_asset_unit_periods(org_id,asset_id,unit_id,valid_from,valid_to) VALUES($1,$2,$3,$4,$5),($1,$2,$6,$5,NULL)`, orgA, sid("a", 1), unitOld, periodOld, periodStart, unitNow},
		[]any{`INSERT INTO org_asset_cost_centre_periods(org_id,asset_id,cost_centre_id,valid_from,valid_to) VALUES($1,$2,$3,$4,NULL)`, orgA, sid("a", 1), centre, periodStart},
		[]any{`INSERT INTO org_meters(org_id,id,asset_id,kind,label,installed_at,created_operation_id) VALUES($1,$2,$3,'ODOMETER','Synthetic odometer',$4,$5),($1,$6,$7,'HOUR_METER','Synthetic hour meter',$4,$8)`, orgA, sid("a", 20), sid("a", 1), periodOld, sid("a", 70), sid("a", 21), sid("a", 2), sid("a", 71)},
		[]any{readingInsert, orgA, sid("a", 31), sid("a", 20), sid("a", 40), int64(123456789), r1At, r1Sub, time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC), "America/Sao_Paulo", int64(-10800), requesterMember, "WEB_SESSION", nil, session, sid("a", 72), hash, "STANDALONE", "TYPED", "VALID"},
		[]any{readingInsert, orgA, sid("a", 30), sid("a", 20), sid("a", 40), int64(123456789), r1At, r1Sub, time.Date(2026, 1, 5, 0, 0, 1, 0, time.UTC), "America/Sao_Paulo", int64(-10800), requesterMember, "WEB_SESSION", nil, session, sid("a", 73), hash, "IMPORT", "IMPORTED", "VALID"},
		[]any{readingInsert, orgA, sid("a", 32), sid("a", 20), sid("a", 40), int64(999999999999), r2At, r2Sub, time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC), "America/Cuiaba", int64(-14400), requesterMember, "ENROLLED_DEVICE", enrollment, nil, sid("a", 74), hash, "FUELING", "TYPED_WITH_PHOTO", "SUSPECT"},
		[]any{readingInsert, orgA, sid("a", 33), sid("a", 21), sid("a", 41), int64(3599999999), r3At, r3Sub, time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC), "UTC", int64(0), requesterMember, "WEB_SESSION", nil, session, sid("a", 75), hash, "METER_INSTALLATION", "TYPED", "VALID"},
		[]any{readingInsert, orgA, sid("a", 34), sid("a", 20), sid("a", 40), int64(5), r3At, r3Sub, lateReceipt, "UTC", int64(0), requesterMember, "WEB_SESSION", nil, session, sid("a", 76), hash, "STANDALONE", "TYPED", "VALID"},
		[]any{`INSERT INTO org_employee_identities(org_id,id,state,revision) VALUES($1,$2,'ACTIVE',2),($1,$3,'INACTIVE',4)`, orgA, sid("a", 10), sid("a", 12)},
		[]any{`INSERT INTO org_employee_identities(org_id,id,state,revision,merged_into) VALUES($1,$2,'TERMINATED',3,$3)`, orgA, sid("a", 11), sid("a", 10)},
		[]any{`INSERT INTO org_employee_profiles(org_id,employee_id,display_name,registration_number,work_email,work_phone,job_title,home_unit_id,revision) VALUES($1,$2,$3,'-0042','contact.a@example.invalid','+5565999990001','@Lead',$4,6),($1,$5,'Synthetic inactive person','','','','',NULL,1)`,
			orgA, sid("a", 10), "=CMD|' /C calc'!A0", unitNow, sid("a", 12)},
		[]any{`INSERT INTO org_memberships(org_id,id,employee_id,account_id,access_model,state,suspension_origin,started_at) VALUES($1,$2,$3,$4,'MANAGED','SUSPENDED','MANAGER',clock_timestamp())`, orgA, sid("a", 92), sid("a", 10), sid("c", 2)},
	)
	// Organization B: same shapes, foreign markers.
	categoryB := sid("b", 50)
	adminExec(t, f, w.b,
		[]any{`INSERT INTO org_asset_categories(org_id,id,label) VALUES($1,$2,'FOREIGN category')`, orgB, categoryB},
		[]any{assetInsert, orgB, sid("b", 1), "VEHICLE", categoryB, "FOREIGN asset", "FOREIGN-1", "FRN1A23", "", "FOREIGN1", "FRN1A23", "DISTANCE", "ACTIVE", 1, nil, 0, nil, nil, 0, nil},
		[]any{`INSERT INTO org_meters(org_id,id,asset_id,kind,label,installed_at,created_operation_id) VALUES($1,$2,$3,'ODOMETER','FOREIGN odometer',$4,$5)`, orgB, sid("b", 20), sid("b", 1), periodOld, sid("b", 70)},
		[]any{readingInsert, orgB, sid("b", 30), sid("b", 20), sid("b", 40), int64(777777777), r1At, r1Sub, time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC), "UTC", int64(0), sid("b", 91), "WEB_SESSION", nil, sid("b", 80), sid("b", 72), hash, "STANDALONE", "TYPED", "VALID"},
		[]any{`INSERT INTO org_employee_identities(org_id,id,state) VALUES($1,$2,'ACTIVE')`, orgB, sid("b", 10)},
		[]any{`INSERT INTO org_employee_profiles(org_id,employee_id,display_name,registration_number,work_email,work_phone,job_title,home_unit_id,revision) VALUES($1,$2,'FOREIGN person','FOREIGN-REG','foreign@example.invalid','+5565999990002','FOREIGN title',NULL,1)`, orgB, sid("b", 10)},
	)
	w.foreign = []string{"FOREIGN", "777777777", "foreign@example.invalid", "+5565999990002", sid("b", 1), sid("b", 10), sid("b", 20), sid("b", 30), orgB}
	w.contacts = []string{"contact.a@example.invalid", "+5565999990001", "foreign@example.invalid", "+5565999990002", session, enrollment}
	w.assetRows = [][]string{
		{sid("a", 1), "VEHICLE", `=HYPERLINK("http://example.invalid","x")`, "+5511", "ABC1D23", "@SUM(A1)", "DISTANCE", categoryA, `=Synthetic fleet, "quoted"`, "ACTIVE", "3", unitNow, centre, w.acquiredNs, "", "", ""},
		{sid("a", 2), "MACHINE", "-Synthetic tractor", "F-02", "", "", "HOURS", categoryA, `=Synthetic fleet, "quoted"`, "DRAFT", "1", "", "", "", "", "", ""},
		{sid("a", 4), "VEHICLE", "Synthetic inactive vehicle", "", "XYZ9K87", "", "DISTANCE", categoryA, `=Synthetic fleet, "quoted"`, "INACTIVE", "7", "", "", "", "", "", ""},
	}
	w.readRows = [][]string{
		{sid("a", 33), sid("a", 2), sid("a", 21), "HOUR_METER", "", "3599999999", "2026-01-01T09:00:00Z", "UTC", "0", "2026-01-05T00:00:00Z", "METER_INSTALLATION", "TYPED", "VALID", requesterMember, "WEB_SESSION"},
		{sid("a", 30), sid("a", 1), sid("a", 20), "ODOMETER", "123456789", "", "2026-01-01T10:00:00.000001999Z", "America/Sao_Paulo", "-10800", "2026-01-05T00:00:01Z", "IMPORT", "IMPORTED", "VALID", requesterMember, "WEB_SESSION"},
		{sid("a", 31), sid("a", 1), sid("a", 20), "ODOMETER", "123456789", "", "2026-01-01T10:00:00.000001999Z", "America/Sao_Paulo", "-10800", "2026-01-05T00:00:00Z", "STANDALONE", "TYPED", "VALID", requesterMember, "WEB_SESSION"},
		{sid("a", 32), sid("a", 1), sid("a", 20), "ODOMETER", "999999999999", "", "2026-01-02T08:00:00Z", "America/Cuiaba", "-14400", "2026-01-06T00:00:00Z", "FUELING", "TYPED_WITH_PHOTO", "SUSPECT", requesterMember, "ENROLLED_DEVICE"},
	}
	w.employees = [][]string{
		{sid("a", 10), "ACTIVE", "", "SUSPENDED", "=CMD|' /C calc'!A0", "-0042", "@Lead", unitNow, "2", "6"},
		{sid("a", 11), "TERMINATED", sid("a", 10), "NONE", "", "", "", "", "3", ""},
		{sid("a", 12), "INACTIVE", "", "NONE", "Synthetic inactive person", "", "", "", "4", "1"},
		{requesterEmployee, "ACTIVE", "", "ACTIVE", "", "", "", "", "1", ""},
	}
	return w
}

func (w *world) produce(t *testing.T, kind, parameters string, asOf time.Time, limits domain.Limits) (domain.File, error) {
	t.Helper()
	format, ok := w.formats[kind]
	if !ok {
		t.Fatal("list kind not composed", kind)
	}
	canonical, err := format.Canonical([]byte(parameters))
	if err != nil {
		t.Fatal("test parameters refused", parameters, err)
	}
	sink, err := domain.NewSink(limits)
	if err != nil {
		t.Fatal(err)
	}
	in := app.ProduceInput{Scope: w.a, ExportID: sid("e", 1), Requester: w.requester, Parameters: canonical, AsOf: asOf}
	if err := format.Produce(t.Context(), in, sink); err != nil {
		if sink.Rows() != 0 && !errors.Is(err, domain.ErrTooLarge) {
			t.Fatal("a failed production counted rows", sink.Rows())
		}
		return domain.File{}, err
	}
	return sink.Result()
}

type pkg struct {
	names    []string
	csv      []byte
	manifest domain.ListManifest
	rawMan   []byte
}

func open(t *testing.T, file domain.File) pkg {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(file.Bytes), int64(len(file.Bytes)))
	if err != nil {
		t.Fatal("not a ZIP package", err)
	}
	var out pkg
	for _, entry := range reader.File {
		out.names = append(out.names, entry.Name)
		rc, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		if entry.Name == domain.ListManifestName {
			out.rawMan = content
			decoder := json.NewDecoder(bytes.NewReader(content))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&out.manifest); err != nil {
				t.Fatal("manifest", err)
			}
		} else {
			out.csv = content
		}
	}
	sum := sha256.Sum256(out.csv)
	if len(out.manifest.Files) != 1 || out.manifest.Files[0].SHA256 != hex.EncodeToString(sum[:]) || out.manifest.Files[0].Bytes != int64(len(out.csv)) || out.manifest.Files[0].Rows != file.Rows || out.manifest.RowCount != file.Rows {
		t.Fatal("manifest does not identify the CSV", out.manifest.Files)
	}
	return out
}

func records(t *testing.T, p pkg) [][]string {
	t.Helper()
	if !bytes.HasPrefix(p.csv, []byte(domain.ByteOrderMark)) {
		t.Fatal("CSV without byte-order mark")
	}
	all, err := csv.NewReader(bytes.NewReader(p.csv[len(domain.ByteOrderMark):])).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	return all
}

// compare checks every cell against the stored value: text cells round-trip
// through the inert scheme and never start a formula, integer and instant
// cells are exact.
func compare(t *testing.T, spec domain.ListSpec, got, want [][]string) {
	t.Helper()
	if len(got) != len(want)+1 {
		t.Fatalf("%s rows = %d, want %d: %q", spec.Kind, len(got)-1, len(want), got)
	}
	for i, c := range spec.Columns {
		if got[0][i] != c.Name {
			t.Fatal("header", got[0])
		}
	}
	for r, row := range want {
		for i, c := range spec.Columns {
			cell := got[r+1][i]
			switch c.Type {
			case domain.TextColumn:
				if cell != domain.InertText(row[i]) || domain.RecoverText(cell) != row[i] {
					t.Fatalf("%s row %d %s = %q, want %q", spec.Kind, r, c.Name, cell, row[i])
				}
				if cell != "" && strings.ContainsRune("=+-@\t\r\n", rune(cell[0])) {
					t.Fatalf("formula-capable cell %q", cell)
				}
			default:
				if cell != row[i] {
					t.Fatalf("%s row %d %s = %q, want exact %q", spec.Kind, r, c.Name, cell, row[i])
				}
			}
		}
	}
}

func (w *world) clean(t *testing.T, p pkg) {
	t.Helper()
	// The stated filters echo the requester's own input; everything else in
	// the manifest and the whole CSV must hold no foreign or omitted value.
	var manifest map[string]any
	if err := json.Unmarshal(p.rawMan, &manifest); err != nil {
		t.Fatal(err)
	}
	delete(manifest, "filters")
	rest, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	content := append(append([]byte{}, p.csv...), rest...)
	for _, value := range append(append([]string{}, w.foreign...), w.contacts...) {
		if bytes.Contains(content, []byte(value)) {
			t.Fatalf("export exposed a foreign or omitted value %q", value)
		}
	}
}

// B-BR-EMP-255/256/257, BUC-EMP-130/131 on real PostGIS: each admitted list
// reads only the job's organization, re-checks the requester in the reading
// transaction and writes exact, inert, deterministic rows with their basis.
func TestListExportsReadExactScopedInertRows(t *testing.T) {
	w := seed(t)
	assets, _ := lists.Spec(lists.AssetListKind)
	readings, _ := lists.Spec(lists.ReadingListKind)
	employees, _ := lists.Spec(lists.EmployeeListKind)
	for _, tc := range []struct {
		spec       domain.ListSpec
		parameters string
		asOf       time.Time
		want       [][]string
	}{
		{assets, `{}`, readingsAsOf, w.assetRows},
		{readings, `{}`, readingsAsOf, w.readRows},
		{employees, `{}`, readingsAsOf, w.employees},
	} {
		t.Run(tc.spec.Kind, func(t *testing.T) {
			before := w.check.checks.Load()
			file, err := w.produce(t, tc.spec.Kind, tc.parameters, tc.asOf, domain.DefaultLimits())
			if err != nil {
				t.Fatal(err)
			}
			if w.check.checks.Load() != before+1 || w.check.kinds.Load() != tc.spec.Kind+"|"+tc.spec.ReadPermission {
				t.Fatal("requester not re-checked in the reading transaction with the kind's permission")
			}
			p := open(t, file)
			if strings.Join(p.names, ",") != tc.spec.File+","+domain.ListManifestName {
				t.Fatal("entries", p.names)
			}
			compare(t, tc.spec, records(t, p), tc.want)
			w.clean(t, p)
			m := p.manifest
			if m.OrganizationID != w.a.OrgID().String() || m.OrganizationName != "Synthetic list organization" || m.OrganizationTimeZone != "America/Cuiaba" ||
				m.RequesterMembershipID != w.requester.MembershipID || m.ExportID != sid("e", 1) || m.Kind != tc.spec.Kind || m.ReadPermission != tc.spec.ReadPermission ||
				m.AsOf != orgInstant(tc.asOf) || m.GeneratedAt == "" || len(m.Filters) != 0 || m.RowCount != int64(len(tc.want)) || len(m.OmittedColumns) != len(tc.spec.Omitted) {
				t.Fatal("manifest basis", string(p.rawMan))
			}
			generated, err := time.Parse(time.RFC3339Nano, m.GeneratedAt)
			if err != nil || time.Since(generated) > time.Hour || time.Until(generated) > time.Minute {
				t.Fatal("generation instant is not the database instant", m.GeneratedAt)
			}
			// Same data and job: byte-identical CSV, manifest equal but for
			// the generation instant.
			again, err := w.produce(t, tc.spec.Kind, tc.parameters, tc.asOf, domain.DefaultLimits())
			if err != nil {
				t.Fatal(err)
			}
			q := open(t, again)
			if !bytes.Equal(p.csv, q.csv) {
				t.Fatal("same data produced different CSV bytes")
			}
			if !bytes.Equal(bytes.Replace(p.rawMan, []byte(m.GeneratedAt), nil, 1), bytes.Replace(q.rawMan, []byte(q.manifest.GeneratedAt), nil, 1)) {
				t.Fatal("manifest differs beyond generated_at")
			}
		})
	}
}

func orgInstant(at time.Time) string { return at.UTC().Format(time.RFC3339Nano) }

// B-BR-EMP-255/256: filters, the reading "as of" bound and exact period
// boundaries with sub-microsecond digits.
func TestListExportFiltersAndAsOf(t *testing.T) {
	w := seed(t)
	assets, _ := lists.Spec(lists.AssetListKind)
	readings, _ := lists.Spec(lists.ReadingListKind)
	employees, _ := lists.Spec(lists.EmployeeListKind)
	pick := func(rows [][]string, ids ...string) [][]string {
		var out [][]string
		for _, id := range ids {
			for _, r := range rows {
				if r[0] == id {
					out = append(out, r)
				}
			}
		}
		return out
	}
	archived := []string{sid("a", 3), "IMPLEMENT", "Synthetic archived implement", "ARC-1", "", "", "HOURS", sid("a", 50), `=Synthetic fleet, "quoted"`, "ARCHIVED", "5", "", "", "", "SALE", "2026-02-03T04:05:06.000000001Z", `'Sold, "as is"`}
	late := []string{sid("a", 34), sid("a", 1), sid("a", 20), "ODOMETER", "5", "", "2026-01-01T09:00:00Z", "UTC", "0", "2026-07-01T00:00:00Z", "STANDALONE", "TYPED", "VALID", w.requester.MembershipID, "WEB_SESSION"}
	for _, tc := range []struct {
		spec       domain.ListSpec
		parameters string
		asOf       time.Time
		want       [][]string
	}{
		{assets, `{"state":"ARCHIVED"}`, readingsAsOf, [][]string{archived}},
		{assets, `{"kind":"MACHINE"}`, readingsAsOf, pick(w.assetRows, sid("a", 2))},
		{assets, `{"kind":"VEHICLE","state":"INACTIVE"}`, readingsAsOf, pick(w.assetRows, sid("a", 4))},
		{readings, `{"asset_id":"` + sid("a", 2) + `"}`, readingsAsOf, pick(w.readRows, sid("a", 33))},
		{readings, `{"asset_id":"` + sid("b", 1) + `"}`, readingsAsOf, nil},
		{readings, `{"state":"SUSPECT"}`, readingsAsOf, pick(w.readRows, sid("a", 32))},
		{readings, `{"meter_kind":"ODOMETER"}`, readingsAsOf, pick(w.readRows, sid("a", 30), sid("a", 31), sid("a", 32))},
		{readings, `{"occurred_from":"2026-01-01T10:00:00.000001999Z"}`, readingsAsOf, pick(w.readRows, sid("a", 30), sid("a", 31), sid("a", 32))},
		{readings, `{"occurred_to":"2026-01-01T10:00:00.000001999Z"}`, readingsAsOf, pick(w.readRows, sid("a", 33))},
		{readings, `{"occurred_from":"2026-01-01T10:00:00.000001998Z","occurred_to":"2026-01-01T10:00:00.000002Z"}`, readingsAsOf, pick(w.readRows, sid("a", 30), sid("a", 31))},
		{readings, `{}`, lateReceipt, append([][]string{w.readRows[0], late}, w.readRows[1:]...)},
		{employees, `{"state":"TERMINATED"}`, readingsAsOf, pick(w.employees, sid("a", 11))},
	} {
		t.Run(tc.spec.Kind+tc.parameters+tc.asOf.Format("2006"), func(t *testing.T) {
			file, err := w.produce(t, tc.spec.Kind, tc.parameters, tc.asOf, domain.DefaultLimits())
			if err != nil {
				t.Fatal(err)
			}
			p := open(t, file)
			compare(t, tc.spec, records(t, p), tc.want)
			w.clean(t, p)
			var filters map[string]any
			if err := json.Unmarshal([]byte(tc.parameters), &filters); err != nil || len(filters) != len(p.manifest.Filters) {
				t.Fatal("filters not stated", p.manifest.Filters)
			}
			for k, v := range filters {
				if p.manifest.Filters[k] != v {
					t.Fatal("filter", k)
				}
			}
		})
	}
}

// B-BR-EMP-255/258, BR-PRIV-001: rights lost in the reading transaction,
// ceilings and read failures write nothing complete; row-level security
// hides the other organization even to a statement that names it.
func TestListExportsFailWithoutPartialOutput(t *testing.T) {
	w := seed(t)
	if len(w.formats) != 3 {
		t.Fatal("composed list formats", len(w.formats))
	}
	w.check.lost.Store(true)
	for kind := range w.formats {
		sink, _ := domain.NewSink(domain.DefaultLimits())
		in := app.ProduceInput{Scope: w.a, ExportID: sid("e", 1), Requester: w.requester, Parameters: []byte(`{}`), AsOf: readingsAsOf}
		if err := w.formats[kind].Produce(t.Context(), in, sink); !errors.Is(err, app.ErrRightsLost) || sink.Bytes() != 0 || sink.Rows() != 0 {
			t.Fatal("rights lost while reading still produced", kind, err)
		}
	}
	w.check.lost.Store(false)
	// The recorded membership is suspended: the in-transaction check refuses.
	org := w.a.OrgID().String()
	adminExec(t, w.f, w.a, []any{`UPDATE org_memberships SET state='SUSPENDED',suspension_origin='MANAGER' WHERE org_id=$1 AND id=$2`, org, w.requester.MembershipID})
	if _, err := w.produce(t, lists.AssetListKind, `{}`, readingsAsOf, domain.DefaultLimits()); !errors.Is(err, app.ErrRightsLost) {
		t.Fatal("suspended requester still read", err)
	}
	adminExec(t, w.f, w.a, []any{`UPDATE org_memberships SET state='ACTIVE',suspension_origin='' WHERE org_id=$1 AND id=$2`, org, w.requester.MembershipID})
	if _, err := w.produce(t, lists.ReadingListKind, `{}`, readingsAsOf, domain.Limits{Rows: 2, Bytes: domain.MaxBytes, Duration: time.Minute}); !errors.Is(err, domain.ErrTooLarge) {
		t.Fatal("row ceiling", err)
	}
	if _, err := w.produce(t, lists.AssetListKind, `{}`, readingsAsOf, domain.Limits{Rows: domain.MaxRows, Bytes: 400, Duration: time.Minute}); !errors.Is(err, domain.ErrTooLarge) {
		t.Fatal("byte ceiling", err)
	}
	// A privilege outage mid-query is a retryable failure, never a ceiling.
	role := pgx.Identifier{w.f.RuntimeRole}.Sanitize()
	if _, err := w.f.Admin.Exec(t.Context(), "REVOKE SELECT(origin) ON org_meter_readings FROM "+role); err != nil {
		t.Fatal(err)
	}
	if _, err := w.produce(t, lists.ReadingListKind, `{}`, readingsAsOf, domain.DefaultLimits()); err == nil || errors.Is(err, domain.ErrTooLarge) || errors.Is(err, app.ErrRightsLost) {
		t.Fatal("read failure", err)
	}
	// Row-level security is the second layer under the explicit filter.
	var foreign int
	if err := database.WithOrgTx(t.Context(), w.f.Runtime, w.a, func(tx pgx.Tx) error {
		return tx.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM org_assets WHERE org_id=$1)+(SELECT count(*) FROM org_employee_profiles WHERE org_id=$1)`, w.b.OrgID().String()).Scan(&foreign)
	}); err != nil || foreign != 0 {
		t.Fatal("row-level security exposed the other organization", foreign, err)
	}
	// Work contact is not even readable by the export role.
	err := database.WithOrgTx(t.Context(), w.f.Runtime, w.a, func(tx pgx.Tx) error {
		var email string
		return tx.QueryRow(t.Context(), `SELECT work_email FROM org_employee_profiles LIMIT 1`).Scan(&email)
	})
	if err == nil {
		t.Fatal("the export role can read work contact")
	}
}
