package application

import (
	"context"
	"errors"
	"strings"
	"time"

	profiledomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/stationprofile/domain"
)

// CommercePorts isolates the station's own attributed commercial source
// (EMP-STN-002/004, stn.price.publish): the live grant and operator checks,
// the announced price and promotion stores and the public projection.
// Every privilege enforces server-side before any app button.
type CommercePorts struct {
	Grants      DecisionStore
	Prices      CommerceStore
	Promotions  CommerceStore
	OperatorOf  func(ctx context.Context, stationID string) (cnpj string, found bool, err error)
	AccountLive func(ctx context.Context, accountID string) (bool, error)
	Now         func() time.Time
	NewID       func() (string, error)
}

// CommerceStore persists the station's commercial source rows.
type CommerceStore interface {
	InsertAnnouncedPrice(ctx context.Context, stationID, id string, in StoredAnnouncedPrice) (StoredAnnouncedPrice, error)
	WithdrawAnnouncedPrice(ctx context.Context, stationID, priceID string) (StoredAnnouncedPrice, error)
	InsertPromotion(ctx context.Context, stationID, id string, in StoredPromotion) (StoredPromotion, error)
	WithdrawPromotion(ctx context.Context, stationID, promotionID string) (StoredPromotion, error)
}

// StoredAnnouncedPrice is one stored announcement revision.
type StoredAnnouncedPrice struct {
	ID              string
	AuthorAccountID string
	Product         string
	Unit            string
	PriceMilliBRL   int64
	Conditions      string
	ValidFrom       time.Time
	ValidUntil      time.Time
	State           string
}

// StoredPromotion is one stored labelled promotion.
type StoredPromotion struct {
	ID              string
	AuthorAccountID string
	Label           string
	Description     string
	Product         string
	ValidFrom       time.Time
	ValidUntil      time.Time
	State           string
}

var (
	ErrCommerceGrant    = errors.New("stationprofile: no active grant with this scope")
	ErrCommerceStale    = errors.New("stationprofile: operator changed since grant")
	ErrCommerceInvalid  = errors.New("stationprofile: commercial input is invalid")
	ErrCommerceNotFound = errors.New("stationprofile: commercial record not found")
)

// liveCommerceGrant proves the caller's current price.publish scope, live
// account and operator currency, exactly like the profile manage path.
func liveCommerceGrant(ctx context.Context, ports CommercePorts, accountID, stationID string) (GrantRow, error) {
	if accountID == "" || stationID == "" {
		return GrantRow{}, ErrClaimNotFound
	}
	grant, found, err := ports.Grants.ActiveGrant(ctx, accountID, stationID)
	if err != nil {
		return GrantRow{}, err
	}
	if !found {
		return GrantRow{}, ErrCommerceGrant
	}
	live, err := ports.AccountLive(ctx, accountID)
	if err != nil {
		return GrantRow{}, err
	}
	if !live {
		return GrantRow{}, ErrAccountGone
	}
	cnpj, found, err := ports.OperatorOf(ctx, stationID)
	if err != nil {
		return GrantRow{}, err
	}
	if !found || cnpj == "" || cnpj != grant.OperatorCNPJ {
		return GrantRow{}, ErrCommerceStale
	}
	for _, granted := range grant.Scopes {
		if granted == "price.publish" {
			return grant, nil
		}
	}
	return GrantRow{}, ErrCommerceGrant
}

// AnnouncePrice publishes one attributed announcement under the current
// approved scope (UC-STN-01). Supersession of the stored announcement of
// the same product and unit happens in the store's one transaction.
func AnnouncePrice(ctx context.Context, ports CommercePorts, accountID, stationID string, in profiledomain.AnnouncedPrice) (StoredAnnouncedPrice, error) {
	if _, err := liveCommerceGrant(ctx, ports, accountID, stationID); err != nil {
		return StoredAnnouncedPrice{}, err
	}
	if err := in.Validate(); err != nil {
		return StoredAnnouncedPrice{}, err
	}
	id, err := ports.NewID()
	if err != nil {
		return StoredAnnouncedPrice{}, err
	}
	out, err := ports.Prices.InsertAnnouncedPrice(ctx, stationID, id, StoredAnnouncedPrice{
		Product: in.Product, Unit: in.Unit, PriceMilliBRL: in.PriceMilliBRL, AuthorAccountID: accountID,
		Conditions: strings.TrimSpace(in.Conditions), ValidFrom: in.ValidFrom, ValidUntil: in.ValidUntil, State: "ACTIVE",
	})
	if err != nil {
		return StoredAnnouncedPrice{}, err
	}
	return out, nil
}

// WithdrawAnnouncedPrice withdraws one announcement; history stays.
func WithdrawAnnouncedPrice(ctx context.Context, ports CommercePorts, accountID, stationID, priceID string) (StoredAnnouncedPrice, error) {
	if _, err := liveCommerceGrant(ctx, ports, accountID, stationID); err != nil {
		return StoredAnnouncedPrice{}, err
	}
	if priceID == "" {
		return StoredAnnouncedPrice{}, ErrCommerceInvalid
	}
	return ports.Prices.WithdrawAnnouncedPrice(ctx, stationID, priceID)
}

// PublishPromotion publishes one labelled, time-bound promotion.
func PublishPromotion(ctx context.Context, ports CommercePorts, accountID, stationID string, in profiledomain.Promotion) (StoredPromotion, error) {
	if _, err := liveCommerceGrant(ctx, ports, accountID, stationID); err != nil {
		return StoredPromotion{}, err
	}
	if err := in.Validate(); err != nil {
		return StoredPromotion{}, err
	}
	id, err := ports.NewID()
	if err != nil {
		return StoredPromotion{}, err
	}
	out, err := ports.Promotions.InsertPromotion(ctx, stationID, id, StoredPromotion{
		Label: strings.TrimSpace(in.Label), Description: strings.TrimSpace(in.Description), Product: in.Product, AuthorAccountID: accountID,
		ValidFrom: in.ValidFrom, ValidUntil: in.ValidUntil, State: "PUBLISHED",
	})
	if err != nil {
		return StoredPromotion{}, err
	}
	return out, nil
}

// WithdrawPromotion withdraws one promotion; history stays.
func WithdrawPromotion(ctx context.Context, ports CommercePorts, accountID, stationID, promotionID string) (StoredPromotion, error) {
	if _, err := liveCommerceGrant(ctx, ports, accountID, stationID); err != nil {
		return StoredPromotion{}, err
	}
	if promotionID == "" {
		return StoredPromotion{}, ErrCommerceInvalid
	}
	return ports.Promotions.WithdrawPromotion(ctx, stationID, promotionID)
}
