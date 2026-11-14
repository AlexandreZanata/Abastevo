// Package domain models pure enterprise expenses: one economic cost per origin
// (EDR-013), exact paid and company centavos, a structural category code, the
// competence reporting day and approved-versus-pending totals. It never
// decides authority, acceptance, review, settlement or persistence.
package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"regexp"

	fueling "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	exact "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgvalue/domain"
)

var (
	ErrInvalid            = errors.New("expense: invalid value")
	ErrScope              = errors.New("expense: organization scope denied")
	ErrNotCostOrigin      = errors.New("expense: settlement, evidence or stock movement is not a cost origin")
	ErrCompanyExceedsPaid = errors.New("expense: company amount exceeds paid amount")
	ErrDuplicateCost      = errors.New("expense: duplicate cost in one basis")
	ErrOutOfPeriod        = errors.New("expense: reporting day outside the basis period")
)

type redacted struct{}

func (redacted) String() string                 { return "EnterpriseExpenseValue(redacted)" }
func (r redacted) Format(f fmt.State, _ rune)   { _, _ = io.WriteString(f, r.String()) }
func (r redacted) LogValue() slog.Value         { return slog.StringValue(r.String()) }
func (r redacted) MarshalJSON() ([]byte, error) { return json.Marshal(r.String()) }

// OriginKind is the closed list of economic cost origins (EDR-131).
type OriginKind string

const (
	KindFueling   OriginKind = "FUELING"
	KindWorkOrder OriginKind = "WORK_ORDER"
	KindDirect    OriginKind = "DIRECT"
)

// notCostOrigins settle, evidence or move money or stock; EDR-013 forbids
// them from recognizing a (second) cost.
var notCostOrigins = map[string]bool{"PAYMENT": true, "REIMBURSEMENT": true, "CARD_STATEMENT": true, "ADVANCE": true,
	"PAYABLE": true, "DOCUMENT": true, "STOCK_PURCHASE": true, "STOCK_TRANSFER": true}

// ParseOriginKind accepts only cost origins. Settlement, evidence and stock
// movement kinds are recognized and refused with ErrNotCostOrigin.
func ParseOriginKind(raw string) (OriginKind, error) {
	switch k := OriginKind(raw); k {
	case KindFueling, KindWorkOrder, KindDirect:
		return k, nil
	}
	if notCostOrigins[raw] {
		return "", ErrNotCostOrigin
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

func scoped(scope kernel.OrgScope, id string) (scopedID, error) {
	if scope.Validate() != nil {
		return scopedID{}, ErrScope
	}
	if !canonical(id) {
		return scopedID{}, ErrInvalid
	}
	return scopedID{org: scope.OrgID(), id: id}, nil
}

// validate checks scope, then shape, then ownership, so an unconstructed value is INVALID.
func (r scopedID) validate(scope kernel.OrgScope) error {
	if scope.Validate() != nil {
		return ErrScope
	}
	if !canonical(r.id) || r.org == (kernel.OrgID{}) {
		return ErrInvalid
	}
	if r.org != scope.OrgID() {
		return ErrScope
	}
	return nil
}

// ExpenseRef is a scoped expense identity; its shape proves no existence.
type ExpenseRef struct{ scopedID }

// SourceRef is the scoped identity of a FUELING or WORK_ORDER source.
type SourceRef struct{ scopedID }

func NewExpenseRef(scope kernel.OrgScope, id string) (ExpenseRef, error) {
	r, e := scoped(scope, id)
	return ExpenseRef{r}, e
}
func NewSourceRef(scope kernel.OrgScope, id string) (SourceRef, error) {
	r, e := scoped(scope, id)
	return SourceRef{r}, e
}

// Origin is one cost origin. DIRECT carries no source: the expense is its own.
type Origin struct {
	redacted
	kind   OriginKind
	source *SourceRef
}

// NewOrigin requires a same-organization source for FUELING and WORK_ORDER
// and refuses one for DIRECT.
func NewOrigin(scope kernel.OrgScope, kind OriginKind, source *SourceRef) (Origin, error) {
	if scope.Validate() != nil {
		return Origin{}, ErrScope
	}
	if _, e := ParseOriginKind(string(kind)); e != nil {
		return Origin{}, e
	}
	if kind == KindDirect {
		if source != nil {
			return Origin{}, ErrInvalid
		}
		return Origin{kind: kind}, nil
	}
	if source == nil {
		return Origin{}, ErrInvalid
	}
	if e := source.validate(scope); e != nil {
		return Origin{}, e
	}
	copied := *source
	return Origin{kind: kind, source: &copied}, nil
}

// NewFuelingOrigin references an EMP-FUEL-001 fueling without re-modelling it.
func NewFuelingOrigin(scope kernel.OrgScope, ref fueling.FuelingRef) (Origin, error) {
	source := SourceRef{scopedID{org: ref.OrganizationID(), id: ref.ID()}}
	return NewOrigin(scope, KindFueling, &source)
}
func (o Origin) Kind() OriginKind { return o.kind }
func (o Origin) Source() *SourceRef {
	if o.source == nil {
		return nil
	}
	v := *o.source
	return &v
}

// OriginKey is the one-origin-one-cost identity: organization, kind and
// source, or the expense identity for DIRECT. It is comparable.
type OriginKey struct {
	redacted
	org    kernel.OrgID
	kind   OriginKind
	source string
}

func (k OriginKey) OrganizationID() kernel.OrgID { return k.org }
func (k OriginKey) Kind() OriginKind             { return k.kind }
func (k OriginKey) Source() string               { return k.source }

// Amounts holds the authoritative paid amount and the company amount, both
// exact centavos with 0 <= company <= paid. Its zero value is not valid.
type Amounts struct {
	redacted
	paid, company int64
	valid         bool
}

// NewAmounts parses required canonical centavos; nil is absence, never zero.
// Bounds and canonical form are the accepted exact-value kernel's.
func NewAmounts(paid, company *string) (Amounts, error) {
	if paid == nil || company == nil {
		return Amounts{}, ErrInvalid
	}
	p, e := exact.ParseCentavos(*paid)
	if e != nil {
		return Amounts{}, ErrInvalid
	}
	c, e := exact.ParseCentavos(*company)
	if e != nil {
		return Amounts{}, ErrInvalid
	}
	if c > p {
		return Amounts{}, ErrCompanyExceedsPaid
	}
	return Amounts{paid: p, company: c, valid: true}, nil
}
func (a Amounts) PaidCentavos() int64    { return a.paid }
func (a Amounts) CompanyCentavos() int64 { return a.company }

// categoryPattern reuses the repository's closed ASCII key profile.
var categoryPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)

// Category is a structurally valid code; the configured catalogue is EMP-EXP-003's.
type Category struct {
	redacted
	code string
}

func ParseCategory(raw string) (Category, error) {
	if !categoryPattern.MatchString(raw) {
		return Category{}, ErrInvalid
	}
	return Category{code: raw}, nil
}
func (c Category) Code() string { return c.code }

// ExpenseInput gathers the required facts. Zones must come from the resolved
// operating context, never from a device claim.
type ExpenseInput struct {
	redacted
	Reference  ExpenseRef
	Origin     Origin
	Category   Category
	Amounts    Amounts
	Competence *orgtime.Declaration
	Zones      orgtime.ReportingZones
}

// Expense is an immutable, validated cost value. Its review state is external.
type Expense struct {
	redacted
	reference  ExpenseRef
	origin     Origin
	category   Category
	amounts    Amounts
	competence orgtime.Declaration
	reporting  orgtime.ReportingAttribution
	valid      bool
}

// NewExpense validates, in order, scope and identity, origin, category,
// amounts, competence and reporting zones. It reads no clock and invents no
// receipt; the reporting day comes from the supplied operating context.
func NewExpense(scope kernel.OrgScope, in ExpenseInput) (Expense, error) {
	if e := in.Reference.validate(scope); e != nil {
		return Expense{}, e
	}
	if in.Origin.kind == "" {
		return Expense{}, ErrInvalid
	}
	origin, e := NewOrigin(scope, in.Origin.kind, in.Origin.source)
	if e != nil {
		return Expense{}, e
	}
	if !categoryPattern.MatchString(in.Category.code) || !in.Amounts.valid || in.Competence == nil {
		return Expense{}, ErrInvalid
	}
	competence, e := orgtime.ValidateDeclaration(*in.Competence)
	if e != nil {
		return Expense{}, ErrInvalid
	}
	reporting, e := orgtime.AttributeReportingDay(competence.OccurredAt, in.Zones)
	if e != nil {
		return Expense{}, ErrInvalid
	}
	return Expense{reference: in.Reference, origin: origin, category: in.Category, amounts: in.Amounts,
		competence: competence, reporting: reporting, valid: true}, nil
}

func (e Expense) Reference() ExpenseRef { return e.reference }
func (e Expense) Origin() Origin        { return Origin{kind: e.origin.kind, source: e.origin.Source()} }

// OriginKey is the one-origin-one-cost identity of this expense.
func (e Expense) OriginKey() OriginKey {
	source := e.reference.id
	if e.origin.source != nil {
		source = e.origin.source.id
	}
	return OriginKey{org: e.reference.org, kind: e.origin.kind, source: source}
}
func (e Expense) Category() Category                      { return e.category }
func (e Expense) Amounts() Amounts                        { return e.amounts }
func (e Expense) Competence() orgtime.Declaration         { return e.competence }
func (e Expense) Reporting() orgtime.ReportingAttribution { return e.reporting }
