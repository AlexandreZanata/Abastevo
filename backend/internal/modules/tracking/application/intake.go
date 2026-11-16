// Package application holds the tracking intake of EMP-GPS-004: the signed
// gps.session.* and gps.track.batch operations, their codecs, the replay of
// stored sessions through the EMP-GPS-001 domain, the server session bounds
// of a batch and the ports of the owning adapters. It never decides
// authority itself and holds no SQL.
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
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/domain"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
)

// Operation types and the entity of every tracking operation (EDR-277).
const (
	StartOperationType     = "gps.session.start"
	ResumeOperationType    = "gps.session.resume"
	SuspendOperationType   = "gps.session.suspend"
	InterruptOperationType = "gps.session.interrupt"
	ReduceOperationType    = "gps.session.reduce"
	EndOperationType       = "gps.session.end"
	BatchOperationType     = "gps.track.batch"
	EntityType             = "tracking_session"
	SourcePhone            = "PHONE"
	// Bounds of one batch operation (B-BR-EMP-431); the protocol bounds the
	// operation to 8 KiB and an upload request to 64 KiB and 50 operations.
	MaxFixes        = 200
	MaxOffsetMillis = 24 * 60 * 60 * 1000
	MaxSequence     = 1<<31 - 1
	MaxAccuracy     = 10000
	MaxSpeed        = 100000
	MaxDropped      = 1000000
	FlagCoarse      = 1
	FlagMock        = 2
)

// SessionOperationTypes lists the six session operations in a stable order.
func SessionOperationTypes() []string {
	return []string{StartOperationType, ResumeOperationType, SuspendOperationType, InterruptOperationType, ReduceOperationType, EndOperationType}
}

var (
	// ErrMalformed is a final MALFORMED refusal that stores nothing.
	ErrMalformed = errors.New("tracking: malformed operation")
	// ErrPolicyOff is the final FORBIDDEN refusal of an opening under OFF.
	ErrPolicyOff   = errors.New("tracking: organization tracking policy is off")
	ErrUnavailable = errors.New("tracking: unavailable")
	ErrNotFound    = errors.New("tracking: not found")
)

type private struct{}

func (private) String() string                 { return "EnterpriseTrackingIntake(redacted)" }
func (p private) Format(f fmt.State, _ rune)   { _, _ = io.WriteString(f, p.String()) }
func (p private) LogValue() slog.Value         { return slog.StringValue(p.String()) }
func (p private) MarshalJSON() ([]byte, error) { return json.Marshal(p.String()) }

// Authority is the trip intake authority: trip.trip.start for writes and
// trip.trip.read for receipts over the stored trip (EDR-278).
type Authority interface {
	CheckTripTx(context.Context, syncapp.DBTX, kernel.OrgScope, syncapp.Source, tripapp.Target, tripapp.Action) error
}

// Trips reads the stored trip a tracking operation names.
type Trips interface {
	ReadTripTx(context.Context, syncapp.DBTX, kernel.OrgScope, string) (tripapp.Record, error)
}

// PolicyVersion is the tracking policy in force at receipt; revision 0
// means no recorded version (OFF).
type PolicyVersion struct {
	Policy   domain.Policy
	Revision int64
}

// StoredTransition is one stored history row.
type StoredTransition struct {
	private
	Seq         int
	State       domain.State
	Cause       domain.Cause
	Actor       domain.Actor
	Permission  domain.Permission
	At          time.Time
	OperationID string
}

// SessionRecord is one stored session with its whole history.
type SessionRecord struct {
	private
	ID, TripID, OwnerMembershipID, OperationID string
	Policy                                     domain.Policy
	PolicyRevision                             int64
	OpenedAt                                   time.Time
	Transitions                                []StoredTransition
}

// Copy returns an independent record.
func (r SessionRecord) Copy() SessionRecord {
	r.Transitions = append([]StoredTransition(nil), r.Transitions...)
	return r
}

// SessionChange is what one accepted session operation stores: the session
// row when it opens, and its new transition rows.
type SessionChange struct {
	private
	Open        bool
	Session     SessionRecord
	Transitions []StoredTransition
}

// Fix is one decoded fix of a batch with its absolute instant and position.
type Fix struct {
	private
	At       time.Time
	Lat, Lon int32
	Accuracy int32
	Speed    *int32
	Bearing  *int16
	Flags    int16
}

// Dropped are the device's own dropped-fix counters.
type Dropped struct {
	Accuracy, Stale, Jump, Mock int32
}

// BatchInput is a parsed gps.track.batch.
type BatchInput struct {
	private
	OperationID, PayloadDigest, SessionID, TripID string
	Seq                                           int64
	Source                                        string
	BootID                                        *string
	BaseElapsed                                   *int64
	Base                                          time.Time
	BaseLat, BaseLon                              int32
	Fixes                                         []Fix
	Dropped                                       Dropped
}

func (b BatchInput) copy() BatchInput {
	b.BootID, b.BaseElapsed = copyPointer(b.BootID), copyPointer(b.BaseElapsed)
	fixes := make([]Fix, len(b.Fixes))
	for i, f := range b.Fixes {
		f.Speed, f.Bearing = copyPointer(f.Speed), copyPointer(f.Bearing)
		fixes[i] = f
	}
	b.Fixes = fixes
	return b
}

// BatchCommand is a validated batch.
type BatchCommand struct {
	private
	value BatchInput
	valid bool
}

// Snapshot returns an independent copy of a validated batch.
func (c BatchCommand) Snapshot() (BatchInput, error) {
	if !c.valid {
		return BatchInput{}, ErrUnavailable
	}
	return c.value.copy(), nil
}

// SessionInput is a parsed session operation.
type SessionInput struct {
	private
	Type, OperationID, PayloadDigest, SessionID, TripID, AuthorMembershipID string
	Declared                                                                time.Time
	Origin                                                                  domain.Origin
	Permission                                                              domain.Permission
	NotificationVisible                                                     bool
	Cause                                                                   domain.Cause
}

// SessionCommand is a validated session operation.
type SessionCommand struct {
	private
	value SessionInput
	valid bool
}

// Snapshot returns a validated session operation.
func (c SessionCommand) Snapshot() (SessionInput, error) {
	if !c.valid {
		return SessionInput{}, ErrUnavailable
	}
	return c.value, nil
}

// StoredFix is one fix kept in a segment.
type StoredFix struct {
	private
	At       time.Time
	Lat, Lon int32
	Accuracy int32
	Speed    *int32
	Bearing  *int16
	Coarse   bool
}

// Segment is what one accepted batch stores (B-BR-EMP-432/433).
type Segment struct {
	private
	Batch                     BatchInput
	AuthorMembershipID        string
	EnrollmentID              string
	Fixes                     []StoredFix
	Duplicates, Outside, Mock int
}

// SegmentRecord is the stored identity of one accepted batch.
type SegmentRecord struct {
	private
	OperationID, PayloadDigest, SessionID, TripID, AuthorMembershipID, EnrollmentID string
	Seq                                                                             int64
	Received, Stored, Duplicates, Outside, Mock                                     int
}

// Pause is one paused interval of a trip; an open one has no end.
type Pause struct {
	From time.Time
	To   *time.Time
}

// TripBounds are the stored trip's start, its end (finish or cancellation)
// and its paused intervals.
type TripBounds struct {
	Start  time.Time
	Finish *time.Time
	Pauses []Pause
}

// Store persists sessions and segments in the intake transaction. Every
// write follows LockTripTx on the same trip.
type Store interface {
	LockTripTx(context.Context, syncapp.DBTX, kernel.OrgScope, string) error
	PolicyTx(context.Context, syncapp.DBTX, kernel.OrgScope) (PolicyVersion, error)
	SessionTx(context.Context, syncapp.DBTX, kernel.OrgScope, string) (SessionRecord, error)
	SessionOfTripTx(context.Context, syncapp.DBTX, kernel.OrgScope, string) (SessionRecord, error)
	RecordSessionTx(context.Context, syncapp.DBTX, kernel.OrgScope, syncapp.Source, SessionChange) error
	SegmentTx(context.Context, syncapp.DBTX, kernel.OrgScope, string) (SegmentRecord, error)
	SequenceTakenTx(context.Context, syncapp.DBTX, kernel.OrgScope, string, int64) (bool, error)
	StoredInstantsTx(context.Context, syncapp.DBTX, kernel.OrgScope, string, time.Time, time.Time) ([]time.Time, error)
	InsertSegmentTx(context.Context, syncapp.DBTX, kernel.OrgScope, syncapp.Source, Segment) error
}

func copyPointer[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}
