package application_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
)

func id(n int) string { return fmt.Sprintf("7b0e2a54-0000-4000-8000-%012d", n) }

const origin = "https://api.example.invalid"

func reading(value string) map[string]any {
	return map[string]any{"meter_id": id(11), "segment_id": id(12), "reading_id": id(13), "kind": "ODOMETER", "value": value}
}

func operation(t *testing.T, kind string, body map[string]any, change func(envelope, payload map[string]any)) protocol.Operation {
	t.Helper()
	envelope := map[string]any{"schema_version": 1, "operation_id": id(20), "owner_account_id": id(1), "api_origin": origin, "org_id": id(65), "membership_id": id(3),
		"enrollment_id": id(4), "type": kind, "operation_schema": 1, "entity": map[string]any{"type": app.EntityType, "id": id(40), "expected_revision": nil},
		"occurred_at": "2026-11-14T12:00:00.000000123Z", "entry_zone": "America/Cuiaba", "device_offset_seconds": -14400, "depends_on": []any{},
		"soft_references": []any{}, "attachments": []any{}, "payload": body}
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

func start(t *testing.T, change func(envelope, payload map[string]any)) protocol.Operation {
	body := map[string]any{"asset_id": id(10), "driver_employee_id": id(30), "reading": reading("184320000"), "missing_reading_reason": nil, "run_clock": nil}
	return operation(t, app.StartOperationType, body, change)
}

func finish(t *testing.T, change func(envelope, payload map[string]any)) protocol.Operation {
	body := map[string]any{"reading": reading("184610000"), "missing_reading_reason": nil, "run_clock": map[string]any{"boot_id": "boot-a1", "elapsed_millis": "10800000"}}
	return operation(t, app.FinishOperationType, body, func(e, p map[string]any) {
		p["reading"].(map[string]any)["reading_id"] = id(14)
		if change != nil {
			change(e, p)
		}
	})
}

// B-BR-EMP-312: the envelope owns identity, author and declaration; the
// payload carries the asset, driver, one evidence form and an optional clock.
func TestParseStartKeepsTheExactEvent(t *testing.T) {
	op := start(t, nil)
	command, err := app.ParseStart(op)
	if err != nil {
		t.Fatal(err)
	}
	v, err := command.Snapshot()
	if err != nil || v.TripID != id(40) || v.AssetID != id(10) || v.DriverID != id(30) || v.AuthorMembershipID != id(3) || v.OperationID != op.ID() ||
		v.PayloadDigest != op.PayloadDigest() || v.Reading == nil || v.Reading.Value.Count() != 184320000 || v.MissingReason != nil || v.Clock != nil ||
		v.Declared.OccurredAt.Nanosecond()%1000 != 123 || v.Declared.EntryZone != "America/Cuiaba" || v.Declared.DeviceOffsetSeconds != -14400 {
		t.Fatal("parsed start lost its exact event", err)
	}
	v.Reading.MeterID = "changed"
	if again, _ := command.Snapshot(); again.Reading.MeterID != id(11) {
		t.Fatal("snapshot exposed the command")
	}
	if _, _, err := app.MeterCommand(v.Event, v.AssetID, meter.TripStart); err == nil {
		t.Fatal("a changed meter identity produced a reading command")
	}
	again, _ := command.Snapshot()
	reading, ok, err := app.MeterCommand(again.Event, again.AssetID, meter.TripStart)
	in, _ := reading.Snapshot()
	if err != nil || !ok || in.Origin != meter.TripStart || in.Method != meter.Typed || in.OperationID != op.ID() || in.AssetID != id(10) {
		t.Fatal("departure reading is not a TRIP_START fleet reading", err)
	}
	absent, err := app.ParseStart(start(t, func(_, p map[string]any) { p["reading"], p["missing_reading_reason"] = nil, "Painel apagado" }))
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := absent.Snapshot(); v.Reading != nil || v.MissingReason == nil || *v.MissingReason != "Painel apagado" {
		t.Fatal("missing reason form")
	}
	if _, ok, err := app.MeterCommand(func() app.Event { v, _ := absent.Snapshot(); return v.Event }(), id(10), meter.TripStart); ok || err != nil {
		t.Fatal("an absence produced a reading")
	}
	if _, err := (app.StartCommand{}).Snapshot(); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("zero command readable")
	}
}

func TestParseFinishKeepsTheExactEventAndClock(t *testing.T) {
	op := finish(t, func(e, _ map[string]any) { e["depends_on"] = []any{id(21)} })
	command, err := app.ParseFinish(op)
	if err != nil {
		t.Fatal(err)
	}
	v, err := command.Snapshot()
	if err != nil || v.TripID != id(40) || v.Reading == nil || v.Reading.ReadingID != id(14) || v.Clock == nil || v.Clock.BootID() != "boot-a1" || v.Clock.ElapsedMillis() != 10800000 ||
		len(v.DependsOn) != 1 || v.DependsOn[0] != id(21) {
		t.Fatal("parsed finish lost its exact event", err)
	}
	v.DependsOn[0] = "changed"
	if again, _ := command.Snapshot(); again.DependsOn[0] != id(21) {
		t.Fatal("snapshot exposed the dependency list")
	}
	if _, err := (app.FinishCommand{}).Snapshot(); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("zero command readable")
	}
}

// B-BR-EMP-312/314/315: malformed forms are final; recognised future forms
// stay unavailable; nothing is guessed or defaulted.
func TestParseRefusesMalformedAndKeepsUnsupportedForms(t *testing.T) {
	malformedStart := map[string]func(e, p map[string]any){
		"wrong-type": func(e, _ map[string]any) { e["type"] = "trip.begin" },
		"wrong-entity": func(e, _ map[string]any) {
			e["entity"] = map[string]any{"type": "fueling", "id": id(40), "expected_revision": nil}
		},
		"expected-revision": func(e, _ map[string]any) {
			e["entity"] = map[string]any{"type": app.EntityType, "id": id(40), "expected_revision": 1}
		},
		"unknown-key":       func(_, p map[string]any) { p["destination"] = "x" },
		"missing-key":       func(_, p map[string]any) { delete(p, "run_clock") },
		"no-evidence":       func(_, p map[string]any) { p["reading"] = nil },
		"both-evidence":     func(_, p map[string]any) { p["missing_reading_reason"] = "Painel apagado" },
		"blank-reason":      func(_, p map[string]any) { p["reading"], p["missing_reading_reason"] = nil, "   " },
		"long-reason":       func(_, p map[string]any) { p["reading"], p["missing_reading_reason"] = nil, strings.Repeat("a", 281) },
		"uppercase-asset":   func(_, p map[string]any) { p["asset_id"] = strings.ToUpper(id(10)) },
		"numeric-value":     func(_, p map[string]any) { p["reading"].(map[string]any)["value"] = 184320000 },
		"decimal-value":     func(_, p map[string]any) { p["reading"].(map[string]any)["value"] = "184320.5" },
		"unknown-kind":      func(_, p map[string]any) { p["reading"].(map[string]any)["kind"] = "GPS" },
		"reading-extra-key": func(_, p map[string]any) { p["reading"].(map[string]any)["photo"] = nil },
		"clock-bad-boot":    func(_, p map[string]any) { p["run_clock"] = map[string]any{"boot_id": "boot 1", "elapsed_millis": "1"} },
		"clock-negative": func(_, p map[string]any) {
			p["run_clock"] = map[string]any{"boot_id": "boot-a1", "elapsed_millis": "-1"}
		},
		"clock-leading-zero": func(_, p map[string]any) {
			p["run_clock"] = map[string]any{"boot_id": "boot-a1", "elapsed_millis": "01"}
		},
		"clock-number":         func(_, p map[string]any) { p["run_clock"] = map[string]any{"boot_id": "boot-a1", "elapsed_millis": 1} },
		"clock-missing-key":    func(_, p map[string]any) { p["run_clock"] = map[string]any{"boot_id": "boot-a1"} },
		"null-driver":          func(_, p map[string]any) { p["driver_employee_id"] = nil },
		"same-asset-unchecked": func(_, p map[string]any) { p["asset_id"] = "not-a-uuid" },
	}
	for name, change := range malformedStart {
		t.Run("start-"+name, func(t *testing.T) {
			if _, err := app.ParseStart(start(t, change)); !errors.Is(err, app.ErrMalformed) {
				t.Fatal("malformed start accepted", err)
			}
		})
	}
	for name, change := range map[string]func(e, p map[string]any){
		"dependency": func(e, _ map[string]any) { e["depends_on"] = []any{id(21)} },
		"soft": func(e, _ map[string]any) {
			e["soft_references"] = []any{map[string]any{"type": "mission", "id": id(22)}}
		},
	} {
		t.Run("start-unsupported-"+name, func(t *testing.T) {
			if _, err := app.ParseStart(start(t, change)); !errors.Is(err, app.ErrUnsupported) {
				t.Fatal("recognised future start form not kept unavailable", err)
			}
		})
	}
	for name, change := range map[string]func(e, p map[string]any){
		"two-dependencies": func(e, _ map[string]any) { e["depends_on"] = []any{id(21), id(22)} },
		"asset-key":        func(_, p map[string]any) { p["asset_id"] = id(10) },
		"no-evidence":      func(_, p map[string]any) { p["reading"] = nil },
		"start-type":       func(e, _ map[string]any) { e["type"] = app.StartOperationType },
	} {
		t.Run("finish-"+name, func(t *testing.T) {
			if _, err := app.ParseFinish(finish(t, change)); !errors.Is(err, app.ErrMalformed) {
				t.Fatal("malformed finish accepted", err)
			}
		})
	}
	if _, err := app.ParseFinish(finish(t, func(e, _ map[string]any) {
		e["soft_references"] = []any{map[string]any{"type": "mission", "id": id(22)}}
	})); !errors.Is(err, app.ErrUnsupported) {
		t.Fatal("recognised future finish form not kept unavailable", err)
	}
}

// The stored trip is rebuilt through the pure domain at receipt: the starting
// member finishes; another member is FORBIDDEN; an earlier instant refused.
func TestFinishDomainAppliesTheTripRulesAtReceipt(t *testing.T) {
	org, _ := kernel.ParseOrgID(id(65))
	scope, _ := kernel.NewOrgScope(org)
	at, _ := orgtime.ParseInstant("2026-11-14T12:00:00Z")
	value := int64(184320000)
	record := app.Record{ID: id(40), OperationID: id(19), AssetID: id(10), DriverID: id(30), AuthorMembershipID: id(3), MeterID: id(11), SegmentID: id(12), ReadingID: id(13),
		ReadingKind: "ODOMETER", ReadingValue: &value, Declared: orgtime.Declaration{OccurredAt: at, EntryZone: "America/Cuiaba", DeviceOffsetSeconds: -14400},
		Reporting: orgtime.ReportingAttribution{Day: "2026-11-14", Zone: "America/Cuiaba", Source: "unit"}, State: trip.InProgress}
	parse := func(change func(e, p map[string]any)) app.FinishInput {
		command, err := app.ParseFinish(finish(t, change))
		if err != nil {
			t.Fatal(err)
		}
		v, _ := command.Snapshot()
		return v
	}
	done, err := app.FinishDomain(scope, record, parse(func(e, _ map[string]any) { e["occurred_at"] = "2026-11-15T04:10:00Z" }))
	if err != nil || done.State() != trip.Completed || done.Crossing().EndDay() != "2026-11-15" || !done.Crossing().CrossesMidnight() {
		t.Fatal("the starting member could not finish across midnight", err)
	}
	if delta, ok := done.MeterUsage().Delta(); !ok || delta != 290000 {
		t.Fatal("VAL-01 usage at receipt", delta, ok)
	}
	other := record
	other.AuthorMembershipID = id(5)
	if _, err := app.FinishDomain(scope, other, parse(nil)); !errors.Is(err, trip.ErrForbidden) {
		t.Fatal("another member finished the trip", err)
	}
	if _, err := app.FinishDomain(scope, record, parse(func(e, _ map[string]any) { e["occurred_at"] = "2026-11-14T11:59:59Z" })); !errors.Is(err, trip.ErrFinishBeforeStart) {
		t.Fatal("a finish before the start was accepted", err)
	}
	if _, err := app.FinishDomain(scope, record, parse(func(_, p map[string]any) { p["reading"].(map[string]any)["reading_id"] = id(13) })); !errors.Is(err, trip.ErrInvalid) {
		t.Fatal("the departure reading identity was reused", err)
	}
	broken := record
	broken.ReadingKind = ""
	if _, err := app.FinishDomain(scope, broken, parse(nil)); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("an incomplete stored record was trusted", err)
	}
}

func TestPoliciesAreTheTripPermissions(t *testing.T) {
	if p := app.Policy(app.RecordAction); p.Permission != "trip.trip.start" || p.Validate() != nil {
		t.Fatal("record policy", p)
	}
	if p := app.Policy(app.ReadAction); p.Permission != "trip.trip.read" || p.Validate() != nil {
		t.Fatal("read policy", p)
	}
	if p := app.Policy(0); p.Permission != "" {
		t.Fatal("unknown action has a policy")
	}
}
