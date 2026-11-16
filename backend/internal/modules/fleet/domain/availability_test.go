package domain_test

import (
	"errors"
	"testing"
	"time"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
)

func confirmedFact(t *testing.T, id, employee string, from, to int) fleet.ReservationFact {
	t.Helper()
	return fleet.ReservationFact{ID: id, EmployeeID: employee, Reservation: fleet.Reservation{State: fleet.ReservationConfirmed, Window: resWindow(t, from, to), Tolerance: time.Hour, Confirmed: true}}
}

func blockFact(t *testing.T, id string, reason fleet.BlockReason, from int, to *int) fleet.BlockFact {
	t.Helper()
	var end *time.Time
	if to != nil {
		at := resAt(*to)
		end = &at
	}
	p, err := fleet.NewAssignmentPeriod(resAt(from), end)
	if err != nil {
		t.Fatal(err)
	}
	return fleet.BlockFact{ID: id, Reason: reason, Period: p}
}

func intp(v int) *int { return &v }

// fleet.md §6.2, every row (AC1, B-BR-EMP-299): the first applying row wins
// and every applying cause is listed in precedence order.
func TestAvailabilityFollowsThePrecedenceTableAndListsEveryCause(t *testing.T) {
	at := resAt(20)
	use := fleet.UseFact{ID: "trip-1", From: resAt(5)}
	hold := fleet.HoldFact{ID: "hold-1", From: resAt(0)}
	block := blockFact(t, "block-1", fleet.SafetyBlock, 0, nil)
	reservation := confirmedFact(t, "res-1", "emp-1", 10, 130)
	for _, c := range []struct {
		name  string
		facts fleet.AvailabilityFacts
		want  fleet.Availability
		kinds []fleet.CauseKind
	}{
		{"row 1 draft", fleet.AvailabilityFacts{Lifecycle: fleet.Draft}, fleet.Unavailable, []fleet.CauseKind{fleet.LifecycleCause}},
		{"row 1 inactive", fleet.AvailabilityFacts{Lifecycle: fleet.Inactive, Blocks: []fleet.BlockFact{block}}, fleet.Unavailable, []fleet.CauseKind{fleet.LifecycleCause, fleet.BlockCause}},
		{"row 1 disposed", fleet.AvailabilityFacts{Lifecycle: fleet.Disposed}, fleet.Unavailable, []fleet.CauseKind{fleet.LifecycleCause}},
		{"row 1 archived", fleet.AvailabilityFacts{Lifecycle: fleet.Archived}, fleet.Unavailable, []fleet.CauseKind{fleet.LifecycleCause}},
		{"row 2 in use", fleet.AvailabilityFacts{Lifecycle: fleet.Active, Uses: []fleet.UseFact{use}, Holds: []fleet.HoldFact{hold}, Blocks: []fleet.BlockFact{block}, Reservations: []fleet.ReservationFact{reservation}}, fleet.InUse, []fleet.CauseKind{fleet.ActiveUseCause, fleet.MaintenanceCause, fleet.BlockCause, fleet.ReservationCause}},
		{"row 3 maintenance", fleet.AvailabilityFacts{Lifecycle: fleet.Active, Holds: []fleet.HoldFact{hold}, Reservations: []fleet.ReservationFact{reservation}}, fleet.InMaintenance, []fleet.CauseKind{fleet.MaintenanceCause, fleet.ReservationCause}},
		{"row 4 blocked", fleet.AvailabilityFacts{Lifecycle: fleet.Active, Blocks: []fleet.BlockFact{block}, Reservations: []fleet.ReservationFact{reservation}}, fleet.Blocked, []fleet.CauseKind{fleet.BlockCause, fleet.ReservationCause}},
		{"row 5 reserved", fleet.AvailabilityFacts{Lifecycle: fleet.Active, Reservations: []fleet.ReservationFact{reservation}}, fleet.Reserved, []fleet.CauseKind{fleet.ReservationCause}},
		{"row 6 available", fleet.AvailabilityFacts{Lifecycle: fleet.Active}, fleet.Available, nil},
		{"ended block and future reservation do not apply", fleet.AvailabilityFacts{Lifecycle: fleet.Active, Blocks: []fleet.BlockFact{blockFact(t, "block-2", fleet.DocumentBlock, 0, intp(20))}, Reservations: []fleet.ReservationFact{confirmedFact(t, "res-2", "", 21, 40)}}, fleet.Available, nil},
	} {
		got, err := fleet.Assess(c.facts, at, at)
		if err != nil {
			t.Fatal(c.name, err)
		}
		if got.Availability != c.want || len(got.Causes) != len(c.kinds) {
			t.Fatalf("%s: %s %+v", c.name, got.Availability, got.Causes)
		}
		for i, kind := range c.kinds {
			if got.Causes[i].Kind != kind {
				t.Fatalf("%s cause %d: %s", c.name, i, got.Causes[i].Kind)
			}
		}
	}
	// Causes carry the record identity and period, never a person or note.
	got, _ := fleet.Assess(fleet.AvailabilityFacts{Lifecycle: fleet.Active, Blocks: []fleet.BlockFact{blockFact(t, "b-2", fleet.LentOutBlock, 15, intp(90)), blockFact(t, "b-1", fleet.SafetyBlock, 15, nil)}, Reservations: []fleet.ReservationFact{reservation}}, at, at)
	if len(got.Causes) != 3 || got.Causes[0].ID != "b-1" || got.Causes[1].ID != "b-2" || got.Causes[1].Block != fleet.LentOutBlock || got.Causes[1].To == nil || !got.Causes[1].To.Equal(resAt(90)) {
		t.Fatalf("cause order or content %+v", got.Causes)
	}
	if r := got.Causes[2]; r.ID != "res-1" || !r.From.Equal(resAt(10)) || r.To == nil || !r.To.Equal(resAt(130)) {
		t.Fatalf("reservation cause %+v", r)
	}
	// AC3 applied to availability: a confirmed reservation past its deadline
	// no longer reserves, with no job.
	if got, _ := fleet.Assess(fleet.AvailabilityFacts{Lifecycle: fleet.Active, Reservations: []fleet.ReservationFact{reservation}}, resAt(70), resAt(70)); got.Availability != fleet.Available {
		t.Fatal("no-show still reserves", got)
	}
	if _, err := fleet.Assess(fleet.AvailabilityFacts{Lifecycle: "LOST"}, at, at); !errors.Is(err, fleet.ErrInvalid) {
		t.Fatal("unknown lifecycle admitted")
	}
	if _, err := fleet.Assess(fleet.AvailabilityFacts{Lifecycle: fleet.Active, Blocks: []fleet.BlockFact{{ID: "x", Reason: "OTHER", Period: block.Period}}}, at, at); !errors.Is(err, fleet.ErrInvalid) {
		t.Fatal("unknown block reason admitted")
	}
}

// BR-FLEET-027/031 start decision (AC4, AC5, B-BR-EMP-300).
func TestStartDecisionHoldsWithAPreciseCode(t *testing.T) {
	at := resAt(20)
	assigned := fleet.Starter{EmployeeID: "emp-2", Assigned: true}
	reservedForOther := fleet.AvailabilityFacts{Lifecycle: fleet.Active, Reservations: []fleet.ReservationFact{confirmedFact(t, "res-1", "emp-1", 10, 130)}}
	reservedForNobody := fleet.AvailabilityFacts{Lifecycle: fleet.Active, Reservations: []fleet.ReservationFact{confirmedFact(t, "res-1", "", 10, 130)}}
	block := blockFact(t, "block-1", fleet.SafetyBlock, 15, nil)
	for _, c := range []struct {
		name    string
		facts   fleet.AvailabilityFacts
		starter fleet.Starter
		admit   bool
		code    fleet.HoldCode
	}{
		{"available and assigned", fleet.AvailabilityFacts{Lifecycle: fleet.Active}, assigned, true, ""},
		{"available but not assigned", fleet.AvailabilityFacts{Lifecycle: fleet.Active}, fleet.Starter{EmployeeID: "emp-2"}, false, fleet.AssetUnavailable},
		{"not active", fleet.AvailabilityFacts{Lifecycle: fleet.Inactive}, assigned, false, fleet.AssetUnavailable},
		{"another trip holds it", fleet.AvailabilityFacts{Lifecycle: fleet.Active, Uses: []fleet.UseFact{{ID: "trip-1", From: resAt(1)}}}, assigned, false, fleet.AssetBusy},
		{"maintenance hold", fleet.AvailabilityFacts{Lifecycle: fleet.Active, Holds: []fleet.HoldFact{{ID: "hold-1", From: resAt(1)}}}, assigned, false, fleet.AssetUnavailable},
		{"blocked", fleet.AvailabilityFacts{Lifecycle: fleet.Active, Blocks: []fleet.BlockFact{block}}, assigned, false, fleet.AssetUnavailable},
		{"AC4 reserved for someone else", reservedForOther, assigned, false, fleet.AssetUnavailable},
		{"reserved for nobody in particular", reservedForNobody, assigned, false, fleet.AssetUnavailable},
		{"reserved for the starter without assignment", reservedForOther, fleet.Starter{EmployeeID: "emp-1"}, true, ""},
	} {
		got, err := fleet.DecideStart(c.facts, c.starter, at, at)
		if err != nil {
			t.Fatal(c.name, err)
		}
		if got.Admitted != c.admit || got.Code != c.code {
			t.Fatalf("%s: %+v", c.name, got)
		}
	}
	// AC5: a block added while a trip is in progress changes nothing for that
	// trip (no command re-decides it; the asset stays IN_USE with BLOCKED as a
	// further cause) and holds the next start once the trip has ended.
	during := fleet.AvailabilityFacts{Lifecycle: fleet.Active, Uses: []fleet.UseFact{{ID: "trip-1", From: resAt(1)}}, Blocks: []fleet.BlockFact{block}}
	if got, _ := fleet.Assess(during, at, at); got.Availability != fleet.InUse || len(got.Causes) != 2 || got.Causes[1].Kind != fleet.BlockCause {
		t.Fatal("block during a trip", got)
	}
	after := fleet.AvailabilityFacts{Lifecycle: fleet.Active, Blocks: []fleet.BlockFact{block}}
	if got, _ := fleet.DecideStart(after, assigned, resAt(40), resAt(40)); got.Admitted || got.Code != fleet.AssetUnavailable {
		t.Fatal("next start after the trip ignored the block", got)
	}
	if _, err := fleet.DecideStart(fleet.AvailabilityFacts{Lifecycle: fleet.Active}, fleet.Starter{EmployeeID: " bad"}, at, at); !errors.Is(err, fleet.ErrInvalid) {
		t.Fatal("malformed starter admitted")
	}
}

// EMP-CHECK-002 (BR-CHECK-010, EDR-285): a BLOCKING checklist run blocks the
// asset from its declared completion until its release, as row 4 after the
// manual blocks, and a new start is held ASSET_UNAVAILABLE.
func TestAvailabilityBlocksOnABlockingChecklistRun(t *testing.T) {
	at := resAt(20)
	released := resAt(20)
	run := fleet.ChecklistBlockFact{ID: "run-1", From: resAt(10)}
	block := blockFact(t, "block-1", fleet.SafetyBlock, 0, nil)
	reservation := confirmedFact(t, "res-1", "emp-1", 10, 130)
	got, err := fleet.Assess(fleet.AvailabilityFacts{Lifecycle: fleet.Active, Blocks: []fleet.BlockFact{block}, Checklists: []fleet.ChecklistBlockFact{run}, Reservations: []fleet.ReservationFact{reservation}}, at, at)
	if err != nil || got.Availability != fleet.Blocked || len(got.Causes) != 3 || got.Causes[0].Kind != fleet.BlockCause || got.Causes[1].Kind != fleet.ChecklistCause ||
		got.Causes[1].ID != "run-1" || got.Causes[1].Availability != fleet.Blocked || !got.Causes[1].From.Equal(resAt(10)) || got.Causes[1].To != nil || got.Causes[2].Kind != fleet.ReservationCause {
		t.Fatalf("blocking run: %+v %v", got, err)
	}
	only, err := fleet.Assess(fleet.AvailabilityFacts{Lifecycle: fleet.Active, Checklists: []fleet.ChecklistBlockFact{run}}, at, at)
	if err != nil || only.Availability != fleet.Blocked || len(only.Causes) != 1 {
		t.Fatalf("run alone: %+v %v", only, err)
	}
	decision, err := fleet.DecideStart(fleet.AvailabilityFacts{Lifecycle: fleet.Active, Checklists: []fleet.ChecklistBlockFact{run}}, fleet.Starter{EmployeeID: "emp-1", Assigned: true}, at, at)
	if err != nil || decision.Admitted || decision.Code != fleet.AssetUnavailable {
		t.Fatalf("start on a blocked asset: %+v %v", decision, err)
	}
	for name, facts := range map[string]fleet.ChecklistBlockFact{
		"released at the instant": {ID: "run-2", From: resAt(10), To: &released},
		"completed later":         {ID: "run-3", From: resAt(21)},
	} {
		got, err := fleet.Assess(fleet.AvailabilityFacts{Lifecycle: fleet.Active, Checklists: []fleet.ChecklistBlockFact{facts}}, at, at)
		if err != nil || got.Availability != fleet.Available || len(got.Causes) != 0 {
			t.Fatal(name, got, err)
		}
	}
	early := resAt(5)
	for name, bad := range map[string]fleet.ChecklistBlockFact{
		"empty id":   {From: resAt(10)},
		"zero start": {ID: "run-4"},
		"end before": {ID: "run-5", From: resAt(10), To: &early},
		"spaced id":  {ID: " run-6", From: resAt(10)},
	} {
		if _, err := fleet.Assess(fleet.AvailabilityFacts{Lifecycle: fleet.Active, Checklists: []fleet.ChecklistBlockFact{bad}}, at, at); !errors.Is(err, fleet.ErrInvalid) {
			t.Fatal("invalid run fact accepted", name, err)
		}
	}
}
