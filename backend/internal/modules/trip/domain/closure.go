package domain

import (
	"errors"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

// EMP-TRIP-014 supervisor closure (B-BR-EMP-399..402, EDR-257). It is a
// server-only operation of the trip, not a lifecycle command: the C1 command
// vocabulary, Transition and the shared trip vectors are unchanged.

// ErrSelfClosure: the closing member is the trip's own author or driver,
// who finishes it instead (BR-REV-003).
var ErrSelfClosure = errors.New("trip: closure by the trip's own author or driver")

// CloseInput is a supervisor's closure of another member's open trip: the
// closing membership, its employee record and the explicit reason why no
// final reading exists. It carries no reading, value or distance.
type CloseInput struct {
	redacted
	Closer         MembershipRef
	CloserEmployee EmployeeRef
	Absence        meter.Missing
}

// Closure is the stored closure of a trip.
type Closure struct {
	redacted
	closer         MembershipRef
	closerEmployee EmployeeRef
	absence        meter.Missing
}

func (c Closure) Closer() MembershipRef       { return c.closer }
func (c Closure) CloserEmployee() EmployeeRef { return c.closerEmployee }
func (c Closure) Absence() meter.Missing      { return c.absence }

// Close completes an IN_PROGRESS trip of another member without a final
// reading, first failure first: scope and trip, the closer references, the
// absence reason, self-closure, then the state (terminal INVALID_TRANSITION,
// any other non-running state UNSUPPORTED_COMMAND). The receiver never
// changes; the result is COMPLETED with the closure and no finish.
func (t Trip) Close(scope kernel.OrgScope, in CloseInput) (Trip, error) {
	if scope.Validate() != nil {
		return Trip{}, ErrScope
	}
	if !t.state.valid() {
		return Trip{}, ErrInvalid
	}
	if t.reference.org != scope.OrgID() {
		return Trip{}, ErrScope
	}
	if e := in.Closer.validate(scope); e != nil {
		return Trip{}, e
	}
	if e := in.CloserEmployee.validate(scope); e != nil {
		return Trip{}, e
	}
	if _, e := meter.NewMissing(in.Absence.Reason()); e != nil {
		return Trip{}, ErrInvalid
	}
	if in.Closer == t.start.author || in.CloserEmployee == t.driver {
		return Trip{}, ErrSelfClosure
	}
	if t.state.Terminal() {
		return Trip{}, ErrInvalidTransition
	}
	if t.state != InProgress {
		return Trip{}, ErrUnsupportedCommand
	}
	out := t
	out.state = Completed
	out.closure = &Closure{closer: in.Closer, closerEmployee: in.CloserEmployee, absence: in.Absence}
	return out, nil
}

// Closed returns the trip's supervisor closure, if any.
func (t Trip) Closed() (Closure, bool) {
	if t.closure == nil {
		return Closure{}, false
	}
	return *t.closure, true
}
