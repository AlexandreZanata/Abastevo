package domain

import (
	"errors"
	"time"
)

// EMP-STN-002/004 (BR-STN-001/002, EDR-022): announced prices and labelled
// promotions are the station's own attributed commercial source. State
// names follow the station-commerce state catalog; expiry applies at read
// time and neither source enters official or community consensus.
var (
	ErrAnnouncedWindowInvalid     = errors.New("stationcommerce: announced window is invalid")
	ErrAnnouncedPriceInvalid      = errors.New("stationcommerce: announced price is invalid")
	ErrAnnouncedProductInvalid    = errors.New("stationcommerce: announced product or unit is invalid")
	ErrAnnouncedConditionsInvalid = errors.New("stationcommerce: announced conditions are invalid")
	ErrPromotionWindowInvalid     = errors.New("stationcommerce: promotion window is invalid")
	ErrPromotionTextInvalid       = errors.New("stationcommerce: promotion label or description is invalid")
)

var commerceProducts = map[string]bool{
	"ETHANOL": true, "GASOLINE_REGULAR": true, "GASOLINE_ADDITIVED": true,
	"DIESEL_S500": true, "DIESEL_S10": true, "CNG": true, "LPG_P13": true,
}

var commerceUnits = map[string]bool{"L": true, "M3": true, "KG_13": true}

// ValidCommerceProduct reports whether the closed commercial vocabulary
// admits the product and unit pair.
func ValidCommerceProduct(product, unit string) bool {
	return commerceProducts[product] && commerceUnits[unit]
}

// AnnouncedPriceState is the stored announced-price state; EXPIRED is
// derived at read time.
type AnnouncedPriceState string

const (
	AnnouncedActive     AnnouncedPriceState = "ACTIVE"
	AnnouncedWithdrawn  AnnouncedPriceState = "WITHDRAWN"
	AnnouncedSuperseded AnnouncedPriceState = "SUPERSEDED"
	// AnnouncedPending and AnnouncedExpired derive at read time.
	AnnouncedPending AnnouncedPriceState = "PENDING"
	AnnouncedExpired AnnouncedPriceState = "EXPIRED"
)

// AnnouncedPrice is one published price revision with its explicit
// validity (EMP-STN-002). Fields are immutable after publication; only
// withdrawal and supersession move the stored state.
type AnnouncedPrice struct {
	Product       string
	Unit          string
	PriceMilliBRL int64
	Conditions    string
	ValidFrom     time.Time
	ValidUntil    time.Time
}

// Validate enforces the closed product, unit, price and condition bounds.
func (p AnnouncedPrice) Validate() error {
	if !ValidCommerceProduct(p.Product, p.Unit) {
		return ErrAnnouncedProductInvalid
	}
	if p.PriceMilliBRL < 1 || p.PriceMilliBRL > 1000000 {
		return ErrAnnouncedPriceInvalid
	}
	if p.ValidFrom.IsZero() || p.ValidUntil.IsZero() || !p.ValidUntil.After(p.ValidFrom) {
		return ErrAnnouncedWindowInvalid
	}
	if len([]rune(p.Conditions)) > 120 {
		return ErrAnnouncedConditionsInvalid
	}
	return nil
}

// AnnouncedDisplayState is the read-time display state: ACTIVE inside the
// window, PENDING before it, EXPIRED after it; the stored WITHDRAWN and
// SUPERSEDED states stay as recorded (EMP-STN-003: every displayed price
// carries source, condition and age).
func AnnouncedDisplayState(state AnnouncedPriceState, now, from, until time.Time) AnnouncedPriceState {
	switch state {
	case AnnouncedWithdrawn, AnnouncedSuperseded:
		return state
	}
	switch {
	case now.Before(from):
		return AnnouncedPending
	case now.After(until):
		return AnnouncedExpired
	default:
		return AnnouncedActive
	}
}

// PromotionState is the stored promotion state; SCHEDULED, ACTIVE and
// ENDED derive at read time, WITHDRAWN is the only persisted exit.
type PromotionState string

const (
	PromotionPublished PromotionState = "PUBLISHED"
	PromotionWithdrawn PromotionState = "WITHDRAWN"
	// PromotionScheduled, PromotionActive and PromotionEnded derive at read.
	PromotionScheduled PromotionState = "SCHEDULED"
	PromotionActive    PromotionState = "ACTIVE"
	PromotionEnded     PromotionState = "ENDED"
)

// Promotion is one labelled time-bound offer (EMP-STN-004): the owner and
// conditions are visible and expiry applies at read time.
type Promotion struct {
	Label       string
	Description string
	Product     string
	ValidFrom   time.Time
	ValidUntil  time.Time
}

// Validate enforces the label, description, optional product and window.
func (p Promotion) Validate() error {
	if len([]rune(p.Label)) < 1 || len([]rune(p.Label)) > 60 || len([]rune(p.Description)) < 1 || len([]rune(p.Description)) > 280 {
		return ErrPromotionTextInvalid
	}
	if p.Product != "" && !commerceProducts[p.Product] {
		return ErrAnnouncedProductInvalid
	}
	if p.ValidFrom.IsZero() || p.ValidUntil.IsZero() || !p.ValidUntil.After(p.ValidFrom) {
		return ErrPromotionWindowInvalid
	}
	return nil
}

// PromotionDisplayState is the read-time display state.
func PromotionDisplayState(state PromotionState, now, from, until time.Time) PromotionState {
	if state == PromotionWithdrawn {
		return PromotionWithdrawn
	}
	switch {
	case now.Before(from):
		return PromotionScheduled
	case now.After(until):
		return PromotionEnded
	default:
		return PromotionActive
	}
}

// CommerceErrorCode is the frozen golden-vector code of a commerce refusal.
func CommerceErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrAnnouncedWindowInvalid):
		return "ANNOUNCED_WINDOW_INVALID"
	case errors.Is(err, ErrAnnouncedPriceInvalid):
		return "ANNOUNCED_PRICE_INVALID"
	case errors.Is(err, ErrAnnouncedProductInvalid):
		return "ANNOUNCED_PRODUCT_INVALID"
	case errors.Is(err, ErrAnnouncedConditionsInvalid):
		return "ANNOUNCED_CONDITIONS_INVALID"
	case errors.Is(err, ErrPromotionWindowInvalid):
		return "PROMOTION_WINDOW_INVALID"
	case errors.Is(err, ErrPromotionTextInvalid):
		return "PROMOTION_TEXT_INVALID"
	}
	return ""
}
