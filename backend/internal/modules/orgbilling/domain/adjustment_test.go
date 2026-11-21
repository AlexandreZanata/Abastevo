package domain

import (
	"testing"

	payments "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
)

func TestReviewedAdjustmentUsesExactPositiveBRLWithARegisteredDirection(t *testing.T) {
	for _, tc := range []struct {
		kind   AdjustmentKind
		amount payments.Money
		want   int64
		ok     bool
	}{
		{ServiceCredit, payments.BRL(1099), -1099, true},
		{DebitCorrection, payments.BRL(1099), 1099, true},
		{DebitCorrection, payments.BRL(payments.MaxAmountCentavos), payments.MaxAmountCentavos, true},
		{ServiceCredit, payments.BRL(0), 0, false},
		{ServiceCredit, payments.BRL(-1), 0, false},
		{ServiceCredit, payments.BRL(payments.MaxAmountCentavos + 1), 0, false},
		{ServiceCredit, payments.Money{Centavos: 1099, Currency: "USD"}, 0, false},
		{AdjustmentKind("SETTLE"), payments.BRL(1099), 0, false},
	} {
		delta, err := AdjustmentDelta(tc.kind, tc.amount)
		if (err == nil) != tc.ok || delta != tc.want {
			t.Fatalf("adjustment %q got %d / %v, want %d / %v", tc.kind, delta, err, tc.want, tc.ok)
		}
	}
}
