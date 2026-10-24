package application

import (
	"context"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
)

type MandateRecord struct {
	ID                    string
	AccountID             string
	OfferID               string
	OrderID               string
	Environment           domain.Environment
	Merchant              string
	Provider              domain.Provider
	RemoteAuthorizationID string
	RemoteSubscriptionID  string
	ContractID            string
	RemoteCustomerID      string
	Status                string
	Frequency             string
	PaymentCreationMode   string
	RetryPolicy           string
	Value                 domain.Money
	StartDate             time.Time
	FinishDate            time.Time
	ConsentVersion        string
	ConsentAt             time.Time
	LastEventAt           time.Time
	CancelRequestedAt     *time.Time
	CancelReason          string
	CancelledAt           *time.Time
}

type PixAutoConfig struct {
	Environment      domain.Environment
	MerchantID       string
	CatalogVersion   string
	NewSalesEnabled  bool
	MerchantEligible bool
	ConsentVersion   string
	MandateMonths    int
	LeaseTTL         time.Duration
}

type PixAutoStartRequest struct {
	OperationID string
	OfferID     string
	PayerName   string
	PayerTaxID  string
	PayerEmail  string
	Consent     bool

	AmountCentavos *int64
	Currency       string
	CustomerID     string
	OrderID        string
	Metadata       map[string]string
}

type PixAutoAuthorization struct {
	ID                   string
	CustomerID           string
	ContractID           string
	RemoteSubscriptionID string
	Status               string
	PaymentCreationMode  string
	RetryPolicy          string
	Value                domain.Money
	StartDate            time.Time
	FinishDate           time.Time
	QRCode               string
	QRImage              string
	ExpiresAt            *time.Time
	Replayed             bool
	CancellationPending  bool
}

type PixAutoInstruction struct {
	ID              string
	AuthorizationID string
	SubscriptionID  string
	PaymentID       string
	Status          string
	Purpose         string
	RetryAttempt    int
	DueDate         time.Time
	RefusalReason   string
}

type InstructionRecord struct {
	ID                    string
	AccountID             string
	OfferID               string
	Environment           domain.Environment
	Merchant              string
	RemoteInstructionID   string
	RemoteAuthorizationID string
	RemoteSubscriptionID  string
	RemotePaymentID       string
	Status                string
	Purpose               string
	RetryAttempt          int
	DueDate               time.Time
	RefusalReason         string
	LastEventAt           time.Time
}

type PixAutoEligibility struct {
	Environment domain.Environment
	Merchant    string
	Status      string
	Reason      string
	LastEventAt time.Time
	ObservedAt  time.Time
}

type PixAutoProviderRequest struct {
	CustomerID          string
	ContractID          string
	Environment         domain.Environment
	MerchantID          string
	Value               domain.Money
	StartDate           time.Time
	FinishDate          time.Time
	ImmediateExpiration time.Duration
}

type PixAutoProvider interface {
	EnsureCustomer(context.Context, PixCustomerRequest) (Customer, error)
	CreatePixAutoAuthorization(context.Context, PixAutoProviderRequest) (PixAutoAuthorization, error)
	RetrievePixAutoAuthorization(context.Context, string) (PixAutoAuthorization, error)
	FindPixAutoAuthorization(context.Context, string, string) (PixAutoAuthorization, bool, bool, error)
	CancelPixAutoAuthorization(context.Context, string) error
	RetrievePixAutoInstruction(context.Context, string) (PixAutoInstruction, error)
	ListPixAutoInstructions(context.Context, string) ([]PixAutoInstruction, bool, error)
	ListSubscriptionPayments(context.Context, string) ([]PixPayment, bool, error)
}

type PixAutoStore interface {
	SourceGateStore
	CreateOrder(context.Context, domain.Order) error
	GetOrder(context.Context, string) (domain.Order, error)
	Customer(context.Context, string, domain.Provider, domain.Environment, string) (string, bool, error)
	MapCustomer(context.Context, CustomerMapping) error
	ClaimAttempt(context.Context, ClaimAttemptInput) (Claim, error)
	MarkDispatched(context.Context, Claim) error
	MarkOutcome(context.Context, Claim, domain.AttemptState, string) error
	UpsertMandate(context.Context, MandateRecord) error
	MandateForAccount(context.Context, string, string) (MandateRecord, error)
	MandateByRemote(context.Context, domain.Environment, string, string) (MandateRecord, error)
	RequestMandateCancel(context.Context, domain.Environment, string, string, time.Time, string) (MandateRecord, error)
	Eligibility(context.Context, domain.Environment, string) (PixAutoEligibility, bool, error)
	UpsertEligibility(context.Context, PixAutoEligibility) error
	ActiveMandates(context.Context, domain.Environment, string, int) ([]MandateRecord, error)
	UpsertInstruction(context.Context, InstructionRecord) error
	InstructionsForMandate(context.Context, domain.Environment, string, string, int) ([]InstructionRecord, error)
	MarkInbox(context.Context, InboxEvent, string, string) error
}
