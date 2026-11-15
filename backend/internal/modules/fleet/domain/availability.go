package domain

import (
	"slices"
	"strings"
	"time"
)

// Availability is the derived answer of fleet.md §6.2 (BR-FLEET-025); it is
// never stored as truth.
type Availability string

const (
	Available     Availability = "AVAILABLE"
	Reserved      Availability = "RESERVED"
	InUse         Availability = "IN_USE"
	InMaintenance Availability = "IN_MAINTENANCE"
	Blocked       Availability = "BLOCKED"
	Unavailable   Availability = "UNAVAILABLE"
)

// CauseKind names the source of one applying row of §6.2.
type CauseKind string

const (
	LifecycleCause   CauseKind = "LIFECYCLE"
	ActiveUseCause   CauseKind = "ACTIVE_USE"
	MaintenanceCause CauseKind = "MAINTENANCE_HOLD"
	BlockCause       CauseKind = "BLOCK"
	ReservationCause CauseKind = "RESERVATION"
)

// Cause is one row of §6.2 that applies at the evaluated instant. It carries
// the source kind, the record identifier (empty for the lifecycle) and the
// record's period, never a person, purpose or note.
type Cause struct {
	Availability Availability
	Kind         CauseKind
	ID           string
	// Lifecycle is set only for a LIFECYCLE cause.
	Lifecycle State
	// Block is set only for a BLOCK cause.
	Block BlockReason
	From  time.Time
	To    *time.Time
}

// UseFact is a trip in progress or paused naming the asset as main asset or
// attached implement (row 2), answered by the active-use port.
type UseFact struct {
	ID   string
	From time.Time
}

// HoldFact is an open maintenance hold (row 3), answered by the
// maintenance-hold port.
type HoldFact struct {
	ID   string
	From time.Time
	To   *time.Time
}

// BlockFact is one stored block with its half-open period (row 4).
type BlockFact struct {
	ID     string
	Reason BlockReason
	Period AssignmentPeriod
}

// ReservationFact is one stored reservation (row 5). EmployeeID is the
// reserved employee record, empty when the reservation names nobody.
type ReservationFact struct {
	ID          string
	EmployeeID  string
	Reservation Reservation
}

// AvailabilityFacts are the stored facts of one asset known at an instant.
type AvailabilityFacts struct {
	Lifecycle    State
	Uses         []UseFact
	Holds        []HoldFact
	Blocks       []BlockFact
	Reservations []ReservationFact
}

// Assessment is the first applying row and every applying cause, in
// precedence order and, inside a kind, by start then identifier.
type Assessment struct {
	Availability Availability
	Causes       []Cause
}

func validFactID(id string) bool { return id != "" && strings.TrimSpace(id) == id && len(id) <= 64 }

func copyEnd(at *time.Time) *time.Time {
	if at == nil {
		return nil
	}
	v := at.UTC()
	return &v
}

func sortCauses(causes []Cause) {
	slices.SortStableFunc(causes, func(a, b Cause) int {
		if c := a.From.Compare(b.From); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
}

// Assess applies fleet.md §6.2 at instant at, with the facts known at now
// (at is normally now; the intake passes the declared instant).
func Assess(f AvailabilityFacts, at, now time.Time) (Assessment, error) {
	if _, err := ParseState(string(f.Lifecycle)); err != nil || !exactBoundary(at.Truncate(time.Microsecond)) || now.IsZero() {
		return Assessment{}, ErrInvalid
	}
	var causes []Cause
	if f.Lifecycle != Active {
		causes = append(causes, Cause{Availability: Unavailable, Kind: LifecycleCause, Lifecycle: f.Lifecycle})
	}
	var uses, holds, blocks, reservations []Cause
	for _, use := range f.Uses {
		if !validFactID(use.ID) || use.From.IsZero() {
			return Assessment{}, ErrInvalid
		}
		uses = append(uses, Cause{Availability: InUse, Kind: ActiveUseCause, ID: use.ID, From: use.From.UTC()})
	}
	for _, hold := range f.Holds {
		if !validFactID(hold.ID) || hold.From.IsZero() || hold.To != nil && !hold.To.After(hold.From) {
			return Assessment{}, ErrInvalid
		}
		if !at.Before(hold.From) && (hold.To == nil || at.Before(*hold.To)) {
			holds = append(holds, Cause{Availability: InMaintenance, Kind: MaintenanceCause, ID: hold.ID, From: hold.From.UTC(), To: copyEnd(hold.To)})
		}
	}
	for _, block := range f.Blocks {
		if !validFactID(block.ID) {
			return Assessment{}, ErrInvalid
		}
		if _, err := ParseBlockReason(string(block.Reason)); err != nil {
			return Assessment{}, err
		}
		period, err := NewAssignmentPeriod(block.Period.From, block.Period.To)
		if err != nil {
			return Assessment{}, err
		}
		if period.Contains(at) {
			blocks = append(blocks, Cause{Availability: Blocked, Kind: BlockCause, ID: block.ID, Block: block.Reason, From: period.From, To: copyEnd(period.To)})
		}
	}
	for _, fact := range f.Reservations {
		if !validFactID(fact.ID) || fact.EmployeeID != "" && !validFactID(fact.EmployeeID) || fact.Reservation.Validate() != nil {
			return Assessment{}, ErrInvalid
		}
		if fact.Reservation.HoldsAt(at, now) {
			end, _ := fact.Reservation.HoldEnd(now)
			reservations = append(reservations, Cause{Availability: Reserved, Kind: ReservationCause, ID: fact.ID, From: fact.Reservation.Window.From, To: &end})
		}
	}
	for _, kind := range [][]Cause{uses, holds, blocks, reservations} {
		sortCauses(kind)
		causes = append(causes, kind...)
	}
	out := Assessment{Availability: Available, Causes: causes}
	if len(causes) > 0 {
		out.Availability = causes[0].Availability
	}
	return out, nil
}

// HoldCode is the review code of a held field operation (BR-FLEET-027).
type HoldCode string

const (
	AssetBusy        HoldCode = "ASSET_BUSY"
	AssetUnavailable HoldCode = "ASSET_UNAVAILABLE"
)

// Starter is the author of a start: the employee record and whether an
// ACTIVE assignment of the asset names it at the declared instant.
type Starter struct {
	EmployeeID string
	Assigned   bool
}

// StartDecision admits a start or holds it for review with a precise code.
type StartDecision struct {
	Admitted   bool
	Code       HoldCode
	Assessment Assessment
}

// DecideStart is the intake check of a start (§6.2 last paragraph, BR-FLEET-
// 027/031): not ACTIVE, in maintenance, blocked, reserved for someone else or
// not assigned → ASSET_UNAVAILABLE; another trip holds it → ASSET_BUSY. A
// reservation that names the starter admits the start; otherwise an ACTIVE
// assignment is required. A trip already in progress is never re-decided.
func DecideStart(f AvailabilityFacts, starter Starter, at, now time.Time) (StartDecision, error) {
	if starter.EmployeeID != "" && !validFactID(starter.EmployeeID) {
		return StartDecision{}, ErrInvalid
	}
	assessment, err := Assess(f, at, now)
	if err != nil {
		return StartDecision{}, err
	}
	out := StartDecision{Assessment: assessment}
	switch assessment.Availability {
	case InUse:
		out.Code = AssetBusy
		return out, nil
	case Unavailable, InMaintenance, Blocked:
		out.Code = AssetUnavailable
		return out, nil
	case Reserved:
		for _, fact := range f.Reservations {
			if fact.Reservation.HoldsAt(at, now) && (starter.EmployeeID == "" || fact.EmployeeID != starter.EmployeeID) {
				out.Code = AssetUnavailable
				return out, nil
			}
		}
		out.Admitted = true
		return out, nil
	}
	if !starter.Assigned {
		out.Code = AssetUnavailable
		return out, nil
	}
	out.Admitted = true
	return out, nil
}
