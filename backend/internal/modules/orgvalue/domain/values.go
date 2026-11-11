// Package domain supplies pure enterprise reconciliation/allocation rules.
// Integers, bounds and L/M3 units reuse the existing financial-v1 vocabulary.
package domain

import (
	"errors"
	"math/big"
	"sort"
	"strconv"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/metrics"
)

const MaxAllocationLines = 1000 // Existing bounded driver allocation precedent.
var ErrInvalid = errors.New("orgvalue: invalid-value")

func parse(raw string, minimum, maximum int64) (int64, error) {
	if len(raw) == 0 || len(raw) > 19 || (len(raw) > 1 && raw[0] == '0') {
		return 0, ErrInvalid
	}
	for _, c := range raw {
		if c < '0' || c > '9' {
			return 0, ErrInvalid
		}
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < minimum || n > maximum {
		return 0, ErrInvalid
	}
	return n, nil
}
func ParseCentavos(raw string) (int64, error)      { return parse(raw, 0, metrics.MaxTotalCentavos) }
func ParseQuantityMilli(raw string) (int64, error) { return parse(raw, 1, metrics.MaxQuantityMilli) }
func ParseUnitPriceMilliBRL(raw string) (int64, error) {
	return parse(raw, kernel.MinMilliBRL, kernel.MaxMilliBRL)
}

type Record struct {
	PaidCentavos, QuantityMilli, UnitPriceMilliBRL, DiscountCentavos *int64
	Unit, SourcePriceText                                            string
}
type Reconciliation struct {
	PaidCentavos, GrossCentavos, ExpectedCentavos, DifferenceCentavos *int64
	Unit, SourcePriceText, DerivedPriceMilliBRL, Warning, Unknown     string
}

func valid(n *int64, minimum, maximum int64) bool {
	return n == nil || (*n >= minimum && *n <= maximum)
}
func copyNumber(n *int64) *int64 {
	if n == nil {
		return nil
	}
	value := *n
	return &value
}

// Reconcile checks supplied integer facts. It does not parse/quantize original
// source-price text, submit a record or turn a derived price into recorded fact.
func Reconcile(in Record) (Reconciliation, error) {
	if !valid(in.PaidCentavos, 0, metrics.MaxTotalCentavos) || !valid(in.DiscountCentavos, 0, metrics.MaxTotalCentavos) ||
		!valid(in.QuantityMilli, 1, metrics.MaxQuantityMilli) || !valid(in.UnitPriceMilliBRL, kernel.MinMilliBRL, kernel.MaxMilliBRL) ||
		(in.Unit != string(kernel.Liter) && in.Unit != string(kernel.CubicMetre)) {
		return Reconciliation{}, ErrInvalid
	}
	out := Reconciliation{PaidCentavos: copyNumber(in.PaidCentavos), Unit: in.Unit, SourcePriceText: in.SourcePriceText}
	if in.PaidCentavos != nil && in.DiscountCentavos != nil {
		gross := *in.PaidCentavos + *in.DiscountCentavos // <= 2e12; no int64 overflow.
		out.GrossCentavos = &gross
	}
	if in.QuantityMilli != nil && in.UnitPriceMilliBRL != nil {
		quantity, price := *in.QuantityMilli, *in.UnitPriceMilliBRL
		expected := (quantity*price + 5000) / 10000 // Product <= 1e15.
		out.ExpectedCentavos = &expected
	}
	if in.UnitPriceMilliBRL == nil && in.PaidCentavos != nil && in.QuantityMilli != nil {
		n := new(big.Int).Mul(big.NewInt(*in.PaidCentavos), big.NewInt(10000))
		out.DerivedPriceMilliBRL = new(big.Rat).SetFrac(n, big.NewInt(*in.QuantityMilli)).RatString()
	}
	switch {
	case in.PaidCentavos == nil:
		out.Unknown = "paid-total-missing"
	case in.QuantityMilli == nil:
		out.Unknown = "quantity-missing"
	case in.DiscountCentavos == nil:
		out.Unknown = "discount-missing"
	case in.UnitPriceMilliBRL == nil:
		out.Unknown = "unit-price-missing"
	}
	if out.GrossCentavos != nil && out.ExpectedCentavos != nil {
		difference := *out.GrossCentavos - *out.ExpectedCentavos
		if difference < 0 {
			difference = -difference
		}
		out.DifferenceCentavos = &difference
		if difference > 1 {
			out.Warning = "ARITHMETIC_MISMATCH"
		}
	}
	return out, nil
}

type Line struct {
	Order  int64
	Weight *int64
}
type Part struct{ Order, Centavos int64 }
type Allocation struct {
	Parts   []Part
	Unknown string
}

// Allocate floors exact shares, then distributes residual cents by remainder
// and explicit line order. The personal driver ID tie rule is independent.
func Allocate(source *int64, lines []Line) (Allocation, error) {
	if !valid(source, 0, metrics.MaxTotalCentavos) || len(lines) > MaxAllocationLines {
		return Allocation{}, ErrInvalid
	}
	total := new(big.Int)
	seen := make(map[int64]bool, len(lines))
	missing := false
	for _, line := range lines {
		if line.Order < 0 || seen[line.Order] || (line.Weight != nil && *line.Weight < 0) {
			return Allocation{}, ErrInvalid
		}
		seen[line.Order] = true
		if line.Weight == nil {
			missing = true
		} else {
			total.Add(total, big.NewInt(*line.Weight))
		}
	}
	if source == nil {
		return Allocation{Unknown: "source-amount-missing"}, nil
	}
	if missing || total.Sign() == 0 {
		return Allocation{Unknown: "allocation-basis-missing"}, nil
	}
	type share struct {
		Part
		remainder *big.Int
	}
	shares := make([]share, 0, len(lines))
	var allocated int64
	for _, line := range lines {
		n := new(big.Int).Mul(big.NewInt(*source), big.NewInt(*line.Weight))
		q, r := new(big.Int), new(big.Int)
		q.QuoRem(n, total, r)
		// Each nonnegative quotient is <= bounded source (1e12).
		shares = append(shares, share{Part{line.Order, q.Int64()}, r})
		allocated += q.Int64()
	}
	sort.Slice(shares, func(i, j int) bool {
		if order := shares[i].remainder.Cmp(shares[j].remainder); order != 0 {
			return order > 0
		}
		return shares[i].Order < shares[j].Order
	})
	// Residual < positive-weight line count, so each receives at most one.
	for i := int64(0); i < *source-allocated; i++ {
		shares[i].Centavos++
	}
	sort.Slice(shares, func(i, j int) bool { return shares[i].Order < shares[j].Order })
	out := Allocation{Parts: make([]Part, len(shares))}
	for i, share := range shares {
		out.Parts[i] = share.Part
	}
	return out, nil
}
