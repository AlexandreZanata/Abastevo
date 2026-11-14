package domain_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"math/rand"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	budget "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/budget/domain"
	expense "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/expense/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
)

var saoPaulo = orgtime.ReportingZones{Organization: "America/Sao_Paulo"}

func scope(t *testing.T, n int) kernel.OrgScope {
	t.Helper()
	s := vectorScope(uuid(n))
	if s.Validate() != nil {
		t.Fatal("invalid test scope")
	}
	return s
}

func str(v string) *string { return &v }

// must panics on a construction error; every test defers recoverFatal, so the
// panic fails only that test.
func must[T any](v T, e error) T {
	if e != nil {
		panic(e)
	}
	return v
}

func recoverFatal(t *testing.T) {
	if r := recover(); r != nil {
		t.Fatal("construction failed:", r)
	}
}

func target(t *testing.T, s kernel.OrgScope, kind budget.DimensionKind, n int) budget.Target {
	t.Helper()
	return must(budget.NewTarget(s, kind, uuid(n)))
}

// line builds line 101 for November 2026 in scope s.
func line(t *testing.T, s kernel.OrgScope, kind budget.DimensionKind, x *budget.Target, filter *string, limit string) budget.Line {
	t.Helper()
	d := must(budget.NewDimension(s, kind, x, filter))
	ref := must(budget.NewLineRef(s, uuid(101)))
	return must(budget.NewLine(s, budget.LineInput{Reference: ref, Start: "2026-11-01", End: "2026-12-01", Dimension: d, Limit: &limit}))
}

func orgLine(t *testing.T, s kernel.OrgScope, limit string) budget.Line {
	t.Helper()
	return line(t, s, budget.DimensionOrganization, nil, nil, limit)
}

// consumption builds a DIRECT expense n with the given company amount.
func consumption(t *testing.T, s kernel.OrgScope, n int, company string, state expense.ReviewState, tags budget.Tags) budget.Consumption {
	t.Helper()
	ref := must(expense.NewExpenseRef(s, uuid(n)))
	origin := must(expense.NewOrigin(s, expense.KindDirect, nil))
	category := must(expense.ParseCategory("FUEL"))
	amounts := must(expense.NewAmounts(&company, &company))
	at := must(orgtime.ParseInstant(defaultAt))
	x := must(expense.NewExpense(s, expense.ExpenseInput{Reference: ref, Origin: origin, Category: category, Amounts: amounts,
		Competence: &orgtime.Declaration{OccurredAt: at, EntryZone: "UTC"}, Zones: saoPaulo}))
	return budget.Consumption{Entry: expense.Entry{Expense: x, State: state}, Tags: tags}
}

func request(t *testing.T, s kernel.OrgScope, n int, amount string, tags budget.Tags) budget.HoldRequest {
	t.Helper()
	at := must(orgtime.ParseInstant(defaultAt))
	return budget.HoldRequest{Reference: must(budget.NewHoldRef(s, uuid(n))), Line: must(budget.NewLineRef(s, uuid(101))),
		Amount: &amount, ReservedAt: at, Zones: saoPaulo, Category: "FUEL", Tags: tags}
}

func activeHold(t *testing.T, s kernel.OrgScope, c budget.Capacity, n int, amount string) budget.Hold {
	t.Helper()
	r := must(budget.Reserve(s, c, request(t, s, n, amount, budget.Tags{})))
	h, ok := r.Hold()
	if !ok {
		t.Fatal("reservation did not activate")
	}
	return h
}

func TestClosedVocabularies(t *testing.T) {
	defer recoverFatal(t)
	for _, raw := range []string{"ORGANIZATION", "UNIT", "COST_CENTRE", "ASSET", "EMPLOYEE"} {
		if k, e := budget.ParseDimensionKind(raw); e != nil || string(k) != raw {
			t.Fatal("documented dimension refused", raw, e)
		}
	}
	for _, raw := range []string{"", "unit", "COST_CENTER", "VEHICLE", "DRIVER", "CATEGORY", "TEAM", "SELF", "UNIT\n"} {
		if _, e := budget.ParseDimensionKind(raw); !errors.Is(e, budget.ErrInvalid) {
			t.Fatal("unknown dimension accepted", raw, e)
		}
	}
	for _, raw := range []string{"ACTIVE", "CONSUMED", "RELEASED", "EXPIRED"} {
		if v, e := budget.ParseHoldState(raw); e != nil || string(v) != raw {
			t.Fatal("documented hold state refused", raw, e)
		}
	}
	for _, raw := range []string{"", "active", "PENDING", "CANCELLED", "RESERVED", "VOIDED"} {
		if _, e := budget.ParseHoldState(raw); !errors.Is(e, budget.ErrInvalid) {
			t.Fatal("unknown hold state accepted", raw, e)
		}
	}
	s := scope(t, 1)
	if _, e := budget.NewDimension(s, budget.DimensionKind("VEHICLE"), nil, nil); !errors.Is(e, budget.ErrInvalid) {
		t.Fatal("cast dimension kind accepted", e)
	}
	for _, kind := range []budget.DimensionKind{"", "OTHER", budget.DimensionOrganization} {
		if _, e := budget.NewTarget(s, kind, uuid(301)); !errors.Is(e, budget.ErrInvalid) {
			t.Fatal("non-target kind accepted", kind, e)
		}
	}
}

func TestZeroValuesNeverDecide(t *testing.T) {
	defer recoverFatal(t)
	s, zero := scope(t, 1), kernel.OrgScope{}
	l := orgLine(t, s, "500000")
	c := must(budget.ComputeCapacity(s, l, nil, nil))
	cons := consumption(t, s, 11, "1000", expense.Approved, budget.Tags{})
	req := request(t, s, 201, "1000", budget.Tags{})
	var (
		noLine     budget.Line
		noCapacity budget.Capacity
		noHold     budget.Hold
	)
	checks := []struct {
		name string
		err  error
		want error
	}{
		{"capacity without line", second(budget.ComputeCapacity(s, noLine, nil, nil)), budget.ErrInvalid},
		{"covers without line", noLine.Covers(cons), budget.ErrInvalid},
		{"reserve without capacity", second(budget.Reserve(s, noCapacity, req)), budget.ErrInvalid},
		{"post without capacity", second(budget.Post(s, noCapacity, cons, nil)), budget.ErrInvalid},
		{"reconcile without capacity", noCapacity.Reconcile("0"), budget.ErrInvalid},
		{"release zero hold", second(noHold.Release()), budget.ErrInvalid},
		{"expire zero hold", second(noHold.Expire()), budget.ErrInvalid},
		{"unconstructed expense", second(budget.ComputeCapacity(s, l, []budget.Consumption{{}}, nil)), budget.ErrInvalid},
		{"unconstructed hold", second(budget.ComputeCapacity(s, l, nil, []budget.Hold{{}})), budget.ErrInvalid},
		{"zero hold reference", second(budget.Post(s, c, cons, &budget.HoldRef{})), budget.ErrInvalid},
		{"zero request reference", second(budget.Reserve(s, c, budget.HoldRequest{})), budget.ErrInvalid},
		{"line without limit", second(budget.NewLine(s, budget.LineInput{Reference: l.Reference(), Start: "2026-11-01",
			End: "2026-12-01", Dimension: l.Dimension()})), budget.ErrInvalid},
		{"line without dimension", second(budget.NewLine(s, budget.LineInput{Reference: l.Reference(), Start: "2026-11-01",
			End: "2026-12-01", Limit: str("1")})), budget.ErrInvalid},
		{"line ref zero scope", second(budget.NewLineRef(zero, uuid(101))), budget.ErrScope},
		{"hold ref zero scope", second(budget.NewHoldRef(zero, uuid(201))), budget.ErrScope},
		{"target zero scope", second(budget.NewTarget(zero, budget.DimensionUnit, uuid(301))), budget.ErrScope},
		{"tags zero scope", second(budget.NewTags(zero)), budget.ErrScope},
		{"dimension zero scope", second(budget.NewDimension(zero, budget.DimensionOrganization, nil, nil)), budget.ErrScope},
		{"line zero scope", second(budget.NewLine(zero, budget.LineInput{Reference: l.Reference()})), budget.ErrScope},
		{"hold zero scope", second(budget.NewHold(zero, budget.HoldInput{})), budget.ErrScope},
		{"capacity zero scope", second(budget.ComputeCapacity(zero, l, nil, nil)), budget.ErrScope},
		{"reserve zero scope", second(budget.Reserve(zero, c, req)), budget.ErrScope},
		{"post zero scope", second(budget.Post(zero, c, cons, nil)), budget.ErrScope},
	}
	for _, check := range checks {
		if !errors.Is(check.err, check.want) {
			t.Fatal(check.name, check.err)
		}
	}
	var r budget.Reservation
	var p budget.Posting
	if r.Outcome() != "" || p.Outcome() != "" {
		t.Fatal("a zero decision carries an outcome")
	}
	if _, ok := r.Hold(); ok {
		t.Fatal("a zero reservation carries a hold")
	}
	if _, ok := p.ConsumedHold(); ok {
		t.Fatal("a zero posting carries a hold")
	}
}

func second[T any](_ T, e error) error { return e }

func TestConstructorsRefuseForeignAndUnconstructedParts(t *testing.T) {
	defer recoverFatal(t)
	s, foreign := scope(t, 1), scope(t, 2)
	ref, lineRef := must(budget.NewHoldRef(s, uuid(201))), must(budget.NewLineRef(s, uuid(101)))
	at := must(orgtime.ParseInstant(defaultAt))
	hold := func(line budget.LineRef, state budget.HoldState, by *expense.ExpenseRef) error {
		return second(budget.NewHold(s, budget.HoldInput{Reference: ref, Line: line, Amount: str("1"), ReservedAt: at, Zones: saoPaulo,
			Category: "FUEL", State: state, ConsumedBy: by}))
	}
	foreignDimension := must(budget.NewDimension(foreign, budget.DimensionOrganization, nil, nil))
	checks := []struct {
		name string
		err  error
		want error
	}{
		{"foreign dimension", second(budget.NewLine(s, budget.LineInput{Reference: lineRef, Start: "2026-11-01", End: "2026-12-01",
			Dimension: foreignDimension, Limit: str("1")})), budget.ErrScope},
		{"unconstructed hold line", hold(budget.LineRef{}, budget.HoldActive, nil), budget.ErrInvalid},
		{"foreign hold line", hold(must(budget.NewLineRef(foreign, uuid(101))), budget.HoldActive, nil), budget.ErrScope},
		{"unconstructed consuming expense", hold(lineRef, budget.HoldConsumed, &expense.ExpenseRef{}), budget.ErrInvalid},
		{"consumed hold without expense", hold(lineRef, budget.HoldConsumed, nil), budget.ErrInvalid},
	}
	for _, check := range checks {
		if !errors.Is(check.err, check.want) {
			t.Fatal(check.name, check.err)
		}
	}
	if hold(lineRef, budget.HoldReleased, nil) != nil {
		t.Fatal("a released hold without an expense was refused")
	}
}

func TestTagsAreOnePerKindAndScoped(t *testing.T) {
	defer recoverFatal(t)
	s, foreign := scope(t, 1), scope(t, 2)
	u, u2 := target(t, s, budget.DimensionUnit, 301), target(t, s, budget.DimensionUnit, 302)
	if _, e := budget.NewTags(s, u, u2); !errors.Is(e, budget.ErrInvalid) {
		t.Fatal("two units accepted", e)
	}
	if _, e := budget.NewTags(s, budget.Target{}); !errors.Is(e, budget.ErrInvalid) {
		t.Fatal("zero target accepted", e)
	}
	if _, e := budget.NewTags(s, target(t, foreign, budget.DimensionAsset, 321)); !errors.Is(e, budget.ErrScope) {
		t.Fatal("foreign target accepted", e)
	}
	if _, e := budget.NewDimension(s, budget.DimensionUnit, &budget.Target{}, nil); !errors.Is(e, budget.ErrInvalid) {
		t.Fatal("zero target accepted in a dimension", e)
	}
	empty := must(budget.NewTags(s))
	if _, ok := empty.Get(budget.DimensionUnit); ok {
		t.Fatal("empty tags carry a unit")
	}
	cc, v, d := target(t, s, budget.DimensionCostCentre, 311), target(t, s, budget.DimensionAsset, 321), target(t, s, budget.DimensionEmployee, 331)
	all := must(budget.NewTags(s, d, v, cc, u))
	for _, want := range []budget.Target{u, cc, v, d} {
		got, ok := all.Get(want.Kind())
		if !ok || got.ID() != want.ID() || got.Kind() != want.Kind() || got.OrganizationID() != s.OrgID() {
			t.Fatal("tag not returned by kind", want.Kind())
		}
	}
	if _, ok := all.Get(budget.DimensionOrganization); ok {
		t.Fatal("organization is not a tag")
	}
}

func TestCoversRoutesOneExpenseToEveryMatchingLine(t *testing.T) {
	defer recoverFatal(t)
	s := scope(t, 1)
	u, cc, v := target(t, s, budget.DimensionUnit, 301), target(t, s, budget.DimensionCostCentre, 311), target(t, s, budget.DimensionAsset, 321)
	cons := consumption(t, s, 11, "97350", expense.Approved, must(budget.NewTags(s, u, cc, v)))
	matching := []budget.Line{orgLine(t, s, "1"), line(t, s, budget.DimensionUnit, &u, nil, "1"),
		line(t, s, budget.DimensionCostCentre, &cc, nil, "1"), line(t, s, budget.DimensionAsset, &v, str("FUEL"), "1")}
	for _, l := range matching {
		if e := l.Covers(cons); e != nil {
			t.Fatal("matching line refused", l.Dimension().Kind(), e)
		}
	}
	d, cc2 := target(t, s, budget.DimensionEmployee, 331), target(t, s, budget.DimensionCostCentre, 312)
	unitAsAsset := target(t, s, budget.DimensionAsset, 301)
	for _, l := range []budget.Line{line(t, s, budget.DimensionEmployee, &d, nil, "1"), line(t, s, budget.DimensionCostCentre, &cc2, nil, "1"),
		line(t, s, budget.DimensionAsset, &unitAsAsset, nil, "1"), line(t, s, budget.DimensionOrganization, nil, str("TOLL"), "1")} {
		if e := l.Covers(cons); !errors.Is(e, budget.ErrDimension) {
			t.Fatal("non-matching line accepted", l.Dimension().Kind(), e)
		}
	}
	if e := orgLine(t, scope(t, 2), "1").Covers(cons); !errors.Is(e, budget.ErrScope) {
		t.Fatal("foreign line covered the expense", e)
	}
}

func TestExactCapacityBeyondTheRecordCeiling(t *testing.T) {
	defer recoverFatal(t)
	const n = 10000
	s := scope(t, 1)
	l := orgLine(t, s, "1000000000000")
	cons := make([]budget.Consumption, 0, n)
	holds := make([]budget.Hold, 0, n)
	empty := must(budget.NewTags(s))
	base := must(budget.ComputeCapacity(s, orgLine(t, s, "1000000000000"), nil, nil))
	for i := 0; i < n; i++ {
		cons = append(cons, consumption(t, s, 100000+i, "1000000000000", expense.Approved, empty))
		holds = append(holds, activeHold(t, s, base, 200000+i, "1000000000000"))
	}
	c := must(budget.ComputeCapacity(s, l, cons, holds))
	each := new(big.Int).Mul(big.NewInt(n), big.NewInt(1000000000000))
	want := new(big.Int).Sub(big.NewInt(1000000000000), new(big.Int).Add(each, each))
	if c.ConsumedCentavos().Cmp(each) != 0 || c.HeldCentavos().Cmp(each) != 0 || c.AvailableCentavos().Cmp(want) != 0 ||
		!c.OverCommitted() || c.ConsumedCount() != n || c.ActiveHolds() != n {
		t.Fatal("inexact capacity", c.ConsumedCentavos(), c.HeldCentavos(), c.AvailableCentavos())
	}
	r := must(budget.Reserve(s, c, request(t, s, 9, "1", empty)))
	shortfall := new(big.Int).Sub(big.NewInt(1), want)
	if r.Outcome() != budget.OutcomeExceeds || r.ShortfallCentavos().Cmp(shortfall) != 0 {
		t.Fatal("over-committed line activated a hold", r.Outcome(), r.ShortfallCentavos())
	}
	if c.Reconcile(want.String()) != nil || !errors.Is(c.Reconcile("-99999999999999999999999999"), budget.ErrStaleAvailable) ||
		!errors.Is(c.Reconcile(strings.Repeat("9", 80)), budget.ErrInvalid) {
		t.Fatal("claimed numbers beyond int64 are not compared exactly")
	}
}

// TestSerializedDecisionsPreserveCapacity replays random serialized steps and
// proves the decisions never over-commit a line, that each decision's
// reported availability equals the recomputation from the new facts, and that
// a hold leaves ACTIVE at most once.
func TestSerializedDecisionsPreserveCapacity(t *testing.T) {
	defer recoverFatal(t)
	s := scope(t, 1)
	empty := must(budget.NewTags(s))
	l := orgLine(t, s, "500000")
	cons := []budget.Consumption{consumption(t, s, 11, "450000", expense.Approved, empty)}
	var holds []budget.Hold
	compute := func() budget.Capacity { return must(budget.ComputeCapacity(s, l, cons, holds)) }
	rng := rand.New(rand.NewSource(14))
	closed := map[string]bool{}
	nextID := 1000
	for step := 0; step < 1500; step++ {
		before := compute()
		avail := before.AvailableCentavos()
		nextID++
		switch op := rng.Intn(4); {
		case op == 0:
			amount := int64(1 + rng.Intn(60000))
			r := must(budget.Reserve(s, before, request(t, s, 200000+nextID, fmt.Sprint(amount), empty)))
			if h, ok := r.Hold(); ok {
				if r.Outcome() != budget.OutcomeActivate || big.NewInt(amount).Cmp(avail) > 0 || h.State() != budget.HoldActive {
					t.Fatal("activation beyond capacity")
				}
				holds = append(holds, h)
			} else if r.Outcome() != budget.OutcomeExceeds || r.ShortfallCentavos().Cmp(new(big.Int).Sub(big.NewInt(amount), avail)) != 0 {
				t.Fatal("refusal without the exact shortfall")
			}
			if compute().AvailableCentavos().Cmp(r.AvailableAfterCentavos()) != 0 {
				t.Fatal("reservation availability differs from recomputation")
			}
		case op == 1 && len(holds) > 0:
			i := rng.Intn(len(holds))
			h := holds[i]
			transition := h.Release
			if rng.Intn(2) == 0 {
				transition = h.Expire
			}
			next, e := transition()
			if closed[h.Reference().ID()] {
				if !errors.Is(e, budget.ErrHoldNotActive) {
					t.Fatal("closed hold transitioned again", e)
				}
				continue
			}
			if e != nil {
				t.Fatal(e)
			}
			holds[i], closed[h.Reference().ID()] = next, true
			restored := new(big.Int).Add(avail, big.NewInt(h.AmountCentavos()))
			if compute().AvailableCentavos().Cmp(restored) != 0 {
				t.Fatal("release or expiry did not restore exactly the hold amount")
			}
		default:
			amount := rng.Intn(60000)
			posted := consumption(t, s, 100000+nextID, fmt.Sprint(amount), expense.Approved, empty)
			var ref *budget.HoldRef
			headroom := new(big.Int).Set(avail)
			if len(holds) > 0 && rng.Intn(2) == 0 {
				h := holds[rng.Intn(len(holds))]
				hr := h.Reference()
				ref = &hr
				if closed[hr.ID()] {
					if _, e := budget.Post(s, before, posted, ref); !errors.Is(e, budget.ErrHoldNotActive) {
						t.Fatal("closed hold converted", e)
					}
					continue
				}
				headroom.Add(headroom, big.NewInt(h.AmountCentavos()))
			}
			p := must(budget.Post(s, before, posted, ref))
			if p.Outcome() == budget.OutcomePost {
				if amount != 0 && big.NewInt(int64(amount)).Cmp(headroom) > 0 {
					t.Fatal("posting beyond headroom")
				}
				cons = append(cons, posted)
				if h, ok := p.ConsumedHold(); ok {
					by, linked := h.ConsumedBy()
					if !linked || by.ID() != posted.Entry.Expense.Reference().ID() || h.State() != budget.HoldConsumed {
						t.Fatal("converted hold not linked to its expense")
					}
					holds[holdIndex(holds, h.Reference().ID())], closed[h.Reference().ID()] = h, true
				}
			} else if p.Outcome() != budget.OutcomeExceeds || p.ShortfallCentavos().Cmp(new(big.Int).Sub(big.NewInt(int64(amount)), headroom)) != 0 {
				t.Fatal("posting refusal without the exact shortfall")
			}
			if compute().AvailableCentavos().Cmp(p.AvailableAfterCentavos()) != 0 {
				t.Fatal("posting availability differs from recomputation")
			}
		}
		if compute().AvailableCentavos().Sign() < 0 {
			t.Fatal("a serialized decision over-committed the line", step)
		}
	}
	if len(closed) == 0 || len(holds) == 0 {
		t.Fatal("the random replay exercised too little")
	}
}

func TestConcurrentReadersShareNoState(t *testing.T) {
	defer recoverFatal(t)
	s := scope(t, 1)
	empty := must(budget.NewTags(s))
	l := orgLine(t, s, "500000")
	base := must(budget.ComputeCapacity(s, l, nil, nil))
	hold := activeHold(t, s, base, 202, "30000")
	c := must(budget.ComputeCapacity(s, l, []budget.Consumption{consumption(t, s, 11, "420000", expense.Approved, empty)}, []budget.Hold{hold}))
	req, posted, ref := request(t, s, 201, "30000", empty), consumption(t, s, 15, "28000", expense.Approved, empty), hold.Reference()
	var wg sync.WaitGroup
	results := make([]string, 16)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r, e1 := budget.Reserve(s, c, req)
			p, e2 := budget.Post(s, c, posted, &ref)
			c.AvailableCentavos().SetInt64(-1)
			r.AvailableAfterCentavos().SetInt64(-1)
			results[i] = fmt.Sprintf("%s %s %s %s %v %v %s %v", r.Outcome(), r.AvailableAfterCentavos(), p.Outcome(),
				p.AvailableAfterCentavos(), e1, e2, c.AvailableCentavos(), c.Reconcile("50000"))
		}(i)
	}
	wg.Wait()
	for _, got := range results {
		if got != "ACTIVATE 20000 POST 52000 <nil> <nil> 50000 <nil>" {
			t.Fatal("shared capacity changed under concurrent readers", got)
		}
	}
}

func TestHoldIsAnImmutableValue(t *testing.T) {
	defer recoverFatal(t)
	s := scope(t, 1)
	unit := "America/Cuiaba"
	at := time.Date(2026, 11, 30, 23, 30, 0, 0, time.FixedZone("x", -4*3600))
	ref, lineRef := must(budget.NewHoldRef(s, uuid(201))), must(budget.NewLineRef(s, uuid(101)))
	tags := must(budget.NewTags(s, target(t, s, budget.DimensionCostCentre, 311)))
	h := must(budget.NewHold(s, budget.HoldInput{Reference: ref, Line: lineRef, Amount: str("30000"), ReservedAt: at,
		Zones: orgtime.ReportingZones{Organization: "America/Sao_Paulo", Unit: &unit}, Category: "FUEL", Tags: tags, State: budget.HoldActive}))
	if h.ReservedAt().Location() != time.UTC || !h.ReservedAt().Equal(at) || h.Reporting().Day != "2026-11-30" ||
		h.Reporting().Source != "unit" || h.AmountCentavos() != 30000 || h.Category().Code() != "FUEL" ||
		h.Line().ID() != uuid(101) || h.Reference().OrganizationID() != s.OrgID() {
		t.Fatal("hold facts not preserved", h.ReservedAt(), h.Reporting())
	}
	if x, ok := h.Tags().Get(budget.DimensionCostCentre); !ok || x.ID() != uuid(311) {
		t.Fatal("hold tags not preserved")
	}
	if _, ok := h.ConsumedBy(); ok {
		t.Fatal("active hold carries a consuming expense")
	}
	released := must(h.Release())
	if h.State() != budget.HoldActive || released.State() != budget.HoldReleased {
		t.Fatal("release mutated the original value")
	}
	for _, e := range []error{second(released.Release()), second(released.Expire())} {
		if !errors.Is(e, budget.ErrHoldNotActive) {
			t.Fatal("released hold transitioned again", e)
		}
	}
	expired := must(h.Expire())
	if expired.State() != budget.HoldExpired || !errors.Is(second(expired.Release()), budget.ErrHoldNotActive) {
		t.Fatal("expired hold transitioned again")
	}
	by := must(expense.NewExpenseRef(s, uuid(15)))
	consumed := must(budget.NewHold(s, budget.HoldInput{Reference: ref, Line: lineRef, Amount: str("30000"), ReservedAt: at,
		Zones: saoPaulo, Category: "FUEL", State: budget.HoldConsumed, ConsumedBy: &by}))
	if got, ok := consumed.ConsumedBy(); !ok || got.ID() != uuid(15) || !errors.Is(second(consumed.Expire()), budget.ErrHoldNotActive) {
		t.Fatal("consumed hold restored incorrectly")
	}
}

func TestGettersReturnCopies(t *testing.T) {
	defer recoverFatal(t)
	s := scope(t, 1)
	c := must(budget.ComputeCapacity(s, orgLine(t, s, "500000"), nil, nil))
	for _, v := range []*big.Int{c.ConsumedCentavos(), c.PendingCentavos(), c.HeldCentavos(), c.AvailableCentavos()} {
		v.SetInt64(-7)
	}
	if c.AvailableCentavos().Int64() != 500000 || c.ConsumedCentavos().Sign() != 0 || c.HeldCentavos().Sign() != 0 ||
		c.PendingCentavos().Sign() != 0 || c.LimitCentavos() != 500000 || c.Line().LimitCentavos() != 500000 {
		t.Fatal("capacity exposed mutable state")
	}
	r := must(budget.Reserve(s, c, request(t, s, 201, "600000", budget.Tags{})))
	for _, v := range []*big.Int{r.AvailableBeforeCentavos(), r.AvailableAfterCentavos(), r.ShortfallCentavos()} {
		v.SetInt64(-7)
	}
	if r.AvailableBeforeCentavos().Int64() != 500000 || r.ShortfallCentavos().Int64() != 100000 || r.RequestedCentavos() != 600000 {
		t.Fatal("reservation exposed mutable state")
	}
	p := must(budget.Post(s, c, consumption(t, s, 15, "1000", expense.Approved, budget.Tags{}), nil))
	for _, v := range []*big.Int{p.AvailableBeforeCentavos(), p.AvailableAfterCentavos(), p.ShortfallCentavos()} {
		v.SetInt64(-7)
	}
	if p.AvailableAfterCentavos().Int64() != 499000 || p.ShortfallCentavos().Sign() != 0 || p.AmountCentavos() != 1000 {
		t.Fatal("posting exposed mutable state")
	}
}

func TestPrivateValuesAreRedacted(t *testing.T) {
	defer recoverFatal(t)
	s := scope(t, 1)
	cc := target(t, s, budget.DimensionCostCentre, 311)
	tags := must(budget.NewTags(s, cc))
	l := line(t, s, budget.DimensionCostCentre, &cc, str("FUEL"), "500000")
	cons := consumption(t, s, 11, "450000", expense.Approved, tags)
	c := must(budget.ComputeCapacity(s, l, []budget.Consumption{cons}, nil))
	req := request(t, s, 201, "30000", tags)
	r := must(budget.Reserve(s, c, req))
	h, _ := r.Hold()
	p := must(budget.Post(s, c, consumption(t, s, 15, "1000", expense.Approved, tags), nil))
	values := []any{cc, tags, l, l.Reference(), l.Dimension(), cons, c, req, r, h, h.Reference(), p,
		budget.LineInput{Reference: l.Reference(), Limit: str("500000")}, budget.HoldInput{Amount: str("30000"), Tags: tags}}
	for _, v := range values {
		var logged bytes.Buffer
		slog.New(slog.NewJSONHandler(&logged, nil)).Info("budget", "value", v)
		encoded, e := json.Marshal(v)
		if e != nil {
			t.Fatal(e)
		}
		for _, text := range []string{fmt.Sprintf("%v", v), fmt.Sprintf("%+v", v), fmt.Sprintf("%#v", v), fmt.Sprint(v),
			logged.String(), string(encoded)} {
			if strings.Contains(text, "9e000000") || strings.Contains(text, "500000") || strings.Contains(text, "30000") ||
				strings.Contains(text, "450000") {
				t.Fatalf("private value leaked from %T: %s", v, text)
			}
		}
	}
}

func TestInputsCarryNoDerivedNumber(t *testing.T) {
	defer recoverFatal(t)
	forbidden := map[string]bool{"Available": true, "Held": true, "Consumed": true, "Remaining": true, "Balance": true, "Pending": true}
	for _, v := range []any{budget.LineInput{}, budget.HoldInput{}, budget.HoldRequest{}, budget.Consumption{}} {
		typ := reflect.TypeOf(v)
		for i := 0; i < typ.NumField(); i++ {
			if forbidden[typ.Field(i).Name] {
				t.Fatal("an input accepts a derived number", typ.Name(), typ.Field(i).Name)
			}
		}
	}
}
