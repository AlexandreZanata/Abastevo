package domain

import (
	"errors"
	"time"
)

// ErrAssignmentEnded refuses ending an assignment that is already ENDED at
// the evaluated instant (B-BR-EMP-244).
var ErrAssignmentEnded = errors.New("fleet: assignment already ended")

// AssignmentKind is the closed BR-FLEET-018 vocabulary.
type AssignmentKind string

const (
	PrimaryAssignment    AssignmentKind = "PRIMARY"
	AdditionalAssignment AssignmentKind = "ADDITIONAL"
	TemporaryAssignment  AssignmentKind = "TEMPORARY"
)

// AssignmentState is derived from the period at a server instant, never stored.
type AssignmentState string

const (
	AssignmentScheduled AssignmentState = "SCHEDULED"
	AssignmentActive    AssignmentState = "ACTIVE"
	AssignmentEnded     AssignmentState = "ENDED"
)

// MaxAssignees bounds one create command (EDR-164, the page bound).
const MaxAssignees = 100

func ParseAssignmentKind(raw string) (AssignmentKind, error) {
	switch kind := AssignmentKind(raw); kind {
	case PrimaryAssignment, AdditionalAssignment, TemporaryAssignment:
		return kind, nil
	}
	return "", ErrInvalid
}

// AssignmentPeriod is one assignment period [From, To) with whole-microsecond
// bounds (B-BR-EMP-243). To is nil while open-ended; To equal to From is the
// empty period of an assignment ended before its start.
type AssignmentPeriod struct {
	From time.Time
	To   *time.Time
}

// NewAssignmentPeriod validates a stored period: exact bounds, To not before
// From. It returns a UTC copy that does not alias the caller's end value.
func NewAssignmentPeriod(from time.Time, to *time.Time) (AssignmentPeriod, error) {
	if !exactBoundary(from) {
		return AssignmentPeriod{}, ErrInvalid
	}
	p := AssignmentPeriod{From: from.UTC()}
	if to != nil {
		if !exactBoundary(*to) || to.Before(from) {
			return AssignmentPeriod{}, ErrInvalid
		}
		end := to.UTC()
		p.To = &end
	}
	return p, nil
}

// Empty reports an assignment ended before its start.
func (p AssignmentPeriod) Empty() bool { return p.To != nil && p.To.Equal(p.From) }

// Contains applies the half-open rule From <= at < To.
func (p AssignmentPeriod) Contains(at time.Time) bool {
	return !at.Before(p.From) && (p.To == nil || at.Before(*p.To))
}

// StateAt derives SCHEDULED, ACTIVE or ENDED at the server instant at.
func (p AssignmentPeriod) StateAt(at time.Time) AssignmentState {
	switch {
	case p.Empty() || p.To != nil && !at.Before(*p.To):
		return AssignmentEnded
	case at.Before(p.From):
		return AssignmentScheduled
	}
	return AssignmentActive
}

// EndAt ends a period that is not ENDED at the instant at: the new end is the
// microsecond floor of at, or From when the period has not started.
func (p AssignmentPeriod) EndAt(at time.Time) (AssignmentPeriod, error) {
	instant, err := AllocationInstant(at)
	if err != nil {
		return AssignmentPeriod{}, err
	}
	if _, err := NewAssignmentPeriod(p.From, p.To); err != nil {
		return AssignmentPeriod{}, err
	}
	if p.StateAt(at) == AssignmentEnded {
		return AssignmentPeriod{}, ErrAssignmentEnded
	}
	end := instant
	if end.Before(p.From) {
		end = p.From
	}
	return NewAssignmentPeriod(p.From, &end)
}

// ValidNewAssignment checks the shape of a period being created: non-empty,
// starting no earlier than now, and TEMPORARY requires an end.
func ValidNewAssignment(kind AssignmentKind, period AssignmentPeriod, now time.Time) error {
	if _, err := ParseAssignmentKind(string(kind)); err != nil {
		return err
	}
	if _, err := NewAssignmentPeriod(period.From, period.To); err != nil || period.Empty() || now.IsZero() || period.From.Before(now) {
		return ErrInvalid
	}
	if kind == TemporaryAssignment && period.To == nil {
		return ErrInvalid
	}
	return nil
}

// ValidAssignees checks the employee list of one create command: 1..100
// distinct identifiers, exactly one for PRIMARY. Identifier syntax is the
// caller's (application) check.
func ValidAssignees(kind AssignmentKind, employees []string) error {
	if _, err := ParseAssignmentKind(string(kind)); err != nil {
		return err
	}
	if len(employees) == 0 || len(employees) > MaxAssignees || kind == PrimaryAssignment && len(employees) != 1 {
		return ErrInvalid
	}
	seen := make(map[string]bool, len(employees))
	for _, id := range employees {
		if seen[id] {
			return ErrInvalid
		}
		seen[id] = true
	}
	return nil
}
