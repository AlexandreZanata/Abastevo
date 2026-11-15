package domain_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"testing"

	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
)

func lifecycle(t *testing.T, tr trip.Trip, expected int64, raw string, reason *string) trip.LifecycleInput {
	t.Helper()
	return trip.LifecycleInput{Author: tr.Responsible(), Expected: expected, Reason: reason, Declared: at(t, raw)}
}

func text(v string) *string { return &v }

// B-BR-EMP-390: one continuous trip; the logbook closes at the cancellation
// as it closes at a finish, and a paused trip still takes events.
func TestCancellationClosesTheLogbookAndPauseKeepsIt(t *testing.T) {
	defer guard(t)
	s := scope(t, 1)
	started := must(trip.Start(s, startInput(t, s)))
	if started.Revision() != 1 || started.Responsible() != started.Started().Author() || len(started.Lifecycle()) != 0 {
		t.Fatal("a started trip is revision 1, owned by its starting member, without lifecycle facts")
	}
	paused := must(started.Pause(s, lifecycle(t, started, 1, "2026-11-14T12:30:00Z", text("Almoço"))))
	event := func(tr trip.Trip, raw string) error {
		_, e := tr.Log(s, trip.LogbookInput{Reference: must(trip.NewLogbookRef(s, uuid(81))), Author: tr.Responsible(), Type: "STOP", Declared: at(t, raw)})
		return e
	}
	if e := event(paused, "2026-11-14T12:40:00Z"); e != nil {
		t.Fatal("a paused trip refused an event", e)
	}
	cancelled := must(paused.Cancel(s, lifecycle(t, paused, 2, "2026-11-14T13:00:00Z", text("Cliente cancelou"))))
	if e := event(cancelled, "2026-11-14T13:00:00Z"); e != nil {
		t.Fatal("an event at the cancellation instant is a late delivery inside the trip", e)
	}
	if e := event(cancelled, "2026-11-14T13:00:00.000000001Z"); !errors.Is(e, trip.ErrEventAfterFinish) {
		t.Fatal("an event after the cancellation was accepted", e)
	}
	fact, ok := cancelled.Cancellation()
	if !ok || fact.Command() != trip.CommandCancel || fact.Revision() != 3 || fact.From() != trip.Paused || fact.To() != trip.Cancelled || *fact.Reason() != "Cliente cancelou" {
		t.Fatal("the cancellation fact is not the applied command")
	}
	if _, ok := paused.Cancellation(); ok {
		t.Fatal("a paused trip reported a cancellation")
	}
	if _, finished := cancelled.Finished(); finished || cancelled.Crossing().Finished() || must(cancelled.Elapsed(0)).Status() != trip.ElapsedNotFinished {
		t.Fatal("a cancellation invented a finish or a duration")
	}
	if u := cancelled.MeterUsage(); u.Status() != trip.NotMeasured || u.Reason() != trip.ReasonCancelled {
		t.Fatal("a cancelled trip fabricated a distance")
	}
	facts := cancelled.Lifecycle()
	facts[0] = trip.LifecycleFact{}
	if cancelled.Lifecycle()[0].Command() != trip.CommandPause || len(cancelled.Lifecycle()) != 2 {
		t.Fatal("the lifecycle facts are not copies")
	}
	r := fact.Reason()
	*r = "changed"
	if *fact.Reason() != "Cliente cancelou" {
		t.Fatal("the reason is not a copy")
	}
}

// A zero trip, a zero author and an unknown command never build a trip.
func TestZeroTripAndInputsAreRefused(t *testing.T) {
	defer guard(t)
	s := scope(t, 1)
	started := must(trip.Start(s, startInput(t, s)))
	for _, run := range []func(trip.Trip, trip.LifecycleInput) (trip.Trip, error){
		func(tr trip.Trip, in trip.LifecycleInput) (trip.Trip, error) { return tr.Pause(s, in) },
		func(tr trip.Trip, in trip.LifecycleInput) (trip.Trip, error) { return tr.Resume(s, in) },
		func(tr trip.Trip, in trip.LifecycleInput) (trip.Trip, error) { return tr.Cancel(s, in) },
	} {
		if _, e := run(trip.Trip{}, lifecycle(t, started, 1, "2026-11-14T13:00:00Z", text("x"))); !errors.Is(e, trip.ErrInvalid) {
			t.Fatal("a zero trip was changed", e)
		}
		if _, e := run(started, trip.LifecycleInput{Expected: 1, Reason: text("x"), Declared: at(t, "2026-11-14T13:00:00Z")}); !errors.Is(e, trip.ErrInvalid) {
			t.Fatal("a zero author was admitted", e)
		}
	}
}

// Concurrent commands on one immutable trip each see revision 1.
func TestConcurrentCommandsShareOneImmutableTrip(t *testing.T) {
	defer guard(t)
	s := scope(t, 1)
	started := must(trip.Start(s, startInput(t, s)))
	var wg sync.WaitGroup
	errs := make(chan error, 32)
	for i := range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var next trip.Trip
			var e error
			if i%2 == 0 {
				next, e = started.Pause(s, lifecycle(t, started, 1, "2026-11-14T13:00:00Z", nil))
			} else {
				next, e = started.Cancel(s, lifecycle(t, started, 1, "2026-11-14T13:00:00Z", text("Cliente cancelou")))
			}
			if e == nil && next.Revision() != 2 {
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
	if started.State() != trip.InProgress || started.Revision() != 1 || len(started.Lifecycle()) != 0 {
		t.Fatal("a shared trip value changed")
	}
}

func TestLifecycleValuesAreRedacted(t *testing.T) {
	defer guard(t)
	s := scope(t, 1)
	started := must(trip.Start(s, startInput(t, s)))
	in := lifecycle(t, started, 1, "2026-11-14T13:00:00Z", text("Painel apagado"))
	cancelled := must(started.Cancel(s, in))
	fact, _ := cancelled.Cancellation()
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, untimedLog))
	for _, v := range []any{in, fact, cancelled} {
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
