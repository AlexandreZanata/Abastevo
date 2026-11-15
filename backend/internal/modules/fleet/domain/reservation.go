package domain

import (
	"errors"
	"time"
)

// ErrReservationEnded refuses a command on a reservation that is no longer
// REQUESTED or CONFIRMED at the evaluated instant (B-BR-EMP-296/297).
var ErrReservationEnded = errors.New("fleet: reservation already ended")

// ErrBlockEnded refuses removing a block that is already removed or ended
// (B-BR-EMP-298).
var ErrBlockEnded = errors.New("fleet: block already ended")

// ReservationState is the closed 08-state-model vocabulary. FULFILLED is
// reached only through a qualifying trip start (EMP-TRIP-023).
type ReservationState string

const (
	ReservationRequested ReservationState = "REQUESTED"
	ReservationConfirmed ReservationState = "CONFIRMED"
	ReservationFulfilled ReservationState = "FULFILLED"
	ReservationCancelled ReservationState = "CANCELLED"
	ReservationNoShow    ReservationState = "NO_SHOW"
	ReservationExpired   ReservationState = "EXPIRED"
)

func ParseReservationState(raw string) (ReservationState, error) {
	switch state := ReservationState(raw); state {
	case ReservationRequested, ReservationConfirmed, ReservationFulfilled, ReservationCancelled, ReservationNoShow, ReservationExpired:
		return state, nil
	}
	return "", ErrInvalid
}

// ReservationCommand names one row of fleet.md §6.4 after creation.
type ReservationCommand string

const (
	ConfirmReservation ReservationCommand = "CONFIRM"
	CancelReservation  ReservationCommand = "CANCEL"
	ExpireReservation  ReservationCommand = "EXPIRE"
	MissReservation    ReservationCommand = "MISS"
	FulfilReservation  ReservationCommand = "FULFIL"
)

// reservationRows is fleet.md §6.4. FULFIL has no source before a trip
// intake exists (EMP-TRIP-023), so it has no row here.
var reservationRows = map[ReservationCommand]map[ReservationState]ReservationState{
	ConfirmReservation: {ReservationRequested: ReservationConfirmed},
	CancelReservation:  {ReservationRequested: ReservationCancelled, ReservationConfirmed: ReservationCancelled},
	ExpireReservation:  {ReservationRequested: ReservationExpired},
	MissReservation:    {ReservationConfirmed: ReservationNoShow},
}

// ReservationTransition answers the target of a command from a stored state.
func ReservationTransition(from ReservationState, command ReservationCommand) (ReservationState, error) {
	if to, ok := reservationRows[command][from]; ok {
		return to, nil
	}
	return "", ErrTransition
}

// No-show tolerance (fleet.md §6.4): default 60 minutes, 15 to 240.
const (
	DefaultNoShowTolerance = 60 * time.Minute
	MinNoShowTolerance     = 15 * time.Minute
	MaxNoShowTolerance     = 240 * time.Minute
)

// ReservationWindow is a half-open window [From, To) with whole-microsecond
// bounds and To after From.
type ReservationWindow struct{ From, To time.Time }

func NewReservationWindow(from, to time.Time) (ReservationWindow, error) {
	if !exactBoundary(from) || !exactBoundary(to) || !to.After(from) {
		return ReservationWindow{}, ErrInvalid
	}
	return ReservationWindow{From: from.UTC(), To: to.UTC()}, nil
}

func (w ReservationWindow) Contains(at time.Time) bool {
	return !at.Before(w.From) && at.Before(w.To)
}

// Reservation holds the stored facts the derived answers need.
type Reservation struct {
	State     ReservationState
	Window    ReservationWindow
	Tolerance time.Duration
	// Confirmed is set once the reservation was CONFIRMED, whatever it is now.
	Confirmed bool
	// EndedAt is the instant of a cancellation (nil otherwise).
	EndedAt *time.Time
}

func (r Reservation) Validate() error {
	if _, err := ParseReservationState(string(r.State)); err != nil {
		return err
	}
	if _, err := NewReservationWindow(r.Window.From, r.Window.To); err != nil {
		return err
	}
	if r.Tolerance < MinNoShowTolerance || r.Tolerance > MaxNoShowTolerance || r.Tolerance%time.Second != 0 {
		return ErrInvalid
	}
	switch r.State {
	case ReservationRequested, ReservationExpired:
		if r.Confirmed {
			return ErrInvalid
		}
	case ReservationConfirmed, ReservationNoShow, ReservationFulfilled:
		if !r.Confirmed {
			return ErrInvalid
		}
	}
	if (r.State == ReservationCancelled) != (r.EndedAt != nil) || r.EndedAt != nil && !exactBoundary(*r.EndedAt) {
		return ErrInvalid
	}
	return nil
}

// NoShowDeadline is the earlier of the start plus the tolerance and the end.
func (r Reservation) NoShowDeadline() time.Time {
	deadline := r.Window.From.Add(r.Tolerance)
	if r.Window.To.Before(deadline) {
		return r.Window.To
	}
	return deadline
}

// DueAt answers when the time-driven transition of the stored state becomes
// due: EXPIRE at the start of a REQUESTED one, MISS at the deadline of a
// CONFIRMED one. Other states have none.
func (r Reservation) DueAt() (time.Time, ReservationCommand, bool) {
	switch r.State {
	case ReservationRequested:
		return r.Window.From, ExpireReservation, true
	case ReservationConfirmed:
		return r.NoShowDeadline(), MissReservation, true
	}
	return time.Time{}, "", false
}

// StateAt is the effective state at the server instant now: the stored state
// with any due time-driven transition applied (BR-FLEET-030, read time).
func (r Reservation) StateAt(now time.Time) ReservationState {
	if due, command, ok := r.DueAt(); ok && !now.Before(due) {
		to, _ := ReservationTransition(r.State, command)
		return to
	}
	return r.State
}

// HoldEnd answers the end of the interval [From, end) during which the
// reservation held the asset, as known at now; ok is false when it never
// held it (never confirmed, or cancelled before its start).
func (r Reservation) HoldEnd(now time.Time) (time.Time, bool) {
	if !r.Confirmed {
		return time.Time{}, false
	}
	end := r.Window.To
	switch r.StateAt(now) {
	case ReservationConfirmed, ReservationFulfilled:
	case ReservationNoShow:
		end = r.NoShowDeadline()
	case ReservationCancelled:
		if r.EndedAt == nil {
			return time.Time{}, false
		}
		if r.EndedAt.Before(end) {
			end = *r.EndedAt
		}
	default:
		return time.Time{}, false
	}
	if !end.After(r.Window.From) {
		return time.Time{}, false
	}
	return end, true
}

// HoldsAt reports whether the reservation held the asset at instant at, as
// known at now (BR-FLEET-031).
func (r Reservation) HoldsAt(at, now time.Time) bool {
	end, ok := r.HoldEnd(now)
	return ok && !at.Before(r.Window.From) && at.Before(end)
}

// ValidNewReservation checks a window being requested: it starts after the
// server instant now, and the tolerance is in its configured range.
func ValidNewReservation(window ReservationWindow, tolerance time.Duration, now time.Time) error {
	if _, err := NewReservationWindow(window.From, window.To); err != nil || now.IsZero() || !window.From.After(now) {
		return ErrInvalid
	}
	if tolerance < MinNoShowTolerance || tolerance > MaxNoShowTolerance || tolerance%time.Second != 0 {
		return ErrInvalid
	}
	return nil
}

// BlockReason is the closed reason-kind vocabulary of fleet.md §3.
type BlockReason string

const (
	AdministrativeBlock BlockReason = "ADMINISTRATIVE"
	SafetyBlock         BlockReason = "SAFETY"
	DocumentBlock       BlockReason = "DOCUMENT"
	InvestigationBlock  BlockReason = "INVESTIGATION"
	LentOutBlock        BlockReason = "LENT_OUT"
)

func ParseBlockReason(raw string) (BlockReason, error) {
	switch reason := BlockReason(raw); reason {
	case AdministrativeBlock, SafetyBlock, DocumentBlock, InvestigationBlock, LentOutBlock:
		return reason, nil
	}
	return "", ErrInvalid
}

// ValidNewBlock checks a block period being created: a known reason kind and
// a non-empty period that starts no earlier than now. Block periods follow
// the half-open rule of AssignmentPeriod (BR-FLEET-032).
func ValidNewBlock(reason BlockReason, period AssignmentPeriod, now time.Time) error {
	if _, err := ParseBlockReason(string(reason)); err != nil {
		return err
	}
	if _, err := NewAssignmentPeriod(period.From, period.To); err != nil || period.Empty() || now.IsZero() || period.From.Before(now) {
		return ErrInvalid
	}
	return nil
}

// RemoveBlockAt ends a block that is not ENDED at now: its end becomes the
// microsecond floor of now, or its start when it has not started.
func RemoveBlockAt(period AssignmentPeriod, now time.Time) (AssignmentPeriod, error) {
	ended, err := period.EndAt(now)
	if errors.Is(err, ErrAssignmentEnded) {
		return AssignmentPeriod{}, ErrBlockEnded
	}
	return ended, err
}
