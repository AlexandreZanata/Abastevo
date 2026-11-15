package application_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/application"
	fuel "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
)

func id(n int) string { return fmt.Sprintf("7b0e2a54-0000-4000-8000-%012d", n) }

const origin = "https://api.example.invalid"

func payload() map[string]any {
	return map[string]any{
		"asset_id": id(10), "responsible_employee_id": id(30), "product": "DIESEL_S10", "unit": "L",
		"quantity": "150000", "paid_total": "97000", "discount": nil, "unit_price": "6490", "unit_price_text": "6,490",
		"fill": "FULL", "payer": "COMPANY", "payment_method": "FUEL_CARD",
		"reading":                map[string]any{"meter_id": id(11), "segment_id": id(12), "reading_id": id(13), "kind": "ODOMETER", "value": "184190000"},
		"missing_reading_reason": nil, "place": nil, "trip_id": nil, "contracted_supplier_id": nil,
	}
}

func operation(t *testing.T, change func(envelope, payload map[string]any)) protocol.Operation {
	t.Helper()
	body := payload()
	envelope := map[string]any{"schema_version": 1, "operation_id": id(20), "owner_account_id": id(1), "api_origin": origin, "org_id": id(65), "membership_id": id(3), "enrollment_id": id(4), "type": app.OperationType, "operation_schema": 1, "entity": map[string]any{"type": app.EntityType, "id": id(40), "expected_revision": nil}, "occurred_at": "2026-11-13T12:00:00.000000123Z", "entry_zone": "America/Cuiaba", "device_offset_seconds": -14400, "depends_on": []any{}, "soft_references": []any{}, "attachments": []any{}, "payload": body}
	if change != nil {
		change(envelope, body)
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := protocol.NewBinding(id(1), origin, id(65))
	if err != nil {
		t.Fatal(err)
	}
	op, err := protocol.Seal(raw, binding)
	if err != nil {
		t.Fatal("invalid unit operation fixture", err)
	}
	return op
}

// B-BR-EMP-263/265: the envelope owns identity and declaration; the payload
// carries exact integers and either a reading or a reason, never both.
func TestParseOperationKeepsTheExactOriginalFact(t *testing.T) {
	op := operation(t, nil)
	command, err := app.ParseOperation(op)
	if err != nil {
		t.Fatal(err)
	}
	v, err := command.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	c := v.Candidate.Snapshot()
	amounts := c.Amounts.Snapshot()
	if v.OperationID != id(20) || v.FuelingID != id(40) || v.AssetID != id(10) || v.ResponsibleID != id(30) || v.AuthorMembershipID != id(3) || v.PayloadDigest != op.PayloadDigest() {
		t.Fatal("identity not taken from the envelope", v.FuelingID)
	}
	if v.Declared.OccurredAt.Nanosecond() != 123 || v.Declared.EntryZone != "America/Cuiaba" || v.Declared.DeviceOffsetSeconds != -14400 {
		t.Fatal("declaration lost precision")
	}
	if c.Quantity.Amount() != 150000 || c.Quantity.Product().Code() != fuel.DieselS10 || amounts.Paid != 97000 || amounts.Discount != nil || *amounts.UnitPrice != 6490 || *amounts.SourcePriceText != "6,490" || c.Fill != fuel.Full || c.Payer != fuel.Company || c.Payment != fuel.FuelCard || c.Origin == nil || *c.Origin != fuel.App {
		t.Fatal("amounts or declarations changed")
	}
	if v.Reading == nil || v.Reading.MeterID != id(11) || v.Reading.SegmentID != id(12) || v.Reading.ReadingID != id(13) || v.Reading.Value.Kind() != meter.Odometer || v.Reading.Value.Count() != 184190000 || v.MissingReason != nil || v.PlaceName != nil {
		t.Fatal("reading evidence changed")
	}
	reading, ok, err := command.MeterCommand()
	if err != nil || !ok {
		t.Fatal("embedded reading unavailable", err)
	}
	r, err := reading.Snapshot()
	if err != nil || r.Origin != meter.Fueling || r.Method != meter.Typed || r.OperationID != id(20) || r.ReadingID != id(13) || !r.Declared.OccurredAt.Equal(v.Declared.OccurredAt) {
		t.Fatal("embedded reading is not the fueling's evidence", err)
	}
	arithmetic, err := fuel.Reconcile(v.Candidate)
	if err != nil || arithmetic.Warning != fuel.ArithmeticMismatch || amounts.Paid != 97000 {
		t.Fatal("arithmetic mismatch must be a warning on the entered total", err)
	}

	missing := operation(t, func(_, p map[string]any) {
		p["reading"], p["missing_reading_reason"] = nil, "Odometer display broken"
		p["place"] = map[string]any{"kind": "NOT_FOUND", "name": "Posto sem cadastro"}
		p["discount"], p["unit_price"], p["unit_price_text"] = "500", nil, nil
		p["product"], p["unit"], p["quantity"] = "ELECTRICITY", "KWH", "30000"
	})
	command, err = app.ParseOperation(missing)
	if err != nil {
		t.Fatal(err)
	}
	v, _ = command.Snapshot()
	if v.Reading != nil || v.MissingReason == nil || *v.MissingReason != "Odometer display broken" || v.PlaceName == nil || *v.PlaceName != "Posto sem cadastro" {
		t.Fatal("missing reason or typed place lost")
	}
	if _, ok, err := command.MeterCommand(); ok || err != nil {
		t.Fatal("a reason produced a reading")
	}
}

func TestParseOperationRefusesMalformedAndKeepsUnsupportedForms(t *testing.T) {
	malformed := map[string]func(e, p map[string]any){
		"unknown-key":         func(_, p map[string]any) { p["author_membership_id"] = id(3) },
		"missing-key":         func(_, p map[string]any) { delete(p, "fill") },
		"unit-not-of-product": func(_, p map[string]any) { p["unit"] = "M3" },
		"unknown-product":     func(_, p map[string]any) { p["product"] = "LPG_P13" },
		"lossy-quantity":      func(_, p map[string]any) { p["quantity"] = "150.000" },
		"numeric-paid":        func(_, p map[string]any) { p["paid_total"] = 97000 },
		"zero-quantity":       func(_, p map[string]any) { p["quantity"] = "0" },
		"leading-zero":        func(_, p map[string]any) { p["paid_total"] = "097000" },
		"price-out-of-bounds": func(_, p map[string]any) { p["unit_price"] = "1000001" },
		"text-without-price":  func(_, p map[string]any) { p["unit_price"] = nil },
		"reading-and-reason":  func(_, p map[string]any) { p["missing_reading_reason"] = "Display broken" },
		"neither-reading":     func(_, p map[string]any) { p["reading"] = nil },
		"blank-reason":        func(_, p map[string]any) { p["reading"], p["missing_reading_reason"] = nil, "  " },
		"reading-extra-key":   func(_, p map[string]any) { p["reading"].(map[string]any)["method"] = "TYPED" },
		"reading-kind":        func(_, p map[string]any) { p["reading"].(map[string]any)["kind"] = "SPEEDOMETER" },
		"reading-value":       func(_, p map[string]any) { p["reading"].(map[string]any)["value"] = "-1" },
		"fill":                func(_, p map[string]any) { p["fill"] = "HALF" },
		"payer":               func(_, p map[string]any) { p["payer"] = "SUPPLIER" },
		"payment":             func(_, p map[string]any) { p["payment_method"] = "BARTER" },
		"non-canonical-asset": func(_, p map[string]any) { p["asset_id"] = strings.ToUpper(id(10)) },
		"blank-place":         func(_, p map[string]any) { p["place"] = map[string]any{"kind": "NOT_FOUND", "name": " "} },
		"unknown-place-kind":  func(_, p map[string]any) { p["place"] = map[string]any{"kind": "GARAGE", "name": "x"} },
		"entity-type": func(e, _ map[string]any) {
			e["entity"] = map[string]any{"type": "fuel_record", "id": id(40), "expected_revision": nil}
		},
		"expected-revision": func(e, _ map[string]any) {
			e["entity"] = map[string]any{"type": app.EntityType, "id": id(40), "expected_revision": 1}
		},
		"nul-in-price-text":     func(_, p map[string]any) { p["unit_price_text"] = "6\u0000490" },
		"place-not-object":      func(_, p map[string]any) { p["place"] = "Posto" },
		"reading-not-object":    func(_, p map[string]any) { p["reading"] = "184190000" },
		"responsible-not-uuid":  func(_, p map[string]any) { p["responsible_employee_id"] = "driver" },
		"discount-out-of-range": func(_, p map[string]any) { p["discount"] = "1000000000001" },
	}
	for name, change := range malformed {
		t.Run(name, func(t *testing.T) {
			if _, err := app.ParseOperation(operation(t, change)); !errors.Is(err, app.ErrMalformed) {
				t.Fatal("expected MALFORMED", err)
			}
		})
	}
	unsupported := map[string]func(e, p map[string]any){
		"trip":       func(_, p map[string]any) { p["trip_id"] = id(50) },
		"contracted": func(_, p map[string]any) { p["contracted_supplier_id"] = id(51) },
		"catalog-station": func(_, p map[string]any) {
			p["place"] = map[string]any{"kind": "CATALOG_STATION", "station_id": id(52)}
		},
		"org-supplier":  func(_, p map[string]any) { p["place"] = map[string]any{"kind": "ORG_SUPPLIER", "supplier_id": id(53)} },
		"internal-tank": func(_, p map[string]any) { p["place"] = map[string]any{"kind": "INTERNAL_TANK", "tank_id": id(54)} },
		"attachment": func(e, _ map[string]any) {
			e["attachments"] = []any{map[string]any{"id": id(55), "sha256": strings.Repeat("a", 64), "bytes": 10, "mime": "image/jpeg"}}
		},
		"dependency":     func(e, _ map[string]any) { e["depends_on"] = []any{id(56)} },
		"soft-reference": func(e, _ map[string]any) { e["soft_references"] = []any{map[string]any{"type": "trip", "id": id(57)}} },
	}
	for name, change := range unsupported {
		t.Run(name, func(t *testing.T) {
			op := operation(t, change)
			if _, err := app.ParseOperation(op); !errors.Is(err, app.ErrUnsupported) {
				t.Fatal("expected a retained unavailable form", err)
			}
		})
	}
	if _, err := (app.Command{}).Snapshot(); err == nil {
		t.Fatal("zero command admitted")
	}
	if _, _, err := (app.Command{}).MeterCommand(); err == nil {
		t.Fatal("zero command produced a reading")
	}
}

func TestPoliciesAreTheCatalogFuelingKeys(t *testing.T) {
	if p := app.Policy(app.RecordAction); p.Permission != "fuel.fueling.record" || !p.Effect.Write() || p.Validate() != nil {
		t.Fatal("record policy")
	}
	if p := app.Policy(app.ReadAction); p.Permission != "fuel.fueling.read" || p.Effect.Write() || p.Validate() != nil {
		t.Fatal("read policy")
	}
	if p := app.Policy(0); p.Permission != "" {
		t.Fatal("unknown action admitted")
	}
}

func TestCommandDiagnosticsRedactPrivateFacts(t *testing.T) {
	command, err := app.ParseOperation(operation(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	v, _ := command.Snapshot()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{fmt.Sprint(v), fmt.Sprintf("%+v", *v.Reading), string(raw), fmt.Sprint(app.Target{FuelingID: id(40)}), fmt.Sprint(app.Prepared{AuthorEmployeeID: id(30)}), fmt.Sprint(app.Record{ID: id(40)})} {
		for _, private := range []string{id(10), id(11), id(30), id(40), "97000", "6,490"} {
			if strings.Contains(text, private) {
				t.Fatal("diagnostic leaked a private fueling fact", text)
			}
		}
	}
}
