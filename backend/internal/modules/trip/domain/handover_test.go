package domain_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
)

func handoverTo(t *testing.T, tr trip.Trip, member, employee int, raw string) trip.HandoverInput {
	t.Helper()
	s := scope(t, 1)
	return trip.HandoverInput{Recorder: must(trip.NewMembershipRef(s, uuid(43))), Receiver: must(trip.NewMembershipRef(s, uuid(member))),
		ReceiverEmployee: must(trip.NewEmployeeRef(s, uuid(employee))), Expected: tr.Revision(), Reason: text("Troca de turno"),
		At: must(orgtime.ParseInstant(raw))}
}

// B-BR-EMP-395: a handover changes who acts next and nothing that happened.
func TestHandoverMovesFutureResponsibilityOnly(t *testing.T) {
	defer guard(t)
	s := scope(t, 1)
	started := must(trip.Start(s, startInput(t, s)))
	if len(started.Handovers()) != 0 || started.ResponsibleAt(time.Date(2026, 11, 14, 23, 0, 0, 0, time.UTC)) != started.Started().Author() {
		t.Fatal("a trip without a handover is the starter's at every instant")
	}
	handed := must(started.Handover(s, handoverTo(t, started, 42, 32, "2026-11-14T13:00:00Z")))
	if handed.Revision() != 2 || handed.State() != trip.InProgress || handed.Responsible().ID() != uuid(42) || handed.Started() != started.Started() ||
		handed.Driver() != started.Driver() || handed.Asset() != started.Asset() || handed.Reporting() != started.Reporting() {
		t.Fatal("a handover must keep the trip and move only its responsibility", handed.Revision(), handed.State())
	}
	facts := handed.Handovers()
	if len(facts) != 1 || facts[0].From() != started.Started().Author() || facts[0].To().ID() != uuid(42) || facts[0].ReceiverEmployee().ID() != uuid(32) ||
		facts[0].Recorder().ID() != uuid(43) || facts[0].Reason() != "Troca de turno" || facts[0].Revision() != 2 {
		t.Fatal("the handover fact is incomplete", facts)
	}
	facts[0] = trip.HandoverFact{}
	if handed.Handovers()[0].Revision() != 2 {
		t.Fatal("handover facts leak their backing array")
	}
	if started.Revision() != 1 || len(started.Handovers()) != 0 || started.Responsible() != started.Started().Author() {
		t.Fatal("a handover changed its input trip")
	}
	// The giving member's offline event before the handover instant is still its own.
	ref := must(trip.NewLogbookRef(s, uuid(81)))
	before := at(t, "2026-11-14T12:59:59Z")
	if _, e := handed.Log(s, trip.LogbookInput{Reference: ref, Author: started.Started().Author(), Type: "STOP", Declared: before}); e != nil {
		t.Fatal("the giving member's earlier fact was refused", e)
	}
	if _, e := handed.Log(s, trip.LogbookInput{Reference: ref, Author: started.Started().Author(), Type: "STOP", Declared: at(t, "2026-11-14T13:00:00Z")}); !errors.Is(e, trip.ErrForbidden) {
		t.Fatal("the giving member acted after the handover", e)
	}
	paused := must(handed.Pause(s, trip.LifecycleInput{Author: handed.Responsible(), Expected: 2, Declared: at(t, "2026-11-14T13:30:00Z")}))
	if paused.Revision() != 3 || paused.Responsible() != handed.Responsible() || len(paused.Handovers()) != 1 {
		t.Fatal("a later command lost the handover")
	}
}

func TestZeroTripAndHandoverInputsAreRefused(t *testing.T) {
	defer guard(t)
	s := scope(t, 1)
	started := must(trip.Start(s, startInput(t, s)))
	in := handoverTo(t, started, 42, 32, "2026-11-14T13:00:00Z")
	if _, e := (trip.Trip{}).Handover(s, in); !errors.Is(e, trip.ErrInvalid) {
		t.Fatal("a zero trip was handed over", e)
	}
	for name, edit := range map[string]func(*trip.HandoverInput){
		"recorder": func(v *trip.HandoverInput) { v.Recorder = trip.MembershipRef{} },
		"receiver": func(v *trip.HandoverInput) { v.Receiver = trip.MembershipRef{} },
		"employee": func(v *trip.HandoverInput) { v.ReceiverEmployee = trip.EmployeeRef{} },
		"instant":  func(v *trip.HandoverInput) { v.At = time.Time{} },
		"year":     func(v *trip.HandoverInput) { v.At = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC) },
	} {
		v := in
		edit(&v)
		if _, e := started.Handover(s, v); !errors.Is(e, trip.ErrInvalid) {
			t.Fatal("an absent handover part was admitted", name, e)
		}
	}
}

// Concurrent handovers of one immutable trip each see revision 1; the stored
// trip (EMP-TRIP-012 database CAS) keeps exactly one of them.
func TestConcurrentHandoversShareOneImmutableTrip(t *testing.T) {
	defer guard(t)
	s := scope(t, 1)
	started := must(trip.Start(s, startInput(t, s)))
	var wg sync.WaitGroup
	errs := make(chan error, 32)
	for i := range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			next, e := started.Handover(s, handoverTo(t, started, 42+i%2*2, 32+i%2*2, "2026-11-14T13:00:00Z"))
			if e == nil && (next.Revision() != 2 || len(next.Handovers()) != 1) {
				e = fmt.Errorf("revision %d", next.Revision())
			}
			errs <- e
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	if started.Revision() != 1 || len(started.Handovers()) != 0 {
		t.Fatal("a shared trip value changed")
	}
}

func TestHandoverValuesAreRedacted(t *testing.T) {
	defer guard(t)
	s := scope(t, 1)
	started := must(trip.Start(s, startInput(t, s)))
	in := handoverTo(t, started, 42, 32, "2026-11-14T13:00:00Z")
	in.Reason = text("Painel quebrado")
	handed := must(started.Handover(s, in))
	fact := handed.Handovers()[0]
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, untimedLog))
	for _, v := range []any{in, fact, handed} {
		for _, format := range []string{"%v", "%+v", "%#v", "%s", "%q"} {
			assertRedacted(t, fmt.Sprintf(format, v))
		}
		raw, e := json.Marshal(v)
		if e != nil {
			t.Fatal(e)
		}
		assertRedacted(t, string(raw))
		logger.Info("trip", "value", v)
	}
	assertRedacted(t, logs.String())
}

// EMP-TRIP-012 with EMP-TRIP-014: the member responsible after a handover
// never closes the trip for another; a third supervisor does, and a closed
// trip takes no handover.
func TestHandedOverTripIsClosedOnlyByAThirdMember(t *testing.T) {
	defer guard(t)
	s := scope(t, 1)
	started := must(trip.Start(s, startInput(t, s)))
	handed := must(started.Handover(s, handoverTo(t, started, 42, 32, "2026-11-14T13:00:00Z")))
	absence := must(meter.NewMissing("Sem leitura final"))
	if _, e := handed.Close(s, trip.CloseInput{Closer: handed.Responsible(), CloserEmployee: must(trip.NewEmployeeRef(s, uuid(32))), Absence: absence}); !errors.Is(e, trip.ErrSelfClosure) {
		t.Fatal("the receiver closed the trip it is responsible for", e)
	}
	closed := must(handed.Close(s, trip.CloseInput{Closer: must(trip.NewMembershipRef(s, uuid(44))), CloserEmployee: must(trip.NewEmployeeRef(s, uuid(34))), Absence: absence}))
	if closed.Revision() != 3 || closed.State() != trip.Completed || len(closed.Handovers()) != 1 {
		t.Fatal("a third member's closure of a handed-over trip", closed.Revision(), closed.State())
	}
	if _, e := closed.Handover(s, handoverTo(t, closed, 41, 31, "2026-11-14T14:00:00Z")); !errors.Is(e, trip.ErrInvalidTransition) {
		t.Fatal("a closed trip was handed over", e)
	}
}
