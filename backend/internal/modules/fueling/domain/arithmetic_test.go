package domain_test

import (
	"testing"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	fuel "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/domain"
)

func ptr(v string) *string { return &v }

// B-BR-EMP-264 on the fuelings §10.2 and BR-MONEY-005 worked examples: the
// paid total is never recomputed and a mismatch is only a warning.
func TestReconcileFollowsTheWorkedExamplesAndKeepsThePaidTotal(t *testing.T) {
	s := scope(t, 1)
	type want struct {
		gross               int64
		expected, different *int64
		derived, warning    string
	}
	n := func(v int64) *int64 { return &v }
	cases := []struct {
		name                string
		product             fuel.ProductCode
		unit                fleet.Unit
		quantity, paid      string
		discount, unitPrice *string
		want                want
	}{
		{"exact", fuel.DieselS10, fleet.Liter, "37415", "22071", nil, ptr("5899"), want{22071, n(22071), n(0), "", ""}},
		{"one-centavo-tolerance", fuel.DieselS10, fleet.Liter, "37415", "22072", nil, ptr("5899"), want{22072, n(22071), n(1), "", ""}},
		{"discount-restores-gross", fuel.DieselS10, fleet.Liter, "37415", "21571", ptr("500"), ptr("5899"), want{22071, n(22071), n(0), "", ""}},
		{"mismatch-without-discount", fuel.DieselS10, fleet.Liter, "37415", "22100", nil, ptr("5899"), want{22100, n(22071), n(29), "", fuel.ArithmeticMismatch}},
		{"rule-example-mismatch", fuel.DieselS10, fleet.Liter, "150000", "97000", nil, ptr("6490"), want{97000, n(97350), n(350), "", fuel.ArithmeticMismatch}},
		{"two-centavos-below", fuel.DieselS10, fleet.Liter, "150000", "97348", ptr("0"), ptr("6490"), want{97348, n(97350), n(2), "", fuel.ArithmeticMismatch}},
		{"kilowatt-hours-same-formula", fuel.Electricity, fleet.KilowattHour, "30000", "5850", nil, ptr("1950"), want{5850, n(5850), n(0), "", ""}},
		{"kilowatt-hours-mismatch", fuel.Electricity, fleet.KilowattHour, "30000", "5900", nil, ptr("1950"), want{5900, n(5850), n(50), "", fuel.ArithmeticMismatch}},
		{"cubic-metres", fuel.CNG, fleet.CubicMetre, "1000", "650", nil, ptr("6495"), want{650, n(650), n(0), "", ""}},
		{"derived-price-only", fuel.DieselS10, fleet.Liter, "40000", "23600", nil, nil, want{23600, nil, nil, "5900", ""}},
		{"derived-price-rational", fuel.DieselS10, fleet.Liter, "3", "1", nil, nil, want{1, nil, nil, "10000/3", ""}},
		{"zero-paid-is-a-fact", fuel.ARLA32, fleet.Liter, "1000", "0", nil, nil, want{0, nil, nil, "0", ""}},
		{"widened-gross", fuel.DieselS10, fleet.Liter, "1000000000", "1000000000000", ptr("1000000000000"), ptr("1000000"), want{2000000000000, n(100000000000), n(1900000000000), "", fuel.ArithmeticMismatch}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := candidateInput(t, s)
			q, err := fuel.NewQuantity(tc.product, tc.unit, tc.quantity)
			if err != nil {
				t.Fatal(err)
			}
			amounts, err := fuel.NewAmounts(tc.paid, tc.discount, tc.unitPrice, nil)
			if err != nil {
				t.Fatal(err)
			}
			in.Quantity, in.Amounts = q, amounts
			c, err := fuel.NewCandidate(s, in)
			if err != nil {
				t.Fatal(err)
			}
			got, err := fuel.Reconcile(c)
			if err != nil {
				t.Fatal(err)
			}
			if got.Gross != tc.want.gross || !sameNumber(got.Expected, tc.want.expected) || !sameNumber(got.Difference, tc.want.different) || got.Derived != tc.want.derived || got.Warning != tc.want.warning {
				t.Fatalf("got gross=%d expected=%v difference=%v derived=%q warning=%q", got.Gross, deref(got.Expected), deref(got.Difference), got.Derived, got.Warning)
			}
			// BR-MONEY-004: the candidate's paid total is untouched.
			if c.Snapshot().Amounts.Snapshot().Paid != mustPaid(t, tc.paid) {
				t.Fatal("paid total recomputed")
			}
		})
	}
	if _, err := fuel.Reconcile(fuel.Candidate{}); err == nil {
		t.Fatal("zero candidate reconciled")
	}
}

func sameNumber(a, b *int64) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }
func deref(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}
func mustPaid(t *testing.T, raw string) int64 {
	t.Helper()
	a, err := fuel.NewAmounts(raw, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return a.Snapshot().Paid
}
