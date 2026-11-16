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

// StartInput is a parsed trip.start.
type StartInput struct {
	Event
	AssetID, DriverID string
}

// FinishInput is a parsed trip.finish; its trip and asset come from storage.
type FinishInput struct {
	Event
	DependsOn []string
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
	syncapp.ReviewProof
}
