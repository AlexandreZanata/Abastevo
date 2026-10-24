// Package domain owns provider-neutral payment and access policy.
//
// It performs no network, SQL, provider or clock I/O. Callers pass authoritative
// facts and time; this package decides whether an offer may be sold, which paid
// period a settlement funds, whether a refund/dispute changes access and whether
// a funding-source switch is safe. Money is exact integer centavos.
package domain

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	PolicyVersion = 1

	CurrencyBRL     = "BRL"
	BillingTimezone = "America/Sao_Paulo"
	QuoteTTL        = 15 * time.Minute

	MaxAmountCentavos int64 = 100_000_000_000
	MaxText           int   = 128
	MaxCapabilities   int   = 16
)

var (
	ErrInvalid            = errors.New("payments: invalid input")
	ErrUnknownOffer       = errors.New("payments: unknown offer")
	ErrSaleDisabled       = errors.New("payments: sale disabled")
	ErrPriceUnresolved    = errors.New("payments: price unresolved")
	ErrChannelUnsupported = errors.New("payments: channel unsupported")
	ErrTermUnsupported    = errors.New("payments: term unsupported")
	ErrFeatureUnsupported = errors.New("payments: feature unsupported")
	ErrAmountMismatch     = errors.New("payments: amount or currency mismatch")
	ErrTransition         = errors.New("payments: invalid transition")
	ErrConflict           = errors.New("payments: conflict")
	ErrBinding            = errors.New("payments: binding mismatch")
	ErrRefundBounds       = errors.New("payments: refund bounds")
	ErrStale              = errors.New("payments: stale")
	ErrBusy               = errors.New("payments: busy")
	ErrUnavailable        = errors.New("payments: unavailable")
	ErrNotFound           = errors.New("payments: not found")
)

// Money is exact BRL minor units. It is never converted through float64.
type Money struct {
	Centavos int64
	Currency string
}

func BRL(centavos int64) Money {
	return Money{Centavos: centavos, Currency: CurrencyBRL}
}

func (m Money) Validate() error {
	if m.Currency != CurrencyBRL {
		return fmt.Errorf("%w: unsupported currency", ErrInvalid)
	}
	if m.Centavos < 0 || m.Centavos > MaxAmountCentavos {
		return fmt.Errorf("%w: amount out of bounds", ErrInvalid)
	}
	return nil
}

func (m Money) Positive() error {
	if err := m.Validate(); err != nil {
		return err
	}
	if m.Centavos == 0 {
		return fmt.Errorf("%w: amount must be positive", ErrInvalid)
	}
	return nil
}

func (m Money) Add(other Money) (Money, error) {
	if err := m.Validate(); err != nil {
		return Money{}, err
	}
	if err := other.Validate(); err != nil {
		return Money{}, err
	}
	if m.Currency != other.Currency {
		return Money{}, fmt.Errorf("%w: currency", ErrAmountMismatch)
	}
	if other.Centavos > MaxAmountCentavos-m.Centavos {
		return Money{}, fmt.Errorf("%w: amount overflow", ErrInvalid)
	}
	return Money{Centavos: m.Centavos + other.Centavos, Currency: m.Currency}, nil
}

func (m Money) Sub(other Money) (Money, error) {
	if err := m.Validate(); err != nil {
		return Money{}, err
	}
	if err := other.Validate(); err != nil {
		return Money{}, err
	}
	if m.Currency != other.Currency {
		return Money{}, fmt.Errorf("%w: currency", ErrAmountMismatch)
	}
	if other.Centavos > m.Centavos {
		return Money{}, fmt.Errorf("%w: negative amount", ErrInvalid)
	}
	return Money{Centavos: m.Centavos - other.Centavos, Currency: m.Currency}, nil
}

func (m Money) IsZero() bool { return m.Centavos == 0 }

func (m Money) Equal(other Money) bool {
	return m.Centavos == other.Centavos && m.Currency == other.Currency
}

// String renders exact BRL decimal text for provider integration.
func (m Money) String() string {
	whole := m.Centavos / 100
	frac := m.Centavos % 100
	if frac < 0 {
		frac = -frac
	}
	return fmt.Sprintf("%d.%02d", whole, frac)
}

type Channel string

const (
	ChannelCard         Channel = "stripe_card"
	ChannelPixManual    Channel = "asaas_pix_manual"
	ChannelPixAutomatic Channel = "asaas_pix_automatic"
)

func (c Channel) Validate() error {
	switch c {
	case ChannelCard, ChannelPixManual, ChannelPixAutomatic:
		return nil
	}
	return fmt.Errorf("%w: channel", ErrChannelUnsupported)
}

func (c Channel) Recurring() bool {
	return c == ChannelCard || c == ChannelPixAutomatic
}

func (c Channel) Provider() Provider {
	switch c {
	case ChannelCard:
		return ProviderStripe
	case ChannelPixManual, ChannelPixAutomatic:
		return ProviderAsaas
	}
	return ""
}

type Provider string

const (
	ProviderStripe Provider = "stripe"
	ProviderAsaas  Provider = "asaas"
)

func (p Provider) Validate() error {
	if p == ProviderStripe || p == ProviderAsaas {
		return nil
	}
	return fmt.Errorf("%w: provider", ErrInvalid)
}

type Environment string

const (
	EnvironmentTest       Environment = "test"
	EnvironmentProduction Environment = "production"
)

func (e Environment) Validate() error {
	if e == EnvironmentTest || e == EnvironmentProduction {
		return nil
	}
	return fmt.Errorf("%w: environment", ErrInvalid)
}

type Term string

const TermMonthly Term = "P1M"

func (t Term) Validate() error {
	if t == TermMonthly {
		return nil
	}
	return fmt.Errorf("%w: term", ErrTermUnsupported)
}

func premiumCapabilities() []string {
	return []string{
		"cloud.sync",
		"cloud.backup",
		"cloud.restore",
		"alerts.advanced",
		"professional.write",
		"professional.report",
		"vehicles.multiple",
	}
}

// PremiumCapabilityList returns a copy so callers cannot mutate policy.
func PremiumCapabilityList() []string {
	return premiumCapabilities()
}

type Offer struct {
	ID           string
	Family       string
	Term         Term
	Price        Money
	Capabilities []string
	Channels     map[Channel]bool
	SaleEnabled  bool
	Enterprise   bool
}

func (o Offer) Validate() error {
	if !validText(o.ID, MaxText) || !validText(o.Family, MaxText) {
		return fmt.Errorf("%w: offer identity", ErrInvalid)
	}
	if err := o.Term.Validate(); err != nil {
		return err
	}
	if o.SaleEnabled {
		if err := o.Price.Positive(); err != nil {
			return fmt.Errorf("%w: %v", ErrPriceUnresolved, err)
		}
	} else if o.Price.Centavos < 0 {
		return fmt.Errorf("%w: offer price", ErrInvalid)
	}
	if o.Price.Centavos > 0 {
		if err := o.Price.Validate(); err != nil {
			return err
		}
	}
	if len(o.Channels) == 0 {
		return fmt.Errorf("%w: offer channels", ErrInvalid)
	}
	for channel := range o.Channels {
		if err := channel.Validate(); err != nil {
			return err
		}
	}
	if o.Enterprise && o.SaleEnabled {
		return fmt.Errorf("%w: enterprise offer", ErrSaleDisabled)
	}
	if len(o.Capabilities) == 0 {
		if !o.Enterprise {
			return fmt.Errorf("%w: offer capabilities", ErrInvalid)
		}
	} else if o.Enterprise {
		return fmt.Errorf("%w: enterprise capability", ErrInvalid)
	}
	if len(o.Capabilities) > MaxCapabilities {
		return fmt.Errorf("%w: too many capabilities", ErrInvalid)
	}
	seen := make(map[string]bool, len(o.Capabilities))
	for _, capability := range o.Capabilities {
		if !validText(capability, MaxText) || seen[capability] {
			return fmt.Errorf("%w: offer capability", ErrInvalid)
		}
		seen[capability] = true
	}
	return nil
}

func (o Offer) clone() Offer {
	o.Capabilities = copyStrings(o.Capabilities)
	channels := make(map[Channel]bool, len(o.Channels))
	for channel, enabled := range o.Channels {
		channels[channel] = enabled
	}
	o.Channels = channels
	return o
}

type Catalog struct {
	Version string
	Offers  map[string]Offer
}

func NewCatalog(version string, offers ...Offer) (Catalog, error) {
	if !validText(version, MaxText) {
		return Catalog{}, fmt.Errorf("%w: catalog version", ErrInvalid)
	}
	catalog := Catalog{Version: version, Offers: make(map[string]Offer, len(offers))}
	for _, offer := range offers {
		if err := offer.Validate(); err != nil {
			return Catalog{}, err
		}
		if _, exists := catalog.Offers[offer.ID]; exists {
			return Catalog{}, fmt.Errorf("%w: duplicate offer", ErrConflict)
		}
		catalog.Offers[offer.ID] = offer.clone()
	}
	return catalog, nil
}

type QuoteRequest struct {
	OfferID string
	Channel Channel
	Amount  *Money
	Term    Term
	Feature string
}

type Quote struct {
	OfferID        string
	Family         string
	CatalogVersion string
	Channel        Channel
	Term           Term
	Amount         Money
	Capabilities   []string
	QuotedAt       time.Time
	ExpiresAt      time.Time
}

func (c Catalog) Quote(request QuoteRequest, now time.Time) (Quote, error) {
	if !validText(request.OfferID, MaxText) || now.IsZero() {
		return Quote{}, fmt.Errorf("%w: quote request", ErrInvalid)
	}
	if err := request.Channel.Validate(); err != nil {
		return Quote{}, err
	}
	if request.Feature != "" {
		return Quote{}, fmt.Errorf("%w: feature", ErrFeatureUnsupported)
	}
	offer, ok := c.Offers[request.OfferID]
	if !ok {
		return Quote{}, ErrUnknownOffer
	}
	if !offer.SaleEnabled {
		if offer.Price.Centavos == 0 {
			return Quote{}, ErrPriceUnresolved
		}
		return Quote{}, ErrSaleDisabled
	}
	if offer.Price.Centavos == 0 {
		return Quote{}, ErrPriceUnresolved
	}
	if !offer.Channels[request.Channel] {
		return Quote{}, ErrChannelUnsupported
	}
	if request.Term != "" && request.Term != offer.Term {
		return Quote{}, ErrTermUnsupported
	}
	if request.Amount != nil && !request.Amount.Equal(offer.Price) {
		return Quote{}, ErrAmountMismatch
	}
	return Quote{
		OfferID:        offer.ID,
		Family:         offer.Family,
		CatalogVersion: c.Version,
		Channel:        request.Channel,
		Term:           offer.Term,
		Amount:         offer.Price,
		Capabilities:   copyStrings(offer.Capabilities),
		QuotedAt:       now.UTC(),
		ExpiresAt:      now.UTC().Add(QuoteTTL),
	}, nil
}

func copyStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}

func ValidID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	_, err := hex.DecodeString(strings.ReplaceAll(value, "-", ""))
	return err == nil
}

func sameStringSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	seen := make(map[string]int, len(left))
	for _, value := range left {
		seen[value]++
	}
	for _, value := range right {
		if seen[value] == 0 {
			return false
		}
		seen[value]--
	}
	return true
}

func validText(value string, max int) bool {
	if value == "" || len(value) > max {
		return false
	}
	for _, r := range value {
		if r <= 32 || r > 126 {
			return false
		}
	}
	return true
}
