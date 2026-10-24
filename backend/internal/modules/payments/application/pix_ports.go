package application

import (
	"context"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
)

const (
	PixPending  = "pending"
	PixSettled  = "settled"
	PixExpired  = "expired"
	PixRefunded = "refunded"
	PixDisputed = "disputed"
	PixUnknown  = "unknown"
)

type PixConfig struct {
	Environment     domain.Environment
	MerchantID      string
	CatalogVersion  string
	NewSalesEnabled bool
	LeaseTTL        time.Duration
}

type PixStartRequest struct {
	OperationID string
	OfferID     string
	PayerName   string
	PayerTaxID  string
	PayerEmail  string

	// Client-supplied commercial/provider values are refused.
	AmountCentavos *int64
	Currency       string
	CustomerID     string
	OrderID        string
	Metadata       map[string]string
}

type PixOrder struct {
	OrderID        string
	PaymentID      string
	Status         string
	ProviderStatus string
	QRCode         string
	QRImage        string
	ExpiresAt      *time.Time
	Replayed       bool
}

type PixCustomerRequest struct {
	AccountID   string
	Environment domain.Environment
	MerchantID  string
	Name        string
	TaxID       string
	Email       string
}

type PixPaymentRequest struct {
	Order       domain.Order
	CustomerID  string
	Environment domain.Environment
	MerchantID  string
	DueDate     time.Time
}

type PixPayment struct {
	ID                string
	CustomerID        string
	SubscriptionID    string
	ExternalReference string
	Amount            domain.Money
	Status            string
	ProviderStatus    string
	Class             string
	DueDate           time.Time
}

type PixQR struct {
	Payload   string
	Image     string
	ExpiresAt time.Time
}

type PixProvider interface {
	EnsureCustomer(context.Context, PixCustomerRequest) (Customer, error)
	CreatePayment(context.Context, PixPaymentRequest) (PixPayment, error)
	RetrievePayment(context.Context, string) (PixPayment, error)
	FindPayment(context.Context, string) (PixPayment, bool, bool, error)
	GetQR(context.Context, string) (PixQR, error)
}
