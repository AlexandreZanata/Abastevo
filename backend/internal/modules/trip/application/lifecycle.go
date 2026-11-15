package application

import (
	"context"
	"slices"
	"time"

	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
)

// EMP-TRIP-011 lifecycle commands (B-BR-EMP-391..393, EDR-253): the device's
// signed trip.pause, trip.resume and trip.cancel on the trip entity at the
// expected trip revision.
const (
	PauseOperationType  = "trip.pause"
	ResumeOperationType = "trip.resume"
	CancelOperationType = "trip.cancel"
	// MaxTripRevision bounds the expected trip revision of a lifecycle command.
	MaxTripRevision = 1_000_000
)

// LifecycleOperationTypes are the three lifecycle owners in command order.
var LifecycleOperationTypes = []string{PauseOperationType, ResumeOperationType, CancelOperationType}

// Exact schema 1 payload (EDR-253).
var transitionKeys = []string{"reason"}

// LifecycleCommandOf maps a lifecycle operation type to its trip command.
func LifecycleCommandOf(kind string) (trip.Command, bool) {
	switch kind {
	case PauseOperationType:
		return trip.CommandPause, true
	case ResumeOperationType:
		return trip.CommandResume, true
	case CancelOperationType:
		return trip.CommandCancel, true
	}
	return "", false
}

// TransitionInput is a parsed lifecycle command; its trip comes from storage.
type TransitionInput struct {
	private
	OperationID, PayloadDigest, TripID, AuthorMembershipID string
	Command                                                trip.Command
	Expected                                               int64
	Reason                                                 *string
	Declared                                               orgtime.Declaration
	DependsOn                                              []string
}

type TransitionCommand struct {
	private
	value TransitionInput
	valid bool
}

func (c TransitionCommand) Snapshot() (TransitionInput, error) {
	if !c.valid {
		return TransitionInput{}, ErrUnavailable
	}
	v := c.value
	v.Reason = copyPointer(v.Reason)
	v.DependsOn = slices.Clone(v.DependsOn)
	return v, nil
}

// ParseTransition decodes one trip.pause, trip.resume or trip.cancel schema 1
// operation (B-BR-EMP-391). ErrMalformed is final (a cancellation without a
// reason included); ErrUnsupported keeps soft references and attachments
// unavailable.
func ParseTransition(op protocol.Operation) (TransitionCommand, error) {
	v := op.Snapshot()
	cmd, ok := LifecycleCommandOf(v.Type)
	if !ok || !op.SupportsSchema() || v.Entity.Type != EntityType || !canonicalID(v.Entity.ID) || v.Entity.ExpectedRevision == nil ||
		*v.Entity.ExpectedRevision < 1 || *v.Entity.ExpectedRevision > MaxTripRevision || len(v.DependsOn) > 1 {
		return TransitionCommand{}, ErrMalformed
	}
	fields, ok := object(op.PayloadBytes(), transitionKeys)
	if !ok {
		return TransitionCommand{}, ErrMalformed
	}
	reason, ok := optional(fields, "reason")
	if !ok || reason != nil && validReason(*reason) != nil || cmd == trip.CommandCancel && reason == nil {
		return TransitionCommand{}, ErrMalformed
	}
	at, err := orgtime.ParseInstant(v.OccurredAt)
	if err != nil {
		return TransitionCommand{}, ErrMalformed
	}
	declared, err := orgtime.ValidateDeclaration(orgtime.Declaration{OccurredAt: at, EntryZone: v.EntryZone, DeviceOffsetSeconds: v.DeviceOffsetSeconds})
	if err != nil {
		return TransitionCommand{}, ErrMalformed
	}
	if len(v.Attachments) != 0 || len(v.SoftReferences) != 0 {
		return TransitionCommand{}, ErrUnsupported
	}
	return TransitionCommand{valid: true, value: TransitionInput{OperationID: op.ID(), PayloadDigest: op.PayloadDigest(), TripID: v.Entity.ID,
		AuthorMembershipID: v.MembershipID, Command: cmd, Expected: *v.Entity.ExpectedRevision, Reason: reason, Declared: declared,
		DependsOn: slices.Clone(v.DependsOn)}}, nil
}

// validReason is the METER-001 reason profile shared with the domain.
func validReason(raw string) error { _, err := meter.NewMissing(raw); return err }

// TransitionRecord is one stored lifecycle row of a trip.
type TransitionRecord struct {
	private
	Revision                                                                       int64
	Command                                                                        trip.Command
	From, To                                                                       trip.State
	Reason                                                                         *string
	OperationID, PayloadDigest, AuthorMembershipID, AuthorEmployeeID, EnrollmentID string
	Declared                                                                       orgtime.Declaration
	ReceivedAt                                                                     time.Time
}

// Revision is the stored trip's operational revision: 1 after the start, one
// more per lifecycle row or finish.
func (r Record) Revision() int64 {
	revision := 1 + int64(len(r.Transitions))
	if r.Finish != nil {
		revision++
	}
	return revision
}

// replayDomain rebuilds the stored start and its lifecycle rows through the
// pure domain; open stops before a final cancellation, giving the trip as a
// finish would find it. Each row must be exactly the domain's next fact.
func replayDomain(scope kernel.OrgScope, record Record, open bool) (trip.Trip, error) {
	current, err := startedDomain(scope, record)
	if err != nil {
		return trip.Trip{}, err
	}
	for _, x := range record.Transitions {
		if open && x.Command == trip.CommandCancel {
			break
		}
		author, err := trip.NewMembershipRef(scope, x.AuthorMembershipID)
		if err != nil {
			return trip.Trip{}, ErrUnavailable
		}
		declared := x.Declared
		next, err := applyCommand(current, scope, x.Command, trip.LifecycleInput{Author: author, Expected: current.Revision(), Reason: copyPointer(x.Reason), Declared: &declared})
		if err != nil || next.Revision() != x.Revision || next.State() != x.To || current.State() != x.From {
			return trip.Trip{}, ErrUnavailable
		}
		current = next
	}
	return current, nil
}

// openDomain is the stored trip as a finish finds it: its start and lifecycle
// rows, without a final cancellation.
func openDomain(scope kernel.OrgScope, record Record) (trip.Trip, error) {
	return replayDomain(scope, record, true)
}

func applyCommand(t trip.Trip, scope kernel.OrgScope, cmd trip.Command, in trip.LifecycleInput) (trip.Trip, error) {
	switch cmd {
	case trip.CommandPause:
		return t.Pause(scope, in)
	case trip.CommandResume:
		return t.Resume(scope, in)
	case trip.CommandCancel:
		return t.Cancel(scope, in)
	}
	return trip.Trip{}, ErrUnavailable
}

// TransitionDomain applies the pure lifecycle rules to the stored trip at
// receipt (B-BR-EMP-390/391); the stored trip is never changed.
func TransitionDomain(scope kernel.OrgScope, record Record, in TransitionInput) (trip.Trip, error) {
	stored, err := StoredDomain(scope, record)
	if err != nil {
		return trip.Trip{}, ErrUnavailable
	}
	author, err := trip.NewMembershipRef(scope, in.AuthorMembershipID)
	if err != nil {
		return trip.Trip{}, err
	}
	declared := in.Declared
	return applyCommand(stored, scope, in.Command, trip.LifecycleInput{Author: author, Expected: in.Expected, Reason: copyPointer(in.Reason), Declared: &declared})
}

// TransitionFacts are the identity facts of a lifecycle operation read under the locks.
type TransitionFacts struct {
	private
	ExistingOperation, HeldOperation string
}

// LifecycleStore persists lifecycle rows and held lifecycle commands in the
// intake transaction (EDR-253) and proves held receipts (OFFL-008).
type LifecycleStore interface {
	TransitionFactsTx(context.Context, syncapp.DBTX, kernel.OrgScope, string) (TransitionFacts, error)
	InsertTransitionTx(context.Context, syncapp.DBTX, kernel.OrgScope, syncapp.Source, TransitionCommand, trip.Trip, string) error
	HoldTransitionTx(context.Context, syncapp.DBTX, syncapp.Source, protocol.Operation, TransitionCommand, string) (string, error)
	syncapp.ReviewProof
}
