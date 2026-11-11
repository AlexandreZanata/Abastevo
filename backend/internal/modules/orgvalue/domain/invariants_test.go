package domain_test

import (
	"errors"
	"math"
	"reflect"
	"testing"

	value "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgvalue/domain"
)

func TestAllocationPreservesSourceAndExplicitOrderWithoutChangingInputs(t *testing.T) {
	for source := int64(0); source <= 101; source++ {
		for _, weights := range [][]int64{{1, 1, 1}, {1, 2, 4}, {0, math.MaxInt64, math.MaxInt64}} {
			lines := make([]value.Line, len(weights))
			for i := range weights {
				lines[i] = value.Line{Order: int64(i), Weight: &weights[i]}
			}
			before := append([]value.Line(nil), lines...)
			allocated, err := value.Allocate(&source, lines)
			if err != nil {
				t.Fatal(err)
			}
			var sum int64
			for i, part := range allocated.Parts {
				if part.Order != int64(i) || part.Centavos < 0 || part.Centavos > source {
					t.Fatal("invalid allocated part")
				}
				sum += part.Centavos
			}
			if sum != source || !reflect.DeepEqual(before, lines) {
				t.Fatal("source changed or total lost")
			}
			reversed := []value.Line{lines[2], lines[1], lines[0]}
			again, err := value.Allocate(&source, reversed)
			if err != nil || !reflect.DeepEqual(again, allocated) {
				t.Fatal("input order changed allocation", err)
			}
		}
	}
}
func TestAllocationBudgetAndUnknownAreNotZero(t *testing.T) {
	one, thousand := int64(1), int64(1000)
	lines := make([]value.Line, value.MaxAllocationLines)
	for i := range lines {
		lines[i] = value.Line{Order: int64(i), Weight: &one}
	}
	got, err := value.Allocate(&thousand, lines)
	if err != nil || len(got.Parts) != 1000 {
		t.Fatal(err)
	}
	for _, part := range got.Parts {
		if part.Centavos != 1 {
			t.Fatal("budget allocation lost cents")
		}
	}
	if _, err := value.Allocate(&one, append(lines, value.Line{Order: 1000, Weight: &one})); !errors.Is(err, value.ErrInvalid) {
		t.Fatal(err)
	}
	unknown, _ := value.Allocate(nil, lines)
	zero := int64(0)
	known, err := value.Allocate(&zero, lines)
	if err != nil || reflect.DeepEqual(known, unknown) || unknown.Parts != nil {
		t.Fatal("unknown became zero allocation")
	}
}
func TestReconciliationPreservesPaidSnapshotAndOriginalText(t *testing.T) {
	paid, quantity, price, discount := int64(97000), int64(150000), int64(6490), int64(0)
	input := value.Record{PaidCentavos: &paid, QuantityMilli: &quantity, UnitPriceMilliBRL: &price, DiscountCentavos: &discount, Unit: "L", SourcePriceText: "6,490123000"}
	got, err := value.Reconcile(input)
	if err != nil || *got.PaidCentavos != paid || *got.ExpectedCentavos != 97350 || got.Warning != "ARITHMETIC_MISMATCH" || got.SourcePriceText != input.SourcePriceText {
		t.Fatal("paid fact or precision changed", err)
	}
	if paid != 97000 {
		t.Fatal("input was overwritten")
	}
	paid = 1
	if *got.PaidCentavos != 97000 {
		t.Fatal("returned paid snapshot aliases input")
	}
}
