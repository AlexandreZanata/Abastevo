package application

import (
	"context"
	"time"

	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
)

const (
	// EventOperationType appends one typed logbook fact to a trip (EMP-TRIP-003).
	EventOperationType = "trip.event.record"
	EventEntityType    = "trip_event"
)

// Exact schema 1 payload (EDR-207).
var eventKeys = []string{"trip_id", "type", "text"}

// EventInput is a parsed trip.event.record; its trip comes from storage.
type EventInput struct {
	private
	OperationID, PayloadDigest, EventID, TripID, AuthorMembershipID string
	Type                                                            trip.LogbookType
	Text                                                            *string
	Declared                                                        orgtime.Declaration
	DependsOn                                                       []string
}

type EventCommand struct {
	private
	value EventInput
	valid bool
}

func (c EventCommand) Snapshot() (EventInput, error) {
	if !c.valid {
		return EventInput{}, ErrUnavailable
	}
	v := c.value
	v.Text = copyPointer(v.Text)
	v.DependsOn = append([]string(nil), v.DependsOn...)
	return v, nil
}

// ParseEvent decodes one trip.event.record schema 1 operation (B-BR-EMP-321).
// ErrMalformed is final (unknown type included); ErrUnsupported keeps a
// reserved type, a soft reference or an attachment unavailable.
func ParseEvent(op protocol.Operation) (EventCommand, error) {
	v := op.Snapshot()
	if v.Type != EventOperationType || !op.SupportsSchema() || v.Entity.Type != EventEntityType || v.Entity.ExpectedRevision != nil || !canonicalID(v.Entity.ID) || len(v.DependsOn) > 1 {
		return EventCommand{}, ErrMalformed
	}
	fields, ok := object(op.PayloadBytes(), eventKeys)
	if !ok {
		return EventCommand{}, ErrMalformed
	}
	tripID, okTrip := text(fields, "trip_id")
	raw, okType := text(fields, "type")
	note, okText := optional(fields, "text")
	at, err := orgtime.ParseInstant(v.OccurredAt)
	if !okTrip || !okType || !okText || err != nil || !canonicalID(tripID) {
		return EventCommand{}, ErrMalformed
	}
	declared, err := orgtime.ValidateDeclaration(orgtime.Declaration{OccurredAt: at, EntryZone: v.EntryZone, DeviceOffsetSeconds: v.DeviceOffsetSeconds})
	if err != nil {
		return EventCommand{}, ErrMalformed
	}
	kind, err := trip.ParseLogbookType(raw)
	if err != nil {
		return EventCommand{}, ErrMalformed
	}
	// EMP-CHECK-003 (EDR-240): incident and damage facts are their own
	// operation, check.incident.report, like the start and the finish.
	if kind == trip.LogbookIncident || kind == trip.LogbookDamage {
		return EventCommand{}, ErrMalformed
	}
	if !kind.Supported() {
		return EventCommand{}, ErrUnsupported
	}
	if trip.ValidLogbookText(kind, note) != nil {
		return EventCommand{}, ErrMalformed
	}
	if len(v.Attachments) != 0 || len(v.SoftReferences) != 0 {
		return EventCommand{}, ErrUnsupported
	}
	return EventCommand{valid: true, value: EventInput{OperationID: op.ID(), PayloadDigest: op.PayloadDigest(), EventID: v.Entity.ID, TripID: tripID,
		AuthorMembershipID: v.MembershipID, Type: kind, Text: note, Declared: declared, DependsOn: v.DependsOn}}, nil
}

// StoredDomain rebuilds a stored trip, with its finish when completed,
// through the pure EMP-TRIP-001 domain.
func StoredDomain(scope kernel.OrgScope, record Record) (trip.Trip, error) {
	// EMP-TRIP-014: a trip closed by a supervisor (never beside a finish).
	if record.Closure != nil {
		return closedDomain(scope, record)
	}
	f := record.Finish
	if f == nil {
		return startedDomain(scope, record)
	}
	in := FinishInput{Event: Event{OperationID: f.OperationID, PayloadDigest: f.PayloadDigest, TripID: record.ID, AuthorMembershipID: f.AuthorMembershipID,
		Declared: f.Declared, MissingReason: copyPointer(f.MissingReason), Clock: copyPointer(f.Clock)}}
	if f.ReadingID != "" {
		if f.ReadingValue == nil || f.ReadingKind == "" {
			return trip.Trip{}, ErrUnavailable
		}
		value, err := meter.NewValue(meter.Kind(f.ReadingKind), *f.ReadingValue, nil)
		if err != nil {
			return trip.Trip{}, ErrUnavailable
		}
		in.Reading = &Reading{MeterID: f.MeterID, SegmentID: f.SegmentID, ReadingID: f.ReadingID, Value: value}
	}
	finished, err := FinishDomain(scope, record, in)
	if err != nil || finished.Crossing().EndDay() != f.EndDay {
		return trip.Trip{}, ErrUnavailable
	}
	return finished, nil
}

// EventDomain applies the pure logbook rules to the stored trip at receipt
// (B-BR-EMP-323); the stored trip is never changed.
func EventDomain(scope kernel.OrgScope, record Record, in EventInput) (trip.LogbookEntry, error) {
	stored, err := StoredDomain(scope, record)
	if err != nil {
		return trip.LogbookEntry{}, ErrUnavailable
	}
	ref, err := trip.NewLogbookRef(scope, in.EventID)
	if err != nil {
		return trip.LogbookEntry{}, err
	}
	author, err := trip.NewMembershipRef(scope, in.AuthorMembershipID)
	if err != nil {
		return trip.LogbookEntry{}, err
	}
	declared := in.Declared
	return stored.Log(scope, trip.LogbookInput{Reference: ref, Author: author, Type: string(in.Type), Text: copyPointer(in.Text), Declared: &declared})
}

// EventFacts are the identity facts of an event read under the locks.
type EventFacts struct {
	private
	ExistingOperation, HeldOperation string
}

// EventRecord is one stored logbook fact.
type EventRecord struct {
	private
	ID, TripID, OperationID, PayloadDigest, AuthorMembershipID, AuthorEmployeeID, EnrollmentID string
	Type                                                                                       trip.LogbookType
	Text                                                                                       *string
	Declared                                                                                   orgtime.Declaration
	ReceivedAt                                                                                 time.Time
}

// TimelineEntry is one item of a trip's timeline: the fact type (START,
// FINISH or a logbook type), its actor, declaration and server receipt.
type TimelineEntry struct {
	private
	Item               trip.TimelineItem
	FactType           string
	AuthorMembershipID string
	Declared           orgtime.Declaration
	ReceivedAt         time.Time
	Text               *string
}

// TimelineReport is one EMP-CHECK-003 report of a trip as a timeline fact
// (EDR-240): its identity, fact type (INCIDENT or DAMAGE), reporter,
// declaration and receipt; never its description.
type TimelineReport struct {
	private
	ID                 string
	Type               trip.LogbookType
	AuthorMembershipID string
	Declared           orgtime.Declaration
	ReceivedAt         time.Time
}

// TimelineReports supplies the accepted reports of one stored trip; the
// timeline reads them from their owner and never copies them (EDR-240).
type TimelineReports interface {
	TripReportsTx(context.Context, syncapp.DBTX, kernel.OrgScope, string) ([]TimelineReport, error)
}

// EventStore persists logbook facts and held events in the intake
// transaction (EDR-208) and proves held receipts (OFFL-008).
type EventStore interface {
	EventFactsTx(context.Context, syncapp.DBTX, kernel.OrgScope, string) (EventFacts, error)
	InsertEventTx(context.Context, syncapp.DBTX, kernel.OrgScope, syncapp.Source, EventCommand, string) error
	HoldEventTx(context.Context, syncapp.DBTX, syncapp.Source, protocol.Operation, EventCommand, string) (string, error)
	ReadEventTx(context.Context, syncapp.DBTX, kernel.OrgScope, string) (EventRecord, error)
	// ReviewEntriesTx reads the trip's review history: a late fact on a
	// submitted, decided or amended revision is held (EMP-TRIP-015).
	ReviewEntriesTx(context.Context, syncapp.DBTX, kernel.OrgScope, string) ([]ReviewEntry, error)
	syncapp.ReviewProof
}
