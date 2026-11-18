// Package domain owns internal fuel stock pools, moving-average valuation,
// transfers, reconciliations, and the single-cost invariant between stock
// purchase and asset consumption (EMP-STOCK-001, EMP-STOCK-005, BR-STOCK-001..004,
// EDR-029, VAL-32).
package domain

import (
	"errors"
	"math/big"
	"strings"
)

var (
	ErrInvalidInput          = errors.New("fuelstock: validation failed")
	ErrInsufficientStock     = errors.New("fuelstock: insufficient stock balance")
	ErrProductMismatch       = errors.New("fuelstock: product or unit mismatch")
	ErrNegativeQuantityError = errors.New("fuelstock: quantity must be positive")
)

const (
	MaxQuantityMilli = int64(1_000_000_000)
	MaxValueCentavos = int64(1_000_000_000_000)
)

// Pool holds exact integer stock quantity and accumulated valuation in centavos
// for a single compatible product and measurement unit.
type Pool struct {
	Product       string
	Unit          string
	QuantityMilli int64
	ValueCentavos int64
	AllowNegative bool
}

// ValuationOutcome describes the state transition of a stock movement plus any
// recognized consumption cost (always 0 for receipts/transfers, equal to the
// moving-average slice for asset dispenses).
type ValuationOutcome struct {
	MovedCentavos             int64
	AfterQuantityMilli        int64
	AfterValueCentavos        int64
	RecognizedExpenseCentavos int64
}

// RoundHalfUpScaled computes floor((qty * value + den/2) / den) using arbitrary-precision
// integer arithmetic so no float drift or int64 overflow occurs.
func RoundHalfUpScaled(numerator, denominator int64) (int64, error) {
	if denominator <= 0 || numerator < 0 {
		return 0, ErrInvalidInput
	}
	num := big.NewInt(numerator)
	den := big.NewInt(denominator)
	half := new(big.Int).Rsh(den, 1)
	sum := new(big.Int).Add(num, half)
	q := new(big.Int).Quo(sum, den)
	if !q.IsInt64() {
		return 0, ErrInvalidInput
	}
	return q.Int64(), nil
}

// ProportionalCost computes the moving-average centavos slice for outgoingMilli
// out of poolQtyMilli worth poolValueCentavos, conserving source value exact to the centavo.
func ProportionalCost(poolQtyMilli, poolValueCentavos, outgoingMilli int64) (int64, error) {
	if poolQtyMilli <= 0 || poolValueCentavos < 0 || outgoingMilli <= 0 || outgoingMilli > poolQtyMilli {
		return 0, ErrInvalidInput
	}
	if outgoingMilli == poolQtyMilli {
		return poolValueCentavos, nil
	}
	num := new(big.Int).Mul(big.NewInt(outgoingMilli), big.NewInt(poolValueCentavos))
	den := big.NewInt(poolQtyMilli)
	half := new(big.Int).Rsh(den, 1)
	sum := new(big.Int).Add(num, half)
	q := new(big.Int).Quo(sum, den)
	if !q.IsInt64() {
		return 0, ErrInvalidInput
	}
	cost := q.Int64()
	if cost > poolValueCentavos {
		cost = poolValueCentavos
	}
	return cost, nil
}

// Receive adds purchased stock of matching product into the pool without
// recognizing an operating expense (EDR-029, VAL-32).
func (p Pool) Receive(product string, qtyMilli, paidCentavos int64) (Pool, ValuationOutcome, error) {
	if strings.TrimSpace(product) == "" || product != p.Product {
		return p, ValuationOutcome{}, ErrProductMismatch
	}
	if qtyMilli <= 0 || qtyMilli > MaxQuantityMilli || paidCentavos < 0 || paidCentavos > MaxValueCentavos {
		return p, ValuationOutcome{}, ErrInvalidInput
	}
	if p.QuantityMilli < 0 {
		// Rebuilding above overdraft requires explicit reconciliation; still preserve exact arithmetic.
		return p, ValuationOutcome{}, ErrInvalidInput
	}
	nextQty := p.QuantityMilli + qtyMilli
	nextVal := p.ValueCentavos + paidCentavos
	if nextQty > MaxQuantityMilli || nextVal > MaxValueCentavos {
		return p, ValuationOutcome{}, ErrInvalidInput
	}
	next := p
	next.QuantityMilli = nextQty
	next.ValueCentavos = nextVal
	return next, ValuationOutcome{
		MovedCentavos:             paidCentavos,
		AfterQuantityMilli:        nextQty,
		AfterValueCentavos:        nextVal,
		RecognizedExpenseCentavos: 0,
	}, nil
}

// Dispense issues fuel from the pool to an asset at exact moving-average cost,
// recognizing that cost once at consumption (VAL-32).
func (p Pool) Dispense(product string, qtyMilli int64) (Pool, ValuationOutcome, error) {
	if strings.TrimSpace(product) == "" || product != p.Product {
		return p, ValuationOutcome{}, ErrProductMismatch
	}
	if qtyMilli <= 0 || qtyMilli > MaxQuantityMilli {
		return p, ValuationOutcome{}, ErrNegativeQuantityError
	}
	if !p.AllowNegative && qtyMilli > p.QuantityMilli {
		return p, ValuationOutcome{}, ErrInsufficientStock
	}
	if p.QuantityMilli <= 0 {
		// Unapproved overdraft without valuation cannot fabricate zero cost unless AllowNegative is explicitly configured.
		return p, ValuationOutcome{}, ErrInsufficientStock
	}
	var movedCentavos int64
	if qtyMilli <= p.QuantityMilli {
		c, err := ProportionalCost(p.QuantityMilli, p.ValueCentavos, qtyMilli)
		if err != nil {
			return p, ValuationOutcome{}, err
		}
		movedCentavos = c
	} else {
		// AllowNegative overdraft uses entire remaining value; negative remainder holds 0 centavos.
		movedCentavos = p.ValueCentavos
	}
	next := p
	next.QuantityMilli = p.QuantityMilli - qtyMilli
	next.ValueCentavos = p.ValueCentavos - movedCentavos
	return next, ValuationOutcome{
		MovedCentavos:             movedCentavos,
		AfterQuantityMilli:        next.QuantityMilli,
		AfterValueCentavos:        next.ValueCentavos,
		RecognizedExpenseCentavos: movedCentavos,
	}, nil
}

// Transfer moves stock between two compatible pools, conserving total quantity and value
// and recognizing zero operating expense (EMP-STOCK-004).
func Transfer(src, dst Pool, product string, qtyMilli int64) (Pool, Pool, ValuationOutcome, error) {
	if src.Product != dst.Product || src.Unit != dst.Unit || product != src.Product {
		return src, dst, ValuationOutcome{}, ErrProductMismatch
	}
	if qtyMilli <= 0 || src.QuantityMilli <= 0 || qtyMilli > src.QuantityMilli {
		return src, dst, ValuationOutcome{}, ErrInsufficientStock
	}
	movedCentavos, err := ProportionalCost(src.QuantityMilli, src.ValueCentavos, qtyMilli)
	if err != nil {
		return src, dst, ValuationOutcome{}, err
	}
	if dst.QuantityMilli+qtyMilli > MaxQuantityMilli || dst.ValueCentavos+movedCentavos > MaxValueCentavos {
		return src, dst, ValuationOutcome{}, ErrInvalidInput
	}
	nextSrc := src
	nextSrc.QuantityMilli -= qtyMilli
	nextSrc.ValueCentavos -= movedCentavos
	nextDst := dst
	nextDst.QuantityMilli += qtyMilli
	nextDst.ValueCentavos += movedCentavos
	return nextSrc, nextDst, ValuationOutcome{
		MovedCentavos:             movedCentavos,
		AfterQuantityMilli:        nextSrc.QuantityMilli,
		AfterValueCentavos:        nextSrc.ValueCentavos,
		RecognizedExpenseCentavos: 0,
	}, nil
}

// Adjust applies an explicit, reasoned quantity adjustment without mutating prior movements.
func (p Pool) Adjust(deltaMilli int64, reason string) (Pool, ValuationOutcome, error) {
	if deltaMilli == 0 || strings.TrimSpace(reason) == "" || len([]rune(reason)) > 500 {
		return p, ValuationOutcome{}, ErrInvalidInput
	}
	nextQty := p.QuantityMilli + deltaMilli
	if !p.AllowNegative && nextQty < 0 {
		return p, ValuationOutcome{}, ErrInsufficientStock
	}
	nextVal := p.ValueCentavos
	movedCentavos := int64(0)
	if deltaMilli < 0 && p.QuantityMilli > 0 && p.ValueCentavos > 0 {
		out := -deltaMilli
		if out > p.QuantityMilli {
			out = p.QuantityMilli
		}
		c, err := ProportionalCost(p.QuantityMilli, p.ValueCentavos, out)
		if err != nil {
			return p, ValuationOutcome{}, err
		}
		movedCentavos = c
		nextVal -= c
	}
	next := p
	next.QuantityMilli = nextQty
	next.ValueCentavos = nextVal
	return next, ValuationOutcome{
		MovedCentavos:             movedCentavos,
		AfterQuantityMilli:        nextQty,
		AfterValueCentavos:        nextVal,
		RecognizedExpenseCentavos: 0,
	}, nil
}
