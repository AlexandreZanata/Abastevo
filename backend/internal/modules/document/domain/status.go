// Package domain models a private enterprise document container: original
// identity, six independent status dimensions, owned links and exact
// allocations. It performs no I/O and never holds content bytes.
package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
)

var (
	ErrInvalid        = errors.New("document: invalid input")
	ErrScope          = errors.New("document: organization scope denied")
	ErrTransition     = errors.New("document: transition not allowed")
	ErrConflict       = errors.New("document: current state differs")
	ErrDerived        = errors.New("document: derived status")
	ErrDuplicate      = errors.New("document: duplicate")
	ErrNotLinked      = errors.New("document: target not linked")
	ErrNotAllocatable = errors.New("document: target not allocatable")
	ErrExceedsAmount  = errors.New("document: allocations exceed amount")
	ErrAmountMissing  = errors.New("document: amount missing")
)

type redacted struct{}

func (redacted) String() string                 { return "EnterpriseDocumentValue(redacted)" }
func (r redacted) Format(f fmt.State, _ rune)   { _, _ = io.WriteString(f, r.String()) }
func (r redacted) LogValue() slog.Value         { return slog.StringValue(r.String()) }
func (r redacted) MarshalJSON() ([]byte, error) { return json.Marshal(r.String()) }

type Dimension string

const (
	Storage        Dimension = "STORAGE"
	Extraction     Dimension = "EXTRACTION"
	Structure      Dimension = "STRUCTURE"
	Conference     Dimension = "CONFERENCE"
	Reconciliation Dimension = "RECONCILIATION"
	Fiscal         Dimension = "FISCAL"
)

var dimensions = [...]Dimension{Storage, Extraction, Structure, Conference, Reconciliation, Fiscal}

// ParseDimension accepts only the exact closed dimension tokens.
func ParseDimension(raw string) (Dimension, error) {
	for _, d := range dimensions {
		if string(d) == raw {
			return d, nil
		}
	}
	return "", ErrInvalid
}

// Derived reports the one dimension computed from exact amounts, never commanded.
func (d Dimension) Derived() bool { return d == Reconciliation }

// State is a token of one dimension; the dimension decides its meaning.
type State string

const (
	Uploading          State = "UPLOADING"
	Received           State = "RECEIVED"
	Stored             State = "STORED"
	Quarantined        State = "QUARANTINED"
	StorageFailed      State = "FAILED"
	Replaced           State = "REPLACED"
	Deleted            State = "DELETED"
	ExtractionNone     State = "NONE"
	ExtractionPending  State = "PENDING"
	ExtractionDone     State = "DONE"
	ExtractionFailed   State = "FAILED"
	NotApplicable      State = "NOT_APPLICABLE"
	Unchecked          State = "UNCHECKED"
	StructureOK        State = "STRUCTURE_OK"
	StructureInvalid   State = "STRUCTURE_INVALID"
	Unconferred        State = "UNCONFERRED"
	ConferredOK        State = "CONFERRED_OK"
	Divergent          State = "DIVERGENT"
	Illegible          State = "ILLEGIBLE"
	Unallocated        State = "UNALLOCATED"
	PartiallyAllocated State = "PARTIALLY_ALLOCATED"
	Reconciled         State = "RECONCILED"
	OverAllocated      State = "OVER_ALLOCATED"
	NotConsulted       State = "NOT_CONSULTED"
	AuthorizedAsOf     State = "AUTHORIZED_AS_OF"
	CancelledAsOf      State = "CANCELLED_AS_OF"
	Unavailable        State = "UNAVAILABLE"
)

// states lists each closed dimension; the first state is the initial one.
var states = map[Dimension][]State{
	Storage:        {Uploading, Received, Stored, Quarantined, StorageFailed, Replaced, Deleted},
	Extraction:     {ExtractionNone, ExtractionPending, ExtractionDone, ExtractionFailed},
	Structure:      {NotApplicable, Unchecked, StructureOK, StructureInvalid},
	Conference:     {Unconferred, ConferredOK, Divergent, Illegible},
	Reconciliation: {Unallocated, PartiallyAllocated, Reconciled, OverAllocated},
	Fiscal:         {NotConsulted, AuthorizedAsOf, CancelledAsOf, Unavailable},
}

// transitions holds the generic commands (EDR-134), sorted by target.
// REPLACED is entered only through Replace; reconciliation has no commands.
var transitions = map[Dimension]map[State][]State{
	Storage: {
		Uploading:   {StorageFailed, Received},
		Received:    {StorageFailed, Quarantined, Stored},
		Stored:      {Deleted},
		Quarantined: {Deleted},
		Replaced:    {Deleted},
	},
	Extraction: {
		ExtractionNone:    {ExtractionPending},
		ExtractionPending: {ExtractionDone, ExtractionFailed},
		ExtractionFailed:  {ExtractionPending},
	},
	Structure: {
		NotApplicable:    {Unchecked},
		Unchecked:        {StructureInvalid, StructureOK},
		StructureInvalid: {Unchecked},
	},
	Conference: {
		Unconferred: {ConferredOK, Divergent, Illegible},
		ConferredOK: {Divergent, Illegible, Unconferred},
		Divergent:   {ConferredOK, Illegible, Unconferred},
		Illegible:   {ConferredOK, Divergent, Unconferred},
	},
	Fiscal: {
		NotConsulted:   {AuthorizedAsOf, CancelledAsOf, Unavailable},
		AuthorizedAsOf: {AuthorizedAsOf, CancelledAsOf, Unavailable},
		CancelledAsOf:  {AuthorizedAsOf, CancelledAsOf, Unavailable},
		Unavailable:    {AuthorizedAsOf, CancelledAsOf, Unavailable},
	},
}

// ParseState accepts only an exact token of the given dimension; a token of
// another dimension, a padded/lowercase token or "VALIDATED" is INVALID.
func ParseState(d Dimension, raw string) (State, error) {
	for _, s := range states[d] {
		if string(s) == raw {
			return s, nil
		}
	}
	return "", ErrInvalid
}

// Initial returns the state of a newly created document in that dimension.
func Initial(d Dimension) (State, error) {
	if _, e := ParseDimension(string(d)); e != nil {
		return "", ErrInvalid
	}
	return states[d][0], nil
}

// Allowed returns the generic command targets from a state, sorted.
func Allowed(d Dimension, from State) ([]State, error) {
	if _, e := ParseState(d, string(from)); e != nil {
		return nil, ErrInvalid
	}
	if d.Derived() {
		return nil, ErrDerived
	}
	return append([]State(nil), transitions[d][from]...), nil
}

func allowed(d Dimension, from, to State) bool {
	for _, s := range transitions[d][from] {
		if s == to {
			return true
		}
	}
	return false
}

type TargetKind string

const (
	Fueling      TargetKind = "FUELING"
	Expense      TargetKind = "EXPENSE"
	WorkOrder    TargetKind = "WORK_ORDER"
	Trip         TargetKind = "TRIP"
	Asset        TargetKind = "ASSET"
	Employee     TargetKind = "EMPLOYEE"
	MeterReading TargetKind = "METER_READING"
)

var targetKinds = [...]TargetKind{Fueling, Expense, WorkOrder, Trip, Asset, Employee, MeterReading}

// ParseTargetKind accepts only the closed link target kinds (EDR-135).
func ParseTargetKind(raw string) (TargetKind, error) {
	for _, k := range targetKinds {
		if string(k) == raw {
			return k, nil
		}
	}
	return "", ErrInvalid
}

// Allocatable reports whether money may be allocated to the kind: only an
// expense, the single cost of a fueling or work order (EDR-013).
func (k TargetKind) Allocatable() bool { return k == Expense }
