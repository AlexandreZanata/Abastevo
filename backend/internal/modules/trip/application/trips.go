// Package application holds the trip intake commands, codecs and ports of
// EMP-TRIP-002: the signed trip.start and trip.finish operations, their
// authority, reading and storage ports. It never decides authority itself.
package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	fleetapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
)

const (
	StartOperationType  = "trip.start"
	FinishOperationType = "trip.finish"
	EntityType          = "trip"
	// Held review codes of the OFFL-001 vocabulary (B-BR-EMP-313/315).
	AssetBusy         = "ASSET_BUSY"
	AssetUnavailable  = "ASSET_UNAVAILABLE"
	PermissionChanged = "PERMISSION_CHANGED"
	TripAlreadyClosed = "TRIP_ALREADY_CLOSED"
	// OrphanReference holds a start whose mission reference was missing,
	// foreign or ineligible at receipt (EMP-TRIP-023, BR-OFFL-007).
	OrphanReference = "ORPHAN_REFERENCE"
	// MissionReferenceType is the soft-reference type naming the mission a
	// start executes (EMP-TRIP-023, EDR-265).
	MissionReferenceType = "mission"
)

var (
	// ErrMalformed is a final MALFORMED refusal of the operation.
	ErrMalformed = errors.New("trip: malformed operation")
	// ErrUnsupported keeps a recognised future form unavailable.
	ErrUnsupported = errors.New("trip: recognised form unavailable")
	ErrUnavailable = errors.New("trip: unavailable")
	ErrNotFound    = errors.New("trip: not found")
)

type private struct{}

func (private) String() string                 { return "EnterpriseTripIntake(redacted)" }
func (p private) Format(f fmt.State, _ rune)   { _, _ = io.WriteString(f, p.String()) }
func (p private) LogValue() slog.Value         { return slog.StringValue(p.String()) }
func (p private) MarshalJSON() ([]byte, error) { return json.Marshal(p.String()) }

func canonicalID(raw string) bool {
	v, e := kernel.ParseOrgID(raw)
	return e == nil && v.String() == raw
}

func copyPointer[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

// Action selects the write (start/finish) or receipt-read policy.
type Action uint8

const (
	RecordAction Action = iota + 1
	ReadAction
)

// Policy is trip.trip.start for both events and trip.trip.read for receipts.
func Policy(action Action) access.Policy {
	switch action {
	case RecordAction:
		return access.Policy{Permission: "trip.trip.start", Capability: "organizations", Effect: access.OperationalWrite}
	case ReadAction:
		return access.Policy{Permission: "trip.trip.read", Capability: "organizations", Effect: access.Read}
	}
	return access.Policy{}
}

// Disposition is the EDR-118 history answer for a new trip event.
type Disposition string

const (
	Apply                 Disposition = "APPLY"
	HoldPermissionChanged Disposition = "HOLD_PERMISSION_CHANGED"
	HoldDisposalEnded     Disposition = "HOLD_DISPOSAL_ENDED"
)

// Target names server-resolved record facts; an empty TripID is the root.
type Target struct {
	private
	TripID, AssetID, UnitID, CostCentreID, AuthorMembershipID string
}

type Authority interface {
	CheckTripTx(context.Context, syncapp.DBTX, kernel.OrgScope, syncapp.Source, Target, Action) error
	ClassifyTripTx(context.Context, syncapp.DBTX, kernel.OrgScope, syncapp.Source, string, bool, time.Time) (Disposition, error)
}

// Readings is the fleet embedded-reading port with its lookups (FUEL-002).
type Readings interface {
	fleetapp.EmbeddedMeterRepository
	fleetapp.MeterIntakeLookup
	fleetapp.MeterAssetLifecycle
}

// Reading is the payload reading of an event.
type Reading struct {
	private
	MeterID, SegmentID, ReadingID string
	Value                         meter.Value
}

// Event is the parsed part shared by start and finish.
type Event struct {
	private
	OperationID, PayloadDigest, TripID, AuthorMembershipID string
	Declared                                               orgtime.Declaration
	Reading                                                *Reading
	MissingReason                                          *string
	Clock                                                  *trip.RunClock
}

func (e Event) copy() Event {
	e.Reading, e.MissingReason, e.Clock = copyPointer(e.Reading), copyPointer(e.MissingReason), copyPointer(e.Clock)
	return e
}

// StartInput is a parsed trip.start; MissionID is the mission named by its
// soft reference, "" when it names none (EMP-TRIP-023); DepartureRunID the
// departure checklist run it names, "" when none (EMP-TRIP-019).
type StartInput struct {
	Event
	AssetID, DriverID, MissionID, DepartureRunID string
}

// FinishInput is a parsed trip.finish; its trip and asset come from storage.
// ReturnRunID is the return checklist run it names, "" when none
// (EMP-TRIP-019).
type FinishInput struct {
	Event
	DependsOn   []string
	ReturnRunID string
}

type StartCommand struct {
	private
	value StartInput
	valid bool
}

type FinishCommand struct {
	private
	value FinishInput
	valid bool
}

func (c StartCommand) Snapshot() (StartInput, error) {
	if !c.valid {
		return StartInput{}, ErrUnavailable
	}
	v := c.value
	v.Event = v.Event.copy()
	return v, nil
}

func (c FinishCommand) Snapshot() (FinishInput, error) {
	if !c.valid {
		return FinishInput{}, ErrUnavailable
	}
	v := c.value
	v.Event = v.Event.copy()
	v.DependsOn = append([]string(nil), v.DependsOn...)
	return v, nil
}

// MeterCommand is the embedded fleet reading of an event on the asset.
func MeterCommand(e Event, asset string, origin meter.Origin) (fleetapp.MeterReadingCommand, bool, error) {
	if e.Reading == nil {
		return fleetapp.MeterReadingCommand{}, false, nil
	}
	command, err := fleetapp.NewMeterReadingCommand(fleetapp.MeterReadingInput{OperationID: e.OperationID, AssetID: asset, MeterID: e.Reading.MeterID,
		SegmentID: e.Reading.SegmentID, ReadingID: e.Reading.ReadingID, Value: e.Reading.Value, Declared: e.Declared, Origin: origin, Method: meter.Typed})
	if err != nil {
		return fleetapp.MeterReadingCommand{}, false, ErrMalformed
	}
	return command, true, nil
}

// StartFacts are the admission facts read under the root and asset locks.
type StartFacts struct {
	private
	AuthorEmployeeID, AssetState, AssetKind, ExistingOperation, HeldOperation, ActiveTripID string
	UnitID, CostCentreID                                                                    string
	DriverActive                                                                            bool
	DisposedAt                                                                              *time.Time
	Zones                                                                                   orgtime.ReportingZones
}

// FinishRecord is the stored finish fact of a trip.
type FinishRecord struct {
	private
	OperationID, PayloadDigest, AuthorMembershipID, EnrollmentID string
	Declared                                                     orgtime.Declaration
	Clock                                                        *trip.RunClock
	ReceivedAt                                                   time.Time
	MeterID, SegmentID, ReadingID, ReadingKind                   string
	ReadingValue                                                 *int64
	MissingReason                                                *string
	EndDay                                                       string
}

// Record is one stored trip with its optional finish or, for a trip closed by
// a supervisor, its closure (EMP-TRIP-014); never both.
type Record struct {
	private
	ID, OperationID, PayloadDigest, AssetID, DriverID, AuthorMembershipID, AuthorEmployeeID, EnrollmentID string
	Declared                                                                                              orgtime.Declaration
	Clock                                                                                                 *trip.RunClock
	ReceivedAt                                                                                            time.Time
	MeterID, SegmentID, ReadingID, ReadingKind, UnitID, CostCentreID                                      string
	ReadingValue                                                                                          *int64
	MissingReason                                                                                         *string
	Reporting                                                                                             orgtime.ReportingAttribution
	State                                                                                                 trip.State
	Finish                                                                                                *FinishRecord
	// Transitions are the trip's lifecycle rows in revision order (EMP-TRIP-011).
	Transitions []TransitionRecord
	// Closure is the supervisor closure of a trip completed without a finish
	// (EMP-TRIP-014); never beside a finish.
	Closure *ClosureRecord
	// Handovers are the trip's driver handovers in revision order (EMP-TRIP-012).
	Handovers []HandoverRecord
}

// Store persists trips, finishes and held facts in the intake transaction.
type Store interface {
	AuthorEmployeeTx(context.Context, syncapp.DBTX, kernel.OrgScope, string) (string, error)
	PrepareStartTx(context.Context, syncapp.DBTX, kernel.OrgScope, syncapp.Source, StartCommand) (StartFacts, error)
	InsertTripTx(context.Context, syncapp.DBTX, kernel.OrgScope, syncapp.Source, StartCommand, StartFacts, trip.Trip) error
	HoldStartTx(context.Context, syncapp.DBTX, syncapp.Source, protocol.Operation, StartCommand, string) (string, error)
	ReadTripTx(context.Context, syncapp.DBTX, kernel.OrgScope, string) (Record, error)
	InsertFinishTx(context.Context, syncapp.DBTX, kernel.OrgScope, syncapp.Source, FinishCommand, string) error
	HoldFinishTx(context.Context, syncapp.DBTX, syncapp.Source, protocol.Operation, FinishCommand, string) (string, error)
	// MissionForStartTx locks and reads the mission a start names
	// (EMP-TRIP-023); ErrNotFound for none in the organization.
	MissionForStartTx(context.Context, syncapp.DBTX, kernel.OrgScope, string) (MissionStart, error)
	// LinkMissionTx stores the accepted start's link to its eligible mission
	// and the mission's START revision in the start's transaction.
	LinkMissionTx(context.Context, syncapp.DBTX, syncapp.Source, StartCommand) error
	// ChecklistLinkedTx answers whether a trip is linked to the checklist
	// run (EMP-TRIP-019: one run, one trip).
	ChecklistLinkedTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, run string) (bool, error)
	// ChecklistLinkTx reads the run linked to the trip for the context kind,
	// "" when none.
	ChecklistLinkTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, trip, kind string) (string, error)
	// LinkChecklistTx stores the trip's departure (its start operation) or
	// return (its finish operation) run link in that operation's transaction.
	LinkChecklistTx(ctx context.Context, db syncapp.DBTX, source syncapp.Source, trip, kind, run, operation string) error
	// HoldChecklistTx stores the gate reason beside a held start; run is the
	// named run when it is stored in the organization, else "".
	HoldChecklistTx(ctx context.Context, db syncapp.DBTX, source syncapp.Source, operation, review, reason, run string) error
	syncapp.ReviewProof
}

// MissionStart are the facts of a named mission read at a start's receipt.
type MissionStart struct {
	private
	State                                    trip.MissionState
	AssigneeMembershipID, AssigneeEmployeeID string
	AssetID                                  string
	Linked                                   bool
}

// MissionEligible is the EMP-TRIP-023 eligibility of a start's mission
// reference at receipt (B-BR-EMP-412): an ACCEPTED mission whose current
// assignee is the start's author membership and driver employee, whose
// vehicle (when named) is the start's asset and which no trip executes yet.
func MissionEligible(m MissionStart, author, driver, asset string) bool {
	return m.State == trip.MissionAccepted && !m.Linked && m.AssigneeMembershipID != "" && m.AssigneeMembershipID == author &&
		m.AssigneeEmployeeID == driver && (m.AssetID == "" || m.AssetID == asset)
}

// StartAvailability is the FLEET-007 start decision with the EMP-TRIP-023
// consumption of a mission's reservation by its linked start.
type StartAvailability interface {
	fleetapp.StartAvailabilityPort
	fleetapp.MissionReservationFulfilment
}

// StartHeldCode reports the held codes of a trip start (B-BR-EMP-313/412).
func StartHeldCode(code string) bool {
	return code == AssetBusy || code == AssetUnavailable || code == PermissionChanged || code == OrphanReference
}
