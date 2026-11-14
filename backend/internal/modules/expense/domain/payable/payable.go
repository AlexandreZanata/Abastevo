// Package payable models pure enterprise payables (EMP-FIN-001): supplier,
// reimbursement and card statement obligations recognized from approved
// expenses, exact settlement entries, the derived settlement state and the
// separation of cost and settlement views (EDR-013, BR-FIN-001). It never
// creates a cost, a payment, an accounting entry or money movement.
package payable

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"unicode"
	"unicode/utf8"

	expense "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/expense/domain"
	fueling "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/metrics"
	exact "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgvalue/domain"
	review "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain"
)

// Closed failure vocabulary (B-BR-EMP-194..197).
var (
	ErrInvalid              = errors.New("payable: invalid value")
	ErrScope                = errors.New("payable: organization scope denied")
	ErrNotEligible          = errors.New("payable: source not eligible for this obligation")
	ErrExceedsSource        = errors.New("payable: line exceeds its source company amount")
	ErrPartialReimbursement = errors.New("payable: reimbursement below the approved company amount")
	ErrDuplicateSource      = errors.New("payable: expense already a source of a live obligation")
	ErrDuplicateEntry       = errors.New("payable: payable or settlement identity repeated")
	ErrExceedsRemaining     = errors.New("payable: settlement exceeds the remaining amount")
	ErrCancelled            = errors.New("payable: payable cancelled")
	ErrHasSettlement        = errors.New("payable: payable has a settlement")
	ErrStateMismatch        = errors.New("payable: asserted state contradicts the derived state")
	// ErrSecondCost is the expense refusal of a settlement-side cost origin:
	// an obligation, settlement or statement identity posing as a cost.
	ErrSecondCost = fmt.Errorf("payable: obligation or settlement presented as a cost: %w", expense.ErrNotCostOrigin)
)

type redacted struct{}

func (redacted) String() string                 { return "EnterprisePayableValue(redacted)" }
func (r redacted) Format(f fmt.State, _ rune)   { _, _ = io.WriteString(f, r.String()) }
func (r redacted) LogValue() slog.Value         { return slog.StringValue(r.String()) }
func (r redacted) MarshalJSON() ([]byte, error) { return json.Marshal(r.String()) }

// Kind is the closed obligation kind; ADVANCE belongs to EMP-FIN-003.
type Kind string

const (
	Supplier      Kind = "SUPPLIER"
	Reimbursement Kind = "REIMBURSEMENT"
	CardStatement Kind = "CARD_STATEMENT"
)

// ParseKind accepts only an exact obligation kind token.
func ParseKind(raw string) (Kind, error) {
	switch k := Kind(raw); k {
	case Supplier, Reimbursement, CardStatement:
		return k, nil
	}
	return "", ErrInvalid
}

// State is the payable state of the canonical catalog (08-state-model).
type State string

const (
	Open             State = "OPEN"
	PartiallySettled State = "PARTIALLY_SETTLED"
	Settled          State = "SETTLED"
	Cancelled        State = "CANCELLED"
)

// ParseState accepts only an exact payable state token.
func ParseState(raw string) (State, error) {
	switch s := State(raw); s {
	case Open, PartiallySettled, Settled, Cancelled:
		return s, nil
	}
	return "", ErrInvalid
}

type scopedID struct {
	redacted
	org kernel.OrgID
	id  string
}

func (r scopedID) ID() string                   { return r.id }
func (r scopedID) OrganizationID() kernel.OrgID { return r.org }

func canonical(id string) bool { v, e := kernel.ParseOrgID(id); return e == nil && v.String() == id }

// shaped reports a constructed scoped identity of any package.
func shaped(id string, org kernel.OrgID) bool { return canonical(id) && org != (kernel.OrgID{}) }

func scoped(scope kernel.OrgScope, id string) (scopedID, error) {
	if scope.Validate() != nil {
		return scopedID{}, ErrScope
	}
	if !canonical(id) {
		return scopedID{}, ErrInvalid
	}
	return scopedID{org: scope.OrgID(), id: id}, nil
}

// PayableRef, SettlementRef and StatementRef are scoped identities; their
// shape proves no existence.
type PayableRef struct{ scopedID }
type SettlementRef struct{ scopedID }
type StatementRef struct{ scopedID }

func NewPayableRef(scope kernel.OrgScope, id string) (PayableRef, error) {
	r, e := scoped(scope, id)
	return PayableRef{r}, e
}
func NewSettlementRef(scope kernel.OrgScope, id string) (SettlementRef, error) {
	r, e := scoped(scope, id)
	return SettlementRef{r}, e
}
func NewStatementRef(scope kernel.OrgScope, id string) (StatementRef, error) {
	r, e := scoped(scope, id)
	return StatementRef{r}, e
}

// Source is the server-resolved fact about one referenced expense: the
// constructed EMP-EXP-001 value, its current review state and its explicit
// payer, with the paying employee exactly for EMPLOYEE.
type Source struct {
	redacted
	Expense expense.Expense
	State   expense.ReviewState
	Payer   fueling.Payer
	PaidBy  *fueling.EmployeeRef
}

// Line attributes an exact amount of the obligation to one source; a nil
// amount is absence, never zero.
type Line struct {
	redacted
	Source Source
	Amount *string
}

// Input gathers one obligation; exactly the counterparty of its kind is set.
type Input struct {
	redacted
	Reference   PayableRef
	Kind        Kind
	Beneficiary *fueling.EmployeeRef
	Supplier    *fueling.SupplierRef
	Document    *fueling.DocumentRef
	Statement   *StatementRef
	Lines       []Line
}

// SourceLine is a recognized line: the source identity and origin key, the
// line amount and the source company amount it was bounded by.
type SourceLine struct {
	redacted
	expense         expense.ExpenseRef
	key             expense.OriginKey
	amount, company int64
}

func (l SourceLine) Expense() expense.ExpenseRef { return l.expense }
func (l SourceLine) Key() expense.OriginKey      { return l.key }
func (l SourceLine) AmountCentavos() int64       { return l.amount }
func (l SourceLine) CompanyCentavos() int64      { return l.company }

// positive parses required canonical centavos of at least 1 with the
// accepted exact-value kernel (financial-v1 bounds).
func positive(raw *string) (int64, bool) {
	if raw == nil {
		return 0, false
	}
	n, e := exact.ParseCentavos(*raw)
	return n, e == nil && n >= 1
}

// Settlement is one exact settlement entry of one payable; it records that
// an obligation was settled, it does not move money.
type Settlement struct {
	redacted
	ref    SettlementRef
	amount int64
	valid  bool
}

// NewSettlement requires a same-organization reference and a positive
// canonical amount.
func NewSettlement(scope kernel.OrgScope, ref SettlementRef, amount *string) (Settlement, error) {
	if scope.Validate() != nil {
		return Settlement{}, ErrScope
	}
	n, ok := positive(amount)
	if !ok || !shaped(ref.id, ref.org) {
		return Settlement{}, ErrInvalid
	}
	if ref.org != scope.OrgID() {
		return Settlement{}, ErrScope
	}
	return Settlement{ref: ref, amount: n, valid: true}, nil
}
func (s Settlement) Reference() SettlementRef { return s.ref }
func (s Settlement) AmountCentavos() int64    { return s.amount }

// Payable is an immutable recognized obligation with its settlement facts.
// Its state and remaining amount are derived, never stored.
type Payable struct {
	redacted
	ref         PayableRef
	kind        Kind
	beneficiary *fueling.EmployeeRef
	supplier    *fueling.SupplierRef
	document    *fueling.DocumentRef
	statement   *StatementRef
	lines       []SourceLine
	obligation  int64
	settlements []Settlement
	settled     int64
	cancelled   bool
	reason      string
	valid       bool
}

// counterpartiesFit requires exactly the shaped counterparty of the kind.
func counterpartiesFit(in Input) bool {
	switch in.Kind {
	case Reimbursement:
		return in.Beneficiary != nil && shaped(in.Beneficiary.ID(), in.Beneficiary.OrganizationID()) &&
			in.Supplier == nil && in.Document == nil && in.Statement == nil
	case Supplier:
		return in.Supplier != nil && shaped(in.Supplier.ID(), in.Supplier.OrganizationID()) &&
			(in.Document == nil || shaped(in.Document.ID(), in.Document.OrganizationID())) &&
			in.Beneficiary == nil && in.Statement == nil
	case CardStatement:
		return in.Statement != nil && shaped(in.Statement.id, in.Statement.org) &&
			in.Beneficiary == nil && in.Supplier == nil && in.Document == nil
	}
	return false
}

func lineShaped(l Line) (int64, bool) {
	s := l.Source
	if s.Expense.Reference().ID() == "" {
		return 0, false
	}
	if _, e := expense.ParseReviewState(string(s.State)); e != nil {
		return 0, false
	}
	if _, e := fueling.ParsePayer(string(s.Payer)); e != nil || (s.Payer == fueling.Employee) != (s.PaidBy != nil) {
		return 0, false
	}
	if s.PaidBy != nil && !shaped(s.PaidBy.ID(), s.PaidBy.OrganizationID()) {
		return 0, false
	}
	return positive(l.Amount)
}

// counterpartyOrgs lists the organizations of the present counterparties.
func counterpartyOrgs(in Input) []kernel.OrgID {
	var out []kernel.OrgID
	if in.Beneficiary != nil {
		out = append(out, in.Beneficiary.OrganizationID())
	}
	if in.Supplier != nil {
		out = append(out, in.Supplier.OrganizationID())
	}
	if in.Document != nil {
		out = append(out, in.Document.OrganizationID())
	}
	if in.Statement != nil {
		out = append(out, in.Statement.org)
	}
	return out
}

func sourceID(x expense.Expense) string {
	if s := x.Origin().Source(); s != nil {
		return s.ID()
	}
	return ""
}

// New recognizes one obligation. Checks run in the B-BR-EMP-195 order, lines
// in input order within each: SCOPE (scope), INVALID (shape and ceiling),
// SCOPE (ownership), NOT_COST_ORIGIN, DUPLICATE_SOURCE, NOT_ELIGIBLE, then
// EXCEEDS_SOURCE or PARTIAL_REIMBURSEMENT. It never changes the source.
func New(scope kernel.OrgScope, in Input) (Payable, error) {
	if scope.Validate() != nil {
		return Payable{}, ErrScope
	}
	n := len(in.Lines)
	if !shaped(in.Reference.id, in.Reference.org) || !counterpartiesFit(in) || n == 0 || n > exact.MaxAllocationLines ||
		(in.Kind == Reimbursement && n != 1) {
		return Payable{}, ErrInvalid
	}
	lines := make([]SourceLine, 0, n)
	var total int64
	for _, l := range in.Lines {
		amount, ok := lineShaped(l)
		if !ok {
			return Payable{}, ErrInvalid
		}
		x := l.Source.Expense
		total += amount // at most 1000 lines of at most 10^12: no int64 overflow
		lines = append(lines, SourceLine{expense: x.Reference(), key: x.OriginKey(), amount: amount,
			company: x.Amounts().CompanyCentavos()})
	}
	if total > metrics.MaxTotalCentavos {
		return Payable{}, ErrInvalid
	}
	org := scope.OrgID()
	owners := append(counterpartyOrgs(in), in.Reference.org)
	for _, l := range in.Lines {
		owners = append(owners, l.Source.Expense.Reference().OrganizationID())
		if l.Source.PaidBy != nil {
			owners = append(owners, l.Source.PaidBy.OrganizationID())
		}
	}
	for _, owner := range owners {
		if owner != org {
			return Payable{}, ErrScope
		}
	}
	posing := map[string]bool{in.Reference.id: true}
	if in.Statement != nil {
		posing[in.Statement.id] = true
	}
	for _, l := range in.Lines {
		if posing[l.Source.Expense.Reference().ID()] || posing[sourceID(l.Source.Expense)] {
			return Payable{}, ErrSecondCost
		}
	}
	ids, keys := map[string]bool{}, map[expense.OriginKey]bool{}
	for _, l := range lines {
		if ids[l.expense.ID()] || keys[l.key] {
			return Payable{}, ErrDuplicateSource
		}
		ids[l.expense.ID()], keys[l.key] = true, true
	}
	for _, l := range in.Lines {
		s := l.Source
		eligible := s.State == expense.Approved
		if in.Kind == Reimbursement {
			eligible = eligible && s.Payer == fueling.Employee && s.PaidBy.ID() == in.Beneficiary.ID()
		} else {
			eligible = eligible && s.Payer == fueling.Company
		}
		if !eligible {
			return Payable{}, ErrNotEligible
		}
	}
	for _, l := range lines {
		if l.amount > l.company {
			return Payable{}, ErrExceedsSource
		}
		if in.Kind == Reimbursement && l.amount < l.company {
			return Payable{}, ErrPartialReimbursement
		}
	}
	p := Payable{ref: in.Reference, kind: in.Kind, lines: lines, obligation: total, valid: true}
	p.beneficiary, p.supplier, p.document, p.statement = copyOf(in.Beneficiary), copyOf(in.Supplier),
		copyOf(in.Document), copyOf(in.Statement)
	return p, nil
}

func copyOf[T any](v *T) *T {
	if v == nil {
		return nil
	}
	c := *v
	return &c
}

func (p Payable) Reference() PayableRef             { return p.ref }
func (p Payable) Kind() Kind                        { return p.kind }
func (p Payable) Beneficiary() *fueling.EmployeeRef { return copyOf(p.beneficiary) }
func (p Payable) Supplier() *fueling.SupplierRef    { return copyOf(p.supplier) }
func (p Payable) Document() *fueling.DocumentRef    { return copyOf(p.document) }
func (p Payable) Statement() *StatementRef          { return copyOf(p.statement) }

// Lines returns a copy of the recognized lines.
func (p Payable) Lines() []SourceLine {
	if p.lines == nil {
		return nil
	}
	return append([]SourceLine(nil), p.lines...)
}

// ObligationCentavos is the exact sum of the lines.
func (p Payable) ObligationCentavos() int64 { return p.obligation }

// SettledCentavos is the exact sum of the settlement entries.
func (p Payable) SettledCentavos() int64 { return p.settled }

// RemainingCentavos is obligation minus settled, and 0 when cancelled.
func (p Payable) RemainingCentavos() int64 {
	if p.cancelled {
		return 0
	}
	return p.obligation - p.settled
}

// State derives the payable state from the facts; "" for an unconstructed value.
func (p Payable) State() State {
	switch {
	case !p.valid:
		return ""
	case p.cancelled:
		return Cancelled
	case p.settled == 0:
		return Open
	case p.settled < p.obligation:
		return PartiallySettled
	}
	return Settled
}

// Settlements returns a copy of the settlement entries in recording order.
func (p Payable) Settlements() []Settlement {
	if p.settlements == nil {
		return nil
	}
	return append([]Settlement(nil), p.settlements...)
}

// CancelReason is the verbatim cancellation reason, "" when not cancelled.
func (p Payable) CancelReason() string { return p.reason }

// CheckState compares a stored or displayed state with the derived state.
func (p Payable) CheckState(asserted State) error {
	if _, e := ParseState(string(asserted)); e != nil || !p.valid {
		return ErrInvalid
	}
	if asserted != p.State() {
		return ErrStateMismatch
	}
	return nil
}

// posesAsCost reports an identity of a cost (source expense or its origin
// source) or of the payable itself or its statement.
func (p Payable) posesAsCost(id string) bool {
	if id == p.ref.id || (p.statement != nil && id == p.statement.id) {
		return true
	}
	for _, l := range p.lines {
		if id == l.expense.ID() || id == l.key.Source() {
			return true
		}
	}
	return false
}

// owned checks the scope, the shape of the receiver and its ownership.
func (p Payable) owned(scope kernel.OrgScope, shapedInput bool) error {
	if scope.Validate() != nil {
		return ErrScope
	}
	if !p.valid || !shapedInput {
		return ErrInvalid
	}
	if p.ref.org != scope.OrgID() {
		return ErrScope
	}
	return nil
}

// Settle records one settlement entry, checking in order scope and shape,
// NOT_COST_ORIGIN, CANCELLED, DUPLICATE_ENTRY and EXCEEDS_REMAINING. A
// refusal returns the zero value and leaves the receiver unchanged.
func (p Payable) Settle(scope kernel.OrgScope, s Settlement) (Payable, error) {
	if e := p.owned(scope, s.valid); e != nil {
		return Payable{}, e
	}
	if s.ref.org != scope.OrgID() {
		return Payable{}, ErrScope
	}
	if p.posesAsCost(s.ref.id) {
		return Payable{}, ErrSecondCost
	}
	if p.cancelled {
		return Payable{}, ErrCancelled
	}
	for _, prior := range p.settlements {
		if prior.ref.id == s.ref.id {
			return Payable{}, ErrDuplicateEntry
		}
	}
	if s.amount > p.obligation-p.settled {
		return Payable{}, ErrExceedsRemaining
	}
	next := p
	next.settlements = append(p.Settlements(), s)
	next.settled += s.amount
	return next, nil
}

// validReason applies the B-BR-EMP-163 bound reused by the review lifecycle:
// valid UTF-8, no control character, nonblank, at most 280 scalars.
func validReason(raw string) bool {
	if !utf8.ValidString(raw) || utf8.RuneCountInString(raw) > review.MaxReasonScalars {
		return false
	}
	says := false
	for _, r := range raw {
		if unicode.IsControl(r) {
			return false
		}
		says = says || !unicode.IsSpace(r)
	}
	return says
}

// Cancel moves an OPEN payable to CANCELLED with a verbatim reason, checking
// in order scope and shape (reason included), CANCELLED and HAS_SETTLEMENT.
func (p Payable) Cancel(scope kernel.OrgScope, reason string) (Payable, error) {
	if e := p.owned(scope, validReason(reason)); e != nil {
		return Payable{}, e
	}
	if p.cancelled {
		return Payable{}, ErrCancelled
	}
	if len(p.settlements) > 0 {
		return Payable{}, ErrHasSettlement
	}
	next := p
	next.cancelled, next.reason = true, reason
	return next, nil
}
