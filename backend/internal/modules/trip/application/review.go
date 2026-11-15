package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	amend "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgamend/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	review "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
)

// EMP-TRIP-004 submission and administrative review (B-BR-EMP-342..347,
// EDR-220..223). The lifecycle is the integrated APPR-001 review/domain; this
// file binds it to a stored trip on the server only (EDR-223).
const (
	SubmitOperationType = "trip.submit"
	ReviewEntityType    = "trip_review"
	// MaxReviewRevision bounds a named review revision (the history bound).
	MaxReviewRevision = 1_000_000
)

var (
	// ErrSubmitPending: the trip is not stored here or not yet COMPLETED.
	ErrSubmitPending = errors.New("trip: submission waits for the completed trip")
	// ErrSubmitStale: the case is no longer DRAFT at the named revision.
	ErrSubmitStale        = errors.New("trip: review case is not a draft at the named revision")
	ErrReviewDisabled     = errors.New("trip: review surface disabled")
	ErrReviewUnauthorized = errors.New("trip: review proof required")
	ErrOperationReused    = errors.New("trip: operation identifier reused")
)

// ReviewAction selects the reviewer read or the decision policy.
type ReviewAction uint8

const (
	ReviewReadAction ReviewAction = iota + 1
	ReviewDecideAction
)

// ReviewPolicy is trip.trip.review: a fresh configuration write for a
// decision (BR-REV-004, online only) and a read for the decision context.
func ReviewPolicy(action ReviewAction) access.Policy {
	switch action {
	case ReviewDecideAction:
		return access.Policy{Permission: "trip.trip.review", Capability: "organizations", Effect: access.ConfigurationWrite, Fresh: true}
	case ReviewReadAction:
		return access.Policy{Permission: "trip.trip.review", Capability: "organizations", Effect: access.Read}
	}
	return access.Policy{}
}

// ---- trip.submit (B-BR-EMP-342/343) ----

// SubmitInput is a parsed trip.submit: the trip, the review revision it
// submits and the declared instant the author marked it ready to send.
type SubmitInput struct {
	private
	OperationID, PayloadDigest, TripID, AuthorMembershipID string
	Declared                                               orgtime.Declaration
	Revision                                               int64
	DependsOn                                              []string
}

type SubmitCommand struct {
	private
	value SubmitInput
	valid bool
}

func (c SubmitCommand) Snapshot() (SubmitInput, error) {
	if !c.valid {
		return SubmitInput{}, ErrUnavailable
	}
	v := c.value
	v.DependsOn = slices.Clone(v.DependsOn)
	return v, nil
}

// ParseSubmit decodes one trip.submit schema 1 operation (EDR-220): entity
// trip_review with the review revision as expected revision, at most one
// dependency, nothing else and the exact empty payload. Any other shape is a
// final MALFORMED.
func ParseSubmit(op protocol.Operation) (SubmitCommand, error) {
	v := op.Snapshot()
	if v.Type != SubmitOperationType || !op.SupportsSchema() || v.Entity.Type != ReviewEntityType || !canonicalID(v.Entity.ID) || v.Entity.ExpectedRevision == nil ||
		*v.Entity.ExpectedRevision < 1 || *v.Entity.ExpectedRevision > MaxReviewRevision || len(v.DependsOn) > 1 || len(v.Attachments) != 0 || len(v.SoftReferences) != 0 {
		return SubmitCommand{}, ErrMalformed
	}
	if fields, ok := object(op.PayloadBytes(), nil); !ok || len(fields) != 0 {
		return SubmitCommand{}, ErrMalformed
	}
	at, err := orgtime.ParseInstant(v.OccurredAt)
	if err != nil {
		return SubmitCommand{}, ErrMalformed
	}
	declared, err := orgtime.ValidateDeclaration(orgtime.Declaration{OccurredAt: at, EntryZone: v.EntryZone, DeviceOffsetSeconds: v.DeviceOffsetSeconds})
	if err != nil {
		return SubmitCommand{}, ErrMalformed
	}
	return SubmitCommand{valid: true, value: SubmitInput{OperationID: op.ID(), PayloadDigest: op.PayloadDigest(), TripID: v.Entity.ID, AuthorMembershipID: v.MembershipID,
		Declared: declared, Revision: *v.Entity.ExpectedRevision, DependsOn: slices.Clone(v.DependsOn)}}, nil
}

// SubmitStore appends the SUBMIT or RESUBMIT entry in the intake
// transaction under the trip row lock (EDR-221, EDR-234) and keeps a stale
// submission as a held fact (EDR-235).
type SubmitStore interface {
	LockTripTx(context.Context, syncapp.DBTX, kernel.OrgScope, string) (trip.State, error)
	ReviewEntriesTx(context.Context, syncapp.DBTX, kernel.OrgScope, string) ([]ReviewEntry, error)
	InsertSubmitTx(context.Context, syncapp.DBTX, kernel.OrgScope, syncapp.Source, SubmitCommand, review.Entry, int64, string) error
	HoldSubmitTx(context.Context, syncapp.DBTX, syncapp.Source, protocol.Operation, SubmitCommand, string) (string, error)
}

// ---- review case binding (EDR-223) ----

// ReviewEntry is one stored entry of a trip's append-only history.
type ReviewEntry struct {
	private
	Sequence                                        int64
	Kind                                            review.Kind
	Revision                                        int64
	State                                           review.State
	Reason                                          string
	ActorMembershipID, ActorEmployeeID, OperationID string
	RecordedAt                                      time.Time
}

// Standing is the trip's independent operational and review state
// (BR-REV-001): the review state is the latest revision's, never the
// operation's.
func Standing(operational trip.State, r Revisions) (trip.Standing, error) {
	return trip.NewStanding(operational, trip.ReviewState(r.State()))
}

// ---- decisions (B-BR-EMP-344/345) ----

// DecisionInput names the trip, the review revision judged, the decision and
// its reason.
type DecisionInput struct {
	private
	OperationID, TripID string
	ExpectedRevision    int64
	Decision            review.Kind
	Reason              string
}

type DecisionCommand struct {
	private
	value DecisionInput
}

// reasonProbe is a synthetic IN_REVIEW case whose only purpose is to apply
// the APPR-001 reason policy of a decision without duplicating it.
const (
	probeActor  = "00000000-0000-4000-8000-0000000000a1"
	probeAuthor = "00000000-0000-4000-8000-0000000000a2"
)

func decisionReason(kind review.Kind, reason string) error {
	c, err := review.Replay([]review.Entry{{Kind: review.Submit, Revision: 1, State: review.Submitted}, {Kind: review.Open, Revision: 1, State: review.InReview}})
	if err != nil {
		return ErrUnavailable
	}
	facts := review.Facts{Actor: review.Identity{Accounts: []string{probeActor}}, Author: review.Identity{Accounts: []string{probeAuthor}}, Permitted: true}
	_, err = c.Apply(review.Command{Kind: kind, Revision: 1, Reason: reason}, facts)
	return err
}

// NewDecisionCommand validates the decision shape and its reason profile.
func NewDecisionCommand(v DecisionInput) (DecisionCommand, error) {
	if !canonicalID(v.OperationID) || !canonicalID(v.TripID) || v.ExpectedRevision < 1 || v.ExpectedRevision > MaxReviewRevision {
		return DecisionCommand{}, review.ErrInvalid
	}
	switch v.Decision {
	case review.Approve, review.Return, review.Reject:
	default:
		return DecisionCommand{}, review.ErrInvalid
	}
	if err := decisionReason(v.Decision, v.Reason); err != nil {
		return DecisionCommand{}, err
	}
	return DecisionCommand{value: v}, nil
}

func (c DecisionCommand) Snapshot() (DecisionInput, error) {
	checked, err := NewDecisionCommand(c.value)
	return checked.value, err
}

// Digest binds every field; the reason travels only inside it.
func (c DecisionCommand) Digest(scope kernel.OrgScope) string {
	v, err := c.Snapshot()
	if err != nil || scope.Validate() != nil {
		return ""
	}
	raw, err := json.Marshal([]any{1, "trip.review.decide", scope.OrgID().String(), v.OperationID, v.TripID, v.ExpectedRevision, v.Decision, v.Reason})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// ReviewActor is the current company proof of the reviewer.
type ReviewActor struct {
	private
	AccountID, SessionID string
}

func (a ReviewActor) Validate() error {
	if !canonicalID(a.AccountID) || !canonicalID(a.SessionID) {
		return ErrReviewUnauthorized
	}
	return nil
}

// Reviewer is the actual reviewer resolved at receipt.
type Reviewer struct {
	private
	AccountID, MembershipID, EmployeeID string
}

// Person is one canonical person: an account, a membership and the employee
// records of its merge chain (APPR-001 Identity, EDR-129).
type Person struct {
	private
	AccountID, MembershipID string
	Employees               []string
}

func (p Person) identity() review.Identity {
	out := review.Identity{Employees: slices.Clone(p.Employees)}
	if p.AccountID != "" {
		out.Accounts = []string{p.AccountID}
	}
	if p.MembershipID != "" {
		out.Memberships = []string{p.MembershipID}
	}
	return out
}

// DecisionResult is rebuilt from the stored decision entry, so a replay
// returns exactly the first answer.
type DecisionResult struct {
	private
	TripID     string
	Decision   review.Kind
	State      review.State
	Revision   int64
	Reason     string
	RecordedAt time.Time
}

// ReviewView is the minimal decision context (B-BR-EMP-346) with the
// effective revision, its derived usage and every amendment
// (B-BR-EMP-367).
type ReviewView struct {
	private
	TripID, AssetID, DriverEmployeeID, ReportingDay string
	Operational                                     trip.State
	StartedAt                                       time.Time
	FinishedAt                                      *time.Time
	Review                                          trip.ReviewState
	Revision                                        int64
	History                                         []ReviewEntry
	EffectiveRevision                               int64
	EffectiveUsage                                  *RevisionUsage
	Amendments                                      []AmendmentView
}

func (v ReviewView) Clone() ReviewView {
	if v.FinishedAt != nil {
		at := *v.FinishedAt
		v.FinishedAt = &at
	}
	v.History = slices.Clone(v.History)
	v.EffectiveUsage = copyPointer(v.EffectiveUsage)
	if v.Amendments != nil {
		amendments := make([]AmendmentView, len(v.Amendments))
		for i, a := range v.Amendments {
			amendments[i] = a.clone()
		}
		v.Amendments = amendments
	}
	return v
}

func absentPort(port any) bool {
	if port == nil {
		return true
	}
	v := reflect.ValueOf(port)
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan:
		return v.IsNil()
	}
	return false
}

// QueueItem is one trip awaiting a decision (SUBMITTED or IN_REVIEW).
type QueueItem struct {
	private
	TripID, AssetID, DriverEmployeeID, ReportingDay string
	Review                                          trip.ReviewState
	Revision                                        int64
	SubmittedAt                                     time.Time
}

// QueueFilter pages the organization review queue after one trip.
type QueueFilter struct {
	private
	AfterID string
	Limit   int
}

func (f QueueFilter) Validate() error {
	if f.Limit < 1 || f.Limit > 100 || f.AfterID != "" && !canonicalID(f.AfterID) {
		return review.ErrInvalid
	}
	return nil
}

type QueuePage struct {
	private
	Items  []QueueItem
	NextID string
}

// ReviewRepository is the trip review store port; it owns the transaction,
// the in-transaction company authority and the audit event.
type ReviewRepository interface {
	DecideTripReview(context.Context, kernel.OrgScope, ReviewActor, string, DecisionCommand) (DecisionResult, bool, error)
	ReadTripReview(context.Context, kernel.OrgScope, ReviewActor, string, string) (ReviewView, error)
	ListTripReviews(context.Context, kernel.OrgScope, ReviewActor, string, QueueFilter) (QueuePage, error)
	ProposeTripAmendment(context.Context, kernel.OrgScope, ReviewActor, string, AmendmentCommand, [2]string) (AmendmentResult, bool, error)
}

// ReviewService serves the review routes and the amendment proposal; NewID
// pre-allocates the readings an approved amendment would create.
type ReviewService struct {
	Enabled    bool
	Repository ReviewRepository
	NewID      func() (string, error)
}

func authorizedReview(ctx context.Context, scope kernel.OrgScope, actor ReviewActor, decision access.Decision, target string, policy access.Policy) error {
	if ctx == nil {
		return ErrReviewUnauthorized
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if scope.Validate() != nil || actor.Validate() != nil || decision.Subject().Channel() != "web" || decision.Policy() != policy || decision.Check(ctx, scope, actor.AccountID, policy.Permission, target) != nil {
		return ErrReviewUnauthorized
	}
	return nil
}

func validEntry(e ReviewEntry) bool {
	return e.Sequence > 0 && slices.Contains(tripKinds, e.Kind) && e.Revision >= 1 && canonicalID(e.ActorMembershipID) && canonicalID(e.ActorEmployeeID) && canonicalID(e.OperationID) && !e.RecordedAt.IsZero()
}

// Decide proves the live request decision before and after the store and
// re-proves the stored result's shape.
func (s ReviewService) Decide(ctx context.Context, scope kernel.OrgScope, actor ReviewActor, decision access.Decision, command DecisionCommand) (DecisionResult, bool, error) {
	if !s.Enabled {
		return DecisionResult{}, false, ErrReviewDisabled
	}
	in, err := command.Snapshot()
	if err != nil {
		return DecisionResult{}, false, err
	}
	policy := ReviewPolicy(ReviewDecideAction)
	if err := authorizedReview(ctx, scope, actor, decision, in.TripID, policy); err != nil {
		return DecisionResult{}, false, err
	}
	if absentPort(s.Repository) {
		return DecisionResult{}, false, ErrUnavailable
	}
	result, replay, err := s.Repository.DecideTripReview(ctx, scope, actor, decision.Subject().MembershipID(), command)
	if guard := authorizedReview(ctx, scope, actor, decision, in.TripID, policy); guard != nil {
		return DecisionResult{}, false, guard
	}
	if err != nil {
		return DecisionResult{}, false, err
	}
	want := map[review.Kind]review.State{review.Approve: review.Approved, review.Return: review.Returned, review.Reject: review.Rejected}[in.Decision]
	if result.TripID != in.TripID || result.Decision != in.Decision || result.State != want || result.Revision != in.ExpectedRevision || result.Reason != in.Reason || result.RecordedAt.IsZero() {
		return DecisionResult{}, false, ErrUnavailable
	}
	return result, replay, nil
}

// List returns the organization review queue (EDR-222): only an ORG grant
// covers this organization-level read; structural reviewers use Read.
func (s ReviewService) List(ctx context.Context, scope kernel.OrgScope, actor ReviewActor, decision access.Decision, filter QueueFilter) (QueuePage, error) {
	if !s.Enabled {
		return QueuePage{}, ErrReviewDisabled
	}
	if err := filter.Validate(); err != nil {
		return QueuePage{}, err
	}
	policy := ReviewPolicy(ReviewReadAction)
	if err := authorizedReview(ctx, scope, actor, decision, scope.OrgID().String(), policy); err != nil {
		return QueuePage{}, err
	}
	if absentPort(s.Repository) {
		return QueuePage{}, ErrUnavailable
	}
	page, err := s.Repository.ListTripReviews(ctx, scope, actor, decision.Subject().MembershipID(), filter)
	if guard := authorizedReview(ctx, scope, actor, decision, scope.OrgID().String(), policy); guard != nil {
		return QueuePage{}, guard
	}
	if err != nil {
		return QueuePage{}, err
	}
	if len(page.Items) > filter.Limit || page.NextID != "" && (len(page.Items) != filter.Limit || page.Items[len(page.Items)-1].TripID != page.NextID) {
		return QueuePage{}, ErrUnavailable
	}
	seen := map[string]bool{}
	for _, v := range page.Items {
		if !canonicalID(v.TripID) || seen[v.TripID] || !canonicalID(v.AssetID) || !canonicalID(v.DriverEmployeeID) || v.ReportingDay == "" || v.Revision < 1 || v.SubmittedAt.IsZero() ||
			v.Review != trip.ReviewSubmitted && v.Review != trip.ReviewInReview {
			return QueuePage{}, ErrUnavailable
		}
		seen[v.TripID] = true
	}
	return QueuePage{Items: slices.Clone(page.Items), NextID: page.NextID}, nil
}

// Read returns the decision context of one trip under the same proof.
func (s ReviewService) Read(ctx context.Context, scope kernel.OrgScope, actor ReviewActor, decision access.Decision, tripID string) (ReviewView, error) {
	if !s.Enabled {
		return ReviewView{}, ErrReviewDisabled
	}
	if !canonicalID(tripID) {
		return ReviewView{}, ErrNotFound
	}
	policy := ReviewPolicy(ReviewReadAction)
	if err := authorizedReview(ctx, scope, actor, decision, tripID, policy); err != nil {
		return ReviewView{}, err
	}
	if absentPort(s.Repository) {
		return ReviewView{}, ErrUnavailable
	}
	view, err := s.Repository.ReadTripReview(ctx, scope, actor, decision.Subject().MembershipID(), tripID)
	if guard := authorizedReview(ctx, scope, actor, decision, tripID, policy); guard != nil {
		return ReviewView{}, guard
	}
	if err != nil {
		return ReviewView{}, err
	}
	for _, e := range view.History {
		if !validEntry(e) {
			return ReviewView{}, ErrUnavailable
		}
	}
	r, err := TripRevisions(view.History)
	if err != nil || view.TripID != tripID || !canonicalID(view.AssetID) || !canonicalID(view.DriverEmployeeID) || view.StartedAt.IsZero() || view.ReportingDay == "" ||
		trip.ReviewState(r.State()) != view.Review || r.Latest() != view.Revision || (view.Operational == trip.Completed) != (view.FinishedAt != nil) ||
		view.EffectiveRevision != r.Effective() || (view.EffectiveUsage == nil) != (r.Effective() == 0) || view.EffectiveUsage != nil && view.EffectiveUsage.Revision != r.Effective() {
		return ReviewView{}, ErrUnavailable
	}
	for _, a := range view.Amendments {
		if state, ok := r.RevisionState(a.Revision); !ok || state != a.State || a.Usage.Revision != a.Revision || a.Start == nil && a.End == nil {
			return ReviewView{}, ErrUnavailable
		}
	}
	if _, err := trip.NewStanding(view.Operational, view.Review); err != nil {
		return ReviewView{}, ErrUnavailable
	}
	return view.Clone(), nil
}

// Propose proves the live request decision (meter.reading.correct over the
// trip) before and after the store, pre-allocates the superseding readings
// and re-proves the stored result's shape (B-BR-EMP-365).
func (s ReviewService) Propose(ctx context.Context, scope kernel.OrgScope, actor ReviewActor, decision access.Decision, command AmendmentCommand) (AmendmentResult, bool, error) {
	if !s.Enabled {
		return AmendmentResult{}, false, ErrReviewDisabled
	}
	in, err := command.Snapshot()
	if err != nil {
		return AmendmentResult{}, false, err
	}
	policy := AmendPolicy()
	if err := authorizedReview(ctx, scope, actor, decision, in.TripID, policy); err != nil {
		return AmendmentResult{}, false, err
	}
	if absentPort(s.Repository) || s.NewID == nil {
		return AmendmentResult{}, false, ErrUnavailable
	}
	var ids [2]string
	for i := range ids {
		if ids[i], err = s.NewID(); err != nil || !canonicalID(ids[i]) {
			return AmendmentResult{}, false, ErrUnavailable
		}
	}
	if ids[0] == ids[1] {
		return AmendmentResult{}, false, ErrUnavailable
	}
	result, replay, err := s.Repository.ProposeTripAmendment(ctx, scope, actor, decision.Subject().MembershipID(), command, ids)
	if guard := authorizedReview(ctx, scope, actor, decision, in.TripID, policy); guard != nil {
		return AmendmentResult{}, false, guard
	}
	if err != nil {
		return AmendmentResult{}, false, err
	}
	if result.TripID != in.TripID || result.Revision < 2 || result.Base < 1 || result.Base >= result.Revision || result.State != review.Submitted || result.RecordedAt.IsZero() ||
		result.Intent != amend.IntentAmend && result.Intent != amend.IntentEdit {
		return AmendmentResult{}, false, ErrUnavailable
	}
	return result, replay, nil
}
