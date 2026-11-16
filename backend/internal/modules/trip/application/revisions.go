package application

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"time"

	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	amend "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgamend/domain"
	review "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain"
	period "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain/period"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
)

// EMP-TRIP-015 trip revisions, late and duplicate facts and amendments after
// closure (B-BR-EMP-363..368, EDR-234..237). Server-only (EDR-236): the
// device only queues trip.submit and trip.event.record; amendments are
// online company commands decided by the orgamend kernel.

const (
	// StaleRevision holds a late fact or a stale submission (OFFL-001 code).
	StaleRevision = "STALE_REVISION"
	// Amend is the history kind that opens an amendment revision. It is a
	// trip history kind, never an APPR-001 command (EDR-234).
	Amend review.Kind = "AMEND"
	// AmendAuditAction is the audit action of a proposed amendment.
	AmendAuditAction = "trip.trip.amended"
	// AmendOperation names the amendment command in its digest.
	amendDigestVersion = "trip.amendment.propose.v1"
)

// ErrInvalidAmendment: a correction that changes no existing reading of the
// effective revision in its own kind (VALIDATION_FAILED).
var ErrInvalidAmendment = errors.New("trip: invalid amendment")

// AmendPolicy is meter.reading.correct over the trip's stored structure: an
// amendment of an approved trip corrects its meter evidence (trips §7,
// EDR-236); fresh proof and configuration write, like METER-004 decisions.
func AmendPolicy() access.Policy {
	return access.Policy{Permission: "meter.reading.correct", Capability: "organizations", Effect: access.ConfigurationWrite, Fresh: true}
}

// Revisions reads a trip's append-only review history as record revisions
// (EDR-234): the field segment (SUBMIT, RESUBMIT and their decisions) and one
// segment per AMEND entry. Each segment is an APPR-001 case; an amendment
// segment starts SUBMITTED at its own revision.
type Revisions struct {
	private
	segment   review.Case
	offset    int64
	effective int64
	amendment *ReviewEntry
	states    map[int64]review.State
	entries   int
}

// State is the review state of the latest revision.
func (r Revisions) State() review.State { return r.segment.State() }

// Latest is the latest revision (1 for a draft without history).
func (r Revisions) Latest() int64 { return r.segment.Revision() + r.offset }

// Effective is the approved revision in effect, 0 when never approved.
func (r Revisions) Effective() int64 { return r.effective }

// Entries is the number of stored entries read.
func (r Revisions) Entries() int { return r.entries }

// Amendment returns the AMEND entry that opened the current segment.
func (r Revisions) Amendment() (ReviewEntry, bool) {
	if r.amendment == nil {
		return ReviewEntry{}, false
	}
	return *r.amendment, true
}

// RevisionState is the latest review state recorded for one revision.
func (r Revisions) RevisionState(revision int64) (review.State, bool) {
	s, ok := r.states[revision]
	return s, ok
}

// FieldOpen reports a draft or a returned field revision: the author's
// field facts still belong to the revision being prepared (BR-AMD-006).
func (r Revisions) FieldOpen() bool {
	return r.amendment == nil && r.effective == 0 && (r.State() == review.Draft || r.State() == review.Returned)
}

var tripKinds = []review.Kind{review.Submit, review.Resubmit, review.Open, review.Approve, review.Return, review.Reject, Amend}

// TripRevisions rebuilds the revisions of stored entries, refusing any
// history the lifecycle, the amendment rules or this cut could not produce.
func TripRevisions(entries []ReviewEntry) (Revisions, error) {
	out := Revisions{segment: review.NewDraft(), states: map[int64]review.State{}, entries: len(entries)}
	var local []review.Entry
	for i, e := range entries {
		if e.Sequence != int64(i+1) || !slices.Contains(tripKinds, e.Kind) || !canonicalID(e.ActorMembershipID) || !canonicalID(e.ActorEmployeeID) ||
			!canonicalID(e.OperationID) || e.RecordedAt.IsZero() || e.Revision < 1 || e.Revision > MaxReviewRevision {
			return Revisions{}, ErrUnavailable
		}
		if e.Kind == Amend {
			state := out.segment.State()
			opens := state == review.Approved || out.amendment != nil && (state == review.Rejected || state == review.Returned)
			if out.effective == 0 || !opens || e.Revision != out.Latest()+1 || e.State != review.Submitted || period.CheckReason(e.Reason) != nil {
				return Revisions{}, ErrUnavailable
			}
			local = []review.Entry{{Kind: review.Submit, Revision: 1, State: review.Submitted}}
			segment, err := review.Replay(local)
			if err != nil {
				return Revisions{}, ErrUnavailable
			}
			opened := e
			out.segment, out.offset, out.amendment = segment, e.Revision-1, &opened
			out.states[e.Revision] = e.State
			continue
		}
		if e.Kind == review.Resubmit && out.amendment != nil || e.Kind == review.Submit && i != 0 || e.Revision <= out.offset {
			return Revisions{}, ErrUnavailable
		}
		local = append(local, review.Entry{Kind: e.Kind, Revision: e.Revision - out.offset, State: e.State, Reason: e.Reason})
		segment, err := review.Replay(local)
		if err != nil {
			return Revisions{}, ErrUnavailable
		}
		out.segment = segment
		out.states[e.Revision] = e.State
		if e.State == review.Approved {
			out.effective = e.Revision
		}
	}
	return out, nil
}

// Facts maps the revisions to the orgamend kernel facts of the trip, whose
// reporting day is the stored start day (EDR-182).
func (r Revisions) Facts(tripID, reportingDay string, p amend.Period) amend.Facts {
	return amend.Facts{RecordID: tripID, State: amend.ReviewState(r.State()), Effective: r.effective, Latest: r.Latest(), ReportingDay: reportingDay, Period: p}
}

// SubmitKind decides a trip.submit at receipt (B-BR-EMP-343/363/364): only
// an operationally COMPLETED trip is submitted; the named revision must be
// the current DRAFT (SUBMIT) or returned field revision (RESUBMIT); any
// other named revision is stale.
func SubmitKind(operational trip.State, r Revisions, revision int64) (review.Kind, error) {
	if operational != trip.Completed {
		return "", ErrSubmitPending
	}
	if !r.FieldOpen() || r.Latest() != revision {
		return "", ErrSubmitStale
	}
	if r.State() == review.Draft {
		return review.Submit, nil
	}
	return review.Resubmit, nil
}

// SubmitEntry applies the author's SUBMIT or RESUBMIT through the APPR-001
// lifecycle and returns the absolute entry to append.
func SubmitEntry(r Revisions, kind review.Kind, revision int64, submitter, author Person) (review.Entry, error) {
	if (kind != review.Submit && kind != review.Resubmit) || r.amendment != nil {
		return review.Entry{}, ErrUnavailable
	}
	next, err := r.segment.Apply(review.Command{Kind: kind, Revision: revision}, review.Facts{Actor: submitter.identity(), Author: author.identity(), Permitted: true})
	if err != nil {
		return review.Entry{}, err
	}
	history := next.History()
	return history[len(history)-1], nil
}

// merge is one identity holding every given person (BR-REV-003 against the
// trip's author and an amendment's author at once).
func merge(people ...Person) review.Identity {
	var out review.Identity
	for _, p := range people {
		id := p.identity()
		out.Accounts = append(out.Accounts, id.Accounts...)
		out.Memberships = append(out.Memberships, id.Memberships...)
		out.Employees = append(out.Employees, id.Employees...)
	}
	return out
}

// DecideRevision applies one decision to the current segment (EDR-222/235):
// a SUBMITTED revision is opened and decided, an IN_REVIEW one decided. The
// reviewer is never the trip's author, its driver, for an amendment the
// amendment's author, nor any other party given (EMP-TRIP-014: the closer;
// EMP-TRIP-012: every party of a handover). It returns the absolute entries
// to append.
func DecideRevision(r Revisions, in DecisionInput, reviewer, author, driver Person, amender *Person, others ...Person) ([]review.Entry, error) {
	people := []Person{author}
	if amender != nil {
		people = append(people, *amender)
	}
	// EMP-TRIP-014 (B-BR-EMP-401): the member who closed the trip is one of
	// its authors for every review decision.
	authors := merge(append(people, others...)...)
	beneficiary := driver.identity()
	facts := review.Facts{Actor: reviewer.identity(), Author: authors, Beneficiary: &beneficiary, Permitted: true}
	// A revision of an earlier segment is stale; keep APPR-001's own order.
	local := in.ExpectedRevision - r.offset
	if local < 1 {
		local = r.segment.Revision() + 1
	}
	c := r.segment
	before := len(c.History())
	if c.State() == review.Submitted {
		opened, err := c.Apply(review.Command{Kind: review.Open, Revision: local}, facts)
		if err != nil {
			return nil, err
		}
		c = opened
	}
	decided, err := c.Apply(review.Command{Kind: in.Decision, Revision: local, Reason: in.Reason}, facts)
	if err != nil {
		return nil, err
	}
	added := decided.History()[before:]
	out := make([]review.Entry, 0, len(added))
	for _, e := range added {
		e.Revision += r.offset
		out = append(out, e)
	}
	return out, nil
}

// ---- amendments (B-BR-EMP-365/366) ----

// Point names a trip reading an amendment corrects.
type Point string

const (
	StartPoint Point = "START"
	EndPoint   Point = "END"
)

// Correction is one corrected reading value in exact base units.
type Correction struct {
	private
	Kind  meter.Kind
	Value int64
}

// AmendmentInput names the trip, the latest revision the actor saw, the
// reason and the corrected start and/or end reading.
type AmendmentInput struct {
	private
	OperationID, TripID string
	ExpectedRevision    int64
	Reason              string
	Start, End          *Correction
}

type AmendmentCommand struct {
	private
	value AmendmentInput
}

func validCorrection(c *Correction) bool {
	if c == nil {
		return true
	}
	if c.Kind != meter.Odometer && c.Kind != meter.HourMeter {
		return false
	}
	_, err := meter.NewValue(c.Kind, c.Value, nil)
	return err == nil
}

// NewAmendmentCommand validates the shape: identifiers, the expected
// revision, the B-BR-EMP-163 reason (required) and at least one correction
// of a known meter kind in range.
func NewAmendmentCommand(v AmendmentInput) (AmendmentCommand, error) {
	if !canonicalID(v.OperationID) || !canonicalID(v.TripID) || v.ExpectedRevision < 1 || v.ExpectedRevision > MaxReviewRevision {
		return AmendmentCommand{}, review.ErrInvalid
	}
	if err := period.CheckReason(v.Reason); err != nil {
		return AmendmentCommand{}, err
	}
	if v.Start == nil && v.End == nil || !validCorrection(v.Start) || !validCorrection(v.End) {
		return AmendmentCommand{}, ErrInvalidAmendment
	}
	v.Start, v.End = copyPointer(v.Start), copyPointer(v.End)
	return AmendmentCommand{value: v}, nil
}

func (c AmendmentCommand) Snapshot() (AmendmentInput, error) {
	checked, err := NewAmendmentCommand(c.value)
	return checked.value, err
}

func correctionDigest(c *Correction) []any {
	if c == nil {
		return []any{"", ""}
	}
	return []any{string(c.Kind), c.Value}
}

// Digest binds every field, values and reason included, to the organization.
func (c AmendmentCommand) Digest(scope kernel.OrgScope) string {
	v, err := c.Snapshot()
	if err != nil || scope.Validate() != nil {
		return ""
	}
	fields := []any{amendDigestVersion, scope.OrgID().String(), v.OperationID, v.TripID, v.ExpectedRevision, v.Reason}
	fields = append(fields, correctionDigest(v.Start)...)
	fields = append(fields, correctionDigest(v.End)...)
	raw, err := json.Marshal(fields)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// ProposeAmendment decides a proposal with the orgamend kernel on server
// facts (EDR-236): AMEND from an approved revision in effect (base = the
// effective revision), EDIT of a returned pending amendment (base = that
// revision); the next revision is appended SUBMITTED. Authority is the
// caller's proven meter.reading.correct; EDIT is gated by the period too.
func ProposeAmendment(r Revisions, tripID, reportingDay string, p amend.Period, c AmendmentCommand, actorAccount string, at time.Time) (amend.Decision, error) {
	v, err := c.Snapshot()
	if err != nil {
		return amend.Decision{}, err
	}
	if r.Latest() >= MaxReviewRevision {
		return amend.Decision{}, ErrUnavailable
	}
	intent, base := amend.IntentAmend, r.effective
	if r.amendment != nil && r.State() == review.Returned {
		intent, base = amend.IntentEdit, r.Latest()
		if p.State == amend.PeriodLocked {
			return amend.Decision{}, amend.ErrPeriodLocked
		}
	}
	return amend.Decide(r.Facts(tripID, reportingDay, p), amend.Request{Intent: intent, Expected: v.ExpectedRevision, Revision: r.Latest() + 1, Base: base,
		ActorID: actorAccount, At: at, Reason: v.Reason, Authority: amend.Authority{Amend: true}})
}

// ResolveAmendment applies a review result to the pending amendment
// (BR-AMD-002): an approval makes it effective in a writable period only.
func ResolveAmendment(r Revisions, tripID, reportingDay string, p amend.Period, result review.State, at time.Time) (amend.Resolution, error) {
	return amend.Resolve(r.Facts(tripID, reportingDay, p), amend.Review{Revision: r.Latest(), Result: amend.ReviewState(result), At: at})
}

// ---- revision usage (B-BR-EMP-367) ----

// CorrectionRecord is one stored correction of an amendment.
type CorrectionRecord struct {
	private
	MeterID, ReadingID, NewReadingID string
	From, To                         int64
}

// AmendmentRecord is one stored amendment revision.
type AmendmentRecord struct {
	private
	Revision, Base                                               int64
	Intent                                                       amend.Intent
	Start, End                                                   *CorrectionRecord
	ActorMembershipID, ActorEmployeeID, OperationID, RequestHash string
	RecordedAt                                                   time.Time
}

// RevisionUsage is the meter usage derived from one named revision.
type RevisionUsage struct {
	private
	Revision int64
	Status   trip.UsageStatus
	Reason   trip.UsageReason
	Kind     meter.Kind
	Delta    int64
}

// Values returns the start and end reading values of a revision: the stored
// field values, then every approved amendment up to it, then the revision's
// own corrections when it is an amendment.
func Values(record Record, amendments []AmendmentRecord, r Revisions, revision int64) (start, end *int64) {
	start = copyPointer(record.ReadingValue)
	if record.Finish != nil {
		end = copyPointer(record.Finish.ReadingValue)
	}
	for _, a := range amendments {
		state, _ := r.RevisionState(a.Revision)
		if a.Revision > revision || a.Revision < revision && state != review.Approved {
			continue
		}
		if a.Start != nil {
			to := a.Start.To
			start = &to
		}
		if a.End != nil {
			to := a.End.To
			end = &to
		}
	}
	return start, end
}

// UsageOf derives the usage of a revision with the TRIP-001 rule on the
// stored trip with that revision's values; nothing stored is rewritten.
func UsageOf(scope kernel.OrgScope, record Record, revision int64, start, end *int64) (RevisionUsage, error) {
	shaped := record
	shaped.ReadingValue = copyPointer(start)
	if record.Finish != nil {
		finish := *record.Finish
		finish.ReadingValue = copyPointer(end)
		shaped.Finish = &finish
	}
	stored, err := StoredDomain(scope, shaped)
	if err != nil {
		return RevisionUsage{}, ErrUnavailable
	}
	usage := stored.MeterUsage()
	delta, _ := usage.Delta()
	return RevisionUsage{Revision: revision, Status: usage.Status(), Reason: usage.Reason(), Kind: usage.Kind(), Delta: delta}, nil
}

// AmendmentResult is rebuilt from the stored amendment and its AMEND entry,
// so a replay returns exactly the first answer.
type AmendmentResult struct {
	private
	TripID         string
	Revision, Base int64
	Intent         amend.Intent
	State          review.State
	RecordedAt     time.Time
}

// AmendmentView is one amendment revision in the decision context
// (B-BR-EMP-367): its corrections, current review state, author, reason and
// the usage derived from it.
type AmendmentView struct {
	private
	Revision, Base     int64
	Intent             amend.Intent
	State              review.State
	Reason             string
	AuthorMembershipID string
	RecordedAt         time.Time
	Start, End         *CorrectionRecord
	Usage              RevisionUsage
}

func (v AmendmentView) clone() AmendmentView {
	v.Start, v.End = copyPointer(v.Start), copyPointer(v.End)
	return v
}
