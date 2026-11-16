package application_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/expense/application"
	expense "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/expense/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
)

func id(n int) string { return fmt.Sprintf("9e5b0000-0000-4000-8000-%012d", n) }

func payload() map[string]any {
	return map[string]any{"asset_id": id(21), "category": "TOLL", "paid_total": "1250", "company_amount": "1250", "payer": "EMPLOYEE", "trip_id": nil}
}

func operation(t *testing.T, change func(envelope, payload map[string]any)) protocol.Operation {
	t.Helper()
	body := payload()
	envelope := map[string]any{"schema_version": 1, "operation_id": id(20), "owner_account_id": id(1), "api_origin": vectorOrigin, "org_id": id(65), "membership_id": id(3), "enrollment_id": id(4),
		"type": app.OperationType, "operation_schema": 1, "entity": map[string]any{"type": app.EntityType, "id": id(40), "expected_revision": nil},
		"occurred_at": "2026-11-13T12:00:00.000000123Z", "entry_zone": "America/Cuiaba", "device_offset_seconds": -14400,
		"depends_on": []any{}, "soft_references": []any{}, "attachments": []any{}, "payload": body}
	if change != nil {
		change(envelope, body)
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := protocol.NewBinding(id(1), vectorOrigin, id(65))
	if err != nil {
		t.Fatal(err)
	}
	op, err := protocol.Seal(raw, binding)
	if err != nil {
		t.Fatal("invalid unit operation fixture", err)
	}
	return op
}

// B-BR-EMP-447: the envelope owns identity and competence; the payload
// carries exact centavos, a v1 direct category, the payer and the asset.
func TestParseOperationKeepsTheExactDirectExpense(t *testing.T) {
	op := operation(t, nil)
	command, err := app.ParseOperation(op)
	if err != nil {
		t.Fatal(err)
	}
	v, err := command.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if v.OperationID != id(20) || v.ExpenseID != id(40) || v.AssetID != id(21) || v.AuthorMembershipID != id(3) || v.PayloadDigest != op.PayloadDigest() {
		t.Fatal("identity not taken from the envelope")
	}
	if v.Declared.OccurredAt.Nanosecond() != 123 || v.Declared.EntryZone != "America/Cuiaba" || v.Declared.DeviceOffsetSeconds != -14400 {
		t.Fatal("competence lost precision")
	}
	if v.Category != "TOLL" || v.Paid != 1250 || v.Company != 1250 || v.Payer != "EMPLOYEE" {
		t.Fatal("amounts or category changed")
	}
	if text := fmt.Sprintf("%v %+v", command, v); strings.Contains(text, id(21)) || strings.Contains(text, "1250") {
		t.Fatal("private values reached formatting")
	}
	if _, err := (app.Command{}).Snapshot(); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("zero command produced a snapshot")
	}
}

// B-BR-EMP-447: envelope shape, exact keys and recognised later forms.
func TestParseOperationRefusesMalformedAndKeepsUnsupportedForms(t *testing.T) {
	malformed := map[string]func(e, p map[string]any){
		"wrong-type": func(e, _ map[string]any) { e["type"] = "expense.record" },
		"wrong-entity": func(e, _ map[string]any) {
			e["entity"] = map[string]any{"type": "fueling", "id": id(40), "expected_revision": nil}
		},
		"expected-revision": func(e, _ map[string]any) {
			e["entity"] = map[string]any{"type": app.EntityType, "id": id(40), "expected_revision": 1}
		},
		"unknown-key":           func(_, p map[string]any) { p["description"] = "pedágio" },
		"missing-trip-key":      func(_, p map[string]any) { delete(p, "trip_id") },
		"missing-asset":         func(_, p map[string]any) { delete(p, "asset_id") },
		"null-asset":            func(_, p map[string]any) { p["asset_id"] = nil },
		"null-category":         func(_, p map[string]any) { p["category"] = nil },
		"null-paid":             func(_, p map[string]any) { p["paid_total"] = nil },
		"null-company":          func(_, p map[string]any) { p["company_amount"] = nil },
		"null-payer":            func(_, p map[string]any) { p["payer"] = nil },
		"numeric-paid":          func(_, p map[string]any) { p["paid_total"] = 1250 },
		"numeric-category":      func(_, p map[string]any) { p["category"] = 7 },
		"fuel-category":         func(_, p map[string]any) { p["category"] = "FUEL" },
		"company-above-paid":    func(_, p map[string]any) { p["company_amount"] = "1251" },
		"trip-not-identifier":   func(_, p map[string]any) { p["trip_id"] = "viagem" },
		"trip-numeric":          func(_, p map[string]any) { p["trip_id"] = 9 },
		"fuel-with-attachments": func(e, p map[string]any) { p["category"] = "FUEL"; e["attachments"] = attachments() },
	}
	for name, change := range malformed {
		t.Run(name, func(t *testing.T) {
			if _, err := app.ParseOperation(operation(t, change)); !errors.Is(err, app.ErrMalformed) {
				t.Fatal("expected MALFORMED", err)
			}
		})
	}
	unsupported := map[string]func(e, p map[string]any){
		"attachments":        func(e, _ map[string]any) { e["attachments"] = attachments() },
		"dependency":         func(e, _ map[string]any) { e["depends_on"] = []any{id(19)} },
		"soft-reference":     func(e, _ map[string]any) { e["soft_references"] = []any{map[string]any{"type": "trip", "id": id(31)}} },
		"trip":               func(_, p map[string]any) { p["trip_id"] = id(31) },
		"configured-catalog": func(_, p map[string]any) { p["category"] = "MEALS" },
	}
	for name, change := range unsupported {
		t.Run(name, func(t *testing.T) {
			if _, err := app.ParseOperation(operation(t, change)); !errors.Is(err, app.ErrUnsupported) {
				t.Fatal("expected UNSUPPORTED", err)
			}
		})
	}
}

func attachments() []any {
	return []any{map[string]any{"id": id(50), "sha256": strings.Repeat("a", 64), "bytes": 10, "mime": "image/jpeg"}}
}

func TestPolicyAndCatalogue(t *testing.T) {
	if p := app.Policy(app.RecordAction); p.Permission != "exp.expense.record" || p.Effect != access.OperationalWrite || p.Validate() != nil {
		t.Fatal("record policy drift")
	}
	if p := app.Policy(app.ReadAction); p.Permission != "exp.expense.read" || p.Effect != access.Read || p.Validate() != nil {
		t.Fatal("read policy drift")
	}
	if p := app.Policy(0); p.Permission != "" {
		t.Fatal("unknown action has a policy")
	}
	if got := strings.Join(app.DirectCategories(), ","); got != "FINE,INSURANCE,LEASE,MAINTENANCE,OTHER,PARKING,TOLL" {
		t.Fatal("direct catalogue drift", got)
	}
	categories := app.DirectCategories()
	categories[0] = "FUEL"
	if app.DirectCategories()[0] != "FINE" {
		t.Fatal("catalogue is shared mutable state")
	}
}

func scope(t *testing.T, n int) kernel.OrgScope {
	t.Helper()
	org, err := kernel.ParseOrgID(id(n))
	if err != nil {
		t.Fatal(err)
	}
	s, err := kernel.NewOrgScope(org)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// The VAL-13 fueling: 97 350 centavos paid by the employee, unit snapshot.
func fueling() app.FuelingSource {
	return app.FuelingSource{ID: id(30), OperationID: id(31), PayloadDigest: "sha256:" + strings.Repeat("b", 64), Revision: 1, AssetID: id(21),
		AuthorMembershipID: id(3), AuthorEmployeeID: id(5), EnrollmentID: id(4),
		Declared: orgtime.Declaration{OccurredAt: time.Date(2026, 12, 1, 2, 30, 0, 123, time.UTC), EntryZone: "America/Cuiaba", DeviceOffsetSeconds: -14400},
		Paid:     97350, Payer: "EMPLOYEE", UnitID: id(60), CostCentreID: id(61), ReviewState: app.Submitted}
}

func zones(unit string) orgtime.ReportingZones {
	return orgtime.ReportingZones{Organization: "America/Sao_Paulo", Unit: &unit}
}

// B-BR-EMP-445: one FUEL expense of exactly the stored paid total, with the
// fueling's identity, declaration, snapshot and the unit-zone reporting day.
func TestProjectFuelingCopiesTheStoredFuelingExactly(t *testing.T) {
	s := scope(t, 65)
	got, err := app.ProjectFueling(s, id(70), fueling(), zones("America/Cuiaba"))
	if err != nil {
		t.Fatal(err)
	}
	f := fueling()
	if got.ID != id(70) || got.OriginKind != string(expense.KindFueling) || got.SourceID != f.ID || got.FuelingID != f.ID || got.SourceRevision != 1 ||
		got.OperationID != f.OperationID || got.PayloadDigest != f.PayloadDigest || got.AssetID != f.AssetID || got.AuthorMembershipID != f.AuthorMembershipID ||
		got.AuthorEmployeeID != f.AuthorEmployeeID || got.EnrollmentID != f.EnrollmentID || got.Category != app.FuelCategory || got.Paid != 97350 || got.Company != 97350 ||
		got.Payer != "EMPLOYEE" || !got.Declared.OccurredAt.Equal(f.Declared.OccurredAt) || got.Declared.EntryZone != f.Declared.EntryZone ||
		got.Declared.DeviceOffsetSeconds != f.Declared.DeviceOffsetSeconds || got.UnitID != f.UnitID || got.CostCentreID != f.CostCentreID ||
		got.ReviewState != app.Submitted || got.Revision != 1 {
		t.Fatalf("projected expense differs from the stored fueling: %+v", got)
	}
	// 02:30 UTC on 1 December is still 30 November in Cuiabá (UTC-4), the unit zone.
	if got.Reporting != (orgtime.ReportingAttribution{Day: "2026-11-30", Zone: "America/Cuiaba", Source: "unit"}) {
		t.Fatal("reporting day not attributed in the unit zone", got.Reporting)
	}
	// Without a unit snapshot the organization zone decides (UTC-3: still 30 November).
	f.UnitID, f.CostCentreID = "", ""
	org, err := app.ProjectFueling(s, id(70), f, orgtime.ReportingZones{Organization: "America/Sao_Paulo"})
	if err != nil || org.Reporting != (orgtime.ReportingAttribution{Day: "2026-11-30", Zone: "America/Sao_Paulo", Source: "organization"}) || org.UnitID != "" {
		t.Fatal("organization-zone attribution", org.Reporting, err)
	}
	// A known zero paid total still has its one expense (B-BR-EMP-181).
	f.Paid = 0
	if zero, err := app.ProjectFueling(s, id(70), f, orgtime.ReportingZones{Organization: "UTC"}); err != nil || zero.Paid != 0 || zero.Company != 0 {
		t.Fatal("zero-paid fueling lost its expense", err)
	}
	var logged strings.Builder
	slog.New(slog.NewTextHandler(&logged, nil)).Info("projection", "record", got)
	if text := fmt.Sprintf("%v %+v", got, got) + logged.String(); strings.Contains(text, "97350") || strings.Contains(text, id(21)) {
		t.Fatal("private projection values reached formatting or logs")
	}
}

// The projection trusts nothing it cannot prove: foreign or malformed
// sources, a revision other than the supported one, unknown payers or
// states, invalid zones and a mismatched unit snapshot are unavailable.
func TestProjectFuelingRefusesUnprovenSources(t *testing.T) {
	s := scope(t, 65)
	for name, change := range map[string]func(*app.FuelingSource, *orgtime.ReportingZones, *string){
		"missing-identity":  func(f *app.FuelingSource, _ *orgtime.ReportingZones, _ *string) { f.ID = "" },
		"bad-expense-id":    func(_ *app.FuelingSource, _ *orgtime.ReportingZones, e *string) { *e = "x" },
		"revision-two":      func(f *app.FuelingSource, _ *orgtime.ReportingZones, _ *string) { f.Revision = 2 },
		"negative-paid":     func(f *app.FuelingSource, _ *orgtime.ReportingZones, _ *string) { f.Paid = -1 },
		"above-ceiling":     func(f *app.FuelingSource, _ *orgtime.ReportingZones, _ *string) { f.Paid = 1_000_000_000_001 },
		"unknown-payer":     func(f *app.FuelingSource, _ *orgtime.ReportingZones, _ *string) { f.Payer = "SUPPLIER" },
		"approved-state":    func(f *app.FuelingSource, _ *orgtime.ReportingZones, _ *string) { f.ReviewState = "APPROVED" },
		"centre-no-unit":    func(f *app.FuelingSource, _ *orgtime.ReportingZones, _ *string) { f.UnitID = "" },
		"unit-zone-missing": func(_ *app.FuelingSource, z *orgtime.ReportingZones, _ *string) { z.Unit = nil },
		"invalid-zone":      func(_ *app.FuelingSource, z *orgtime.ReportingZones, _ *string) { u := "Mars/Base"; z.Unit = &u },
		"zero-instant":      func(f *app.FuelingSource, _ *orgtime.ReportingZones, _ *string) { f.Declared.OccurredAt = time.Time{} },
		"bad-digest":        func(f *app.FuelingSource, _ *orgtime.ReportingZones, _ *string) { f.PayloadDigest = "sha256:x" },
		"bad-asset":         func(f *app.FuelingSource, _ *orgtime.ReportingZones, _ *string) { f.AssetID = "FR-001" },
	} {
		t.Run(name, func(t *testing.T) {
			f, z, e := fueling(), zones("America/Cuiaba"), id(70)
			change(&f, &z, &e)
			if _, err := app.ProjectFueling(s, e, f, z); !errors.Is(err, app.ErrUnavailable) {
				t.Fatal("unproven source projected", err)
			}
		})
	}
	if _, err := app.ProjectFueling(kernel.OrgScope{}, id(70), fueling(), zones("America/Cuiaba")); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("zero scope projected")
	}
}

// EDR-290: no stored cost inserts; an equal stored cost is unchanged; a
// stored newer revision makes the request stale; any divergence or a newer
// request is unavailable and never overwrites.
func TestDecideProjectionIsIdempotentAndNeverOverwrites(t *testing.T) {
	s := scope(t, 65)
	requested, err := app.ProjectFueling(s, id(70), fueling(), zones("America/Cuiaba"))
	if err != nil {
		t.Fatal(err)
	}
	if decision, err := app.DecideProjection(requested, nil); err != nil || decision != app.Insert {
		t.Fatal("no stored cost must insert", decision, err)
	}
	stored := requested
	stored.ReceivedAt = time.Date(2026, 12, 1, 3, 0, 0, 0, time.UTC)
	if decision, err := app.DecideProjection(requested, &stored); err != nil || decision != app.Unchanged {
		t.Fatal("an equal stored cost must be unchanged", decision, err)
	}
	// The stored expense of another identity but the same origin is still the one cost.
	other := stored
	other.ID = id(71)
	if decision, err := app.DecideProjection(requested, &other); err != nil || decision != app.Unchanged {
		t.Fatal("the stored cost of the origin keeps its own identity", decision, err)
	}
	newer := stored
	newer.SourceRevision = 2
	if _, err := app.DecideProjection(requested, &newer); !errors.Is(err, app.ErrStale) {
		t.Fatal("a stale projection did not refuse", err)
	}
	ahead := requested
	ahead.SourceRevision = 2
	if _, err := app.DecideProjection(ahead, &stored); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("a newer revision overwrote the stored cost", err)
	}
	for name, change := range map[string]func(*app.Record){
		"paid":      func(r *app.Record) { r.Paid = 97000; r.Company = 97000 },
		"company":   func(r *app.Record) { r.Company = 90000 },
		"category":  func(r *app.Record) { r.Category = "OTHER" },
		"fueling":   func(r *app.Record) { r.FuelingID, r.SourceID = id(39), id(39) },
		"kind":      func(r *app.Record) { r.OriginKind = string(expense.KindDirect) },
		"unit":      func(r *app.Record) { r.UnitID = id(69) },
		"reporting": func(r *app.Record) { r.Reporting.Day = "2026-12-01" },
		"state":     func(r *app.Record) { r.ReviewState = "APPROVED" },
	} {
		t.Run(name, func(t *testing.T) {
			divergent := stored
			change(&divergent)
			if _, err := app.DecideProjection(requested, &divergent); !errors.Is(err, app.ErrUnavailable) {
				t.Fatal("a divergent stored cost was accepted or overwritten", err)
			}
		})
	}
	if _, err := app.DecideProjection(app.Record{}, nil); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("an empty request inserted")
	}
}
