package payable_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"testing"

	expense "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/expense/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/expense/domain/payable"
	fueling "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	exact "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgvalue/domain"
)

func scopeOf(t *testing.T, alias string) kernel.OrgScope {
	t.Helper()
	s := vectorScope(alias)
	if s.Validate() != nil {
		t.Fatal("invalid test scope")
	}
	return s
}

func str(v string) *string { return &v }

// approved builds an APPROVED expense n of org "A" with its own fueling source.
func approved(t *testing.T, n int, company string) expense.Entry {
	t.Helper()
	x, e := sourceExpense("A", uuid(n), "FUELING", uuid(100000+n), company, company, fixtureAt)
	if e != nil {
		t.Fatal(e)
	}
	return expense.Entry{Expense: x, State: expense.Approved}
}

func payableRef(t *testing.T, n int) payable.PayableRef {
	t.Helper()
	r, e := payable.NewPayableRef(scopeOf(t, "A"), uuid(n))
	if e != nil {
		t.Fatal(e)
	}
	return r
}

func companyLine(x expense.Entry, amount string) payable.Line {
	return payable.Line{Source: payable.Source{Expense: x.Expense, State: x.State, Payer: fueling.Company}, Amount: str(amount)}
}

func statementInput(t *testing.T, n int, lines ...payable.Line) payable.Input {
	t.Helper()
	st, e := payable.NewStatementRef(scopeOf(t, "A"), uuid(n+1))
	if e != nil {
		t.Fatal(e)
	}
	return payable.Input{Reference: payableRef(t, n), Kind: payable.CardStatement, Statement: &st, Lines: lines}
}

func reimbursement(t *testing.T, n int, x expense.Entry) payable.Payable {
	t.Helper()
	u := employeeRef("A", "U")
	in := payable.Input{Reference: payableRef(t, n), Kind: payable.Reimbursement, Beneficiary: u,
		Lines: []payable.Line{{Source: payable.Source{Expense: x.Expense, State: x.State, Payer: fueling.Employee, PaidBy: u},
			Amount: str(strconv.FormatInt(x.Expense.Amounts().CompanyCentavos(), 10))}}}
	p, e := payable.New(scopeOf(t, "A"), in)
	if e != nil {
		t.Fatal(e)
	}
	return p
}

func entry(t *testing.T, n int, amount string) payable.Settlement {
	t.Helper()
	s := scopeOf(t, "A")
	r, e := payable.NewSettlementRef(s, uuid(n))
	if e != nil {
		t.Fatal(e)
	}
	v, e := payable.NewSettlement(s, r, &amount)
	if e != nil {
		t.Fatal(e)
	}
	return v
}

func TestKindsAndStatesAreClosed(t *testing.T) {
	for _, k := range []payable.Kind{payable.Supplier, payable.Reimbursement, payable.CardStatement} {
		if v, e := payable.ParseKind(string(k)); e != nil || v != k {
			t.Fatal("kind refused", k, e)
		}
	}
	for _, s := range []payable.State{payable.Open, payable.PartiallySettled, payable.Settled, payable.Cancelled} {
		if v, e := payable.ParseState(string(s)); e != nil || v != s {
			t.Fatal("state refused", s, e)
		}
	}
	for _, raw := range []string{"ADVANCE", "PAYMENT", "PAYABLE", "", "supplier", "SUPPLIER\x00"} {
		if _, e := payable.ParseKind(raw); !errors.Is(e, payable.ErrInvalid) {
			t.Fatal("unknown kind accepted", raw, e)
		}
	}
	for _, raw := range []string{"PAID", "APPROVED", "", "open"} {
		if _, e := payable.ParseState(raw); !errors.Is(e, payable.ErrInvalid) {
			t.Fatal("unknown state accepted", raw, e)
		}
	}
}

// Obligation and settlement kinds can never be recognized as a cost origin by
// the reused expense domain (EDR-013).
func TestObligationTokensAreNeverCostOrigins(t *testing.T) {
	for _, raw := range []string{string(payable.Supplier), string(payable.Reimbursement), string(payable.CardStatement),
		"PAYMENT", "PAYABLE", "ADVANCE"} {
		if _, e := expense.ParseOriginKind(raw); e == nil {
			t.Fatal("obligation token accepted as a cost origin", raw)
		}
	}
	if !errors.Is(payable.ErrSecondCost, expense.ErrNotCostOrigin) {
		t.Fatal("second cost does not wrap the expense refusal")
	}
}

func TestReferencesAreScopedAndCanonical(t *testing.T) {
	a := scopeOf(t, "A")
	type ctor func(kernel.OrgScope, string) (string, kernel.OrgID, error)
	for name, build := range map[string]ctor{
		"payable": func(s kernel.OrgScope, id string) (string, kernel.OrgID, error) {
			r, e := payable.NewPayableRef(s, id)
			return r.ID(), r.OrganizationID(), e
		},
		"settlement": func(s kernel.OrgScope, id string) (string, kernel.OrgID, error) {
			r, e := payable.NewSettlementRef(s, id)
			return r.ID(), r.OrganizationID(), e
		},
		"statement": func(s kernel.OrgScope, id string) (string, kernel.OrgID, error) {
			r, e := payable.NewStatementRef(s, id)
			return r.ID(), r.OrganizationID(), e
		},
	} {
		if id, org, e := build(a, uuid(81)); e != nil || id != uuid(81) || org != a.OrgID() {
			t.Fatal(name, "valid reference refused", e)
		}
		for _, raw := range []string{"", "not-a-uuid", strings.ToUpper(uuid(81)), "00000000-0000-0000-0000-000000000000",
			" " + uuid(81)} {
			if id, _, e := build(a, raw); !errors.Is(e, payable.ErrInvalid) || id != "" {
				t.Fatal(name, "malformed reference accepted", raw, e)
			}
		}
		if _, _, e := build(kernel.OrgScope{}, uuid(81)); !errors.Is(e, payable.ErrScope) {
			t.Fatal(name, "zero scope accepted", e)
		}
	}
}

func TestSettlementEntryAmounts(t *testing.T) {
	a := scopeOf(t, "A")
	r, _ := payable.NewSettlementRef(a, uuid(91))
	if _, e := payable.NewSettlement(a, r, nil); !errors.Is(e, payable.ErrInvalid) {
		t.Fatal("absent amount accepted", e)
	}
	if _, e := payable.NewSettlement(a, payable.SettlementRef{}, str("1")); !errors.Is(e, payable.ErrInvalid) {
		t.Fatal("zero reference accepted", e)
	}
	if _, e := payable.NewSettlement(kernel.OrgScope{}, r, str("1")); !errors.Is(e, payable.ErrScope) {
		t.Fatal("zero scope accepted", e)
	}
	if s, e := payable.NewSettlement(a, r, str("1000000000000")); e != nil || s.AmountCentavos() != 1000000000000 ||
		s.Reference() != r {
		t.Fatal("ceiling settlement refused", e)
	}
}

func TestLineBoundAndTotalCeiling(t *testing.T) {
	a := scopeOf(t, "A")
	lines := make([]payable.Line, 0, exact.MaxAllocationLines+1)
	for i := 0; i <= exact.MaxAllocationLines; i++ {
		lines = append(lines, companyLine(approved(t, 1000+i, "1000000000"), "1000000000"))
	}
	p, e := payable.New(a, statementInput(t, 81, lines[:exact.MaxAllocationLines]...))
	if e != nil || p.ObligationCentavos() != 1000000000000 || len(p.Lines()) != exact.MaxAllocationLines {
		t.Fatal("bounded statement at the ceiling refused", e)
	}
	if _, e := payable.New(a, statementInput(t, 81, lines...)); !errors.Is(e, payable.ErrInvalid) {
		t.Fatal("more lines than the allocation bound accepted", e)
	}
	small := append([]payable.Line{}, lines[:exact.MaxAllocationLines-1]...)
	small = append(small, companyLine(approved(t, 5000, "1000000001"), "1000000001"))
	if _, e := payable.New(a, statementInput(t, 81, small...)); !errors.Is(e, payable.ErrInvalid) {
		t.Fatal("obligation above the total ceiling accepted", e)
	}
}

func TestPayableValuesAreImmutable(t *testing.T) {
	a := scopeOf(t, "A")
	x := approved(t, 11, "97350")
	p := reimbursement(t, 81, x)
	lines := p.Lines()
	lines[0] = payable.SourceLine{}
	if p.Lines()[0].AmountCentavos() != 97350 {
		t.Fatal("lines alias internal state")
	}
	b := p.Beneficiary()
	*b = fueling.EmployeeRef{}
	if p.Beneficiary().ID() != uuid(41) {
		t.Fatal("beneficiary aliases internal state")
	}
	q, e := p.Settle(a, entry(t, 91, "30000"))
	if e != nil || p.SettledCentavos() != 0 || p.State() != payable.Open || q.SettledCentavos() != 30000 {
		t.Fatal("settle mutated its receiver", e)
	}
	settlements := q.Settlements()
	settlements[0] = payable.Settlement{}
	if q.Settlements()[0].AmountCentavos() != 30000 {
		t.Fatal("settlements alias internal state")
	}
	// Two settlements from one parent never share storage.
	r1, _ := q.Settle(a, entry(t, 92, "1"))
	r2, _ := q.Settle(a, entry(t, 93, "2"))
	if r1.Settlements()[1].Reference().ID() != uuid(92) || r2.Settlements()[1].Reference().ID() != uuid(93) {
		t.Fatal("sibling settlements share storage")
	}
	if z, e := q.Settle(a, entry(t, 94, "67351")); !errors.Is(e, payable.ErrExceedsRemaining) || z.State() != "" ||
		q.SettledCentavos() != 30000 {
		t.Fatal("refused settlement changed state or returned a value", e)
	}
	c, e := p.Cancel(a, " Duplicated obligation ")
	if e != nil || c.CancelReason() != " Duplicated obligation " || p.CancelReason() != "" || p.State() != payable.Open {
		t.Fatal("cancel lost its verbatim reason or mutated its receiver", e)
	}
}

func TestCounterpartyAccessorsByKind(t *testing.T) {
	a := scopeOf(t, "A")
	x := approved(t, 11, "97350")
	v, d := supplierRef("A", "V"), documentRef("A", "D")
	p, e := payable.New(a, payable.Input{Reference: payableRef(t, 81), Kind: payable.Supplier, Supplier: v, Document: d,
		Lines: []payable.Line{companyLine(x, "97350")}})
	if e != nil || p.Supplier().ID() != uuid(51) || p.Document().ID() != uuid(61) || p.Beneficiary() != nil ||
		p.Statement() != nil || p.Kind() != payable.Supplier || p.Reference().ID() != uuid(81) {
		t.Fatal("supplier accessors", e)
	}
	*p.Supplier(), *p.Document() = fueling.SupplierRef{}, fueling.DocumentRef{}
	if p.Supplier().ID() != uuid(51) || p.Document().ID() != uuid(61) {
		t.Fatal("supplier references alias internal state")
	}
	c, e := payable.New(a, statementInput(t, 81, companyLine(x, "97350")))
	if e != nil || c.Statement().ID() != uuid(82) || c.Supplier() != nil || c.Document() != nil {
		t.Fatal("statement accessors", e)
	}
	*c.Statement() = payable.StatementRef{}
	if c.Statement().ID() != uuid(82) {
		t.Fatal("statement aliases internal state")
	}
	l := c.Lines()[0]
	if l.Expense().ID() != uuid(11) || l.Key() != x.Expense.OriginKey() || l.CompanyCentavos() != 97350 {
		t.Fatal("line accessors")
	}
}

func TestZeroValuesAreRefused(t *testing.T) {
	a := scopeOf(t, "A")
	var zero payable.Payable
	if zero.State() != "" || zero.RemainingCentavos() != 0 || zero.Lines() != nil || zero.Settlements() != nil {
		t.Fatal("zero payable reports facts")
	}
	if _, e := zero.Settle(a, entry(t, 91, "1")); !errors.Is(e, payable.ErrInvalid) {
		t.Fatal("zero payable settled", e)
	}
	if _, e := zero.Cancel(a, "reason"); !errors.Is(e, payable.ErrInvalid) {
		t.Fatal("zero payable cancelled", e)
	}
	if e := zero.CheckState(payable.Open); !errors.Is(e, payable.ErrInvalid) {
		t.Fatal("zero payable state checked", e)
	}
	p := reimbursement(t, 81, approved(t, 11, "97350"))
	if _, e := p.Settle(a, payable.Settlement{}); !errors.Is(e, payable.ErrInvalid) {
		t.Fatal("zero settlement recorded", e)
	}
	if _, e := p.Settle(kernel.OrgScope{}, entry(t, 91, "1")); !errors.Is(e, payable.ErrScope) {
		t.Fatal("zero scope settled", e)
	}
	if _, e := p.Cancel(kernel.OrgScope{}, "reason"); !errors.Is(e, payable.ErrScope) {
		t.Fatal("zero scope cancelled", e)
	}
	if _, e := payable.New(a, payable.Input{}); !errors.Is(e, payable.ErrInvalid) {
		t.Fatal("zero input recognized", e)
	}
	period, _ := expense.NewReportingPeriod("2026-11-01", "2026-12-01")
	if _, e := payable.ComputeViews(a, period, expense.BasisApproved, nil, []payable.Payable{zero}); !errors.Is(e, payable.ErrInvalid) {
		t.Fatal("zero payable entered the views", e)
	}
	if _, e := payable.ComputeViews(kernel.OrgScope{}, period, expense.BasisApproved, nil, nil); !errors.Is(e, payable.ErrScope) {
		t.Fatal("zero scope views", e)
	}
	// A payable from another organization cannot be settled or cancelled here.
	b := scopeOf(t, "B")
	if _, e := p.Settle(b, payable.Settlement{}); !errors.Is(e, payable.ErrInvalid) {
		t.Fatal("shape must precede ownership", e)
	}
	rb, _ := payable.NewSettlementRef(b, uuid(91))
	sb, _ := payable.NewSettlement(b, rb, str("1"))
	if _, e := p.Settle(b, sb); !errors.Is(e, payable.ErrScope) {
		t.Fatal("foreign scope settled", e)
	}
}

// The payable domain itself, not only the expense parser, refuses identities
// posing as costs.
func TestIdentitySeparationUsesTheSecondCostError(t *testing.T) {
	a := scopeOf(t, "A")
	p := reimbursement(t, 81, approved(t, 11, "97350"))
	for _, n := range []int{81, 11, 100011} {
		if _, e := p.Settle(a, entry(t, n, "1")); !errors.Is(e, payable.ErrSecondCost) {
			t.Fatal("settlement identity posing as cost accepted", n, e)
		}
	}
	posing := approved(t, 81, "97350")
	u := employeeRef("A", "U")
	in := payable.Input{Reference: payableRef(t, 81), Kind: payable.Reimbursement, Beneficiary: u,
		Lines: []payable.Line{{Source: payable.Source{Expense: posing.Expense, State: expense.Approved, Payer: fueling.Employee,
			PaidBy: u}, Amount: str("97350")}}}
	if _, e := payable.New(a, in); !errors.Is(e, payable.ErrSecondCost) {
		t.Fatal("payable identity as its own source accepted", e)
	}
	period, _ := expense.NewReportingPeriod("2026-11-01", "2026-12-01")
	q, _ := p.Settle(a, entry(t, 91, "1"))
	for _, n := range []int{81, 91} {
		cost := approved(t, n, "5")
		if _, e := payable.ComputeViews(a, period, expense.BasisApproved, []expense.Entry{approved(t, 11, "97350"), cost},
			[]payable.Payable{q}); !errors.Is(e, payable.ErrSecondCost) {
			t.Fatal("payable or settlement identity posing as a cost entry accepted", n, e)
		}
	}
}

func TestPrivateValuesAreRedacted(t *testing.T) {
	a := scopeOf(t, "A")
	x := approved(t, 11, "97350")
	p := reimbursement(t, 81, x)
	p, _ = p.Settle(a, entry(t, 91, "30000"))
	c, _ := reimbursement(t, 82, approved(t, 12, "5")).Cancel(a, "Secret synthetic reason")
	period, _ := expense.NewReportingPeriod("2026-11-01", "2026-12-01")
	v, e := payable.ComputeViews(a, period, expense.BasisApproved, []expense.Entry{x}, []payable.Payable{p})
	if e != nil {
		t.Fatal(e)
	}
	st, _ := payable.NewStatementRef(a, uuid(71))
	values := []any{p, c, p.Reference(), p.Settlements()[0], p.Lines()[0], st, v,
		payable.Input{Reference: p.Reference(), Beneficiary: p.Beneficiary()},
		payable.Line{Amount: str("97350")}, payable.Source{Expense: x.Expense}}
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	for _, value := range values {
		js, _ := json.Marshal(value)
		logger.Info("value", "v", value)
		for _, text := range []string{fmt.Sprintf("%v %+v %#v %s %q", value, value, value, value, value), string(js)} {
			for _, secret := range []string{"9e000000", "97350", "30000", "Secret"} {
				if strings.Contains(text, secret) {
					t.Fatalf("private value leaked: %T", value)
				}
			}
		}
	}
	for _, secret := range []string{"9e000000", "97350", "30000", "Secret"} {
		if strings.Contains(logs.String(), secret) {
			t.Fatal("private value leaked to slog")
		}
	}
}
