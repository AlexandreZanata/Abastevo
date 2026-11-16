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

// EMP-TRIP-012 driver handover (B-BR-EMP-394..398, EDR-254..256). A handover
// is a supervisor's online company command under trip.trip.close_for_other;
// the organization's handovers are listed under the same permission and the
// receiving member reads its own open trips under trip.trip.start.

// HandoverAuditAction is the handover's audit action on target type trip; it
// carries the command digest and no reason text (BR-AUD-004).
const HandoverAuditAction = "trip.trip.handed_over"

var (
	// ErrInvalidHandover: the command's shape or reason is invalid.
	ErrInvalidHandover = errors.New("trip: invalid handover")
	// ErrHandoverConflict: a closed trip or a revision that is not the current one.
	ErrHandoverConflict = errors.New("trip: handover conflicts with the current trip")
	// ErrReceiverIneligible: the receiver is not an eligible member (no detail).
	ErrReceiverIneligible = errors.New("trip: handover receiver not eligible")
	// ErrSelfHandover: the recorder is the current responsible member or the receiver.
	ErrSelfHandover = errors.New("trip: handover by a party of the handover")
)

// HandoverAction selects one of the three handover policies.
type HandoverAction uint8

const (
	HandoverWriteAction HandoverAction = iota + 1
	HandoverReadAction
	OwnTripsAction
)

// HandoverPolicy is trip.trip.close_for_other (a fresh configuration write
// for a handover, a read for the organization's handovers) or trip.trip.start
// for the member's own open trips.
func HandoverPolicy(action HandoverAction) access.Policy {
	switch action {
	case HandoverWriteAction:
		return access.Policy{Permission: "trip.trip.close_for_other", Capability: "organizations", Effect: access.ConfigurationWrite, Fresh: true}
	case HandoverReadAction:
		return access.Policy{Permission: "trip.trip.close_for_other", Capability: "organizations", Effect: access.Read}
	case OwnTripsAction:
		return access.Policy{Permission: "trip.trip.start", Capability: "organizations", Effect: access.Read}
	}
	return access.Policy{}
}

// HandoverInput names the trip, the revision the supervisor saw, the
// receiving membership and the reason.
type HandoverInput struct {
	private
	OperationID, TripID, ReceiverMembershipID string
	ExpectedRevision                          int64
	Reason                                    string
}

type HandoverCommand struct {
	private
	value HandoverInput
}

// NewHandoverCommand validates the identities, the revision bound and the
// reason with the METER-001 text profile.
func NewHandoverCommand(v HandoverInput) (HandoverCommand, error) {
	if !canonicalID(v.OperationID) || v.OperationID == nilID || !canonicalID(v.TripID) || !canonicalID(v.ReceiverMembershipID) ||
		v.ExpectedRevision < 1 || v.ExpectedRevision > MaxTripRevision {
		return HandoverCommand{}, ErrInvalidHandover
	}
	if _, err := meter.NewMissing(v.Reason); err != nil {
		return HandoverCommand{}, ErrInvalidHandover
	}
	return HandoverCommand{value: v}, nil
}

const nilID = "00000000-0000-0000-0000-000000000000"

func (c HandoverCommand) Snapshot() (HandoverInput, error) {
	checked, err := NewHandoverCommand(c.value)
	return checked.value, err
}

// Digest binds every field; the reason travels only inside it.
func (c HandoverCommand) Digest(scope kernel.OrgScope) string {
	v, err := c.Snapshot()
	if err != nil || scope.Validate() != nil {
		return ""
	}
	raw, err := json.Marshal([]any{1, "trip.handover", scope.OrgID().String(), v.OperationID, v.TripID, v.ExpectedRevision, v.ReceiverMembershipID, v.Reason})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// HandoverRecord is one stored handover of a trip.
type HandoverRecord struct {
	private
	Revision                                 int64
	OperationID, RequestHash                 string
	FromMembershipID, FromEmployeeID         string
	ToMembershipID, ToEmployeeID             string
	RecorderMembershipID, RecorderEmployeeID string
	Reason                                   string
	HandedOverAt                             time.Time
}

func (h HandoverRecord) valid() bool {
	for _, id := range []string{h.OperationID, h.FromMembershipID, h.FromEmployeeID, h.ToMembershipID, h.ToEmployeeID, h.RecorderMembershipID, h.RecorderEmployeeID} {
		if !canonicalID(id) {
			return false
		}
	}
	return h.Revision >= 2 && h.Revision <= MaxTripRevision && h.Reason != "" && !h.HandedOverAt.IsZero()
}

// HandoverResult is rebuilt from the stored trip and handover, so a replay
// returns exactly the first answer.
type HandoverResult struct {
	private
	TripID      string
	Operational trip.State
	Revision    int64
	Handover    HandoverRecord
}

// HandoverItem is one handover on the organization's handover list.
type HandoverItem struct {
	private
	TripID, AssetID string
	Handover        HandoverRecord
}

type HandoverPage struct {
	private
	Items  []HandoverItem
	NextID string
}

// OwnTrip is one open trip the requesting member is currently responsible
// for, with the latest handover to it when it was handed over.
type OwnTrip struct {
	private
	TripID, AssetID, ReportingDay string
	StartedAt                     time.Time
	Revision                      int64
	State                         trip.State
	Handover                      *HandoverRecord
}

type OwnTripPage struct {
	private
	Items  []OwnTrip
	NextID string
}

// HandoverRepository owns the transaction, the in-transaction company
// authority, the receiver's eligibility and the audit event.
type HandoverRepository interface {
	HandOver(context.Context, kernel.OrgScope, ReviewActor, string, HandoverCommand) (HandoverResult, bool, error)
	ListHandovers(context.Context, kernel.OrgScope, ReviewActor, string, QueueFilter) (HandoverPage, error)
	ListOwnTrips(context.Context, kernel.OrgScope, ReviewActor, string, QueueFilter) (OwnTripPage, error)
}

// HandoverService serves the three handover routes.
type HandoverService struct {
	Enabled    bool
	Repository HandoverRepository
}

// HandOver proves the live request decision before and after the store and
// re-proves the stored result's shape.
func (s HandoverService) HandOver(ctx context.Context, scope kernel.OrgScope, actor ReviewActor, decision access.Decision, command HandoverCommand) (HandoverResult, bool, error) {
	if !s.Enabled {
		return HandoverResult{}, false, ErrReviewDisabled
	}
	in, err := command.Snapshot()
	if err != nil {
		return HandoverResult{}, false, err
	}
	policy := HandoverPolicy(HandoverWriteAction)
	if err := authorizedReview(ctx, scope, actor, decision, in.TripID, policy); err != nil {
		return HandoverResult{}, false, err
	}
	if absentPort(s.Repository) {
		return HandoverResult{}, false, ErrUnavailable
	}
	result, replay, err := s.Repository.HandOver(ctx, scope, actor, decision.Subject().MembershipID(), command)
	if guard := authorizedReview(ctx, scope, actor, decision, in.TripID, policy); guard != nil {
		return HandoverResult{}, false, guard
	}
	if err != nil {
		return HandoverResult{}, false, err
	}
	h := result.Handover
	if result.TripID != in.TripID || result.Operational != trip.InProgress && result.Operational != trip.Paused || result.Revision != in.ExpectedRevision+1 ||
		!h.valid() || h.Revision != result.Revision || h.OperationID != in.OperationID || h.ToMembershipID != in.ReceiverMembershipID || h.Reason != in.Reason ||
		h.RecorderMembershipID != decision.Subject().MembershipID() {
		return HandoverResult{}, false, ErrUnavailable
	}
	return result, replay, nil
}

// List returns the organization's handovers: an organization-level read that
// only an ORG grant covers.
func (s HandoverService) List(ctx context.Context, scope kernel.OrgScope, actor ReviewActor, decision access.Decision, filter QueueFilter) (HandoverPage, error) {
	if !s.Enabled {
		return HandoverPage{}, ErrReviewDisabled
	}
	if err := filter.Validate(); err != nil {
		return HandoverPage{}, err
	}
	policy := HandoverPolicy(HandoverReadAction)
	if err := authorizedReview(ctx, scope, actor, decision, scope.OrgID().String(), policy); err != nil {
		return HandoverPage{}, err
	}
	if absentPort(s.Repository) {
		return HandoverPage{}, ErrUnavailable
	}
	page, err := s.Repository.ListHandovers(ctx, scope, actor, decision.Subject().MembershipID(), filter)
	if guard := authorizedReview(ctx, scope, actor, decision, scope.OrgID().String(), policy); guard != nil {
		return HandoverPage{}, guard
	}
	if err != nil {
		return HandoverPage{}, err
	}
	if len(page.Items) > filter.Limit || page.NextID != "" && (len(page.Items) != filter.Limit || page.Items[len(page.Items)-1].Handover.OperationID != page.NextID) {
		return HandoverPage{}, ErrUnavailable
	}
	seen := map[string]bool{}
	for _, v := range page.Items {
		if !canonicalID(v.TripID) || !canonicalID(v.AssetID) || !v.Handover.valid() || seen[v.Handover.OperationID] {
			return HandoverPage{}, ErrUnavailable
		}
		seen[v.Handover.OperationID] = true
	}
	return HandoverPage{Items: slices.Clone(page.Items), NextID: page.NextID}, nil
}

// Mine returns the open trips the requesting member is currently responsible
// for; its own membership is the target.
func (s HandoverService) Mine(ctx context.Context, scope kernel.OrgScope, actor ReviewActor, decision access.Decision, filter QueueFilter) (OwnTripPage, error) {
	if !s.Enabled {
		return OwnTripPage{}, ErrReviewDisabled
	}
	if err := filter.Validate(); err != nil {
		return OwnTripPage{}, err
	}
	member := decision.Subject().MembershipID()
	policy := HandoverPolicy(OwnTripsAction)
	if err := authorizedReview(ctx, scope, actor, decision, member, policy); err != nil {
		return OwnTripPage{}, err
	}
	if absentPort(s.Repository) {
		return OwnTripPage{}, ErrUnavailable
	}
	page, err := s.Repository.ListOwnTrips(ctx, scope, actor, member, filter)
	if guard := authorizedReview(ctx, scope, actor, decision, member, policy); guard != nil {
		return OwnTripPage{}, guard
	}
	if err != nil {
		return OwnTripPage{}, err
	}
	if len(page.Items) > filter.Limit || page.NextID != "" && (len(page.Items) != filter.Limit || page.Items[len(page.Items)-1].TripID != page.NextID) {
		return OwnTripPage{}, ErrUnavailable
	}
	seen := map[string]bool{}
	for _, v := range page.Items {
		if !canonicalID(v.TripID) || seen[v.TripID] || !canonicalID(v.AssetID) || v.ReportingDay == "" || v.StartedAt.IsZero() || v.Revision < 1 ||
			v.State != trip.InProgress && v.State != trip.Paused || v.Handover != nil && (!v.Handover.valid() || v.Handover.ToMembershipID != member) {
			return OwnTripPage{}, ErrUnavailable
		}
		seen[v.TripID] = true
	}
	out := make([]OwnTrip, len(page.Items))
	for i, v := range page.Items {
		v.Handover = copyPointer(v.Handover)
		out[i] = v
	}
	return OwnTripPage{Items: out, NextID: page.NextID}, nil
}

// HandoverDomain applies the pure handover to the stored trip at receipt.
func HandoverDomain(scope kernel.OrgScope, record Record, recorder, receiver, receiverEmployee string, expected int64, reason string, at time.Time) (trip.Trip, error) {
	stored, err := StoredDomain(scope, record)
	if err != nil {
		return trip.Trip{}, ErrUnavailable
	}
	in := trip.HandoverInput{Expected: expected, Reason: &reason, At: at}
	if in.Recorder, err = trip.NewMembershipRef(scope, recorder); err != nil {
		return trip.Trip{}, ErrUnavailable
	}
	if in.Receiver, err = trip.NewMembershipRef(scope, receiver); err != nil {
		return trip.Trip{}, ErrUnavailable
	}
	if in.ReceiverEmployee, err = trip.NewEmployeeRef(scope, receiverEmployee); err != nil {
		return trip.Trip{}, ErrUnavailable
	}
	return stored.Handover(scope, in)
}

// handoverStep replays one stored handover through the pure domain.
func handoverStep(scope kernel.OrgScope, current trip.Trip, h HandoverRecord) (trip.Trip, error) {
	from := current.Responsible()
	next, err := HandoverDomainOf(scope, current, h)
	if err != nil || next.Revision() != h.Revision || from.ID() != h.FromMembershipID || next.Responsible().ID() != h.ToMembershipID {
		return trip.Trip{}, ErrUnavailable
	}
	return next, nil
}

// HandoverDomainOf applies one stored handover to a domain trip.
func HandoverDomainOf(scope kernel.OrgScope, current trip.Trip, h HandoverRecord) (trip.Trip, error) {
	reason := h.Reason
	in := trip.HandoverInput{Expected: current.Revision(), Reason: &reason, At: h.HandedOverAt}
	var err error
	if in.Recorder, err = trip.NewMembershipRef(scope, h.RecorderMembershipID); err != nil {
		return trip.Trip{}, ErrUnavailable
	}
	if in.Receiver, err = trip.NewMembershipRef(scope, h.ToMembershipID); err != nil {
		return trip.Trip{}, ErrUnavailable
	}
	if in.ReceiverEmployee, err = trip.NewEmployeeRef(scope, h.ToEmployeeID); err != nil {
		return trip.Trip{}, ErrUnavailable
	}
	return current.Handover(scope, in)
}

// ---- responsibility of a stored trip (B-BR-EMP-395) ----

// Responsible is the membership and employee record currently responsible
// for the stored trip: the receiver of its latest handover, else its author.
func (r Record) Responsible() (string, string) {
	if n := len(r.Handovers); n > 0 {
		return r.Handovers[n-1].ToMembershipID, r.Handovers[n-1].ToEmployeeID
	}
	return r.AuthorMembershipID, r.AuthorEmployeeID
}

// ResponsibleAt is the membership and employee record responsible at an
// instant: the receiver of the latest handover recorded at or before it.
func (r Record) ResponsibleAt(at time.Time) (string, string) {
	member, employee := r.AuthorMembershipID, r.AuthorEmployeeID
	for _, h := range r.Handovers {
		if !h.HandedOverAt.After(at) {
			member, employee = h.ToMembershipID, h.ToEmployeeID
		}
	}
	return member, employee
}

// ResponsibleBefore is the responsible membership and employee record just
// before a revision (the author of the command that produced it).
func (r Record) ResponsibleBefore(revision int64) (string, string) {
	member, employee := r.AuthorMembershipID, r.AuthorEmployeeID
	for _, h := range r.Handovers {
		if h.Revision < revision {
			member, employee = h.ToMembershipID, h.ToEmployeeID
		}
	}
	return member, employee
}

// Driver is the employee who currently drives the trip: the receiving
// employee of the latest handover, else the start driver.
func (r Record) Driver() string {
	if n := len(r.Handovers); n > 0 {
		return r.Handovers[n-1].ToEmployeeID
	}
	return r.DriverID
}

// DriverAt is the employee driving the trip at an instant.
func (r Record) DriverAt(at time.Time) string {
	driver := r.DriverID
	for _, h := range r.Handovers {
		if !h.HandedOverAt.After(at) {
			driver = h.ToEmployeeID
		}
	}
	return driver
}

// Party reports whether the membership was ever responsible for the trip
// (its author or a giving or receiving member of a handover); such a member
// may re-read the receipts of the operations it sent.
func (r Record) Party(membership string) bool {
	if membership == r.AuthorMembershipID {
		return true
	}
	for _, h := range r.Handovers {
		if membership == h.FromMembershipID || membership == h.ToMembershipID {
			return true
		}
	}
	return false
}

// HandoverParties lists every party of every handover: giving, receiving and
// recording memberships with their employee records (B-BR-EMP-397).
func (r Record) HandoverParties() [][2]string {
	out := make([][2]string, 0, 3*len(r.Handovers))
	for _, h := range r.Handovers {
		out = append(out, [2]string{h.FromMembershipID, h.FromEmployeeID}, [2]string{h.ToMembershipID, h.ToEmployeeID},
			[2]string{h.RecorderMembershipID, h.RecorderEmployeeID})
	}
	return out
}
