package domain

import (
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
)

// EMP-TRIP-011 (B-BR-EMP-390, EDR-252): pause, resume and cancellation are
// lifecycle commands of one continuous trip with an operational revision.

// MissingReason names the absent reason of a cancellation.
const MissingReason Missing = "REASON"

// LifecycleInput is a pause, resume or cancellation by a member at the
// expected trip revision; the reason is required for CANCEL.
type LifecycleInput struct {
	redacted
	Author   MembershipRef
	Expected int64
	Reason   *string
	Declared *orgtime.Declaration
}

// LifecycleFact is one accepted lifecycle command of a trip.
type LifecycleFact struct {
	redacted
	command  Command
	revision int64
	from, to State
	author   MembershipRef
	declared orgtime.Declaration
	reason   *string
}

func (f LifecycleFact) Command() Command              { return f.command }
func (f LifecycleFact) Revision() int64               { return f.revision }
func (f LifecycleFact) From() State                   { return f.from }
func (f LifecycleFact) To() State                     { return f.to }
func (f LifecycleFact) Author() MembershipRef         { return f.author }
func (f LifecycleFact) Declared() orgtime.Declaration { return f.declared }
func (f LifecycleFact) Reason() *string               { return copyPointer(f.reason) }

// Revision is the trip's operational revision: 1 after START, one more per
// lifecycle command or FINISH.
func (t Trip) Revision() int64 { return t.revision }

// Lifecycle lists the accepted PAUSE, RESUME and CANCEL facts in revision order.
func (t Trip) Lifecycle() []LifecycleFact {
	out := make([]LifecycleFact, len(t.lifecycle))
	for i, f := range t.lifecycle {
		f.reason = copyPointer(f.reason)
		out[i] = f
	}
	return out
}

// Responsible is the member who may change the trip: the member who started
// it (EMP-TRIP-012 hands it over).
func (t Trip) Responsible() MembershipRef { return t.start.author }

// Cancellation returns the CANCEL fact of a cancelled trip.
func (t Trip) Cancellation() (LifecycleFact, bool) {
	if t.state != Cancelled || len(t.lifecycle) == 0 {
		return LifecycleFact{}, false
	}
	f := t.lifecycle[len(t.lifecycle)-1]
	f.reason = copyPointer(f.reason)
	return f, f.command == CommandCancel
}

// Pause moves an IN_PROGRESS trip to PAUSED.
func (t Trip) Pause(scope kernel.OrgScope, in LifecycleInput) (Trip, error) {
	return t.change(scope, CommandPause, in)
}

// Resume moves a PAUSED trip back to IN_PROGRESS.
func (t Trip) Resume(scope kernel.OrgScope, in LifecycleInput) (Trip, error) {
	return t.change(scope, CommandResume, in)
}

// Cancel ends an IN_PROGRESS or PAUSED trip as CANCELLED, with a reason.
func (t Trip) Cancel(scope kernel.OrgScope, in LifecycleInput) (Trip, error) {
	return t.change(scope, CommandCancel, in)
}

// change applies one lifecycle command (B-BR-EMP-390), first failure first:
// scope and trip, author, present reason and declaration, expected revision,
// absent parts, the responsible member, a terminal trip, a stale revision,
// the table, then an instant before the start. The receiver never changes.
func (t Trip) change(scope kernel.OrgScope, cmd Command, in LifecycleInput) (Trip, error) {
	if scope.Validate() != nil {
		return Trip{}, ErrScope
	}
	if !t.state.valid() {
		return Trip{}, ErrInvalid
	}
	if t.reference.org != scope.OrgID() {
		return Trip{}, ErrScope
	}
	if e := in.Author.validate(scope); e != nil {
		return Trip{}, e
	}
	if in.Reason != nil {
		if _, e := meter.NewMissing(*in.Reason); e != nil {
			return Trip{}, ErrInvalid
		}
	}
	var declared orgtime.Declaration
	if in.Declared != nil {
		var e error
		if declared, e = orgtime.ValidateDeclaration(*in.Declared); e != nil {
			return Trip{}, ErrInvalid
		}
	}
	if in.Expected < 1 {
		return Trip{}, ErrInvalid
	}
	var missing []Missing
	if cmd == CommandCancel && in.Reason == nil {
		missing = append(missing, MissingReason)
	}
	if in.Declared == nil {
		missing = append(missing, MissingDeclaration)
	}
	if len(missing) != 0 {
		return Trip{}, &IncompleteError{missing: missing}
	}
	if in.Author != t.Responsible() {
		return Trip{}, ErrForbidden
	}
	if t.state.Terminal() {
		return Trip{}, ErrInvalidTransition
	}
	if in.Expected != t.revision {
		return Trip{}, ErrConflict
	}
	next, e := Transition(t.state, cmd)
	if e != nil {
		return Trip{}, e
	}
	if declared.OccurredAt.Before(t.start.declared.OccurredAt) {
		return Trip{}, ErrBeforeStart
	}
	out := t
	out.state, out.revision = next, t.revision+1
	out.lifecycle = append(t.Lifecycle(), LifecycleFact{command: cmd, revision: out.revision, from: t.state, to: next, author: in.Author,
		declared: declared, reason: copyPointer(in.Reason)})
	return out, nil
}
