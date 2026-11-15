package application_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
)

// lifecycleOp is one lifecycle operation of the codec fixture on trip id(40).
func lifecycleOp(t *testing.T, kind string, expected any, body map[string]any, change func(envelope, payload map[string]any)) protocol.Operation {
	t.Helper()
	return operation(t, kind, body, func(e, p map[string]any) {
		e["entity"] = map[string]any{"type": app.EntityType, "id": id(40), "expected_revision": expected}
		if change != nil {
			change(e, p)
		}
	})
}

// B-BR-EMP-391: the three lifecycle codecs, their exact payload and refusals.
func TestParseTransitionAcceptsTheExactLifecyclePayloads(t *testing.T) {
	for kind, cmd := range map[string]trip.Command{app.PauseOperationType: trip.CommandPause, app.ResumeOperationType: trip.CommandResume, app.CancelOperationType: trip.CommandCancel} {
		op := lifecycleOp(t, kind, 3, map[string]any{"reason": "Cliente cancelou"}, nil)
		command, err := app.ParseTransition(op)
		if err != nil {
			t.Fatal(kind, err)
		}
		v, err := command.Snapshot()
		if err != nil || v.Command != cmd || v.Expected != 3 || v.Reason == nil || *v.Reason != "Cliente cancelou" || v.TripID != id(40) || v.OperationID != op.ID() ||
			v.AuthorMembershipID != id(3) || v.PayloadDigest != op.PayloadDigest() {
			t.Fatal("parsed lifecycle command differs", kind)
		}
		*v.Reason = "changed"
		if again, _ := command.Snapshot(); *again.Reason != "Cliente cancelou" {
			t.Fatal("snapshot is not a copy")
		}
		if got, ok := app.LifecycleCommandOf(kind); !ok || got != cmd {
			t.Fatal("lifecycle command of", kind)
		}
	}
	for _, kind := range []string{app.PauseOperationType, app.ResumeOperationType} {
		if _, err := app.ParseTransition(lifecycleOp(t, kind, 1, map[string]any{"reason": nil}, nil)); err != nil {
			t.Fatal("a pause or resume without reason refused", kind, err)
		}
	}
	if _, err := (app.TransitionCommand{}).Snapshot(); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("zero command produced a snapshot")
	}
	if _, ok := app.LifecycleCommandOf(app.FinishOperationType); ok {
		t.Fatal("finish is not a lifecycle operation")
	}
}

func TestParseTransitionRefusesWrongShapes(t *testing.T) {
	none := func() map[string]any { return map[string]any{"reason": nil} }
	for name, op := range map[string]protocol.Operation{
		"cancel-without-reason": lifecycleOp(t, app.CancelOperationType, 1, none(), nil),
		"blank-reason":          lifecycleOp(t, app.PauseOperationType, 1, map[string]any{"reason": "  "}, nil),
		"control-reason":        lifecycleOp(t, app.PauseOperationType, 1, map[string]any{"reason": "a\nb"}, nil),
		"number-reason":         lifecycleOp(t, app.PauseOperationType, 1, map[string]any{"reason": 7}, nil),
		"missing-revision":      lifecycleOp(t, app.PauseOperationType, nil, none(), nil),
		"revision-above-bound":  lifecycleOp(t, app.PauseOperationType, app.MaxTripRevision+1, none(), nil),
		"missing-reason-key":    lifecycleOp(t, app.PauseOperationType, 1, map[string]any{}, nil),
		"extra-key":             lifecycleOp(t, app.PauseOperationType, 1, map[string]any{"reason": nil, "state": "PAUSED"}, nil),
		"other-entity": lifecycleOp(t, app.PauseOperationType, 1, none(), func(e, _ map[string]any) {
			e["entity"] = map[string]any{"type": app.EventEntityType, "id": id(40), "expected_revision": 1}
		}),
		"two-dependencies": lifecycleOp(t, app.PauseOperationType, 1, none(), func(e, _ map[string]any) { e["depends_on"] = []any{id(21), id(22)} }),
		"other-type":       lifecycleOp(t, app.FinishOperationType, 1, none(), nil),
	} {
		if _, err := app.ParseTransition(op); !errors.Is(err, app.ErrMalformed) {
			t.Fatal("malformed lifecycle command accepted", name, err)
		}
	}
	attached := lifecycleOp(t, app.PauseOperationType, 1, none(), func(e, _ map[string]any) {
		e["soft_references"] = []any{map[string]any{"type": "trip", "id": id(41)}}
	})
	if _, err := app.ParseTransition(attached); err == nil {
		t.Fatal("a soft reference was accepted")
	}
}

// A stored record replays only the exact rows the domain would produce, and
// the command at receipt names its expected revision (B-BR-EMP-390/393).
func TestStoredLifecycleRowsReplayThroughTheDomain(t *testing.T) {
	org, _ := kernel.ParseOrgID(id(65))
	scope, _ := kernel.NewOrgScope(org)
	at, _ := orgtime.ParseInstant("2026-11-14T12:00:00Z")
	value := int64(184320000)
	record := app.Record{ID: id(40), OperationID: id(19), AssetID: id(10), DriverID: id(30), AuthorMembershipID: id(3), MeterID: id(11), SegmentID: id(12), ReadingID: id(13),
		ReadingKind: "ODOMETER", ReadingValue: &value, Declared: orgtime.Declaration{OccurredAt: at, EntryZone: "America/Cuiaba", DeviceOffsetSeconds: -14400},
		Reporting: orgtime.ReportingAttribution{Day: "2026-11-14", Zone: "America/Cuiaba", Source: "unit"}, State: trip.InProgress}
	if record.Revision() != 1 {
		t.Fatal("a started trip is revision 1")
	}
	declared := func(d time.Duration) orgtime.Declaration {
		return orgtime.Declaration{OccurredAt: at.Add(d), EntryZone: "America/Cuiaba", DeviceOffsetSeconds: -14400}
	}
	reason := "Almoço"
	record.Transitions = []app.TransitionRecord{{Revision: 2, Command: trip.CommandPause, From: trip.InProgress, To: trip.Paused, Reason: &reason,
		OperationID: id(21), AuthorMembershipID: id(3), Declared: declared(time.Minute)}}
	record.State = trip.Paused
	stored, err := app.StoredDomain(scope, record)
	if err != nil || stored.State() != trip.Paused || stored.Revision() != 2 || record.Revision() != 2 {
		t.Fatal("a paused record did not replay", err)
	}
	for name, forge := range map[string]func(*app.TransitionRecord){
		"next-state":  func(x *app.TransitionRecord) { x.To = trip.Cancelled },
		"revision":    func(x *app.TransitionRecord) { x.Revision = 3 },
		"author":      func(x *app.TransitionRecord) { x.AuthorMembershipID = id(5) },
		"before-trip": func(x *app.TransitionRecord) { x.Declared = declared(-time.Minute) },
	} {
		forged := record
		forged.Transitions = []app.TransitionRecord{record.Transitions[0]}
		forge(&forged.Transitions[0])
		if _, err := app.StoredDomain(scope, forged); !errors.Is(err, app.ErrUnavailable) {
			t.Fatal("a row the domain would not produce replayed", name, err)
		}
	}
	in := app.TransitionInput{TripID: id(40), AuthorMembershipID: id(3), Command: trip.CommandResume, Expected: 2, Declared: declared(2 * time.Minute)}
	resumed, err := app.TransitionDomain(scope, record, in)
	if err != nil || resumed.State() != trip.InProgress || resumed.Revision() != 3 {
		t.Fatal("resume at receipt", err)
	}
	stale := in
	stale.Expected = 1
	if _, err := app.TransitionDomain(scope, record, stale); !errors.Is(err, trip.ErrConflict) {
		t.Fatal("a stale revision was applied", err)
	}
	// A finish of the paused trip completes it at the next revision.
	command, err := app.ParseFinish(finish(t, func(e, _ map[string]any) { e["occurred_at"] = "2026-11-14T15:00:00Z" }))
	if err != nil {
		t.Fatal(err)
	}
	v, _ := command.Snapshot()
	done, err := app.FinishDomain(scope, record, v)
	if err != nil || done.State() != trip.Completed || done.Revision() != 3 {
		t.Fatal("a paused trip could not be finished", err)
	}
	// A cancelled record is finished from its open state, so a late finish is
	// recognised (and held) rather than refused by the domain.
	cancel := "Cliente cancelou"
	cancelled := record
	cancelled.Transitions = append(append([]app.TransitionRecord(nil), record.Transitions...), app.TransitionRecord{Revision: 3, Command: trip.CommandCancel,
		From: trip.Paused, To: trip.Cancelled, Reason: &cancel, OperationID: id(22), AuthorMembershipID: id(3), Declared: declared(3 * time.Minute)})
	cancelled.State = trip.Cancelled
	if s, err := app.StoredDomain(scope, cancelled); err != nil || s.State() != trip.Cancelled || s.Revision() != 3 {
		t.Fatal("a cancelled record did not replay", err)
	}
	if open, err := app.FinishDomain(scope, cancelled, v); err != nil || open.Revision() != 3 {
		t.Fatal("the open state of a cancelled record", err)
	}
	raw, _ := json.Marshal(in)
	if string(raw) != `"EnterpriseTripIntake(redacted)"` {
		t.Fatal("lifecycle input printed private values", string(raw))
	}
}
