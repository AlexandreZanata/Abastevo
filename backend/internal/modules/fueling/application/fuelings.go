// Package application owns the signed fueling.submit command, its ports and
// the stored fueling record (EMP-FUEL-002, B-BR-EMP-263..267, EDR-175..177).
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
	fuel "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
)

const (
	// OperationType is the protocol catalog name (10-offline-sync §3, EDR-175).
	OperationType = "fueling.submit"
	EntityType    = "fueling"
)

var (
	// ErrMalformed is a final MALFORMED refusal: nothing is stored.
	ErrMalformed = errors.New("fueling: malformed operation")
	// ErrUnsupported is a recognised form whose owner does not exist yet: the
	// operation stays RETRY/UNAVAILABLE with nothing stored (EDR-175).
	ErrUnsupported = errors.New("fueling: recognised form unavailable")
	ErrUnavailable = errors.New("fueling: unavailable")
	ErrNotFound    = errors.New("fueling: not found")
)

type private struct{}

func (private) String() string                 { return "EnterpriseFuelingIntake(redacted)" }
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

// Action selects the permission of a current check.
type Action uint8

const (
	RecordAction Action = iota + 1
	ReadAction
)

// Policy is the owning permission of each action (fuelings §7).
func Policy(action Action) access.Policy {
	switch action {
	case RecordAction:
		return access.Policy{Permission: "fuel.fueling.record", Capability: "organizations", Effect: access.OperationalWrite}
	case ReadAction:
		return access.Policy{Permission: "fuel.fueling.read", Capability: "organizations", Effect: access.Read}
	}
	return access.Policy{}
}

// Disposition is the retained-history answer for a new fueling at receipt.
type Disposition string

const (
	Apply                 Disposition = "APPLY"
	HoldPermissionChanged Disposition = "HOLD_PERMISSION_CHANGED"
	HoldDisposalEnded     Disposition = "HOLD_DISPOSAL_ENDED"
)

// Target carries the server-resolved facts of a current check. An empty
// FuelingID selects the organization root (malformed admission only).
type Target struct {
	private
	FuelingID, AssetID, UnitID, CostCentreID string
}

// Authority binds the signed device source to the owning permissions in the
// borrowed intake transaction (B-BR-EMP-266, EDR-177).
type Authority interface {
	// CheckFuelingTx proves current authority over a stored or would-be
	// fueling (ReadAction) or over the root selector (empty FuelingID).
	CheckFuelingTx(context.Context, syncapp.DBTX, kernel.OrgScope, syncapp.Source, Target, Action) error
	// ClassifyFuelingTx decides a new fueling from retained history: APPLY,
	// a hold, FORBIDDEN (never covered) or unavailable.
	ClassifyFuelingTx(context.Context, syncapp.DBTX, kernel.OrgScope, syncapp.Source, string, bool, time.Time) (Disposition, error)
}

// Readings is the fleet embedded-reading port plus its lookups (METER-002).
type Readings interface {
	fleetapp.EmbeddedMeterRepository
	fleetapp.MeterIntakeLookup
	fleetapp.MeterAssetLifecycle
}

// Reading is the observed meter value declared with a fueling.
type Reading struct {
	private
	MeterID, SegmentID, ReadingID string
	Value                         meter.Value
}

// CommandInput is one parsed, validated fueling.submit operation.
type CommandInput struct {
	private
	OperationID, PayloadDigest, FuelingID, AssetID, ResponsibleID, AuthorMembershipID string
	Declared                                                                          orgtime.Declaration
	Candidate                                                                         fuel.Candidate
	Reading                                                                           *Reading
	MissingReason, PlaceName                                                          *string
}

type Command struct {
	private
	value CommandInput
	valid bool
}

func (c Command) Snapshot() (CommandInput, error) {
	if !c.valid {
		return CommandInput{}, ErrUnavailable
	}
	v := c.value
	v.Reading, v.MissingReason, v.PlaceName = copyPointer(v.Reading), copyPointer(v.MissingReason), copyPointer(v.PlaceName)
	return v, nil
}

// MeterCommand is the embedded reading, origin FUELING, under the fueling's
// operation and declaration (B-BR-EMP-265); false when a reason was given.
func (c Command) MeterCommand() (fleetapp.MeterReadingCommand, bool, error) {
	v, err := c.Snapshot()
	if err != nil {
		return fleetapp.MeterReadingCommand{}, false, err
	}
	if v.Reading == nil {
		return fleetapp.MeterReadingCommand{}, false, nil
	}
	command, err := fleetapp.NewMeterReadingCommand(fleetapp.MeterReadingInput{OperationID: v.OperationID, AssetID: v.AssetID, MeterID: v.Reading.MeterID, SegmentID: v.Reading.SegmentID, ReadingID: v.Reading.ReadingID, Value: v.Reading.Value, Declared: v.Declared, Origin: meter.Fueling, Method: meter.Typed})
	if err != nil {
		return fleetapp.MeterReadingCommand{}, false, ErrMalformed
	}
	return command, true, nil
}

// Record is a stored fueling as read back from its owner.
type Record struct {
	private
	ID, OperationID, PayloadDigest, AssetID, ResponsibleID, AuthorMembershipID, AuthorEmployeeID, EnrollmentID string
	Declared                                                                                                   orgtime.Declaration
	ReceivedAt                                                                                                 time.Time
	Product, Unit                                                                                              string
	Quantity, Paid, Gross, Revision                                                                            int64
	Discount, UnitPrice, Expected, Difference                                                                  *int64
	UnitPriceText, PlaceName, MissingReason                                                                    *string
	Fill, Payer, Payment, Origin, Derived, ReviewState                                                         string
	MeterID, SegmentID, ReadingID, UnitID, CostCentreID                                                        string
	Warnings                                                                                                   []string
}

// Prepared are the admission facts read under the root and asset locks.
type Prepared struct {
	private
	AuthorEmployeeID, AssetState, ExistingOperation, UnitID, CostCentreID string
	ResponsibleActive, ProductAccepted                                    bool
}

// CostProjection stores the one FUELING expense of a fueling inserted in
// the same intake transaction (EMP-EXP-002, EDR-287): a fueling never
// commits without its cost, and a failure rolls both back.
type CostProjection interface {
	ProjectFuelingTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, fueling string, revision int64) error
}

// Store persists fuelings in the borrowed intake transaction.
type Store interface {
	AuthorEmployeeTx(context.Context, syncapp.DBTX, kernel.OrgScope, string) (string, error)
	PrepareFuelingTx(context.Context, syncapp.DBTX, kernel.OrgScope, syncapp.Source, Command) (Prepared, error)
	InsertFuelingTx(context.Context, syncapp.DBTX, kernel.OrgScope, syncapp.Source, Command, Prepared, fuel.Arithmetic) error
	ReadFuelingTx(context.Context, syncapp.DBTX, kernel.OrgScope, string) (Record, error)
}
