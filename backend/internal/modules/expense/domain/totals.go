package domain

import (
	"math/big"
	"regexp"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

// ReviewState is a closed input: the BR-REV states plus VOIDED (08-state-model).
// The review lifecycle that produces it is owned elsewhere.
type ReviewState string

const (
	Draft     ReviewState = "DRAFT"
	Submitted ReviewState = "SUBMITTED"
	InReview  ReviewState = "IN_REVIEW"
	Approved  ReviewState = "APPROVED"
	Returned  ReviewState = "RETURNED"
	Rejected  ReviewState = "REJECTED"
	Cancelled ReviewState = "CANCELLED"
	Voided    ReviewState = "VOIDED"
)

func ParseReviewState(raw string) (ReviewState, error) {
	s := ReviewState(raw)
	if _, e := s.Counting(); e != nil {
		return "", e
	}
	return s, nil
}

// Counting says how a review state enters cost totals (BR-REV-009, BR-EXP-002).
type Counting string

const (
	CountsApproved Counting = "APPROVED"
	CountsPending  Counting = "PENDING"
	CountsExcluded Counting = "EXCLUDED"
)

// Counting classifies a state: only APPROVED is cost, SUBMITTED and IN_REVIEW
// are the pending part, every other documented state is excluded.
func (s ReviewState) Counting() (Counting, error) {
	switch s {
	case Approved:
		return CountsApproved, nil
	case Submitted, InReview:
		return CountsPending, nil
	case Draft, Returned, Rejected, Cancelled, Voided:
		return CountsExcluded, nil
	}
	return "", ErrInvalid
}

// Basis selects the figure: approved only, or approved plus pending.
type Basis string

const (
	BasisApproved        Basis = "APPROVED"
	BasisIncludesPending Basis = "INCLUDES_PENDING"
)

func ParseBasis(raw string) (Basis, error) {
	switch b := Basis(raw); b {
	case BasisApproved, BasisIncludesPending:
		return b, nil
	}
	return "", ErrInvalid
}

var dayPattern = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)

// validDay accepts a real YYYY-MM-DD calendar day in years 0001..9999. The
// fixed width makes lexical order equal calendar order.
func validDay(raw string) bool {
	if !dayPattern.MatchString(raw) || raw[:4] == "0000" {
		return false
	}
	_, e := time.Parse("2006-01-02", raw)
	return e == nil
}

// ReportingPeriod is a half-open [start, end) range of YYYY-MM-DD reporting days.
type ReportingPeriod struct{ start, end string }

func NewReportingPeriod(start, end string) (ReportingPeriod, error) {
	if !validDay(start) || !validDay(end) || start >= end {
		return ReportingPeriod{}, ErrInvalid
	}
	return ReportingPeriod{start: start, end: end}, nil
}
func (p ReportingPeriod) Start() string { return p.start }
func (p ReportingPeriod) End() string   { return p.end }

// Contains reports start <= day < end; a malformed day or zero period contains nothing.
func (p ReportingPeriod) Contains(day string) bool {
	return p.start != "" && validDay(day) && day >= p.start && day < p.end
}

// Entry pairs an expense with its current review state.
type Entry struct {
	Expense Expense
	State   ReviewState
}

// CostTotals is an exact review-basis total for one organization and period.
type CostTotals struct {
	redacted
	org                                        kernel.OrgID
	period                                     ReportingPeriod
	basis                                      Basis
	approved, pending                          big.Int
	approvedCount, pendingCount, excludedCount int
}

// Totals sums company amounts on the declared basis. Each entry, in input
// order, must be a constructed expense with a documented state, belong to the
// scope (ErrScope) and period (ErrOutOfPeriod) and carry an identity and origin
// key not seen before (ErrDuplicateCost); the first failure returns no total.
// Sums are widened big integers, so no selection size can overflow them.
func Totals(scope kernel.OrgScope, period ReportingPeriod, basis Basis, entries []Entry) (CostTotals, error) {
	if scope.Validate() != nil {
		return CostTotals{}, ErrScope
	}
	if period.start == "" {
		return CostTotals{}, ErrInvalid
	}
	if _, e := ParseBasis(string(basis)); e != nil {
		return CostTotals{}, e
	}
	out := CostTotals{org: scope.OrgID(), period: period, basis: basis}
	identities := make(map[string]bool, len(entries))
	keys := make(map[OriginKey]bool, len(entries))
	for _, entry := range entries {
		x := entry.Expense
		if !x.valid {
			return CostTotals{}, ErrInvalid
		}
		counting, e := entry.State.Counting()
		if e != nil {
			return CostTotals{}, e
		}
		if x.reference.org != scope.OrgID() {
			return CostTotals{}, ErrScope
		}
		if !period.Contains(x.reporting.Day) {
			return CostTotals{}, ErrOutOfPeriod
		}
		key := x.OriginKey()
		if identities[x.reference.id] || keys[key] {
			return CostTotals{}, ErrDuplicateCost
		}
		identities[x.reference.id], keys[key] = true, true
		company := big.NewInt(x.amounts.company)
		switch counting {
		case CountsApproved:
			out.approved.Add(&out.approved, company)
			out.approvedCount++
		case CountsPending:
			out.pending.Add(&out.pending, company)
			out.pendingCount++
		default:
			out.excludedCount++
		}
	}
	return out, nil
}
func (t CostTotals) OrganizationID() kernel.OrgID { return t.org }
func (t CostTotals) Period() ReportingPeriod      { return t.period }
func (t CostTotals) Basis() Basis                 { return t.basis }

// ApprovedCentavos is the approved cost: APPROVED company amounts only.
func (t CostTotals) ApprovedCentavos() *big.Int { return new(big.Int).Set(&t.approved) }

// PendingCentavos is the separately identified SUBMITTED/IN_REVIEW part.
func (t CostTotals) PendingCentavos() *big.Int { return new(big.Int).Set(&t.pending) }
func (t CostTotals) ApprovedCount() int        { return t.approvedCount }
func (t CostTotals) PendingCount() int         { return t.pendingCount }
func (t CostTotals) ExcludedCount() int        { return t.excludedCount }

// FigureCentavos is the approved cost, plus the pending part only under
// BasisIncludesPending.
func (t CostTotals) FigureCentavos() *big.Int {
	figure := t.ApprovedCentavos()
	if t.IncludesPending() {
		figure.Add(figure, &t.pending)
	}
	return figure
}

// IncludesPending is the "includes pending" label; it follows the declared basis.
func (t CostTotals) IncludesPending() bool { return t.basis == BasisIncludesPending }
