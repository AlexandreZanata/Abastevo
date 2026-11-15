package payable_test

import (
	"errors"
	"math/big"
	"math/rand"
	"reflect"
	"strconv"
	"sync"
	"testing"

	expense "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/expense/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/expense/domain/payable"
	fueling "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/domain"
)

var allStates = []payable.State{payable.Open, payable.PartiallySettled, payable.Settled, payable.Cancelled}

// derived is the B-BR-EMP-196 state written independently of the package.
func derived(obligation, settled int64, cancelled bool) payable.State {
	switch {
	case cancelled:
		return payable.Cancelled
	case settled == 0:
		return payable.Open
	case settled < obligation:
		return payable.PartiallySettled
	}
	return payable.Settled
}

func checkFacts(t *testing.T, p payable.Payable, obligation, settled int64, cancelled bool) {
	t.Helper()
	want := derived(obligation, settled, cancelled)
	remaining := obligation - settled
	if cancelled {
		remaining = 0
	}
	if p.State() != want || p.ObligationCentavos() != obligation || p.SettledCentavos() != settled ||
		p.RemainingCentavos() != remaining {
		t.Fatalf("facts: state %s obligation %d settled %d remaining %d", p.State(), p.ObligationCentavos(),
			p.SettledCentavos(), p.RemainingCentavos())
	}
	var sum int64
	for _, s := range p.Settlements() {
		sum += s.AmountCentavos()
	}
	if sum != settled {
		t.Fatal("settled amount is not the exact sum of its entries")
	}
	for _, s := range allStates {
		e := p.CheckState(s)
		if (s == want) != (e == nil) || (e != nil && !errors.Is(e, payable.ErrStateMismatch)) {
			t.Fatal("asserted state check disagrees with the derived state", s, e)
		}
	}
}

// Random settlement sequences never overpay, keep the remaining amount
// derived and leave refused attempts without effect (B-BR-EMP-196).
func TestSettlementSequencesKeepDerivedState(t *testing.T) {
	a := scopeOf(t, "A")
	rng := rand.New(rand.NewSource(20261114))
	for round := 0; round < 300; round++ {
		company := []int64{1, 2, 97350, 1000000000000}[round%4]
		p := reimbursement(t, 81, approved(t, 11, strconv.FormatInt(company, 10)))
		var settled int64
		cancelled := false
		checkFacts(t, p, company, 0, false)
		for step := 0; step < 8; step++ {
			if rng.Intn(6) == 0 {
				next, e := p.Cancel(a, "Synthetic reason")
				switch {
				case cancelled:
					if !errors.Is(e, payable.ErrCancelled) {
						t.Fatal("cancelled twice", e)
					}
				case settled > 0:
					if !errors.Is(e, payable.ErrHasSettlement) {
						t.Fatal("cancelled after a settlement", e)
					}
				default:
					if e != nil {
						t.Fatal(e)
					}
					p, cancelled = next, true
				}
				checkFacts(t, p, company, settled, cancelled)
				continue
			}
			remaining := company - settled
			amount := int64(1)
			switch rng.Intn(4) {
			case 0:
				amount = remaining
			case 1:
				amount = remaining + 1
			case 2:
				if remaining > 1 {
					amount = 1 + rng.Int63n(remaining)
				}
			}
			if amount < 1 {
				amount = 1
			}
			if amount > 1000000000000 { // a settlement entry is itself bounded by financial-v1
				amount = 1000000000000
			}
			next, e := p.Settle(a, entry(t, 1000+step, strconv.FormatInt(amount, 10)))
			switch {
			case cancelled:
				if !errors.Is(e, payable.ErrCancelled) {
					t.Fatal("settled a cancelled payable", e)
				}
			case amount > remaining:
				if !errors.Is(e, payable.ErrExceedsRemaining) {
					t.Fatal("overpayment accepted", e)
				}
			default:
				if e != nil {
					t.Fatal(e)
				}
				p, settled = next, settled+amount
			}
			if e != nil && next.State() != "" {
				t.Fatal("a refusal returned a payable")
			}
			checkFacts(t, p, company, settled, cancelled)
		}
	}
}

type settlementSums struct{ obligation, settled, remaining int64 }

// Any number of payables and settlements leaves the cost view equal to the
// totals of the costs alone, and the settlement view equals the payables'
// own facts (B-BR-EMP-197, VAL-13).
func TestCostViewIsUnchangedByPayables(t *testing.T) {
	a := scopeOf(t, "A")
	period, _ := expense.NewReportingPeriod("2026-11-01", "2026-12-01")
	rng := rand.New(rand.NewSource(13))
	states := []expense.ReviewState{expense.Draft, expense.Submitted, expense.InReview, expense.Approved,
		expense.Returned, expense.Rejected, expense.Cancelled, expense.Voided}
	for round := 0; round < 60; round++ {
		var costs, eligible []expense.Entry
		for i := 0; i < 1+rng.Intn(12); i++ {
			x := approved(t, 2000+i, strconv.FormatInt(1+rng.Int63n(200000), 10))
			x.State = states[rng.Intn(len(states))]
			costs = append(costs, x)
			if x.State == expense.Approved {
				eligible = append(eligible, x)
			}
		}
		for _, basis := range []expense.Basis{expense.BasisApproved, expense.BasisIncludesPending} {
			alone, e := expense.Totals(a, period, basis, costs)
			if e != nil {
				t.Fatal(e)
			}
			var payables []payable.Payable
			var want settlementSums
			counts := map[payable.State]int{}
			for i, x := range eligible {
				company := x.Expense.Amounts().CompanyCentavos()
				var p payable.Payable
				switch i % 3 {
				case 0:
					p = reimbursement(t, 3000+i, x)
				case 1:
					v := supplierRef("A", "V")
					amount := 1 + rng.Int63n(company)
					q, e := payable.New(a, payable.Input{Reference: payableRef(t, 3000+i), Kind: payable.Supplier, Supplier: v,
						Lines: []payable.Line{companyLine(x, strconv.FormatInt(amount, 10))}})
					if e != nil {
						t.Fatal(e)
					}
					p = q
				default:
					q, e := payable.New(a, statementInput(t, 4000+2*i, companyLine(x, strconv.FormatInt(company, 10))))
					if e != nil {
						t.Fatal(e)
					}
					p = q
				}
				if rng.Intn(4) == 0 {
					p, _ = p.Cancel(a, "Synthetic reason")
				} else {
					for j := 0; j < rng.Intn(4); j++ {
						rest := p.RemainingCentavos()
						if rest == 0 {
							break
						}
						p, _ = p.Settle(a, entry(t, 6000+10*i+j, strconv.FormatInt(1+rng.Int63n(rest), 10)))
					}
				}
				payables = append(payables, p)
				counts[p.State()]++
				if p.State() != payable.Cancelled {
					want.obligation += p.ObligationCentavos()
				}
				want.settled += p.SettledCentavos()
				want.remaining += p.RemainingCentavos()
				views, e := payable.ComputeViews(a, period, basis, costs, payables)
				if e != nil {
					t.Fatal(e)
				}
				if !reflect.DeepEqual(costFields(views.Cost()), costFields(alone)) {
					t.Fatal("a payable or settlement changed the cost view")
				}
				if views.PayableCount() != len(payables) || views.OpenCount() != counts[payable.Open] ||
					views.PartiallySettledCount() != counts[payable.PartiallySettled] ||
					views.SettledCount() != counts[payable.Settled] || views.CancelledCount() != counts[payable.Cancelled] ||
					views.ObligationCentavos().Cmp(big.NewInt(want.obligation)) != 0 ||
					views.SettledCentavos().Cmp(big.NewInt(want.settled)) != 0 ||
					views.RemainingCentavos().Cmp(big.NewInt(want.remaining)) != 0 {
					t.Fatal("settlement view disagrees with the payables")
				}
			}
		}
	}
}

// Settlement sums are widened: many payables at the ceiling exceed int64.
func TestSettlementViewSumsAreWidened(t *testing.T) {
	a := scopeOf(t, "A")
	period, _ := expense.NewReportingPeriod("2026-11-01", "2026-12-01")
	var payables []payable.Payable
	for i := 0; i < 10000; i++ {
		x := approved(t, 10000+i, "1000000000000")
		p, e := payable.New(a, statementInput(t, 40000+2*i, companyLine(x, "1000000000000")))
		if e != nil {
			t.Fatal(e)
		}
		payables = append(payables, p)
	}
	v, e := payable.ComputeViews(a, period, expense.BasisApproved, nil, payables)
	want, _ := new(big.Int).SetString("10000000000000000", 10)
	if e != nil || v.ObligationCentavos().Cmp(want) != 0 || v.RemainingCentavos().Cmp(want) != 0 ||
		v.SettledCentavos().Sign() != 0 || v.OpenCount() != 10000 {
		t.Fatal("widened sums", e)
	}
	if v.ObligationCentavos().Add(v.ObligationCentavos(), big.NewInt(1)); v.ObligationCentavos().Cmp(want) != 0 {
		t.Fatal("views expose internal sums")
	}
}

// Views are pure reads of immutable values and are safe to share.
func TestViewsAreSafeForConcurrentReads(t *testing.T) {
	a := scopeOf(t, "A")
	period, _ := expense.NewReportingPeriod("2026-11-01", "2026-12-01")
	x := approved(t, 11, "97350")
	p, e := reimbursement(t, 81, x).Settle(a, entry(t, 91, "97350"))
	if e != nil {
		t.Fatal(e)
	}
	costs, payables := []expense.Entry{x}, []payable.Payable{p}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, e := payable.ComputeViews(a, period, expense.BasisApproved, costs, payables)
			if e != nil || v.Cost().FigureCentavos().Int64() != 97350 || v.SettledCount() != 1 {
				t.Error("concurrent views", e)
			}
			if _, e := p.Settle(a, entry(t, 92, "1")); !errors.Is(e, payable.ErrExceedsRemaining) {
				t.Error("concurrent settle of a settled payable", e)
			}
		}()
	}
	wg.Wait()
}

// Payer and eligibility: a COMPANY payer never yields a reimbursement and an
// EMPLOYEE payer never yields a supplier or card obligation.
func TestPayerDecidesWhoIsOwed(t *testing.T) {
	a := scopeOf(t, "A")
	x := approved(t, 11, "97350")
	u := employeeRef("A", "U")
	for _, payer := range []fueling.Payer{fueling.Company, fueling.Employee} {
		var paidBy *fueling.EmployeeRef
		if payer == fueling.Employee {
			paidBy = u
		}
		src := payable.Source{Expense: x.Expense, State: expense.Approved, Payer: payer, PaidBy: paidBy}
		line := []payable.Line{{Source: src, Amount: str("97350")}}
		_, er := payable.New(a, payable.Input{Reference: payableRef(t, 81), Kind: payable.Reimbursement, Beneficiary: u,
			Lines: line})
		_, es := payable.New(a, payable.Input{Reference: payableRef(t, 81), Kind: payable.Supplier,
			Supplier: supplierRef("A", "V"), Lines: line})
		_, ec := payable.New(a, statementInput(t, 81, line...))
		if payer == fueling.Employee && (er != nil || !errors.Is(es, payable.ErrNotEligible) || !errors.Is(ec, payable.ErrNotEligible)) ||
			payer == fueling.Company && (!errors.Is(er, payable.ErrNotEligible) || es != nil || ec != nil) {
			t.Fatal("payer does not decide who is owed", payer, er, es, ec)
		}
	}
}
