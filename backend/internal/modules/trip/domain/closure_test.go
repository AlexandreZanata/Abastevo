package domain_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
)

const closureAbsence = "Leitura final nao registrada pelo motorista"

func closeInput(t *testing.T, s kernel.OrgScope) trip.CloseInput {
	t.Helper()
	return trip.CloseInput{Closer: must(trip.NewMembershipRef(s, uuid(81))), CloserEmployee: must(trip.NewEmployeeRef(s, uuid(82))), Absence: must(meter.NewMissing(closureAbsence))}
}

// EMP-TRIP-014 (B-BR-EMP-399/400/402, EDR-257): a supervisor closure
// completes the running trip with the closure and no finish; the usage is
// NOT_MEASURED with READING_ABSENT, never a distance; the receiver never
// changes; a later finish or second closure is INVALID_TRANSITION and every
// later logbook event is after the closure.
func TestTripCloseCompletesWithoutInventingEvidence(t *testing.T) {
	defer guard(t)
	s := scope(t, 1)
	started := must(trip.Start(s, startInput(t, s)))
	in := closeInput(t, s)
	closed, err := started.Close(s, in)
	if err != nil {
		t.Fatal("supervisor closure refused", err)
	}
	if closed.State() != trip.Completed || closed.Reference() != started.Reference() || closed.Asset() != started.Asset() || closed.Driver() != started.Driver() ||
		closed.Reporting() != started.Reporting() || closed.Started().Author() != started.Started().Author() {
		t.Fatal("closure changed the trip's start facts or did not complete it", closed.State())
	}
	if _, ok := closed.Finished(); ok {
		t.Fatal("a closure invented a finish")
	}
	c, ok := closed.Closed()
	if !ok || c.Closer() != in.Closer || c.CloserEmployee() != in.CloserEmployee || c.Absence().Reason() != closureAbsence {
		t.Fatal("closure facts not kept", ok)
	}
	usage := closed.MeterUsage()
	if delta, measured := usage.Delta(); usage.Status() != trip.NotMeasured || usage.Reason() != trip.ReasonReadingAbsent || measured || delta != 0 || usage.Kind() != "" {
		t.Fatal("a closed trip reported a distance", usage.Status(), usage.Reason(), delta)
	}
	if started.State() != trip.InProgress {
		t.Fatal("closure changed its receiver")
	}
	if _, ok := started.Closed(); ok {
		t.Fatal("the running trip reports a closure")
	}
	if u := started.MeterUsage(); u.Reason() != trip.ReasonNotFinished {
		t.Fatal("running trip usage", u.Reason())
	}
	if _, err := closed.Finish(s, finishInput(t, s, started)); !errors.Is(err, trip.ErrInvalidTransition) {
		t.Fatal("a closed trip accepted the driver's finish", err)
	}
	if _, err := closed.Close(s, in); !errors.Is(err, trip.ErrInvalidTransition) {
		t.Fatal("a second closure was not refused", err)
	}
	log := func(raw string) error {
		_, err := closed.Log(s, trip.LogbookInput{Reference: must(trip.NewLogbookRef(s, uuid(91))), Author: started.Started().Author(), Type: "STOP", Declared: at(t, raw)})
		return err
	}
	for _, raw := range []string{"2026-11-14T12:00:00Z", "2026-11-14T12:30:00Z", "2026-11-20T08:00:00Z"} {
		if err := log(raw); !errors.Is(err, trip.ErrEventAfterFinish) {
			t.Fatal("a logbook event of a closed trip was not after the closure", raw, err)
		}
	}
	if err := log("2026-11-14T11:59:59Z"); !errors.Is(err, trip.ErrEventBeforeStart) {
		t.Fatal("an event before the start is not refused first", err)
	}
	// A finished trip keeps its own evidence and has no closure.
	finished := must(started.Finish(s, finishInput(t, s, started)))
	if _, ok := finished.Closed(); ok {
		t.Fatal("a finished trip reports a closure")
	}
	if delta, ok := finished.MeterUsage().Delta(); !ok || delta != 290_000 {
		t.Fatal("finished usage", delta)
	}
}

// B-BR-EMP-401 and the EDR-257 failure order: scope and trip, closer
// references, absence reason, self-closure, then state.
func TestTripCloseFailureOrder(t *testing.T) {
	defer guard(t)
	s, other := scope(t, 1), scope(t, 2)
	started := must(trip.Start(s, startInput(t, s)))
	finished := must(started.Finish(s, finishInput(t, s, started)))
	author := started.Started().Author()
	driver := started.Driver()
	edit := func(f func(*trip.CloseInput)) trip.CloseInput {
		in := closeInput(t, s)
		f(&in)
		return in
	}
	for _, c := range []struct {
		name  string
		scope kernel.OrgScope
		trip  trip.Trip
		in    trip.CloseInput
		want  error
	}{
		{"invalid scope", kernel.OrgScope{}, started, closeInput(t, s), trip.ErrScope},
		{"another organization", other, started, closeInput(t, other), trip.ErrScope},
		{"unconstructed trip", s, trip.Trip{}, closeInput(t, s), trip.ErrInvalid},
		{"absent closer", s, started, edit(func(in *trip.CloseInput) { in.Closer = trip.MembershipRef{} }), trip.ErrInvalid},
		{"foreign closer", s, started, edit(func(in *trip.CloseInput) { in.Closer = must(trip.NewMembershipRef(other, uuid(81))) }), trip.ErrScope},
		{"absent closer employee", s, started, edit(func(in *trip.CloseInput) { in.CloserEmployee = trip.EmployeeRef{} }), trip.ErrInvalid},
		{"foreign closer employee", s, started, edit(func(in *trip.CloseInput) { in.CloserEmployee = must(trip.NewEmployeeRef(other, uuid(82))) }), trip.ErrScope},
		{"absent missing-reading reason", s, started, edit(func(in *trip.CloseInput) { in.Absence = meter.Missing{} }), trip.ErrInvalid},
		{"the author closes", s, started, edit(func(in *trip.CloseInput) { in.Closer = author }), trip.ErrSelfClosure},
		{"the driver closes", s, started, edit(func(in *trip.CloseInput) { in.CloserEmployee = driver }), trip.ErrSelfClosure},
		{"self before state", s, finished, edit(func(in *trip.CloseInput) { in.Closer = author }), trip.ErrSelfClosure},
		{"finished trip", s, finished, closeInput(t, s), trip.ErrInvalidTransition},
	} {
		if _, err := c.trip.Close(c.scope, c.in); !errors.Is(err, c.want) {
			t.Fatalf("%s: got %v, want %v", c.name, err, c.want)
		}
	}
}

// Closure values are redacted on every formatting and serialization surface.
func TestTripClosureValuesAreRedacted(t *testing.T) {
	defer guard(t)
	s := scope(t, 1)
	in := closeInput(t, s)
	closed := must(must(trip.Start(s, startInput(t, s))).Close(s, in))
	c, _ := closed.Closed()
	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, &slog.HandlerOptions{ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
		if a.Key == slog.TimeKey {
			return slog.Attr{}
		}
		return a
	}}))
	for _, v := range []any{in, c, closed} {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		logger.Info("closure", "value", v)
		for _, surface := range []string{fmt.Sprint(v), fmt.Sprintf("%+v", v), fmt.Sprintf("%#v", v), string(raw), buffer.String()} {
			if strings.Contains(surface, closureAbsence) || strings.Contains(surface, uuid(81)) || strings.Contains(surface, uuid(82)) {
				t.Fatal("closure value leaked", surface)
			}
		}
	}
}
