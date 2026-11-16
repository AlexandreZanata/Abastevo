// Package application owns the stored expense record, the signed
// expense.submit command for DIRECT expenses and the fueling-to-expense
// projection rules (EMP-EXP-002, B-BR-EMP-444..449, EDR-287..290).
package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
)

const (
	// OperationType is the signed DIRECT expense operation (EDR-289).
	OperationType = "expense.submit"
	EntityType    = "expense"
	// FuelCategory is the category of every FUELING expense (EDR-288).
	FuelCategory = "FUEL"
	// SourceRevision is the only fueling revision until FUEL-003/015.
	SourceRevision int64 = 1
	// Submitted is the stored review state of a new expense (B-BR-EMP-444).
	Submitted = "SUBMITTED"
)

var (
	// ErrMalformed is a final MALFORMED refusal: nothing is stored.
	ErrMalformed = errors.New("expense: malformed operation")
	// ErrUnsupported is a recognised form whose owner does not exist yet: the
	// operation stays RETRY/UNAVAILABLE with nothing stored (EDR-289).
	ErrUnsupported = errors.New("expense: recognised form unavailable")
	ErrUnavailable = errors.New("expense: unavailable")
	ErrNotFound    = errors.New("expense: not found")
	// ErrStale refuses a projection older than the stored expense (EDR-290).
	ErrStale = errors.New("expense: stale projection")
)

// DirectCategories is the v1 catalogue of DIRECT expenses until EMP-EXP-003
// (EDR-288): the REQ-11.1 names other than fuel, sorted.
func DirectCategories() []string {
	return []string{"FINE", "INSURANCE", "LEASE", "MAINTENANCE", "OTHER", "PARKING", "TOLL"}
}

type private struct{}

func (private) String() string                 { return "EnterpriseExpenseIntake(redacted)" }
func (p private) Format(f fmt.State, _ rune)   { _, _ = io.WriteString(f, p.String()) }
func (p private) LogValue() slog.Value         { return slog.StringValue(p.String()) }
func (p private) MarshalJSON() ([]byte, error) { return json.Marshal(p.String()) }

func canonicalID(raw string) bool {
	v, e := kernel.ParseOrgID(raw)
	return e == nil && v.String() == raw
}

// Action selects the permission of a current check.
type Action uint8

const (
	RecordAction Action = iota + 1
	ReadAction
)

// Policy is the owning permission of each action (07-permissions).
func Policy(action Action) access.Policy {
	switch action {
	case RecordAction:
		return access.Policy{Permission: "exp.expense.record", Capability: "organizations", Effect: access.OperationalWrite}
	case ReadAction:
		return access.Policy{Permission: "exp.expense.read", Capability: "organizations", Effect: access.Read}
	}
	return access.Policy{}
}

// Disposition is the retained-history answer for a new direct expense.
type Disposition string

const (
	Apply                 Disposition = "APPLY"
	HoldPermissionChanged Disposition = "HOLD_PERMISSION_CHANGED"
	HoldDisposalEnded     Disposition = "HOLD_DISPOSAL_ENDED"
)

// Target carries the server-resolved facts of a current check. An empty
// ExpenseID selects the organization root (malformed admission only).
type Target struct {
	private
	ExpenseID, AssetID, UnitID, CostCentreID string
}

// Authority binds the signed device source to the owning permissions in the
// borrowed intake transaction (B-BR-EMP-448).
type Authority interface {
	CheckExpenseTx(context.Context, syncapp.DBTX, kernel.OrgScope, syncapp.Source, Target, Action) error
	ClassifyExpenseTx(context.Context, syncapp.DBTX, kernel.OrgScope, syncapp.Source, string, time.Time) (Disposition, error)
}

// Lifecycle takes the organization root and asset locks and answers the
// asset's disposal instant (the fleet lifecycle port of METER-002).
type Lifecycle interface {
	MeterAssetDisposalTx(context.Context, syncapp.DBTX, kernel.OrgScope, string) (*time.Time, error)
}

// CommandInput is one parsed, validated expense.submit operation.
type CommandInput struct {
	private
	OperationID, PayloadDigest, ExpenseID, AssetID, AuthorMembershipID string
	Declared                                                           orgtime.Declaration
	Category, Payer                                                    string
	Paid, Company                                                      int64
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
	return c.value, nil
}

// Record is a stored expense as read back from its owner.
type Record struct {
	private
	ID, OriginKind, SourceID, FuelingID                                 string
	SourceRevision                                                      int64
	OperationID, PayloadDigest, AssetID                                 string
	AuthorMembershipID, AuthorEmployeeID, EnrollmentID, Category, Payer string
	Paid, Company                                                       int64
	Declared                                                            orgtime.Declaration
	Reporting                                                           orgtime.ReportingAttribution
	UnitID, CostCentreID, ReviewState                                   string
	ReceivedAt                                                          time.Time
	Revision                                                            int64
}

// Prepared are the admission facts read under the root and asset locks.
type Prepared struct {
	private
	AuthorEmployeeID, AssetState, ExistingOperation, UnitID, CostCentreID string
	Reporting                                                             orgtime.ReportingAttribution
}

// Store persists direct expenses in the borrowed intake transaction.
type Store interface {
	AuthorEmployeeTx(context.Context, syncapp.DBTX, kernel.OrgScope, string) (string, error)
	PrepareExpenseTx(context.Context, syncapp.DBTX, kernel.OrgScope, syncapp.Source, Command) (Prepared, error)
	InsertDirectTx(context.Context, syncapp.DBTX, kernel.OrgScope, syncapp.Source, Command, Prepared) error
	ReadExpenseTx(context.Context, syncapp.DBTX, kernel.OrgScope, string) (Record, error)
}

// FuelingSource is the stored fueling a FUELING expense is projected from.
type FuelingSource struct {
	private
	ID, OperationID, PayloadDigest                              string
	Revision                                                    int64
	AssetID, AuthorMembershipID, AuthorEmployeeID, EnrollmentID string
	Declared                                                    orgtime.Declaration
	Paid                                                        int64
	Payer, UnitID, CostCentreID, ReviewState                    string
}

// ProjectionDecision is the EDR-290 answer for one requested projection.
type ProjectionDecision string

const (
	Insert    ProjectionDecision = "INSERT"
	Unchanged ProjectionDecision = "UNCHANGED"
)
