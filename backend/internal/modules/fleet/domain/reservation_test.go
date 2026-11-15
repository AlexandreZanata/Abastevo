package domain_test

import (
	"errors"
	"testing"
	"time"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
)

var resBase = time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)

func resAt(minutes int) time.Time { return resBase.Add(time.Duration(minutes) * time.Minute) }

func resWindow(t *testing.T, from, to int) fleet.ReservationWindow {
	t.Helper()
	w, err := fleet.NewReservationWindow(resAt(from), resAt(to))
	if err != nil {
		t.Fatal(err)
	}
	return w
}

// fleet.md §6.4, every row (B-BR-EMP-294/296/297): commands from each stored
// state; FULFIL has no source before EMP-TRIP-023 and therefore no row.
func TestReservationTransitionTableIsExactlyFleetSection64(t *testing.T) {
	states := []fleet.ReservationState{fleet.ReservationRequested, fleet.ReservationConfirmed, fleet.ReservationFulfilled, fleet.ReservationCancelled, fleet.ReservationNoShow, fleet.ReservationExpired}
	want := map[fleet.ReservationCommand]map[fleet.ReservationState]fleet.ReservationState{
		fleet.ConfirmReservation: {fleet.ReservationRequested: fleet.ReservationConfirmed},
		fleet.CancelReservation:  {fleet.ReservationRequested: fleet.ReservationCancelled, fleet.ReservationConfirmed: fleet.ReservationCancelled},
		fleet.ExpireReservation:  {fleet.ReservationRequested: fleet.ReservationExpired},
		fleet.MissReservation:    {fleet.ReservationConfirmed: fleet.ReservationNoShow},
		fleet.FulfilReservation:  {},
	}
	cells := 0
	for command, rows := range want {
		for _, from := range states {
			got, err := fleet.ReservationTransition(from, command)
			to, ok := rows[from]
			if ok != (err == nil) || ok && got != to || !ok && !errors.Is(err, fleet.ErrTransition) {
				t.Fatalf("%s from %s: got %q %v", command, from, got, err)
			}
			cells++
		}
	}
	if cells != 30 {
		t.Fatal("incomplete transition table", cells)
	}
	for _, raw := range []string{"", "requested", "RESERVED", "MISSED", " CONFIRMED"} {
		if _, err := fleet.ParseReservationState(raw); !errors.Is(err, fleet.ErrInvalid) {
			t.Fatal("unknown state admitted", raw)
		}
	}
	for _, state := range states {
		if got, err := fleet.ParseReservationState(string(state)); err != nil || got != state {
			t.Fatal("closed state refused", state)
		}
	}
}

func TestReservationWindowAndNewReservationShape(t *testing.T) {
	if _, err := fleet.NewReservationWindow(resAt(10), resAt(10)); !errors.Is(err, fleet.ErrInvalid) {
		t.Fatal("empty window admitted")
	}
	if _, err := fleet.NewReservationWindow(resAt(10), resAt(5)); !errors.Is(err, fleet.ErrInvalid) {
		t.Fatal("reversed window admitted")
	}
	if _, err := fleet.NewReservationWindow(resAt(0).Add(time.Nanosecond), resAt(5)); !errors.Is(err, fleet.ErrInvalid) {
		t.Fatal("sub-microsecond bound admitted")
	}
	w := resWindow(t, 10, 70)
	if !w.Contains(resAt(10)) || w.Contains(resAt(70)) || w.Contains(resAt(9)) {
		t.Fatal("window is not half-open")
	}
	if err := fleet.ValidNewReservation(w, fleet.DefaultNoShowTolerance, resAt(0)); err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct {
		now       time.Time
		tolerance time.Duration
	}{
		"start equal to now": {resAt(10), fleet.DefaultNoShowTolerance},
		"start in the past":  {resAt(11), fleet.DefaultNoShowTolerance},
		"tolerance too low":  {resAt(0), 14 * time.Minute},
		"tolerance too high": {resAt(0), 241 * time.Minute},
		"fractional second":  {resAt(0), time.Hour + time.Millisecond},
		"zero now":           {time.Time{}, fleet.DefaultNoShowTolerance},
	} {
		if err := fleet.ValidNewReservation(w, c.tolerance, c.now); !errors.Is(err, fleet.ErrInvalid) {
			t.Fatal(name, err)
		}
	}
	if fleet.DefaultNoShowTolerance != time.Hour || fleet.MinNoShowTolerance != 15*time.Minute || fleet.MaxNoShowTolerance != 240*time.Minute {
		t.Fatal("tolerance defaults drifted from fleet.md §6.4")
	}
}

// BR-FLEET-030 at read time (AC3): a REQUESTED reservation past its start is
// EXPIRED and a CONFIRMED one past its deadline is NO_SHOW, with no job.
func TestReservationOutcomesFollowTheClockAtReadTime(t *testing.T) {
	requested := fleet.Reservation{State: fleet.ReservationRequested, Window: resWindow(t, 10, 130), Tolerance: time.Hour}
	confirmed := requested
	confirmed.State, confirmed.Confirmed = fleet.ReservationConfirmed, true
	short := confirmed
	short.Window = resWindow(t, 10, 40)
	for _, c := range []struct {
		name string
		r    fleet.Reservation
		now  int
		want fleet.ReservationState
	}{
		{"requested before start", requested, 9, fleet.ReservationRequested},
		{"requested at start", requested, 10, fleet.ReservationExpired},
		{"requested after end", requested, 200, fleet.ReservationExpired},
		{"confirmed inside tolerance", confirmed, 69, fleet.ReservationConfirmed},
		{"confirmed at deadline", confirmed, 70, fleet.ReservationNoShow},
		{"short window deadline is its end", short, 39, fleet.ReservationConfirmed},
		{"short window at end", short, 40, fleet.ReservationNoShow},
	} {
		if err := c.r.Validate(); err != nil {
			t.Fatal(c.name, err)
		}
		if got := c.r.StateAt(resAt(c.now)); got != c.want {
			t.Fatalf("%s: %s", c.name, got)
		}
	}
	if due, command, ok := requested.DueAt(); !ok || !due.Equal(resAt(10)) || command != fleet.ExpireReservation {
		t.Fatal("requested due")
	}
	if due, command, ok := confirmed.DueAt(); !ok || !due.Equal(resAt(70)) || command != fleet.MissReservation {
		t.Fatal("confirmed due")
	}
	if !short.NoShowDeadline().Equal(resAt(40)) || !confirmed.NoShowDeadline().Equal(resAt(70)) {
		t.Fatal("deadline is the earlier of start plus tolerance and end")
	}
	for _, terminal := range []fleet.ReservationState{fleet.ReservationExpired, fleet.ReservationCancelled} {
		r := requested
		r.State = terminal
		if terminal == fleet.ReservationCancelled {
			at := resAt(1)
			r.EndedAt = &at
		}
		if _, _, ok := r.DueAt(); ok || r.StateAt(resAt(500)) != terminal {
			t.Fatal("terminal state moved by time", terminal)
		}
	}
}

// BR-FLEET-031: the interval during which a reservation holds the asset.
func TestReservationHoldInterval(t *testing.T) {
	confirmed := fleet.Reservation{State: fleet.ReservationConfirmed, Window: resWindow(t, 10, 130), Tolerance: time.Hour, Confirmed: true}
	cancelledAt := resAt(30)
	cancelled := confirmed
	cancelled.State, cancelled.EndedAt = fleet.ReservationCancelled, &cancelledAt
	early := resAt(5)
	cancelledBefore := cancelled
	cancelledBefore.EndedAt = &early
	requested := fleet.Reservation{State: fleet.ReservationRequested, Window: resWindow(t, 10, 130), Tolerance: time.Hour}
	for _, c := range []struct {
		name    string
		r       fleet.Reservation
		at, now int
		want    bool
	}{
		{"confirmed before start", confirmed, 9, 9, false},
		{"confirmed at start", confirmed, 10, 10, true},
		{"confirmed before deadline", confirmed, 69, 69, true},
		{"confirmed after deadline is no-show", confirmed, 70, 70, false},
		{"no-show held earlier instants", confirmed, 50, 120, true},
		{"cancelled during window held before", cancelled, 20, 40, true},
		{"cancelled releases the window", cancelled, 30, 40, false},
		{"cancelled before start never held", cancelledBefore, 20, 40, false},
		{"requested never holds", requested, 9, 9, false},
	} {
		if err := c.r.Validate(); err != nil {
			t.Fatal(c.name, err)
		}
		if got := c.r.HoldsAt(resAt(c.at), resAt(c.now)); got != c.want {
			t.Fatalf("%s: %v", c.name, got)
		}
	}
	bad := confirmed
	bad.Confirmed = false
	if !errors.Is(bad.Validate(), fleet.ErrInvalid) {
		t.Fatal("confirmed without confirmation fact")
	}
	noEnd := cancelled
	noEnd.EndedAt = nil
	if !errors.Is(noEnd.Validate(), fleet.ErrInvalid) {
		t.Fatal("cancelled without end instant")
	}
}

// BR-FLEET-032 / B-BR-EMP-298: block reasons, creation shape and removal.
func TestBlockReasonsCreationAndRemoval(t *testing.T) {
	for _, reason := range []fleet.BlockReason{fleet.AdministrativeBlock, fleet.SafetyBlock, fleet.DocumentBlock, fleet.InvestigationBlock, fleet.LentOutBlock} {
		if got, err := fleet.ParseBlockReason(string(reason)); err != nil || got != reason {
			t.Fatal("closed reason refused", reason)
		}
	}
	for _, raw := range []string{"", "MAINTENANCE", "safety", "LENT"} {
		if _, err := fleet.ParseBlockReason(raw); !errors.Is(err, fleet.ErrInvalid) {
			t.Fatal("unknown reason admitted", raw)
		}
	}
	open, _ := fleet.NewAssignmentPeriod(resAt(10), nil)
	if err := fleet.ValidNewBlock(fleet.SafetyBlock, open, resAt(10)); err != nil {
		t.Fatal("block starting now refused", err)
	}
	if err := fleet.ValidNewBlock(fleet.SafetyBlock, open, resAt(11)); !errors.Is(err, fleet.ErrInvalid) {
		t.Fatal("retroactive block admitted")
	}
	if err := fleet.ValidNewBlock("OTHER", open, resAt(0)); !errors.Is(err, fleet.ErrInvalid) {
		t.Fatal("unknown reason block admitted")
	}
	removed, err := fleet.RemoveBlockAt(open, resAt(20).Add(1500*time.Nanosecond))
	if err != nil || removed.To == nil || !removed.To.Equal(resAt(20).Add(time.Microsecond)) {
		t.Fatal("removal floors to the microsecond", removed, err)
	}
	future, _ := fleet.NewAssignmentPeriod(resAt(30), nil)
	if removed, err := fleet.RemoveBlockAt(future, resAt(20)); err != nil || !removed.Empty() {
		t.Fatal("removing a block before its start leaves an empty period", err)
	}
	end := resAt(15)
	ended, _ := fleet.NewAssignmentPeriod(resAt(10), &end)
	if _, err := fleet.RemoveBlockAt(ended, resAt(20)); !errors.Is(err, fleet.ErrBlockEnded) {
		t.Fatal("removing an ended block", err)
	}
}
