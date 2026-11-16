package domain

import (
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

// EMP-TRIP-012 (B-BR-EMP-394/395, EDR-254): a supervisor's handover moves the
// responsibility for an open trip to another member at a server instant. It
// is a trip revision; the trip keeps its identity, state, asset, start driver
// and reporting attribution, and every earlier fact keeps its actor.

// HandoverInput is one recorded handover: the recording member, the
// receiving member and its employee record, the expected revision, the
// reason and the server instant (never a device claim, BR-TIME-003).
type HandoverInput struct {
	redacted
	Recorder         MembershipRef
	Receiver         MembershipRef
	ReceiverEmployee EmployeeRef
	Expected         int64
	Reason           *string
	At               time.Time
}

// HandoverFact is one accepted handover of a trip.
type HandoverFact struct {
	redacted
	revision         int64
	from, to         MembershipRef
	receiverEmployee EmployeeRef
	recorder         MembershipRef
	at               time.Time
	reason           string
}

func (f HandoverFact) Revision() int64               { return f.revision }
func (f HandoverFact) From() MembershipRef           { return f.from }
func (f HandoverFact) To() MembershipRef             { return f.to }
func (f HandoverFact) ReceiverEmployee() EmployeeRef { return f.receiverEmployee }
func (f HandoverFact) Recorder() MembershipRef       { return f.recorder }
func (f HandoverFact) At() time.Time                 { return f.at }
func (f HandoverFact) Reason() string                { return f.reason }

// Handovers lists the accepted handovers in revision order.
func (t Trip) Handovers() []HandoverFact { return append([]HandoverFact(nil), t.handovers...) }

// ResponsibleAt is the member responsible for the trip at an instant: the
// receiver of the latest handover recorded at or before it, else the member
// who started the trip (B-BR-EMP-395).
func (t Trip) ResponsibleAt(at time.Time) MembershipRef {
	out := t.start.author
	for _, h := range t.handovers {
		if !h.at.After(at) {
			out = h.to
		}
	}
	return out
}

// validInstant refuses the unset instant and anything outside years 1..9999.
func validInstant(at time.Time) bool {
	year := at.UTC().Year()
	return !at.IsZero() && year >= 1 && year <= 9999
}

// Handover records a handover (B-BR-EMP-394), first failure first: scope and
// trip, recorder, receiver and receiving employee, present reason and
// instant, expected revision, the absent reason, a terminal trip, a stale
// revision, a recorder who is the current responsible member or the
// receiver, a receiver who already is responsible, then an instant before
// the previous handover. The receiver never changes.
func (t Trip) Handover(scope kernel.OrgScope, in HandoverInput) (Trip, error) {
	if scope.Validate() != nil {
		return Trip{}, ErrScope
	}
	if !t.state.valid() {
		return Trip{}, ErrInvalid
	}
	if t.reference.org != scope.OrgID() {
		return Trip{}, ErrScope
	}
	for _, r := range []scopedID{in.Recorder.scopedID, in.Receiver.scopedID, in.ReceiverEmployee.scopedID} {
		if e := r.validate(scope); e != nil {
			return Trip{}, e
		}
	}
	if in.Reason != nil {
		if _, e := meter.NewMissing(*in.Reason); e != nil {
			return Trip{}, ErrInvalid
		}
	}
	if !validInstant(in.At) || in.Expected < 1 {
		return Trip{}, ErrInvalid
	}
	if in.Reason == nil {
		return Trip{}, &IncompleteError{missing: []Missing{MissingReason}}
	}
	if t.state.Terminal() {
		return Trip{}, ErrInvalidTransition
	}
	if in.Expected != t.revision {
		return Trip{}, ErrConflict
	}
	current := t.Responsible()
	if in.Recorder == current || in.Recorder == in.Receiver {
		return Trip{}, ErrForbidden
	}
	if in.Receiver == current {
		return Trip{}, ErrInvalid
	}
	if n := len(t.handovers); n > 0 && in.At.Before(t.handovers[n-1].at) {
		return Trip{}, ErrInvalid
	}
	out := t
	out.revision = t.revision + 1
	out.handovers = append(t.Handovers(), HandoverFact{revision: out.revision, from: current, to: in.Receiver, receiverEmployee: in.ReceiverEmployee,
		recorder: in.Recorder, at: in.At.UTC(), reason: *in.Reason})
	return out, nil
}
