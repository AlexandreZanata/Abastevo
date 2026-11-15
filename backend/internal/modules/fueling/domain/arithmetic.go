package domain

import (
	"strconv"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	exact "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgvalue/domain"
)

// Arithmetic is the BR-MONEY-005 reconciliation of one accepted candidate
// (EMP-FUEL-002, B-BR-EMP-264). It informs the reviewer; it never replaces
// the paid total, which stays the entered fact (BR-MONEY-004).
type Arithmetic struct {
	redacted
	// Gross is paid plus discount; an absent discount reconciles as zero.
	Gross int64
	// Expected and Difference exist exactly when a unit price was given.
	Expected, Difference *int64
	// Derived is the exact milli-BRL rational paid×10000/quantity, present
	// exactly when no unit price was given; it is never a recorded price.
	Derived string
	// Warning is "ARITHMETIC_MISMATCH" when Difference exceeds one centavo.
	Warning string
}

// ArithmeticMismatch is the BR-MONEY-005 warning code.
const ArithmeticMismatch = "ARITHMETIC_MISMATCH"

// Reconcile applies the accepted orgvalue rule to a validated candidate. An
// absent discount reconciles as zero (fuelings §10.2 "with no discount") and
// stays absent in the candidate. orgvalue checks only the financial-v1 L/M3
// tokens; BR-MONEY-005 is the same integer formula for every product unit, so
// KWH reconciles under that profile and keeps its own unit (EDR-176).
func Reconcile(c Candidate) (Arithmetic, error) {
	in := c.input
	if !in.Amounts.valid {
		return Arithmetic{}, ErrInvalid
	}
	quantity, err := NewQuantity(in.Quantity.product.code, in.Quantity.product.unit, strconv.FormatInt(in.Quantity.amount, 10))
	if err != nil {
		return Arithmetic{}, ErrInvalid
	}
	amounts := in.Amounts.Snapshot()
	discount := int64(0)
	if amounts.Discount != nil {
		discount = *amounts.Discount
	}
	amount := quantity.Amount()
	profile := string(quantity.Product().Unit())
	if quantity.Product().Unit() == fleet.KilowattHour {
		profile = string(kernel.Liter)
	}
	out, err := exact.Reconcile(exact.Record{PaidCentavos: &amounts.Paid, QuantityMilli: &amount, UnitPriceMilliBRL: amounts.UnitPrice, DiscountCentavos: &discount, Unit: profile})
	if err != nil || out.GrossCentavos == nil || out.PaidCentavos == nil || *out.PaidCentavos != amounts.Paid {
		return Arithmetic{}, ErrInvalid
	}
	result := Arithmetic{Gross: *out.GrossCentavos, Expected: copyPointer(out.ExpectedCentavos), Difference: copyPointer(out.DifferenceCentavos), Derived: out.DerivedPriceMilliBRL, Warning: out.Warning}
	if (amounts.UnitPrice == nil) != (result.Expected == nil) || (result.Expected == nil) != (result.Difference == nil) || (amounts.UnitPrice == nil) == (result.Derived == "") || result.Warning != "" && result.Warning != ArithmeticMismatch {
		return Arithmetic{}, ErrInvalid
	}
	return result, nil
}
