package domain

import (
	"errors"
	"time"
)

var (
	ErrAuthorizationExpiryInvalid     = errors.New("authorization: expiry is invalid")
	ErrAuthorizationLimitsInvalid     = errors.New("authorization: limits are invalid")
	ErrAuthorizationProductInvalid    = errors.New("authorization: product or unit is invalid")
	ErrAuthorizationTransitionRefused = errors.New("authorization: transition refused")
	ErrAuthorizationExpired           = errors.New("authorization: expired")
	ErrAuthorizationProductMismatch   = errors.New("authorization: product or unit does not match")
	ErrAuthorizationQuantityExceeded  = errors.New("authorization: quantity exceeds the limit")
	ErrAuthorizationTotalExceeded     = errors.New("authorization: total exceeds the limit")
	ErrAuthorizationLocked            = errors.New("authorization: locked after bounded attempts")
)

// MaxFailedAttempts bounds the invalid redemption tries before the voucher
// locks (BR-TEN-005, mirroring the frozen proof attempt bound).
const MaxFailedAttempts = 5

// VoucherState is the stored authorization state (state-model.md); EXPIRED
// is derived at read time.
type VoucherState string

const (
	Issued    VoucherState = "ISSUED"
	Redeemed  VoucherState = "REDEEMED"
	Cancelled VoucherState = "CANCELLED"
	Locked    VoucherState = "LOCKED"
	// ExpiredVoucher is derived; it is never stored.
	ExpiredVoucher VoucherState = "EXPIRED"
)

// Voucher is one bounded, expiring, single-use fueling authorization
// (EMP-AGREE-004). The voucher code itself never enters this domain: the
// adapters hold only its digest.
type Voucher struct {
	state            VoucherState
	product, unit    string
	maxQuantityMilli int64
	maxCentavos      int64
	validUntil       time.Time
	failedAttempts   int
}

// Issue validates the documented bounds and opens the ISSUED voucher.
func Issue(product, unit string, maxQuantityMilli, maxCentavos int64, validUntil, now time.Time) (Voucher, error) {
	if !ValidProduct(product, unit) {
		return Voucher{}, ErrAuthorizationProductInvalid
	}
	if maxQuantityMilli < 1 || maxCentavos < 1 {
		return Voucher{}, ErrAuthorizationLimitsInvalid
	}
	if validUntil.IsZero() || !validUntil.After(now) {
		return Voucher{}, ErrAuthorizationExpiryInvalid
	}
	return Voucher{state: Issued, product: product, unit: unit, maxQuantityMilli: maxQuantityMilli, maxCentavos: maxCentavos, validUntil: validUntil}, nil
}

func (v Voucher) State() VoucherState     { return v.state }
func (v Voucher) Product() string         { return v.product }
func (v Voucher) Unit() string            { return v.unit }
func (v Voucher) MaxQuantityMilli() int64 { return v.maxQuantityMilli }
func (v Voucher) MaxCentavos() int64      { return v.maxCentavos }
func (v Voucher) ValidUntil() time.Time   { return v.validUntil }
func (v Voucher) FailedAttempts() int     { return v.failedAttempts }

// Cancel ends an issued voucher; a consumed or locked one is refused.
func (v Voucher) Cancel() (Voucher, error) {
	if v.state != Issued {
		return Voucher{}, ErrAuthorizationTransitionRefused
	}
	return Voucher{state: Cancelled, product: v.product, unit: v.unit, maxQuantityMilli: v.maxQuantityMilli, maxCentavos: v.maxCentavos, validUntil: v.validUntil, failedAttempts: v.failedAttempts}, nil
}

// DerivedState is the read-time state: an ISSUED voucher past its expiry
// reads EXPIRED; every stored terminal state stays as recorded.
func (v Voucher) DerivedState(now time.Time) VoucherState {
	if v.state == Issued && !v.validUntil.IsZero() && now.After(v.validUntil) {
		return ExpiredVoucher
	}
	return v.state
}

// EligibleAt reports whether the voucher may be consumed at the instant.
func (v Voucher) EligibleAt(now time.Time) (bool, VoucherState) {
	state := v.DerivedState(now)
	return state == Issued, state
}

// Attempt records one invalid redemption try. Five bounded failures lock
// the voucher; a locked or terminal voucher never counts again.
func (v Voucher) Attempt() (Voucher, error) {
	if v.state != Issued {
		return Voucher{}, ErrAuthorizationTransitionRefused
	}
	next := v.failedAttempts + 1
	if next >= MaxFailedAttempts {
		return Voucher{state: Locked, product: v.product, unit: v.unit, maxQuantityMilli: v.maxQuantityMilli, maxCentavos: v.maxCentavos, validUntil: v.validUntil, failedAttempts: next}, nil
	}
	return Voucher{state: Issued, product: v.product, unit: v.unit, maxQuantityMilli: v.maxQuantityMilli, maxCentavos: v.maxCentavos, validUntil: v.validUntil, failedAttempts: next}, nil
}

// TotalCentavos derives the settlement total from the negotiated price:
// quantity in milli-units times price in milli-BRL rounded half-up to
// centavos (BR-MONEY).
func TotalCentavos(quantityMilli, priceMilliBRL int64) int64 {
	return (quantityMilli*priceMilliBRL + 500) / 1000
}

// VoucherOption rebuilds one stored voucher head (the adapters' read and
// confirm paths); it never bypasses the consume rules.
type VoucherOption func(*Voucher)

// VoucherOf is the stored-head constructor.
func VoucherOf(state VoucherState, opts ...VoucherOption) Voucher {
	v := Voucher{state: state}
	for _, opt := range opts {
		opt(&v)
	}
	return v
}

// VoucherProduct sets the recorded product and unit.
func VoucherProduct(product, unit string) VoucherOption {
	return func(v *Voucher) { v.product, v.unit = product, unit }
}

// VoucherLimits sets the recorded quantity and total bounds.
func VoucherLimits(maxQuantityMilli, maxCentavos int64) VoucherOption {
	return func(v *Voucher) { v.maxQuantityMilli, v.maxCentavos = maxQuantityMilli, maxCentavos }
}

// VoucherValidUntil sets the recorded expiry.
func VoucherValidUntil(validUntil time.Time) VoucherOption {
	return func(v *Voucher) { v.validUntil = validUntil }
}

// VoucherAttempts sets the recorded failed attempts.
func VoucherAttempts(failedAttempts int) VoucherOption {
	return func(v *Voucher) { v.failedAttempts = failedAttempts }
}

// Consume applies the redemption rules at the instant under the recorded
// limits (EMP-AGREE-005). Matching retries are answered by the stored
// receipt in the adapters; this pure check decides one consume.
func (v Voucher) Consume(product, unit string, quantityMilli, totalCentavos int64, now time.Time) error {
	switch v.DerivedState(now) {
	case Locked:
		return ErrAuthorizationLocked
	case ExpiredVoucher:
		return ErrAuthorizationExpired
	case Cancelled, Redeemed:
		return ErrAuthorizationTransitionRefused
	}
	if product != v.product || unit != v.unit {
		return ErrAuthorizationProductMismatch
	}
	if quantityMilli < 1 || quantityMilli > v.maxQuantityMilli {
		return ErrAuthorizationQuantityExceeded
	}
	if totalCentavos < 1 || totalCentavos > v.maxCentavos {
		return ErrAuthorizationTotalExceeded
	}
	return nil
}
