package application_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
)

func event(t *testing.T, kind string, text any, change func(envelope, payload map[string]any)) protocol.Operation {
	t.Helper()
	body := map[string]any{"trip_id": id(40), "type": kind, "text": text}
	return operation(t, app.EventOperationType, body, func(e, p map[string]any) {
		e["entity"] = map[string]any{"type": app.EventEntityType, "id": id(50), "expected_revision": nil}
		e["occurred_at"] = "2026-11-14T13:00:00.000000123Z"
		if change != nil {
			change(e, p)
		}
	})
}

// B-BR-EMP-321: the envelope owns identity, author and declaration; the
// payload carries the trip, the closed type and an optional text.
func TestParseEventKeepsTheExactFact(t *testing.T) {
	op := event(t, "ARRIVAL", "Cliente Norte, doca 3", nil)
	command, err := app.ParseEvent(op)
	if err != nil {
		t.Fatal(err)
	}
	v, err := command.Snapshot()
	if err != nil || v.EventID != id(50) || v.TripID != id(40) || v.Type != trip.LogbookArrival || v.Text == nil || *v.Text != "Cliente Norte, doca 3" ||
		v.AuthorMembershipID != id(3) || v.OperationID != op.ID() || v.PayloadDigest != op.PayloadDigest() || v.Declared.OccurredAt.Nanosecond()%1000 != 123 ||
		v.Declared.EntryZone != "America/Cuiaba" || v.Declared.DeviceOffsetSeconds != -14400 || len(v.DependsOn) != 0 {
		t.Fatal("parsed event lost its exact fact", err)
	}
	*v.Text = "changed"
	if again, _ := command.Snapshot(); *again.Text != "Cliente Norte, doca 3" {
		t.Fatal("snapshot shares its text")
	}
	for _, kind := range []string{"STOP", "ARRIVAL", "DEPARTURE"} {
		command, err := app.ParseEvent(event(t, kind, nil, func(e, _ map[string]any) { e["depends_on"] = []any{id(19)} }))
		v, _ := command.Snapshot()
		if err != nil || v.Text != nil || len(v.DependsOn) != 1 || v.DependsOn[0] != id(19) {
			t.Fatal("a supported type without text or with one dependency", kind, err)
		}
	}
	if _, err := (app.EventCommand{}).Snapshot(); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("zero command", err)
	}
}

// B-BR-EMP-320/323: unknown types and malformed shapes are final; reserved
// types, soft references and attachments stay recognised but unavailable.
func TestParseEventRefusesMalformedAndKeepsReservedTypesUnavailable(t *testing.T) {
	for name, op := range map[string]protocol.Operation{
		"unknown-lowercase": event(t, "note", "Texto", nil),
		"unknown-start":     event(t, "START", nil, nil),
		"unknown-finish":    event(t, "FINISH", nil, nil),
		"unknown-padded":    event(t, "NOTE ", "Texto", nil),
		"note-no-text":      event(t, "NOTE", nil, nil),
		"note-blank":        event(t, "NOTE", "   ", nil),
		"text-control":      event(t, "STOP", "a\tb", nil),
		"text-too-long":     event(t, "NOTE", strings.Repeat("a", 281), nil),
		"text-not-string":   event(t, "NOTE", 1, nil),
		"type-not-string":   event(t, "NOTE", "x", func(_, p map[string]any) { p["type"] = nil }),
		"extra-key":         event(t, "NOTE", "x", func(_, p map[string]any) { p["asset_id"] = id(10) }),
		"missing-text-key":  event(t, "STOP", nil, func(_, p map[string]any) { delete(p, "text") }),
		"trip-not-uuid":     event(t, "STOP", nil, func(_, p map[string]any) { p["trip_id"] = "trip" }),
		"trip-uppercase":    event(t, "STOP", nil, func(_, p map[string]any) { p["trip_id"] = strings.ToUpper(id(40)) }),
		"entity-trip":       event(t, "STOP", nil, func(e, _ map[string]any) { e["entity"].(map[string]any)["type"] = app.EntityType }),
		"two-dependencies":  event(t, "STOP", nil, func(e, _ map[string]any) { e["depends_on"] = []any{id(19), id(18)} }),
		"start-operation":   operation(t, app.StartOperationType, map[string]any{"trip_id": id(40), "type": "NOTE", "text": "x"}, nil),
		// EDR-240: incident and damage facts are check.incident.report.
		"incident-own-operation": event(t, "INCIDENT", nil, nil),
		"damage-own-operation":   event(t, "DAMAGE", "Lataria", nil),
	} {
		if _, err := app.ParseEvent(op); !errors.Is(err, app.ErrMalformed) {
			t.Fatal("malformed event accepted", name, err)
		}
	}
	for _, kind := range []string{"FUELING", "TOLL", "PARKING", "HANDOVER", "PAUSE", "RESUME", "CANCEL"} {
		if _, err := app.ParseEvent(event(t, kind, nil, nil)); !errors.Is(err, app.ErrUnsupported) {
			t.Fatal("reserved type not kept unavailable", kind, err)
		}
	}
	if _, err := app.ParseEvent(event(t, "STOP", nil, func(e, _ map[string]any) {
		e["soft_references"] = []any{map[string]any{"type": "fueling", "id": id(60)}}
	})); !errors.Is(err, app.ErrUnsupported) {
		t.Fatal("recognised future event form not kept unavailable", err)
	}
}

// B-BR-EMP-323: the stored trip is rebuilt through the pure domain at
// receipt, with its finish when completed; the event never changes it.
func TestEventDomainAppliesTheLogbookRulesAtReceipt(t *testing.T) {
	org, _ := kernel.ParseOrgID(id(65))
	scope, _ := kernel.NewOrgScope(org)
	at, _ := orgtime.ParseInstant("2026-11-14T12:00:00Z")
	end, _ := orgtime.ParseInstant("2026-11-14T15:00:00Z")
	start, final := int64(184320000), int64(184610000)
	record := app.Record{ID: id(40), OperationID: id(19), AssetID: id(10), DriverID: id(30), AuthorMembershipID: id(3), MeterID: id(11), SegmentID: id(12), ReadingID: id(13),
		ReadingKind: "ODOMETER", ReadingValue: &start, Declared: orgtime.Declaration{OccurredAt: at, EntryZone: "America/Cuiaba", DeviceOffsetSeconds: -14400},
		Reporting: orgtime.ReportingAttribution{Day: "2026-11-14", Zone: "America/Cuiaba", Source: "unit"}, State: trip.InProgress}
	completed := record
	completed.State = trip.Completed
	completed.Finish = &app.FinishRecord{OperationID: id(21), AuthorMembershipID: id(3), Declared: orgtime.Declaration{OccurredAt: end, EntryZone: "America/Cuiaba", DeviceOffsetSeconds: -14400},
		MeterID: id(11), SegmentID: id(12), ReadingID: id(14), ReadingKind: "ODOMETER", ReadingValue: &final, EndDay: "2026-11-14"}
	parse := func(change func(e, p map[string]any)) app.EventInput {
		command, err := app.ParseEvent(event(t, "NOTE", "Cliente ausente", change))
		if err != nil {
			t.Fatal(err)
		}
		v, _ := command.Snapshot()
		return v
	}
	at13 := parse(nil)
	if entry, err := app.EventDomain(scope, record, at13); err != nil || entry.Type() != trip.LogbookNote || entry.Reference().ID() != id(50) || entry.Trip().ID() != id(40) {
		t.Fatal("event inside an in-progress trip", err)
	}
	if _, err := app.EventDomain(scope, completed, at13); err != nil {
		t.Fatal("late event inside a completed trip", err)
	}
	if _, err := app.EventDomain(scope, completed, parse(func(e, _ map[string]any) { e["occurred_at"] = "2026-11-14T15:00:00Z" })); err != nil {
		t.Fatal("event at the finish instant", err)
	}
	if _, err := app.EventDomain(scope, completed, parse(func(e, _ map[string]any) { e["occurred_at"] = "2026-11-14T15:00:00.000000001Z" })); !errors.Is(err, trip.ErrEventAfterFinish) {
		t.Fatal("event after the finish", err)
	}
	if _, err := app.EventDomain(scope, record, parse(func(e, _ map[string]any) { e["occurred_at"] = "2026-11-14T11:59:59Z" })); !errors.Is(err, trip.ErrEventBeforeStart) {
		t.Fatal("event before the start", err)
	}
	other := record
	other.AuthorMembershipID = id(5)
	if _, err := app.EventDomain(scope, other, at13); !errors.Is(err, trip.ErrForbidden) {
		t.Fatal("another member logged on the trip", err)
	}
	broken := completed
	finish := *completed.Finish
	finish.EndDay = "2026-11-15"
	broken.Finish = &finish
	if _, err := app.EventDomain(scope, broken, at13); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("an inconsistent stored finish was trusted", err)
	}
	missing := completed
	finishValue := *completed.Finish
	finishValue.ReadingValue = nil
	missing.Finish = &finishValue
	if _, err := app.EventDomain(scope, missing, at13); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("a stored finish without its reading value was trusted", err)
	}
}
