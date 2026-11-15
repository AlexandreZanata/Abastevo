// Package domain models the pure enterprise trip (EMP-TRIP-001): the closed
// operational vocabulary, the C1 driver-initiated lifecycle (START creates
// IN_PROGRESS, FINISH completes it), the eligible start context, honest meter
// evidence and the independence of operational, review and delivery state.
// It holds no persistence, clock, authority, assignment or review decision.
package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
)

// Closed failure vocabulary shared with Kotlin through trip-v1.
var (
	ErrInvalid            = errors.New("trip: invalid value")
	ErrScope              = errors.New("trip: organization scope denied")
	ErrIncomplete         = errors.New("trip: eligible context incomplete")
	ErrForbidden          = errors.New("trip: command by another member")
	ErrInvalidTransition  = errors.New("trip: transition not allowed")
	ErrUnsupportedCommand = errors.New("trip: command owned by the mission lifecycle")
	ErrFinishBeforeStart  = errors.New("trip: finish declared before start")
	// ErrBeforeStart is a pause, resume or cancellation declared strictly
	// before the start instant.
	ErrBeforeStart = errors.New("trip: lifecycle command declared before start")
)

type redacted struct{}

func (redacted) String() string                 { return "EnterpriseTripValue(redacted)" }
func (r redacted) Format(f fmt.State, _ rune)   { _, _ = io.WriteString(f, r.String()) }
func (r redacted) LogValue() slog.Value         { return slog.StringValue(r.String()) }
func (r redacted) MarshalJSON() ([]byte, error) { return json.Marshal(r.String()) }

// State is a trip operational state of the canonical catalog (08-state-model §3).
type State string

const (
	Planned    State = "PLANNED"
	Assigned   State = "ASSIGNED"
	Accepted   State = "ACCEPTED"
	InProgress State = "IN_PROGRESS"
	Paused     State = "PAUSED"
	Completed  State = "COMPLETED"
	Cancelled  State = "CANCELLED"
)

// ParseState accepts only an exact catalog token.
func ParseState(raw string) (State, error) {
	s := State(raw)
	if !s.valid() {
		return "", ErrInvalid
	}
	return s, nil
}

func (s State) valid() bool {
	switch s {
	case Planned, Assigned, Accepted, InProgress, Paused, Completed, Cancelled:
		return true
	}
	return false
}

// Terminal reports COMPLETED and CANCELLED.
func (s State) Terminal() bool { return s == Completed || s == Cancelled }

// Command is an operational trip command.
type Command string

const (
	CommandStart  Command = "START"
	CommandFinish Command = "FINISH"
	CommandPause  Command = "PAUSE"
	CommandResume Command = "RESUME"
	CommandCancel Command = "CANCEL"
)

// ParseCommand accepts only an exact command token.
func ParseCommand(raw string) (Command, error) {
	c := Command(raw)
	switch c {
	case CommandStart, CommandFinish, CommandPause, CommandResume, CommandCancel:
		return c, nil
	}
	return "", ErrInvalid
}

// Transition is the operational table (EDR-178, EMP-TRIP-011 EDR-252), first
// failure first: unknown tokens, terminal states (a new attempt is a new
// trip), the mission-owned states PLANNED, ASSIGNED and ACCEPTED, then the
// moves of an open trip: PAUSE IN_PROGRESS → PAUSED, RESUME PAUSED →
// IN_PROGRESS, CANCEL → CANCELLED and FINISH → COMPLETED from either; START
// on an existing trip and any other pair is INVALID_TRANSITION. START itself
// creates a trip only through Start.
func Transition(from State, cmd Command) (State, error) {
	if _, e := ParseState(string(from)); e != nil {
		return "", e
	}
	if _, e := ParseCommand(string(cmd)); e != nil {
		return "", e
	}
	if from.Terminal() {
		return "", ErrInvalidTransition
	}
	if from != InProgress && from != Paused {
		return "", ErrUnsupportedCommand
	}
	switch {
	case cmd == CommandFinish:
		return Completed, nil
	case cmd == CommandCancel:
		return Cancelled, nil
	case cmd == CommandPause && from == InProgress:
		return Paused, nil
	case cmd == CommandResume && from == Paused:
		return InProgress, nil
	}
	return "", ErrInvalidTransition
}

// ReviewState is the closed trip review vocabulary: BR-REV states plus VOIDED.
// It is an input owned by the review lifecycle, never derived from operation.
type ReviewState string

const (
	ReviewDraft     ReviewState = "DRAFT"
	ReviewSubmitted ReviewState = "SUBMITTED"
	ReviewInReview  ReviewState = "IN_REVIEW"
	ReviewApproved  ReviewState = "APPROVED"
	ReviewReturned  ReviewState = "RETURNED"
	ReviewRejected  ReviewState = "REJECTED"
	ReviewCancelled ReviewState = "CANCELLED"
	ReviewVoided    ReviewState = "VOIDED"
)

// ParseReviewState accepts only an exact review token.
func ParseReviewState(raw string) (ReviewState, error) {
	r := ReviewState(raw)
	switch r {
	case ReviewDraft, ReviewSubmitted, ReviewInReview, ReviewApproved, ReviewReturned, ReviewRejected, ReviewCancelled, ReviewVoided:
		return r, nil
	}
	return "", ErrInvalid
}

// Standing combines independent operational and review states (BR-REV-001).
type Standing struct {
	operational State
	review      ReviewState
}

// NewStanding validates both closed tokens; any combination is representable.
func NewStanding(operational State, review ReviewState) (Standing, error) {
	if _, e := ParseState(string(operational)); e != nil {
		return Standing{}, e
	}
	if _, e := ParseReviewState(string(review)); e != nil {
		return Standing{}, e
	}
	return Standing{operational: operational, review: review}, nil
}
func (s Standing) Operational() State  { return s.operational }
func (s Standing) Review() ReviewState { return s.review }

// Completed is operational completion only.
func (s Standing) Completed() bool { return s.operational == Completed }

// Approved is administrative approval only.
func (s Standing) Approved() bool { return s.review == ReviewApproved }
