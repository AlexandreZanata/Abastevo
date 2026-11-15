package lists_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/adapters/lists"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/domain"
)

func formats(t *testing.T) map[string]app.Format {
	t.Helper()
	out := map[string]app.Format{}
	for _, f := range lists.New(nil, nil) {
		out[f.Kind()] = f
	}
	return out
}

// B-BR-EMP-255, EDR-172: exactly three kinds, each with its declared
// exportable read permission, the package media type and the infrastructure
// ceilings; spreadsheet kinds are not admitted.
func TestAdmittedListKindsAndPermissions(t *testing.T) {
	all := lists.New(nil, nil)
	registry, err := app.NewRegistry(all...)
	if err != nil {
		t.Fatal("list formats refused by the closed registry", err)
	}
	want := map[string]string{lists.AssetListKind: "fleet.asset.read", lists.ReadingListKind: "fleet.asset.read", lists.EmployeeListKind: "idn.employee.read"}
	if strings.Join(registry.Kinds(), ",") != "fleet.asset_list.csv,fleet.meter_reading_list.csv,idn.employee_list.csv" || len(all) != 3 {
		t.Fatal("admitted kinds", registry.Kinds())
	}
	for kind, read := range want {
		f, ok := registry.Lookup(kind)
		if !ok || string(f.ReadPermission()) != read || !app.ExportableRead(f.ReadPermission()) || f.MediaType() != domain.ListMediaType || f.Limits() != domain.DefaultLimits() {
			t.Fatal("format declaration", kind)
		}
		spec, ok := lists.Spec(kind)
		if !ok || spec.Validate() != nil || spec.Kind != kind || spec.ReadPermission != read {
			t.Fatal("spec", kind)
		}
	}
	for _, kind := range []string{"fleet.asset_list.xlsx", "fleet.meter_reading_list.xlsx", "idn.employee_list.xlsx", "doc.document_list.csv"} {
		if _, ok := registry.Lookup(kind); ok {
			t.Fatal("an unreviewed kind is admitted", kind)
		}
		if _, ok := lists.Spec(kind); ok {
			t.Fatal("an unreviewed kind has a spec", kind)
		}
	}
}

// B-BR-EMP-257: sensitive columns are declared as omissions with reasons and
// are never columns.
func TestSpecsOmitSensitiveColumnsWithReasons(t *testing.T) {
	for kind, omitted := range map[string]map[string]string{
		lists.AssetListKind:    {},
		lists.ReadingListKind:  {"capture_locator": "CAPTURE_LOCATOR"},
		lists.EmployeeListKind: {"work_email": "PURPOSE_LIMITED", "work_phone": "PURPOSE_LIMITED"},
	} {
		spec, _ := lists.Spec(kind)
		if len(spec.Omitted) != len(omitted) {
			t.Fatal("omissions", kind, spec.Omitted)
		}
		for _, o := range spec.Omitted {
			if omitted[o.Column] != o.Reason || o.Note == "" {
				t.Fatal("omission", kind, o)
			}
		}
		for _, c := range spec.Columns {
			if _, isOmitted := omitted[c.Name]; isOmitted || strings.Contains(c.Name, "email") || strings.Contains(c.Name, "phone") || strings.Contains(c.Name, "session") || strings.Contains(c.Name, "enrollment") ||
				strings.Contains(c.Name, "latitude") || strings.Contains(c.Name, "location") || strings.Contains(c.Name, "storage") {
				t.Fatal("a sensitive column is exported", kind, c.Name)
			}
			if c.Type == domain.IntegerColumn && c.Unit == "" {
				t.Fatal("integer column without unit", kind, c.Name)
			}
		}
	}
	readings, _ := lists.Spec(lists.ReadingListKind)
	units := map[string]string{}
	for _, c := range readings.Columns {
		units[c.Name] = c.Unit
	}
	if units["value_m"] != "m" || units["value_s"] != "s" || units["device_offset_s"] != "s" {
		t.Fatal("reading units", units)
	}
	if readings.Review != domain.ApprovedOnly || readings.ReviewLabel != "somente aprovados" || !slices.Equal(readings.Ordering, []string{"occurred_at", "reading_id"}) {
		t.Fatal("reading basis", readings.Review, readings.Ordering)
	}
	for _, kind := range []string{lists.AssetListKind, lists.EmployeeListKind} {
		if spec, _ := lists.Spec(kind); spec.Review != domain.NotApplicable {
			t.Fatal("register review basis", kind)
		}
	}
}

// B-BR-EMP-255: filters are a closed, canonical object per kind.
func TestCanonicalFiltersAreClosedAndBounded(t *testing.T) {
	all := formats(t)
	for kind, cases := range map[string]map[string]bool{
		lists.AssetListKind: {
			`{}`: true, `{"state":"ACTIVE"}`: true, `{"state":"ARCHIVED"}`: true, `{"kind":"VEHICLE","state":"DRAFT"}`: true, `{"kind":"IMPLEMENT"}`: true,
			`{"state":""}`: false, `{"state":"active"}`: false, `{"state":"DELETED"}`: false, `{"kind":"TRUCK"}`: false, `{"unit_id":"7b0e2a54-0000-4000-8000-000000000001"}`: false,
			`{"state":1}`: false, `{"state":["ACTIVE"]}`: false, `{"state":{"x":"y"}}`: false, `{"rows":3}`: false,
		},
		lists.ReadingListKind: {
			`{}`: true, `{"asset_id":"7b0e2a54-0000-4000-8000-000000000001"}`: true, `{"meter_kind":"HOUR_METER","state":"SUSPECT"}`: true,
			`{"occurred_from":"2026-01-01T00:00:00Z","occurred_to":"2026-02-01T00:00:00.000000001Z"}`: true, `{"occurred_from":"2026-01-01T00:00:00Z"}`: true, `{"occurred_to":"2026-01-01T00:00:00Z"}`: true,
			`{"asset_id":"7B0E2A54-0000-4000-8000-000000000001"}`: false, `{"asset_id":"00000000-0000-0000-0000-000000000000"}`: false, `{"asset_id":""}`: false,
			`{"meter_kind":"FUEL_GAUGE"}`: false, `{"state":"PENDING"}`: false,
			`{"occurred_from":"2026-01-01T00:00:00-03:00"}`: false, `{"occurred_from":"2026-01-01"}`: false, `{"occurred_from":"2026-01-01T00:00:00.000Z"}`: false,
			`{"occurred_from":"2026-02-01T00:00:00Z","occurred_to":"2026-01-01T00:00:00Z"}`: false, `{"occurred_from":"2026-01-01T00:00:00Z","occurred_to":"2026-01-01T00:00:00Z"}`: false,
			`{"kind":"ODOMETER"}`: false,
		},
		lists.EmployeeListKind: {
			`{}`: true, `{"state":"TERMINATED"}`: true, `{"state":"INACTIVE"}`: true,
			`{"state":"MERGED"}`: false, `{"state":""}`: false, `{"work_email":"x@example.invalid"}`: false, `{"include_contact":true}`: false,
		},
	} {
		f, admitted := all[kind]
		if !admitted {
			t.Fatal("kind not admitted", kind)
		}
		for raw, ok := range cases {
			canonical, err := domain.CanonicalParameters([]byte(raw))
			if err != nil {
				t.Fatal("test case is not infrastructure-canonical", raw)
			}
			got, err := f.Canonical(canonical)
			if ok != (err == nil) {
				t.Fatalf("%s Canonical(%s) error = %v", kind, raw, err)
			}
			if ok && string(got) != string(canonical) {
				t.Fatalf("%s Canonical(%s) = %s", kind, raw, got)
			}
		}
		if _, err := f.Canonical([]byte(`{"state": "ACTIVE"}`)); err == nil {
			t.Fatal("non-canonical bytes admitted", kind)
		}
	}
}

// EDR-172: a format composed without its pool or authority is unavailable
// and writes nothing.
func TestUncomposedFormatProducesNothing(t *testing.T) {
	org, err := kernel.ParseOrgID("7b0e2a54-0000-4000-8000-000000000001")
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(org)
	if err != nil {
		t.Fatal(err)
	}
	all := lists.New(nil, nil)
	if len(all) != 3 {
		t.Fatal("admitted formats", len(all))
	}
	for _, f := range all {
		sink, _ := domain.NewSink(domain.DefaultLimits())
		in := app.ProduceInput{Scope: scope, ExportID: "7b0e2a54-0000-4000-8000-000000000050", Requester: app.Requester{AccountID: "7b0e2a54-0000-4000-8000-000000000002", MembershipID: "7b0e2a54-0000-4000-8000-000000000004"},
			Parameters: []byte(`{}`), AsOf: time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)}
		if err := f.Produce(t.Context(), in, sink); !errors.Is(err, app.ErrUnavailable) || sink.Bytes() != 0 || sink.Rows() != 0 {
			t.Fatal("uncomposed format produced", f.Kind(), err)
		}
	}
}
