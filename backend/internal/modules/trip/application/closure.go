package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"time"

	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
)

// EMP-TRIP-014 supervisor closure of another member's open trip
// (B-BR-EMP-399..403, EDR-257..259). The closure is server-only and online:
// a company command under trip.trip.close_for_other with fresh proof, and an
// organization-level list of trips in progress.

// CloseAuditAction is the closure's audit action on target type trip; it
// carries the command digest and no reason text (BR-AUD-004).
const CloseAuditAction = "trip.trip.closed"

// OpenRevision is the operational revision of a trip in progress (its
// start ACK); a completed trip is at revision 2 (its finish or closure).
const OpenRevision int64 = 1

var (
	// ErrInvalidClosure: the closure command's shape or reasons are invalid.
	ErrInvalidClosure = errors.New("trip: invalid closure")
	// ErrClosureConflict: a stale revision or a trip that is no longer in
	// progress (finished or already closed).
	ErrClosureConflict = errors.New("trip: closure conflicts with the current trip")
)

// ClosureAction selects the open-trip list or the closure command policy.
type ClosureAction uint8

const (
	ClosureReadAction ClosureAction = iota + 1
	ClosureWriteAction
)

// ClosePolicy is trip.trip.close_for_other: a fresh configuration write for a
// closure (online only) and a read for the organization's open-trip list.
func ClosePolicy(action ClosureAction) access.Policy {
	switch action {
	case ClosureWriteAction:
		return access.Policy{Permission: "trip.trip.close_for_other", Capability: "organizations", Effect: access.ConfigurationWrite, Fresh: true}
	case ClosureReadAction:
		return access.Policy{Permission: "trip.trip.close_for_other", Capability: "organizations", Effect: access.Read}
	}
	return access.Policy{}
}

// CloseInput names the trip, the operational revision the closer saw, why
// the trip is closed and why no final reading exists. It has no reading,
// value or distance field (B-BR-EMP-400).
type CloseInput struct {
	private
	OperationID, TripID   string
	ExpectedRevision      int64
	Reason, MissingReason string
}

type CloseCommand struct {
	private
	value CloseInput
}

// NewCloseCommand validates the identities, a positive revision and both
// reasons with the METER-001 text profile.
func NewCloseCommand(v CloseInput) (CloseCommand, error) {
	if !canonicalID(v.OperationID) || v.OperationID == zeroID || !canonicalID(v.TripID) || v.ExpectedRevision < 1 || v.ExpectedRevision > MaxReviewRevision {
		return CloseCommand{}, ErrInvalidClosure
	}
	if _, err := meter.NewMissing(v.Reason); err != nil {
		return CloseCommand{}, ErrInvalidClosure
	}
	if _, err := meter.NewMissing(v.MissingReason); err != nil {
		return CloseCommand{}, ErrInvalidClosure
	}
	return CloseCommand{value: v}, nil
}

const zeroID = "00000000-0000-0000-0000-000000000000"

func (c CloseCommand) Snapshot() (CloseInput, error) {
	checked, err := NewCloseCommand(c.value)
	return checked.value, err
}

// Digest binds every field; the reasons travel only inside it.
func (c CloseCommand) Digest(scope kernel.OrgScope) string {
	v, err := c.Snapshot()
	if err != nil || scope.Validate() != nil {
		return ""
	}
	raw, err := json.Marshal([]any{1, "trip.close", scope.OrgID().String(), v.OperationID, v.TripID, v.ExpectedRevision, v.Reason, v.MissingReason})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// ClosureRecord is the stored closure of a trip.
type ClosureRecord struct {
	private
	OperationID, RequestHash, CloserMembershipID, CloserEmployeeID string
	Reason, MissingReason                                          string
	ClosedAt                                                       time.Time
}

// CloseResult is rebuilt from the stored closure, so a replay returns exactly
// the first answer; the usage is derived through the pure trip.
type CloseResult struct {
	private
	TripID      string
	Operational trip.State
	Revision    int64
	Closure     ClosureRecord
	UsageStatus trip.UsageStatus
	UsageReason trip.UsageReason
}

// OpenTrip is one trip in progress on the organization's open-trip list.
type OpenTrip struct {
	private
	TripID, AssetID, DriverEmployeeID, AuthorMembershipID, ReportingDay string
	StartedAt                                                           time.Time
	Revision                                                            int64
	// AuthorState is the author's membership state (EMP-TRIP-018): ENDED or
	// SUSPENDED shows a trip left open by a member who lost access.
	AuthorState string
}

type OpenTripPage struct {
	private
	Items  []OpenTrip
	NextID string
}

// ClosureRepository owns the transaction, the in-transaction company
// authority and the audit event.
type ClosureRepository interface {
	CloseTrip(context.Context, kernel.OrgScope, ReviewActor, string, CloseCommand) (CloseResult, bool, error)
	ListOpenTrips(context.Context, kernel.OrgScope, ReviewActor, string, QueueFilter) (OpenTripPage, error)
}

// ClosureService serves the two closure routes.
type ClosureService struct {
	Enabled    bool
	Repository ClosureRepository
}

// Close proves the live request decision before and after the store and
// re-proves the stored result's shape.
func (s ClosureService) Close(ctx context.Context, scope kernel.OrgScope, actor ReviewActor, decision access.Decision, command CloseCommand) (CloseResult, bool, error) {
	if !s.Enabled {
		return CloseResult{}, false, ErrReviewDisabled
	}
	in, err := command.Snapshot()
	if err != nil {
		return CloseResult{}, false, err
	}
	policy := ClosePolicy(ClosureWriteAction)
	if err := authorizedReview(ctx, scope, actor, decision, in.TripID, policy); err != nil {
		return CloseResult{}, false, err
	}
	if absentPort(s.Repository) {
		return CloseResult{}, false, ErrUnavailable
	}
	result, replay, err := s.Repository.CloseTrip(ctx, scope, actor, decision.Subject().MembershipID(), command)
	if guard := authorizedReview(ctx, scope, actor, decision, in.TripID, policy); guard != nil {
		return CloseResult{}, false, guard
	}
	if err != nil {
		return CloseResult{}, false, err
	}
	if result.TripID != in.TripID || result.Operational != trip.Completed || result.Revision != OpenRevision+1 || result.Closure.Reason != in.Reason ||
		result.Closure.MissingReason != in.MissingReason || result.Closure.OperationID != in.OperationID || !canonicalID(result.Closure.CloserMembershipID) ||
		result.Closure.ClosedAt.IsZero() || result.UsageStatus != trip.NotMeasured || result.UsageReason != trip.ReasonReadingAbsent {
		return CloseResult{}, false, ErrUnavailable
	}
	return result, replay, nil
}

// ListOpen returns the organization's trips in progress: an
// organization-level read that only an ORG grant covers.
func (s ClosureService) ListOpen(ctx context.Context, scope kernel.OrgScope, actor ReviewActor, decision access.Decision, filter QueueFilter) (OpenTripPage, error) {
	if !s.Enabled {
		return OpenTripPage{}, ErrReviewDisabled
	}
	if err := filter.Validate(); err != nil {
		return OpenTripPage{}, err
	}
	policy := ClosePolicy(ClosureReadAction)
	if err := authorizedReview(ctx, scope, actor, decision, scope.OrgID().String(), policy); err != nil {
		return OpenTripPage{}, err
	}
	if absentPort(s.Repository) {
		return OpenTripPage{}, ErrUnavailable
	}
	page, err := s.Repository.ListOpenTrips(ctx, scope, actor, decision.Subject().MembershipID(), filter)
	if guard := authorizedReview(ctx, scope, actor, decision, scope.OrgID().String(), policy); guard != nil {
		return OpenTripPage{}, guard
	}
	if err != nil {
		return OpenTripPage{}, err
	}
	if len(page.Items) > filter.Limit || page.NextID != "" && (len(page.Items) != filter.Limit || page.Items[len(page.Items)-1].TripID != page.NextID) {
		return OpenTripPage{}, ErrUnavailable
	}
	seen := map[string]bool{}
	for _, v := range page.Items {
		if !canonicalID(v.TripID) || seen[v.TripID] || !canonicalID(v.AssetID) || !canonicalID(v.DriverEmployeeID) || !canonicalID(v.AuthorMembershipID) ||
			v.ReportingDay == "" || v.StartedAt.IsZero() || v.Revision != OpenRevision || v.AuthorState != "ACTIVE" && v.AuthorState != "SUSPENDED" && v.AuthorState != "ENDED" {
			return OpenTripPage{}, ErrUnavailable
		}
		seen[v.TripID] = true
	}
	return OpenTripPage{Items: slices.Clone(page.Items), NextID: page.NextID}, nil
}

// CloseDomain applies the pure closure to the stored trip at receipt.
func CloseDomain(scope kernel.OrgScope, record Record, closerMembership, closerEmployee, missingReason string) (trip.Trip, error) {
	stored, err := StoredDomain(scope, record)
	if err != nil {
		return trip.Trip{}, err
	}
	var in trip.CloseInput
	if in.Closer, err = trip.NewMembershipRef(scope, closerMembership); err != nil {
		return trip.Trip{}, ErrUnavailable
	}
	if in.CloserEmployee, err = trip.NewEmployeeRef(scope, closerEmployee); err != nil {
		return trip.Trip{}, ErrUnavailable
	}
	if in.Absence, err = meter.NewMissing(missingReason); err != nil {
		return trip.Trip{}, ErrInvalidClosure
	}
	return stored.Close(scope, in)
}

// closedDomain rebuilds a stored closure through the pure domain.
func closedDomain(scope kernel.OrgScope, record Record) (trip.Trip, error) {
	c := record.Closure
	if record.Finish != nil || record.State != trip.Completed {
		return trip.Trip{}, ErrUnavailable
	}
	started := record
	started.State, started.Closure = trip.InProgress, nil
	closed, err := CloseDomain(scope, started, c.CloserMembershipID, c.CloserEmployeeID, c.MissingReason)
	if err != nil {
		return trip.Trip{}, ErrUnavailable
	}
	return closed, nil
}
