package application

import (
	"context"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
)

// OperatorProof verifies a fresh, single-use operator credential for one
// restricted financial action. Implementations live outside payments;
// a nil proof fails closed.
type OperatorProof interface {
	VerifyOperator(ctx context.Context, actor, proof, action string) error
}

type RefundRequest struct {
	Provider        domain.Provider
	Environment     domain.Environment
	Merchant        string
	RemotePaymentID string
	Amount          *domain.Money
	Reason          string
	ActorID         string
	Proof           string
	ApprovedBy      string
	ApprovalProof   string
	OperationKey    string
}

type RefundRecord struct {
	ID              string
	AccountID       string
	OfferID         string
	OrderID         string
	GrantID         string
	SettlementID    string
	Provider        domain.Provider
	Environment     domain.Environment
	Merchant        string
	OperationKey    string
	RemotePaymentID string
	RemoteChargeID  string
	RemoteRefundID  string
	Amount          domain.Money
	State           domain.RefundState
	Reason          string
	RequestedBy     string
	ApprovedBy      string
	RequestedAt     time.Time
	UpdatedAt       time.Time
	ResolvedAt      time.Time
	LastError       string
}

type RefundProviderRequest struct {
	RefundID        string
	RemotePaymentID string
	RemoteChargeID  string
	Amount          domain.Money
	Reason          string
	IdempotencyKey  string
	Metadata        map[string]string
}

type RefundProviderResult struct {
	ID        string
	State     domain.RefundState
	RawStatus string
	Amount    domain.Money
	At        time.Time
}

type RefundProvider interface {
	CreateRefund(context.Context, RefundProviderRequest) (RefundProviderResult, error)
	FindRefund(context.Context, RefundProviderRequest) (RefundProviderResult, bool, bool, error)
}

type RefundTarget struct {
	AccountID       string
	OfferID         string
	OrderID         string
	GrantID         string
	SettlementID    string
	Provider        domain.Provider
	Environment     domain.Environment
	Merchant        string
	RemotePaymentID string
	RemoteChargeID  string
	Paid            domain.Money
	Reserved        domain.Money
	GrantState      string
}

type RefundReservation struct {
	Provider        domain.Provider
	Environment     domain.Environment
	Merchant        string
	RemotePaymentID string
	Amount          domain.Money
	Reason          string
	RequestedBy     string
	ApprovedBy      string
	OperationKey    string
}

type DisputeFact struct {
	Provider        domain.Provider
	Environment     domain.Environment
	Merchant        string
	RemoteDisputeID string
	RemotePaymentID string
	RemoteChargeID  string
	Amount          domain.Money
	State           domain.DisputeState
	OpenedAt        time.Time
	LastEventAt     time.Time
}

type DisputeRecord struct {
	ID              string
	AccountID       string
	OfferID         string
	GrantID         string
	SettlementID    string
	Provider        domain.Provider
	Environment     domain.Environment
	Merchant        string
	RemoteDisputeID string
	RemotePaymentID string
	RemoteChargeID  string
	Amount          domain.Money
	State           domain.DisputeState
	OpenedAt        time.Time
	UpdatedAt       time.Time
	ResolvedAt      time.Time
	LastEventAt     time.Time
}

type ProviderHold struct {
	ID              string
	Provider        domain.Provider
	Environment     domain.Environment
	Merchant        string
	RemoteHoldID    string
	RemotePaymentID string
	Amount          domain.Money
	AmountKnown     bool
	Reason          string
	State           string
	ObservedAt      time.Time
	ReviewedAt      time.Time
	ReviewedBy      string
	ReviewReason    string
}

type OperatorAction struct {
	Actor     string
	Action    string
	TargetID  string
	AccountID string
	Reason    string
	Amount    domain.Money
	HasAmount bool
	Result    string
	CreatedAt time.Time
}

const (
	OperatorActionRefundRequest = "refund.request"
	OperatorActionRefundReplay  = "refund.replay"
	OperatorActionHoldReview    = "hold.review"
)

type RefundStore interface {
	RefundTarget(ctx context.Context, provider domain.Provider, environment domain.Environment, merchant, remotePaymentID string) (RefundTarget, error)
	ReserveRefund(ctx context.Context, input RefundReservation) (RefundRecord, bool, error)
	GetRefund(ctx context.Context, id string) (RefundRecord, error)
	UpdateRefundState(ctx context.Context, id string, previous, next domain.RefundState, remoteRefundID, lastError string, at time.Time) (RefundRecord, error)
	RecordOperatorAction(ctx context.Context, input OperatorAction) error
}

type DisputeStore interface {
	RecordDispute(ctx context.Context, input DisputeRecord) (DisputeRecord, bool, error)
	GetDisputeByRemote(ctx context.Context, provider domain.Provider, environment domain.Environment, merchant, remoteDisputeID string) (DisputeRecord, bool, error)
	UpdateDisputeState(ctx context.Context, id string, previous, next domain.DisputeState, at time.Time) (DisputeRecord, error)
	RecordProviderHold(ctx context.Context, input ProviderHold) (ProviderHold, bool, error)
	GetProviderHold(ctx context.Context, id string) (ProviderHold, error)
	ReviewProviderHold(ctx context.Context, id, reviewedBy, decision, reviewReason string, at time.Time) (ProviderHold, error)
	RecordOperatorAction(ctx context.Context, input OperatorAction) error
	MarkInbox(ctx context.Context, event InboxEvent, state, lastError string) error
}

type RefundServiceConfig struct {
	SingleApprovalAboveCents int64
}

type DisputeVerifier interface {
	VerifyDispute(ctx context.Context, event InboxRecord) (DisputeFact, error)
}

type RefundEventVerifier interface {
	VerifyRefundEvent(ctx context.Context, event InboxRecord) (RefundEventFact, error)
}

type RefundEventFact struct {
	Provider        domain.Provider
	Environment     domain.Environment
	Merchant        string
	RemotePaymentID string
	RemoteChargeID  string
}
