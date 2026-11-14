package domain_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"math/rand"
	"strings"
	"sync"
	"testing"
	"time"

	expense "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/expense/domain"
	fueling "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
)

func scope(t *testing.T, n int) kernel.OrgScope {
	t.Helper()
	s := vectorScope(uuid(n))
	if s.Validate() != nil {
		t.Fatal("invalid test scope")
	}
	return s
}

func str(v string) *string { return &v }

func amounts(t *testing.T, paid, company string) expense.Amounts {
	t.Helper()
	a, e := expense.NewAmounts(&paid, &company)
	if e != nil {
		t.Fatal(e)
	}
	return a
}

// input returns a valid fueling-origin input for expense n in scope s.
func input(t *testing.T, s kernel.OrgScope, n int, company string) expense.ExpenseInput {
	t.Helper()
	ref, e := expense.NewExpenseRef(s, uuid(n))
	if e != nil {
		t.Fatal(e)
	}
	src, e := expense.NewSourceRef(s, uuid(1000+n))
	if e != nil {
		t.Fatal(e)
	}
	origin, e := expense.NewOrigin(s, expense.KindFueling, &src)
	if e != nil {
		t.Fatal(e)
	}
	category, e := expense.ParseCategory("FUEL")
	if e != nil {
		t.Fatal(e)
	}
	at, e := orgtime.ParseInstant("2026-11-14T12:00:00Z")
	if e != nil {
		t.Fatal(e)
	}
	return expense.ExpenseInput{Reference: ref, Origin: origin, Category: category, Amounts: amounts(t, company, company),
		Competence: &orgtime.Declaration{OccurredAt: at, EntryZone: "UTC"}, Zones: orgtime.ReportingZones{Organization: "America/Sao_Paulo"}}
}

func build(t *testing.T, s kernel.OrgScope, in expense.ExpenseInput) expense.Expense {
	t.Helper()
	e, err := expense.NewExpense(s, in)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func november(t *testing.T) expense.ReportingPeriod {
	t.Helper()
	p, e := expense.NewReportingPeriod("2026-11-01", "2026-12-01")
	if e != nil {
		t.Fatal(e)
	}
	return p
}

func TestOriginKindsAreClosedAndSettlementIsRefused(t *testing.T) {
	for _, raw := range []string{"FUELING", "WORK_ORDER", "DIRECT"} {
		if k, e := expense.ParseOriginKind(raw); e != nil || string(k) != raw {
			t.Fatal("cost origin refused", raw, e)
		}
	}
	for _, raw := range []string{"PAYMENT", "REIMBURSEMENT", "CARD_STATEMENT", "ADVANCE", "PAYABLE", "DOCUMENT", "STOCK_PURCHASE", "STOCK_TRANSFER"} {
		if _, e := expense.ParseOriginKind(raw); !errors.Is(e, expense.ErrNotCostOrigin) {
			t.Fatal("settlement accepted as cost origin", raw, e)
		}
		s := scope(t, 1)
		src, _ := expense.NewSourceRef(s, uuid(21))
		if _, e := expense.NewOrigin(s, expense.OriginKind(raw), &src); !errors.Is(e, expense.ErrNotCostOrigin) {
			t.Fatal("typed settlement kind bypassed the parser", raw, e)
		}
	}
	for _, raw := range []string{"", "fueling", "Payment", "MAINTENANCE", "MANUAL", " DIRECT", "DIRECT\n"} {
		if _, e := expense.ParseOriginKind(raw); !errors.Is(e, expense.ErrInvalid) {
			t.Fatal("unknown origin token accepted", raw, e)
		}
	}
	if _, e := expense.NewOrigin(scope(t, 1), expense.OriginKind("OTHER"), nil); !errors.Is(e, expense.ErrInvalid) {
		t.Fatal("cast origin kind accepted", e)
	}
}

func TestFuelingOriginReusesTheFuelingIdentity(t *testing.T) {
	s, foreign := scope(t, 1), scope(t, 2)
	ref, e := fueling.NewFuelingRef(s, uuid(21))
	if e != nil {
		t.Fatal(e)
	}
	origin, e := expense.NewFuelingOrigin(s, ref)
	if e != nil || origin.Kind() != expense.KindFueling || origin.Source() == nil || origin.Source().ID() != ref.ID() ||
		origin.Source().OrganizationID() != s.OrgID() {
		t.Fatal("fueling origin lost its source", e)
	}
	other, _ := fueling.NewFuelingRef(foreign, uuid(21))
	if _, e = expense.NewFuelingOrigin(s, other); !errors.Is(e, expense.ErrScope) {
		t.Fatal("foreign fueling accepted", e)
	}
	if _, e = expense.NewFuelingOrigin(s, fueling.FuelingRef{}); !errors.Is(e, expense.ErrInvalid) {
		t.Fatal("unconstructed fueling reference accepted", e)
	}
	if _, e = expense.NewFuelingOrigin(kernel.OrgScope{}, ref); !errors.Is(e, expense.ErrScope) {
		t.Fatal("zero scope accepted", e)
	}
	in := input(t, s, 11, "97350")
	in.Origin = origin
	key := build(t, s, in).OriginKey()
	if key.OrganizationID() != s.OrgID() || key.Kind() != expense.KindFueling || key.Source() != uuid(21) {
		t.Fatal("fueling origin key changed")
	}
}

func TestOriginShapeAndOneCostKey(t *testing.T) {
	s, foreign := scope(t, 1), scope(t, 2)
	src, _ := expense.NewSourceRef(s, uuid(21))
	far, _ := expense.NewSourceRef(foreign, uuid(21))
	for name, c := range map[string]struct {
		kind   expense.OriginKind
		source *expense.SourceRef
		want   error
	}{
		"direct-with-source":    {expense.KindDirect, &src, expense.ErrInvalid},
		"fueling-no-source":     {expense.KindFueling, nil, expense.ErrInvalid},
		"work-order-no-source":  {expense.KindWorkOrder, nil, expense.ErrInvalid},
		"foreign-source":        {expense.KindWorkOrder, &far, expense.ErrScope},
		"unconstructed-source":  {expense.KindFueling, &expense.SourceRef{}, expense.ErrInvalid},
		"direct-without-source": {expense.KindDirect, nil, nil},
	} {
		if _, e := expense.NewOrigin(s, c.kind, c.source); !errors.Is(e, c.want) && !(c.want == nil && e == nil) {
			t.Fatal(name, e)
		}
	}
	direct, _ := expense.NewOrigin(s, expense.KindDirect, nil)
	work, _ := expense.NewOrigin(s, expense.KindWorkOrder, &src)
	fuel, _ := expense.NewOrigin(s, expense.KindFueling, &src)
	keys := map[expense.OriginKey]bool{}
	for n, origin := range []expense.Origin{direct, work, fuel} {
		in := input(t, s, 11+n, "100")
		in.Origin = origin
		keys[build(t, s, in).OriginKey()] = true
	}
	in := input(t, s, 30, "100")
	in.Origin = direct
	d1, d2 := build(t, s, in).OriginKey(), build(t, s, in).OriginKey()
	if len(keys) != 3 || d1 != d2 || d1.Source() != uuid(30) || d1.Kind() != expense.KindDirect {
		t.Fatal("origin keys do not identify one cost per kind and source")
	}
	// An origin built for another organization cannot be attached.
	foreignOrigin, _ := expense.NewOrigin(foreign, expense.KindFueling, &far)
	in = input(t, s, 31, "100")
	in.Origin = foreignOrigin
	if _, e := expense.NewExpense(s, in); !errors.Is(e, expense.ErrScope) {
		t.Fatal("foreign origin accepted", e)
	}
}

func TestAmountsNeverDefaultRoundOrRecompute(t *testing.T) {
	for _, c := range []struct{ paid, company *string }{{nil, str("1")}, {str("1"), nil}, {nil, nil}, {str(""), str("0")}} {
		if _, e := expense.NewAmounts(c.paid, c.company); !errors.Is(e, expense.ErrInvalid) {
			t.Fatal("absent or empty amount accepted", e)
		}
	}
	zero := amounts(t, "0", "0")
	if zero.PaidCentavos() != 0 || zero.CompanyCentavos() != 0 {
		t.Fatal("known zero changed")
	}
	if _, e := expense.NewAmounts(str("97350"), str("97351")); !errors.Is(e, expense.ErrCompanyExceedsPaid) {
		t.Fatal("company above paid accepted", e)
	}
	s := scope(t, 1)
	in := input(t, s, 11, "97350")
	in.Amounts = expense.Amounts{}
	if _, e := expense.NewExpense(s, in); !errors.Is(e, expense.ErrInvalid) {
		t.Fatal("unconstructed amounts became a zero cost", e)
	}
	in = input(t, s, 11, "97350")
	in.Amounts = amounts(t, "97350", "90000")
	got := build(t, s, in).Amounts()
	if got.PaidCentavos() != 97350 || got.CompanyCentavos() != 90000 {
		t.Fatal("paid or company amount recomputed")
	}
}

func TestExpenseRequiresEveryFactAndScope(t *testing.T) {
	s, foreign := scope(t, 1), scope(t, 2)
	bad := "Mars/Olympus"
	for name, c := range map[string]struct {
		alter func(*expense.ExpenseInput)
		want  error
	}{
		"reference":  {func(i *expense.ExpenseInput) { i.Reference = expense.ExpenseRef{} }, expense.ErrInvalid},
		"origin":     {func(i *expense.ExpenseInput) { i.Origin = expense.Origin{} }, expense.ErrInvalid},
		"category":   {func(i *expense.ExpenseInput) { i.Category = expense.Category{} }, expense.ErrInvalid},
		"competence": {func(i *expense.ExpenseInput) { i.Competence = nil }, expense.ErrInvalid},
		"entry-zone": {func(i *expense.ExpenseInput) { i.Competence.EntryZone = "Mars/Olympus" }, expense.ErrInvalid},
		"zones":      {func(i *expense.ExpenseInput) { i.Zones = orgtime.ReportingZones{} }, expense.ErrInvalid},
		"unit-zone":  {func(i *expense.ExpenseInput) { i.Zones.Unit = &bad }, expense.ErrInvalid},
		"foreign-reference": {func(i *expense.ExpenseInput) {
			i.Reference, _ = expense.NewExpenseRef(foreign, uuid(11))
		}, expense.ErrScope},
	} {
		in := input(t, s, 11, "100")
		c.alter(&in)
		if _, e := expense.NewExpense(s, in); !errors.Is(e, c.want) {
			t.Fatal(name, e)
		}
	}
	if _, e := expense.NewExpense(kernel.OrgScope{}, input(t, s, 11, "100")); !errors.Is(e, expense.ErrScope) {
		t.Fatal("zero scope accepted", e)
	}
	if _, e := expense.NewExpenseRef(kernel.OrgScope{}, uuid(11)); !errors.Is(e, expense.ErrScope) {
		t.Fatal("zero scope reference accepted", e)
	}
	for _, id := range []string{"", "not-a-uuid", strings.ToUpper(uuid(11)), "00000000-0000-0000-0000-000000000000"} {
		if _, e := expense.NewExpenseRef(s, id); !errors.Is(e, expense.ErrInvalid) {
			t.Fatal("non-canonical identity accepted", id, e)
		}
		if _, e := expense.NewSourceRef(s, id); !errors.Is(e, expense.ErrInvalid) {
			t.Fatal("non-canonical source accepted", id, e)
		}
	}
	for _, raw := range []string{"FUEL\n", "FU\x00EL", "FUEL\u200b"} {
		if _, e := expense.ParseCategory(raw); !errors.Is(e, expense.ErrInvalid) {
			t.Fatal("control or invisible category accepted", e)
		}
	}
}

func TestCompetenceUsesOperatingZoneNotEntryZone(t *testing.T) {
	s := scope(t, 1)
	in := input(t, s, 11, "97350")
	at, _ := orgtime.ParseInstant("2026-11-01T02:30:00Z")
	in.Competence = &orgtime.Declaration{OccurredAt: at.In(time.FixedZone("device", 9*3600)), EntryZone: "Asia/Tokyo", DeviceOffsetSeconds: 32400}
	e := build(t, s, in)
	if r := e.Reporting(); r.Day != "2026-10-31" || r.Zone != "America/Sao_Paulo" || r.Source != "organization" {
		t.Fatal("reporting day did not follow the organization zone", r)
	}
	if c := e.Competence(); c.OccurredAt.Location() != time.UTC || !c.OccurredAt.Equal(at) || c.EntryZone != "Asia/Tokyo" || c.DeviceOffsetSeconds != 32400 {
		t.Fatal("competence declaration not retained exactly")
	}
	unit := "America/Cuiaba"
	in.Zones.Unit = &unit
	at, _ = orgtime.ParseInstant("2026-11-01T03:30:00Z")
	in.Competence = &orgtime.Declaration{OccurredAt: at, EntryZone: "UTC"}
	if r := build(t, s, in).Reporting(); r.Day != "2026-10-31" || r.Zone != unit || r.Source != "unit" {
		t.Fatal("unit zone ignored", r)
	}
}

func TestExpenseSnapshotsAreImmutableAndRedacted(t *testing.T) {
	s := scope(t, 1)
	in := input(t, s, 11, "97350")
	unit := "America/Cuiaba"
	in.Zones.Unit = &unit
	e := build(t, s, in)
	in.Competence.EntryZone = "Asia/Tokyo"
	in.Competence.OccurredAt = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	unit = "Mars/Olympus"
	if e.Competence().EntryZone != "UTC" || e.Competence().OccurredAt.Year() != 2026 || e.Reporting().Zone != "America/Cuiaba" {
		t.Fatal("expense shares mutable input")
	}
	src := e.Origin().Source()
	*src = expense.SourceRef{}
	if e.Origin().Source().ID() != uuid(1011) {
		t.Fatal("origin source accessor shares state")
	}
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	logger.Info("x", "expense", e, "input", in, "key", e.OriginKey(), "amounts", e.Amounts(), "origin", e.Origin())
	encoded, _ := json.Marshal(map[string]any{"e": e, "in": in, "k": e.OriginKey(), "c": e.Category()})
	text := fmt.Sprintf("%v %+v %#v %s", e, in, e.OriginKey(), e.Amounts()) + buf.String() + string(encoded)
	for _, secret := range []string{uuid(11), uuid(1011), "97350", "FUEL\""} {
		if strings.Contains(text, secret) {
			t.Fatal("private expense value leaked", secret)
		}
	}
}

func TestReviewStatesAndBasisAreClosed(t *testing.T) {
	want := map[expense.ReviewState]expense.Counting{expense.Draft: expense.CountsExcluded, expense.Submitted: expense.CountsPending,
		expense.InReview: expense.CountsPending, expense.Approved: expense.CountsApproved, expense.Returned: expense.CountsExcluded,
		expense.Rejected: expense.CountsExcluded, expense.Cancelled: expense.CountsExcluded, expense.Voided: expense.CountsExcluded}
	for state, counting := range want {
		parsed, e := expense.ParseReviewState(string(state))
		got, e2 := parsed.Counting()
		if e != nil || e2 != nil || got != counting {
			t.Fatal("review state counting changed", state)
		}
	}
	for _, raw := range []string{"", "approved", "PAID", "SETTLED", "PARTIALLY_APPROVED", "CANCELED"} {
		if _, e := expense.ParseReviewState(raw); !errors.Is(e, expense.ErrInvalid) {
			t.Fatal("unknown review state accepted", raw)
		}
		if _, e := expense.ReviewState(raw).Counting(); !errors.Is(e, expense.ErrInvalid) {
			t.Fatal("cast review state counted", raw)
		}
	}
	for _, raw := range []string{"", "ALL", "PENDING", "approved"} {
		if _, e := expense.ParseBasis(raw); !errors.Is(e, expense.ErrInvalid) {
			t.Fatal("unknown basis accepted", raw)
		}
	}
}

func TestReportingPeriodIsHalfOpen(t *testing.T) {
	p := november(t)
	for day, in := range map[string]bool{"2026-10-31": false, "2026-11-01": true, "2026-11-30": true, "2026-12-01": false,
		"": false, "2026-11-1": false, "2026-11-15T00:00:00Z": false, "2026-11-31": false} {
		if p.Contains(day) != in {
			t.Fatal("half-open period membership changed", day)
		}
	}
	if (expense.ReportingPeriod{}).Contains("2026-11-01") {
		t.Fatal("zero period contains a day")
	}
}

func TestTotalsRefuseUnconstructedOrForeignInputs(t *testing.T) {
	s := scope(t, 1)
	ok := []expense.Entry{{Expense: build(t, s, input(t, s, 11, "100")), State: expense.Approved}}
	if _, e := expense.Totals(s, november(t), expense.BasisApproved, []expense.Entry{{State: expense.Approved}}); !errors.Is(e, expense.ErrInvalid) {
		t.Fatal("unconstructed expense counted", e)
	}
	if _, e := expense.Totals(s, november(t), expense.BasisApproved, []expense.Entry{{Expense: ok[0].Expense, State: "PAID"}}); !errors.Is(e, expense.ErrInvalid) {
		t.Fatal("cast review state counted", e)
	}
	if _, e := expense.Totals(s, expense.ReportingPeriod{}, expense.BasisApproved, ok); !errors.Is(e, expense.ErrInvalid) {
		t.Fatal("zero period accepted", e)
	}
	if _, e := expense.Totals(s, november(t), expense.Basis("ALL"), ok); !errors.Is(e, expense.ErrInvalid) {
		t.Fatal("cast basis accepted", e)
	}
	if _, e := expense.Totals(kernel.OrgScope{}, november(t), expense.BasisApproved, ok); !errors.Is(e, expense.ErrScope) {
		t.Fatal("zero scope accepted", e)
	}
	if _, e := expense.Totals(scope(t, 2), november(t), expense.BasisApproved, ok); !errors.Is(e, expense.ErrScope) {
		t.Fatal("foreign entry counted", e)
	}
}

func TestTotalsAreExactWidenedAndPartitioned(t *testing.T) {
	s := scope(t, 1)
	r := rand.New(rand.NewSource(20261114))
	states := []expense.ReviewState{expense.Draft, expense.Submitted, expense.InReview, expense.Approved, expense.Returned,
		expense.Rejected, expense.Cancelled, expense.Voided}
	for round := 0; round < 40; round++ {
		var entries []expense.Entry
		approved, pending := new(big.Int), new(big.Int)
		var approvedCount, pendingCount, excludedCount int
		size := r.Intn(60)
		for n := 0; n < size; n++ {
			company := r.Int63n(1_000_000_000_001)
			state := states[r.Intn(len(states))]
			entries = append(entries, expense.Entry{Expense: build(t, s, input(t, s, 100+n, fmt.Sprint(company))), State: state})
			switch state {
			case expense.Approved:
				approved.Add(approved, big.NewInt(company))
				approvedCount++
			case expense.Submitted, expense.InReview:
				pending.Add(pending, big.NewInt(company))
				pendingCount++
			default:
				excludedCount++
			}
		}
		before := append([]expense.Entry(nil), entries...)
		for _, basis := range []expense.Basis{expense.BasisApproved, expense.BasisIncludesPending} {
			got, e := expense.Totals(s, november(t), basis, entries)
			if e != nil {
				t.Fatal(e)
			}
			figure := new(big.Int).Set(approved)
			if basis == expense.BasisIncludesPending {
				figure.Add(figure, pending)
			}
			if got.ApprovedCentavos().Cmp(approved) != 0 || got.PendingCentavos().Cmp(pending) != 0 || got.FigureCentavos().Cmp(figure) != 0 ||
				got.ApprovedCount() != approvedCount || got.PendingCount() != pendingCount || got.ExcludedCount() != excludedCount ||
				got.IncludesPending() != (basis == expense.BasisIncludesPending) || got.Basis() != basis || got.OrganizationID() != s.OrgID() ||
				got.Period() != november(t) {
				t.Fatal("totals differ from the independent exact partition", round, basis)
			}
		}
		for i := range entries {
			if entries[i] != before[i] {
				t.Fatal("totals mutated their input")
			}
		}
	}
}

func TestTotalsStayExactBeyondTheRecordCeilingAndReturnCopies(t *testing.T) {
	s := scope(t, 1)
	const n = 5000
	entries := make([]expense.Entry, 0, n)
	for i := 0; i < n; i++ {
		entries = append(entries, expense.Entry{Expense: build(t, s, input(t, s, 10_000+i, "1000000000000")), State: expense.Approved})
	}
	got, e := expense.Totals(s, november(t), expense.BasisIncludesPending, entries)
	want, _ := new(big.Int).SetString("5000000000000000", 10)
	if e != nil || got.ApprovedCentavos().Cmp(want) != 0 || got.FigureCentavos().Cmp(want) != 0 || got.ApprovedCount() != n {
		t.Fatal("ceiling amounts were not summed exactly", e)
	}
	got.ApprovedCentavos().SetInt64(1)
	got.FigureCentavos().SetInt64(1)
	got.PendingCentavos().SetInt64(1)
	if got.ApprovedCentavos().Cmp(want) != 0 || got.FigureCentavos().Cmp(want) != 0 || got.PendingCentavos().Sign() != 0 {
		t.Fatal("totals share mutable integers")
	}
}

func TestTotalsAreSafeForConcurrentReaders(t *testing.T) {
	s := scope(t, 1)
	entries := []expense.Entry{
		{Expense: build(t, s, input(t, s, 11, "97350")), State: expense.Approved},
		{Expense: build(t, s, input(t, s, 12, "74200")), State: expense.InReview},
	}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, e := expense.Totals(s, november(t), expense.BasisIncludesPending, entries)
			if e != nil || got.FigureCentavos().String() != "171550" || got.ApprovedCentavos().String() != "97350" {
				t.Error("concurrent totals changed", e)
			}
		}()
	}
	wg.Wait()
}
