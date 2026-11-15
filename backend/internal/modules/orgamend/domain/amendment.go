// Package domain decides append-only corrections of organization records
// (BR-AMD-001..006). It is pure: a consuming module supplies the record facts
// and the authority it evaluated at server receipt, then applies the decision
// inside its own transaction. It performs no I/O, persistence or audit write.
package domain

import (
	"errors"
	"math"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	orgaudit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
)

// Closed refusals. Each maps to one shared-vector error code.
var (
	ErrInvalid            = errors.New("orgamend: invalid")
	ErrConflict           = errors.New("orgamend: conflict")
	ErrApprovedImmutable  = errors.New("orgamend: approved revision is immutable")
	ErrNotEditable        = errors.New("orgamend: not editable")
	ErrNotApproved        = errors.New("orgamend: not approved")
	ErrAmendmentPending   = errors.New("orgamend: amendment pending")
	ErrAlreadyVoided      = errors.New("orgamend: already voided")
	ErrLostOriginal       = errors.New("orgamend: lost original")
	ErrReasonRequired     = errors.New("orgamend: reason required")
	ErrReasonInvalid      = errors.New("orgamend: reason invalid")
	ErrUnauthorized       = errors.New("orgamend: unauthorized")
	ErrPeriodLocked       = errors.New("orgamend: period locked")
	ErrPeriodNotLocked    = errors.New("orgamend: period not locked")
	ErrReopenUnauthorized = errors.New("orgamend: reopening unauthorized")
)

// MaxReasonRunes is the existing free-text reason bound (fleet module data
// table, meter reasons): 280 code points.
const MaxReasonRunes = 280

// ReviewState is the canonical BR-REV state of the latest revision, or VOIDED.
type ReviewState string

const (
	StateDraft     ReviewState = "DRAFT"
	StateSubmitted ReviewState = "SUBMITTED"
	StateInReview  ReviewState = "IN_REVIEW"
	StateReturned  ReviewState = "RETURNED"
	StateApproved  ReviewState = "APPROVED"
	StateRejected  ReviewState = "REJECTED"
	StateCancelled ReviewState = "CANCELLED"
	StateVoided    ReviewState = "VOIDED"
)

// PeriodState is the lock state of the period holding a reporting day.
type PeriodState string

const (
	PeriodOpen     PeriodState = "OPEN"
	PeriodLocked   PeriodState = "LOCKED"
	PeriodReopened PeriodState = "REOPENED"
)

// Intent is what the actor asks for. The kernel never reclassifies it.
type Intent string

const (
	IntentEdit   Intent = "EDIT"
	IntentAmend  Intent = "AMEND"
	IntentVoid   Intent = "VOID"
	IntentAdjust Intent = "ADJUST"
)

// Outcome is the accepted decision.
type Outcome string

const (
	OutcomeEdit       Outcome = "EDIT"
	OutcomeAmendment  Outcome = "AMENDMENT"
	OutcomeVoid       Outcome = "VOID"
	OutcomeAdjustment Outcome = "ADJUSTMENT"
)

// Cause names why derived data must be recalculated (BR-AMD-005).
type Cause string

const (
	CauseAmended  Cause = "AMENDED"
	CauseVoided   Cause = "VOIDED"
	CauseAdjusted Cause = "ADJUSTED"
)

// Reopening is the period owner's evidence that a reopening was authorized at
// its server receipt with a reason. A REOPENED state alone proves nothing.
type Reopening struct {
	AuthorityProven bool
	Reason          string
}

// Period is the state of the period holding a day. Reopening is supplied only
// for REOPENED.
type Period struct {
	State     PeriodState
	Reopening *Reopening
}

// Facts are the record's current server facts, read under the consumer's lock.
// Effective is the approved revision in effect (0 when never approved).
type Facts struct {
	RecordID     string
	State        ReviewState
	Effective    int64
	Latest       int64
	ReportingDay string
	Period       Period
}

// Authority holds the consumer's receipt-time permission results: Edit for a
// never-approved record, Amend for amendments, edits of a pending amendment
// and adjustments, Void for voids.
type Authority struct {
	Edit  bool
	Amend bool
	Void  bool
}

// Request is the actor's command plus the consumer's append plan. Expected is
// the latest revision the actor saw; Revision is the revision the consumer
// will append (0 for VOID/ADJUST); Base is the retained revision it links to.
// At is the server receipt. PostingDay/PostingPeriod are used only by ADJUST.
type Request struct {
	Intent        Intent
	Expected      int64
	Revision      int64
	Base          int64
	ActorID       string
	At            time.Time
	Reason        string
	Authority     Authority
	PostingDay    string
	PostingPeriod *Period
}

// Recalculation names the record and the revision derived data must now use
// (or exclude, for a void), from Day and At. Consumers own the jobs.
type Recalculation struct {
	RecordID   string
	Revision   int64
	Superseded int64
	Cause      Cause
	Day        string
	At         time.Time
}

// Decision is what the consumer appends. Prior revisions are never changed.
type Decision struct {
	Outcome      Outcome
	RecordID     string
	Revision     int64
	Base         int64
	Effective    int64
	Voided       bool
	ActorID      string
	At           time.Time
	Reason       string
	ReportingDay string
	PostingDay   string
	Derived      *Recalculation
}

// Review is a review result on the pending amendment revision.
type Review struct {
	Revision int64
	Result   ReviewState
	At       time.Time
}

// Resolution is the effect of a review result on the effective revision.
type Resolution struct {
	Result     ReviewState
	RecordID   string
	Revision   int64
	Effective  int64
	Superseded int64
	Pending    bool
	Derived    *Recalculation
}

// Decide classifies one correction or refuses it with a closed error. The
// first failing check wins: INVALID, CONFLICT, ALREADY_VOIDED, state, append
// plan, reason, authority, period.
func Decide(f Facts, r Request) (Decision, error) {
	if err := validFacts(f); err != nil {
		return Decision{}, err
	}
	if err := validRequest(f, r); err != nil {
		return Decision{}, err
	}
	if r.Expected != f.Latest {
		return Decision{}, ErrConflict
	}
	if f.State == StateVoided {
		return Decision{}, ErrAlreadyVoided
	}
	if err := stateAllows(f, r.Intent); err != nil {
		return Decision{}, err
	}
	if err := appendPlan(f, r); err != nil {
		return Decision{}, err
	}
	if err := checkReason(r.Reason, r.Intent != IntentEdit); err != nil {
		return Decision{}, err
	}
	if !authorized(f, r) {
		return Decision{}, ErrUnauthorized
	}
	if err := periodAllows(f, r); err != nil {
		return Decision{}, err
	}
	at := r.At.UTC()
	d := Decision{
		RecordID: f.RecordID, Revision: r.Revision, Base: r.Base, Effective: f.Effective,
		ActorID: r.ActorID, At: at, Reason: r.Reason, ReportingDay: f.ReportingDay, PostingDay: r.PostingDay,
	}
	switch r.Intent {
	case IntentEdit:
		d.Outcome = OutcomeEdit
	case IntentAmend:
		d.Outcome = OutcomeAmendment
	case IntentVoid:
		d.Outcome, d.Voided = OutcomeVoid, true
		d.Derived = &Recalculation{RecordID: f.RecordID, Revision: f.Effective, Cause: CauseVoided, Day: f.ReportingDay, At: at}
	case IntentAdjust:
		d.Outcome = OutcomeAdjustment
		d.Derived = &Recalculation{RecordID: f.RecordID, Revision: f.Effective, Cause: CauseAdjusted, Day: r.PostingDay, At: at}
	}
	return d, nil
}

// Resolve applies a review result to the pending amendment. Review transition
// legality, reviewer authority and self-approval belong to the review lifecycle.
func Resolve(f Facts, v Review) (Resolution, error) {
	if err := validFacts(f); err != nil {
		return Resolution{}, err
	}
	switch v.Result {
	case StateApproved, StateReturned, StateRejected, StateCancelled:
	default:
		return Resolution{}, ErrInvalid
	}
	if v.Revision < 1 || !validInstant(v.At) {
		return Resolution{}, ErrInvalid
	}
	if v.Revision != f.Latest {
		return Resolution{}, ErrConflict
	}
	if f.State == StateVoided {
		return Resolution{}, ErrAlreadyVoided
	}
	if f.Effective == 0 {
		return Resolution{}, ErrNotApproved
	}
	if !pending(f) {
		return Resolution{}, ErrConflict
	}
	out := Resolution{Result: v.Result, RecordID: f.RecordID, Revision: f.Latest, Effective: f.Effective}
	switch v.Result {
	case StateApproved:
		open, err := writable(f.Period)
		if err != nil {
			return Resolution{}, err
		}
		if !open {
			return Resolution{}, ErrPeriodLocked
		}
		out.Effective, out.Superseded = f.Latest, f.Effective
		out.Derived = &Recalculation{
			RecordID: f.RecordID, Revision: f.Latest, Superseded: f.Effective,
			Cause: CauseAmended, Day: f.ReportingDay, At: v.At.UTC(),
		}
	case StateReturned:
		out.Pending = true
	}
	return out, nil
}

func validFacts(f Facts) error {
	if !orgaudit.ValidID(f.RecordID) || !validState(f.State) || !validDay(f.ReportingDay) || !validPeriod(f.Period) ||
		f.Latest < 1 || f.Effective < 0 || f.Effective > f.Latest {
		return ErrInvalid
	}
	approved := f.State == StateApproved || f.State == StateVoided
	switch {
	case f.Effective == 0 && approved,
		f.Effective > 0 && f.Effective == f.Latest && !approved,
		f.Effective > 0 && f.Effective < f.Latest && f.State == StateApproved:
		return ErrInvalid
	}
	return nil
}

func validRequest(f Facts, r Request) error {
	switch r.Intent {
	case IntentEdit, IntentAmend, IntentVoid, IntentAdjust:
	default:
		return ErrInvalid
	}
	if !orgaudit.ValidID(r.ActorID) || !validInstant(r.At) || r.Expected < 1 || r.Revision < 0 || r.Base < 0 {
		return ErrInvalid
	}
	if (r.Intent == IntentEdit || r.Intent == IntentAmend) && f.Latest == math.MaxInt64 {
		return ErrInvalid
	}
	if r.Intent != IntentAdjust {
		if r.PostingDay != "" || r.PostingPeriod != nil {
			return ErrInvalid
		}
		return nil
	}
	// Equal-length canonical days order lexically.
	if !validDay(r.PostingDay) || r.PostingDay <= f.ReportingDay || r.PostingPeriod == nil || !validPeriod(*r.PostingPeriod) {
		return ErrInvalid
	}
	return nil
}

func validState(s ReviewState) bool {
	switch s {
	case StateDraft, StateSubmitted, StateInReview, StateReturned,
		StateApproved, StateRejected, StateCancelled, StateVoided:
		return true
	}
	return false
}

// validPeriod accepts the closed states; reopening evidence belongs only to REOPENED.
func validPeriod(p Period) bool {
	switch p.State {
	case PeriodOpen, PeriodLocked:
		return p.Reopening == nil
	case PeriodReopened:
		return true
	}
	return false
}

// validDay accepts a canonical YYYY-MM-DD reporting day (BR-TIME-004 format).
func validDay(day string) bool {
	parsed, err := time.Parse(time.DateOnly, day)
	return err == nil && parsed.Year() >= 1 && parsed.Format(time.DateOnly) == day
}

// validInstant refuses the zero instant: an unset receipt is never authority time.
func validInstant(at time.Time) bool {
	year := at.UTC().Year()
	return !at.IsZero() && year >= 1 && year <= 9999
}

// pending reports an amendment above the effective revision still under way.
func pending(f Facts) bool {
	switch f.State {
	case StateDraft, StateSubmitted, StateInReview, StateReturned:
		return f.Effective > 0 && f.Latest > f.Effective
	}
	return false
}

func stateAllows(f Facts, intent Intent) error {
	if intent == IntentEdit {
		switch {
		case f.State == StateDraft || f.State == StateReturned:
			return nil
		case f.State == StateSubmitted || f.State == StateInReview || f.Effective == 0:
			return ErrNotEditable
		}
		return ErrApprovedImmutable
	}
	if f.Effective == 0 {
		return ErrNotApproved
	}
	if pending(f) {
		return ErrAmendmentPending
	}
	return nil
}

// appendPlan verifies that the consumer appends exactly the next revision (or
// none) and links it to the revision that must be retained.
func appendPlan(f Facts, r Request) error {
	base := f.Effective
	switch r.Intent {
	case IntentEdit, IntentAmend:
		if r.Revision <= f.Latest {
			return ErrLostOriginal
		}
		if r.Revision != f.Latest+1 {
			return ErrInvalid
		}
		if r.Intent == IntentEdit {
			base = f.Latest
		}
	default:
		if r.Revision != 0 {
			return ErrInvalid
		}
	}
	if r.Base != base {
		return ErrLostOriginal
	}
	return nil
}

// checkReason applies the existing 280 code point free-text profile. An
// optional reason may be absent, but a supplied one must be a real reason.
func checkReason(raw string, required bool) error {
	switch {
	case raw == "":
		if required {
			return ErrReasonRequired
		}
		return nil
	case !utf8.ValidString(raw):
		return ErrReasonInvalid
	case strings.TrimSpace(raw) == "":
		if required {
			return ErrReasonRequired
		}
		return ErrReasonInvalid
	case utf8.RuneCountInString(raw) > MaxReasonRunes:
		return ErrReasonInvalid
	}
	for _, point := range raw {
		if unicode.IsControl(point) {
			return ErrReasonInvalid
		}
	}
	return nil
}

func authorized(f Facts, r Request) bool {
	switch r.Intent {
	case IntentEdit:
		if f.Effective == 0 {
			return r.Authority.Edit
		}
		return r.Authority.Amend
	case IntentVoid:
		return r.Authority.Void
	}
	return r.Authority.Amend
}

// writable reports whether a period accepts corrections of approved data. A
// REOPENED state qualifies only with proven, reasoned reopening evidence.
func writable(p Period) (bool, error) {
	switch p.State {
	case PeriodOpen:
		return true, nil
	case PeriodLocked:
		return false, nil
	}
	if p.Reopening == nil || !p.Reopening.AuthorityProven {
		return false, ErrReopenUnauthorized
	}
	if err := checkReason(p.Reopening.Reason, true); err != nil {
		return false, err
	}
	return true, nil
}

func periodAllows(f Facts, r Request) error {
	if r.Intent == IntentEdit {
		return nil
	}
	open, err := writable(f.Period)
	if err != nil {
		return err
	}
	if r.Intent != IntentAdjust {
		if !open {
			return ErrPeriodLocked
		}
		return nil
	}
	if open {
		return ErrPeriodNotLocked
	}
	if open, err = writable(*r.PostingPeriod); err != nil {
		return err
	}
	if !open {
		return ErrPeriodLocked
	}
	return nil
}
