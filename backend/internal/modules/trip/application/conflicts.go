package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	review "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
)

// EMP-TRIP-007 asset-overlap conflict resolution (B-BR-EMP-407..411,
// EDR-262..264). The held trip facts of the OFFL-008 mechanism are listed,
// read and resolved once by a reviewer holding trip.trip.review. The rules
// are server-only: the device never decides a resolution and the
// ReviewResolution wire it receives is the shared OFFL-008 one.

// Conflict item states (the OFFL-008 review lifecycle, 10-offline-sync §6).
const (
	ConflictOpen     = "OPEN"
	ConflictExpired  = "EXPIRED_UNRESOLVED"
	ConflictAccepted = "RESOLVED_ACCEPTED"
	ConflictRefused  = "RESOLVED_REFUSED"
)

// ConflictDecisionKind is the reviewer's decision (B-BR-EMP-408).
type ConflictDecisionKind string

const (
	ConflictAccept ConflictDecisionKind = "ACCEPT"
	ConflictRefuse ConflictDecisionKind = "REFUSE"
)

// ErrConflictUnacceptable is the CONFLICT of an acceptance that would
// rewrite a closed trip or a reviewed revision, or make two trips of the
// asset count as in progress (B-BR-EMP-408/409). It wraps review.ErrConflict.
var ErrConflictUnacceptable = fmt.Errorf("%w: held trip fact cannot be accepted", review.ErrConflict)

// ConflictPolicy is the EMP-TRIP-004 trip.trip.review policy: the queue and
// the read, or a fresh configuration write for a resolution (EDR-263).
func ConflictPolicy(write bool) access.Policy {
	if write {
		return ReviewPolicy(ReviewDecideAction)
	}
	return ReviewPolicy(ReviewReadAction)
}

// heldCodes are the held codes each trip owner keeps (B-BR-EMP-407).
var heldCodes = map[string][]string{
	StartOperationType:  {AssetBusy, AssetUnavailable, PermissionChanged, OrphanReference},
	FinishOperationType: {TripAlreadyClosed},
	EventOperationType:  {TripAlreadyClosed, StaleRevision},
	SubmitOperationType: {StaleRevision},
	// EMP-TRIP-011 lifecycle commands on a closed trip or a stale revision.
	PauseOperationType:  {TripAlreadyClosed, StaleRevision},
	ResumeOperationType: {TripAlreadyClosed, StaleRevision},
	CancelOperationType: {TripAlreadyClosed, StaleRevision},
}

// HeldCode reports whether an owner keeps a held fact under the code.
func HeldCode(operationType, code string) bool {
	return slices.Contains(heldCodes[operationType], code)
}

// ConflictTrip is one stored trip the held fact conflicts with: for a held
// start a trip of its asset, otherwise the trip the fact names. FinishedAt is
// the end of a terminal trip: its finish, or its cancellation (EMP-TRIP-011).
type ConflictTrip struct {
	private
	TripID     string
	State      trip.State
	StartedAt  time.Time
	FinishedAt *time.Time
}

// Overlaps is the B-BR-EMP-409 rule: a trip that has not ended (in progress
// or paused), or ended after the held start's declared instant, would hold
// the asset at an instant the accepted trip is in progress too.
func (t ConflictTrip) Overlaps(declared time.Time) bool {
	return t.FinishedAt == nil || t.FinishedAt.After(declared)
}

// HeldStartFact is the exact held trip.start (TRIP-002 held row).
type HeldStartFact struct {
	private
	TripID, AssetID, DriverID string
	Declared                  orgtime.Declaration
	Clock                     *trip.RunClock
	MeterID, SegmentID        string
	ReadingID, ReadingKind    string
	ReadingValue              *int64
	MissingReason             *string
}

// HeldEventFact is the exact held logbook event.
type HeldEventFact struct {
	private
	EventID, EventType string
	Text               *string
}

// HeldTransitionFact is an exact held EMP-TRIP-011 lifecycle command.
type HeldTransitionFact struct {
	private
	Command          string
	ExpectedRevision int64
	Reason           *string
}

// ConflictFact is the exact held fact of one item: Start for a held start,
// Event for a held event, SubmittedRevision for a held submission,
// Transition for a held lifecycle command; Finish facts reuse Start's
// declaration, reading and clock fields with no asset.
type ConflictFact struct {
	private
	TripID            string
	Declared          orgtime.Declaration
	Start             *HeldStartFact
	Finish            *HeldStartFact
	Event             *HeldEventFact
	SubmittedRevision *int64
	Transition        *HeldTransitionFact
}

// ConflictView is the reviewer's read of one held trip item.
type ConflictView struct {
	private
	ReviewItemID, OperationID, OperationType, PayloadDigest, Code, State string
	Revision                                                             int64
	AuthorAccountID, AuthorMembershipID, AuthorEmployeeID, EnrollmentID  string
	ReceivedAt, ExpiresAt                                                time.Time
	Fact                                                                 ConflictFact
	Trips                                                                []ConflictTrip
	Resolution                                                           *protocol.ReviewResolution
}

// Clone copies the view so a caller cannot alias the store's values.
func (v ConflictView) Clone() ConflictView {
	v.Trips = slices.Clone(v.Trips)
	if v.Resolution != nil {
		r := *v.Resolution
		v.Resolution = &r
	}
	return v
}

// Validate checks the complete projection of one item.
func (v ConflictView) Validate() error {
	if !validConflictView(v) {
		return ErrUnavailable
	}
	return nil
}

func validConflictView(v ConflictView) bool {
	for _, id := range []string{v.ReviewItemID, v.OperationID, v.AuthorAccountID, v.AuthorMembershipID, v.AuthorEmployeeID, v.EnrollmentID, v.Fact.TripID} {
		if !canonicalID(id) {
			return false
		}
	}
	if len(v.PayloadDigest) != 71 || !strings.HasPrefix(v.PayloadDigest, "sha256:") || v.PayloadDigest != strings.ToLower(v.PayloadDigest) {
		return false
	}
	if _, err := hex.DecodeString(v.PayloadDigest[7:]); err != nil || !HeldCode(v.OperationType, v.Code) || v.ReceivedAt.IsZero() || !v.ExpiresAt.After(v.ReceivedAt) {
		return false
	}
	f := v.Fact
	lifecycle := map[string]string{PauseOperationType: "PAUSE", ResumeOperationType: "RESUME", CancelOperationType: "CANCEL"}
	shape := map[string]bool{
		StartOperationType:  f.Start != nil && f.Finish == nil && f.Event == nil && f.SubmittedRevision == nil && f.Transition == nil && f.Start.TripID == f.TripID && canonicalID(f.Start.AssetID) && canonicalID(f.Start.DriverID),
		FinishOperationType: f.Start == nil && f.Finish != nil && f.Event == nil && f.SubmittedRevision == nil && f.Transition == nil,
		EventOperationType:  f.Start == nil && f.Finish == nil && f.Event != nil && f.SubmittedRevision == nil && f.Transition == nil && canonicalID(f.Event.EventID),
		SubmitOperationType: f.Start == nil && f.Finish == nil && f.Event == nil && f.SubmittedRevision != nil && f.Transition == nil && *f.SubmittedRevision >= 1,
	}
	if command, ok := lifecycle[v.OperationType]; ok {
		t := f.Transition
		shape[v.OperationType] = f.Start == nil && f.Finish == nil && f.Event == nil && f.SubmittedRevision == nil && t != nil && t.Command == command && t.ExpectedRevision >= 1 &&
			(command != "CANCEL" || t.Reason != nil)
	}
	if !shape[v.OperationType] || f.Declared.OccurredAt.IsZero() || f.Declared.EntryZone == "" {
		return false
	}
	seen := map[string]bool{}
	for _, t := range v.Trips {
		if !canonicalID(t.TripID) || seen[t.TripID] || t.StartedAt.IsZero() || t.State.Terminal() != (t.FinishedAt != nil) {
			return false
		}
		seen[t.TripID] = true
	}
	switch v.State {
	case ConflictOpen, ConflictExpired:
		return v.Revision == 1 && v.Resolution == nil
	case ConflictAccepted, ConflictRefused:
		if v.Revision != 2 || v.Resolution == nil {
			return false
		}
		r := v.Resolution.Snapshot()
		if _, err := protocol.NewReviewResolution(r); err != nil {
			return false
		}
		resolved, err := orgtime.ParseInstant(r.ResolvedAt)
		if err != nil || resolved.Before(v.ReceivedAt) || r.OperationID != v.OperationID || r.PayloadDigest != v.PayloadDigest || r.ReviewItemID != v.ReviewItemID {
			return false
		}
		if v.State == ConflictAccepted {
			return r.Outcome == protocol.ReviewAccepted && v.OperationType == StartOperationType && r.EntityRevision != nil && *r.EntityRevision == 1
		}
		return r.Outcome == protocol.ReviewRefused
	}
	return false
}

// ConflictFilter pages the organization conflict queue after one item.
type ConflictFilter struct {
	private
	AfterID, State string
	Limit          int
}

func (f ConflictFilter) Validate() error {
	if f.Limit < 1 || f.Limit > 100 || f.AfterID != "" && !canonicalID(f.AfterID) {
		return review.ErrInvalid
	}
	switch f.State {
	case "", ConflictOpen, ConflictExpired, ConflictAccepted, ConflictRefused:
		return nil
	}
	return review.ErrInvalid
}

type ConflictPage struct {
	private
	Items  []ConflictView
	NextID string
}

// ConflictDecisionInput is one resolution command.
type ConflictDecisionInput struct {
	private
	OperationID, ReviewItemID string
	ExpectedRevision          int64
	Decision                  ConflictDecisionKind
	Reason                    string
}

// ConflictDecision is a validated resolution command.
type ConflictDecision struct {
	private
	value ConflictDecisionInput
	valid bool
}

// NewConflictDecision validates the command shape: identifiers, a positive
// revision, the decision kind and the OFFL-008 reason profile (280 scalars,
// no control character, a refusal states a reason).
func NewConflictDecision(in ConflictDecisionInput) (ConflictDecision, error) {
	if !canonicalID(in.OperationID) || !canonicalID(in.ReviewItemID) || in.ExpectedRevision < 1 || in.ExpectedRevision == math.MaxInt64 ||
		in.Decision != ConflictAccept && in.Decision != ConflictRefuse || !utf8.ValidString(in.Reason) || utf8.RuneCountInString(in.Reason) > review.MaxReasonScalars {
		return ConflictDecision{}, review.ErrInvalid
	}
	for _, r := range in.Reason {
		if unicode.IsControl(r) {
			return ConflictDecision{}, review.ErrInvalid
		}
	}
	blank := strings.TrimSpace(in.Reason) == ""
	if in.Decision == ConflictRefuse && blank {
		return ConflictDecision{}, review.ErrReasonRequired
	}
	if in.Reason != "" && blank {
		return ConflictDecision{}, review.ErrInvalid
	}
	return ConflictDecision{value: in, valid: true}, nil
}

func (c ConflictDecision) Snapshot() (ConflictDecisionInput, error) {
	if !c.valid {
		return ConflictDecisionInput{}, ErrUnavailable
	}
	return c.value, nil
}

// Digest binds the command to the organization (replay and reuse).
func (c ConflictDecision) Digest(scope kernel.OrgScope) string {
	in, err := c.Snapshot()
	if err != nil || scope.Validate() != nil {
		return ""
	}
	raw, err := json.Marshal([]any{1, "trip.conflict.resolve", scope.OrgID().String(), in.OperationID, in.ReviewItemID, in.ExpectedRevision, in.Decision, in.Reason})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// ConflictFacts are the server facts a decision is judged against, read in
// the resolving transaction under the organization root and asset locks.
type ConflictFacts struct {
	private
	// AssetActive and DisposedAt: the held start's asset now.
	AssetActive bool
	DisposedAt  *time.Time
	// AuthorCurrent: the author's membership is ACTIVE with its enrolled
	// device; DriverActive: the declared driver is an ACTIVE unmerged record.
	AuthorCurrent, DriverActive bool
}

// DecideConflict applies B-BR-EMP-408/409 in a fixed order: the named
// revision and an unresolved item (CONFLICT, BR-REV-007), then a refusal
// (always admitted), then an acceptance only of a held start whose asset is
// still ACTIVE and not disposed at the declaration, whose author and driver
// are current and which overlaps no stored trip of the asset.
func DecideConflict(view ConflictView, in ConflictDecisionInput, facts ConflictFacts) error {
	if view.Revision != 1 || view.Resolution != nil || in.ExpectedRevision != view.Revision || in.ReviewItemID != view.ReviewItemID {
		return review.ErrConflict
	}
	switch in.Decision {
	case ConflictRefuse:
		return nil
	case ConflictAccept:
	default:
		return review.ErrInvalid
	}
	start := view.Fact.Start
	if view.OperationType != StartOperationType || start == nil {
		return ErrConflictUnacceptable
	}
	if !facts.AssetActive || facts.DisposedAt != nil && !start.Declared.OccurredAt.Before(*facts.DisposedAt) || !facts.AuthorCurrent || !facts.DriverActive {
		return ErrConflictUnacceptable
	}
	for _, t := range view.Trips {
		if t.TripID != start.TripID && t.Overlaps(start.Declared.OccurredAt) {
			return ErrConflictUnacceptable
		}
	}
	return nil
}

// HeldStartTrip rebuilds the EMP-TRIP-001 trip of an accepted held start
// with the zones of its declared instant (B-BR-EMP-408): the domain proves
// the eligible context and derives the start reporting day again.
func HeldStartTrip(scope kernel.OrgScope, view ConflictView, zones orgtime.ReportingZones) (trip.Trip, error) {
	start := view.Fact.Start
	if scope.Validate() != nil || view.OperationType != StartOperationType || start == nil || start.TripID != view.Fact.TripID {
		return trip.Trip{}, ErrUnavailable
	}
	in := StartInput{Event: Event{OperationID: view.OperationID, PayloadDigest: view.PayloadDigest, TripID: start.TripID, AuthorMembershipID: view.AuthorMembershipID,
		Declared: start.Declared, MissingReason: copyPointer(start.MissingReason), Clock: copyPointer(start.Clock)}, AssetID: start.AssetID, DriverID: start.DriverID}
	if start.ReadingID != "" {
		if start.ReadingValue == nil {
			return trip.Trip{}, ErrUnavailable
		}
		value, err := meter.NewValue(meter.Kind(start.ReadingKind), *start.ReadingValue, nil)
		if err != nil {
			return trip.Trip{}, ErrUnavailable
		}
		in.Reading = &Reading{MeterID: start.MeterID, SegmentID: start.SegmentID, ReadingID: start.ReadingID, Value: value}
	}
	return StartDomain(protocol.Envelope{OrgID: scope.OrgID().String()}, in, zones)
}

// ConflictParty is one canonical person of a held fact for the self-review
// rule: an account, a membership and canonical employee records.
type ConflictParty struct {
	private
	AccountID, MembershipID string
	Employees               []string
}

// SelfReview is B-BR-EMP-410: the reviewer shares an account, a membership
// or a canonical employee record with the author or, for a held start, the
// declared driver (BR-REV-003).
func SelfReview(reviewer ConflictParty, parties ...ConflictParty) bool {
	for _, p := range parties {
		if reviewer.AccountID != "" && reviewer.AccountID == p.AccountID || reviewer.MembershipID != "" && reviewer.MembershipID == p.MembershipID {
			return true
		}
		for _, e := range reviewer.Employees {
			if e != "" && slices.Contains(p.Employees, e) {
				return true
			}
		}
	}
	return false
}

// ConflictRepository is the store port; it owns the transaction, the
// in-transaction company authority and the audit event.
type ConflictRepository interface {
	ListTripConflicts(context.Context, kernel.OrgScope, ReviewActor, string, ConflictFilter) (ConflictPage, error)
	ReadTripConflict(context.Context, kernel.OrgScope, ReviewActor, string, string) (ConflictView, error)
	ResolveTripConflict(context.Context, kernel.OrgScope, ReviewActor, string, ConflictDecision) (protocol.ReviewResolution, bool, error)
}

// ConflictService serves the three conflict routes (EDR-263).
type ConflictService struct {
	Enabled    bool
	Repository ConflictRepository
}

// List returns the organization queue: only an ORG grant covers it.
func (s ConflictService) List(ctx context.Context, scope kernel.OrgScope, actor ReviewActor, decision access.Decision, filter ConflictFilter) (ConflictPage, error) {
	if !s.Enabled {
		return ConflictPage{}, ErrReviewDisabled
	}
	if err := filter.Validate(); err != nil {
		return ConflictPage{}, err
	}
	policy := ConflictPolicy(false)
	if err := authorizedReview(ctx, scope, actor, decision, scope.OrgID().String(), policy); err != nil {
		return ConflictPage{}, err
	}
	if absentPort(s.Repository) {
		return ConflictPage{}, ErrUnavailable
	}
	page, err := s.Repository.ListTripConflicts(ctx, scope, actor, decision.Subject().MembershipID(), filter)
	if guard := authorizedReview(ctx, scope, actor, decision, scope.OrgID().String(), policy); guard != nil {
		return ConflictPage{}, guard
	}
	if err != nil {
		return ConflictPage{}, err
	}
	if len(page.Items) > filter.Limit || page.NextID != "" && (len(page.Items) != filter.Limit || page.Items[len(page.Items)-1].ReviewItemID != page.NextID) {
		return ConflictPage{}, ErrUnavailable
	}
	prior := filter.AfterID
	items := make([]ConflictView, 0, len(page.Items))
	for _, v := range page.Items {
		if !validConflictView(v) || v.ReviewItemID <= prior || filter.State != "" && v.State != filter.State {
			return ConflictPage{}, ErrUnavailable
		}
		prior = v.ReviewItemID
		items = append(items, v.Clone())
	}
	return ConflictPage{Items: items, NextID: page.NextID}, nil
}

// Read returns one item under the same proof, its structure covered.
func (s ConflictService) Read(ctx context.Context, scope kernel.OrgScope, actor ReviewActor, decision access.Decision, id string) (ConflictView, error) {
	if !s.Enabled {
		return ConflictView{}, ErrReviewDisabled
	}
	if !canonicalID(id) {
		return ConflictView{}, ErrNotFound
	}
	policy := ConflictPolicy(false)
	if err := authorizedReview(ctx, scope, actor, decision, id, policy); err != nil {
		return ConflictView{}, err
	}
	if absentPort(s.Repository) {
		return ConflictView{}, ErrUnavailable
	}
	view, err := s.Repository.ReadTripConflict(ctx, scope, actor, decision.Subject().MembershipID(), id)
	if guard := authorizedReview(ctx, scope, actor, decision, id, policy); guard != nil {
		return ConflictView{}, guard
	}
	if err != nil {
		return ConflictView{}, err
	}
	if !validConflictView(view) || view.ReviewItemID != id {
		return ConflictView{}, ErrUnavailable
	}
	return view.Clone(), nil
}

// Resolve applies one decision and re-proves the stored resolution.
func (s ConflictService) Resolve(ctx context.Context, scope kernel.OrgScope, actor ReviewActor, decision access.Decision, command ConflictDecision) (protocol.ReviewResolution, bool, error) {
	if !s.Enabled {
		return protocol.ReviewResolution{}, false, ErrReviewDisabled
	}
	in, err := command.Snapshot()
	if err != nil {
		return protocol.ReviewResolution{}, false, err
	}
	policy := ConflictPolicy(true)
	if err := authorizedReview(ctx, scope, actor, decision, in.ReviewItemID, policy); err != nil {
		return protocol.ReviewResolution{}, false, err
	}
	if absentPort(s.Repository) {
		return protocol.ReviewResolution{}, false, ErrUnavailable
	}
	result, replay, err := s.Repository.ResolveTripConflict(ctx, scope, actor, decision.Subject().MembershipID(), command)
	if guard := authorizedReview(ctx, scope, actor, decision, in.ReviewItemID, policy); guard != nil {
		return protocol.ReviewResolution{}, false, guard
	}
	if err != nil {
		return protocol.ReviewResolution{}, false, err
	}
	v := result.Snapshot()
	want := map[ConflictDecisionKind]string{ConflictAccept: protocol.ReviewAccepted, ConflictRefuse: protocol.ReviewRefused}[in.Decision]
	if _, err := protocol.NewReviewResolution(v); err != nil || v.ReviewItemID != in.ReviewItemID || v.ResolutionID != in.OperationID || v.Outcome != want || v.Reason != in.Reason {
		return protocol.ReviewResolution{}, false, ErrUnavailable
	}
	return result, replay, nil
}
