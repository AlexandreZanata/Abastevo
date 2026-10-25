// Package application owns payment use cases and their explicit ports.
package application

import (
	"context"
	"errors"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
)

var ErrAmbiguous = errors.New("payments: provider outcome unknown")

// ErrProviderUnauthorized reports a rejected provider credential (revoked
// key, mismatched account). Unlike domain.ErrUnauthorized — which refuses
// user, owner and operator authorization terminally — it preserves the
// event: nothing is marked, granted, revoked or failed, so rotation and
// retry converge without loss.
var ErrProviderUnauthorized = errors.New("payments: provider credential rejected")

type Principal struct {
	AccountID string
}

type StartRequest struct {
	OperationID string
	OfferID     string

	// Client-supplied values are always refused. They exist so a caller cannot
	// silently pass a price, customer, return URL or metadata through this path.
	AmountCentavos *int64
	Currency       string
	CustomerID     string
	OrderID        string
	SuccessURL     string
	CancelURL      string
	Metadata       map[string]string
}

type Checkout struct {
	OrderID   string
	SessionID string
	URL       string
	Replayed  bool
}

type Config struct {
	Environment     domain.Environment
	MerchantID      string
	PriceID         string
	CatalogVersion  string
	ReturnOrigin    string
	NewSalesEnabled bool
	LeaseTTL        time.Duration
}

type CustomerMapping struct {
	AccountID        string
	Provider         domain.Provider
	Environment      domain.Environment
	Merchant         string
	RemoteCustomerID string
}

type ClaimAttemptInput struct {
	AccountID    string
	Provider     domain.Provider
	Environment  domain.Environment
	Merchant     string
	OperationKey string
	Kind         string
	PayloadHash  string
	OrderID      string
	LeaseUntil   time.Time
}

type Claim struct {
	OperationID string
	AttemptID   string
	Generation  int64
	AttemptNo   int32
	State       string
	RemoteID    string
	LeaseUntil  time.Time
	Replay      bool
}

type InboxEvent struct {
	Provider       domain.Provider
	Environment    domain.Environment
	Merchant       string
	EventID        string
	PayloadHash    string
	EventType      string
	ResourceType   string
	ResourceID     string
	ResourceStatus string
}

type InboxResult struct {
	Created bool
	State   string
}

type InboxRecord struct {
	InboxEvent
	State      string
	ReceivedAt time.Time
}

type AttemptRecord struct {
	ID          string
	OperationID string
	Provider    domain.Provider
	Environment domain.Environment
	Merchant    string
	State       string
	RemoteID    string
	Generation  int64
}

type SettlementInput struct {
	Offer      domain.Offer
	Settlement domain.Settlement
}

type SettlementOutcome struct {
	SettlementID string
	OrderID      string
	GrantID      string
	Excess       bool
	Replay       bool
}

type SettlementStore interface {
	Inbox(context.Context, domain.Provider, domain.Environment, string, string) (InboxRecord, error)
	PendingInbox(context.Context, domain.Provider, domain.Environment, string, int) ([]InboxRecord, error)
	GetOrder(context.Context, string) (domain.Order, error)
	AttemptForOrder(context.Context, string) (AttemptRecord, error)
	Customer(context.Context, string, domain.Provider, domain.Environment, string) (string, bool, error)
	MandateForSubscription(context.Context, domain.Environment, string, string) (MandateRecord, error)
	RecordSettlement(context.Context, SettlementInput) (SettlementOutcome, error)
	EnsurePixAutoCycleOrder(context.Context, PixAutoCycleOrderInput) (string, error)
	MarkInbox(context.Context, InboxEvent, string, string) error
	RevokeGrant(context.Context, string, time.Time) error
}

type InboxRecorder interface {
	RecordInbox(context.Context, InboxEvent) (InboxResult, error)
}

type Store interface {
	SourceGateStore
	CreateOrder(context.Context, domain.Order) error
	GetOrder(context.Context, string) (domain.Order, error)
	Customer(context.Context, string, domain.Provider, domain.Environment, string) (string, bool, error)
	MapCustomer(context.Context, CustomerMapping) error
	ClaimAttempt(context.Context, ClaimAttemptInput) (Claim, error)
	MarkDispatched(context.Context, Claim) error
	MarkOutcome(context.Context, Claim, domain.AttemptState, string) error
}

type CustomerRequest struct {
	AccountID   string
	Environment domain.Environment
	MerchantID  string
}

type Customer struct {
	ID string
}

type CheckoutRequest struct {
	Order          domain.Order
	CustomerID     string
	PriceID        string
	MerchantID     string
	Environment    domain.Environment
	SuccessURL     string
	CancelURL      string
	IdempotencyKey string
}

type CheckoutSession struct {
	ID             string
	URL            string
	Status         string
	Mode           string
	CustomerID     string
	SubscriptionID string
	Livemode       bool
	AmountTotal    int64
	Currency       string
}

type Provider interface {
	EnsureCustomer(context.Context, CustomerRequest) (Customer, error)
	CreateCheckout(context.Context, CheckoutRequest) (CheckoutSession, error)
	RetrieveCheckout(context.Context, string) (CheckoutSession, error)
	FindCheckout(context.Context, string, string) (CheckoutSession, bool, bool, error)
}
