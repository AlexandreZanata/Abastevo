package application

import (
	"context"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
)

const (
	BillingExitOwnerRequest    = "OWNER_REQUEST"
	BillingExitAccountDeletion = "ACCOUNT_DELETION"

	billingCancelAction = "billing.cancel"
	billingExportAction = "billing.export"
)

type OwnerProof interface {
	VerifyFreshOwner(context.Context, string, string, string) error
}

type BillingSource struct {
	Source      domain.Channel
	LocalID     string
	RemoteID    string
	OfferID     string
	Provider    domain.Provider
	Environment domain.Environment
	Merchant    string
}

type BillingExit struct {
	ID           string
	AccountID    string
	OperationKey string
	Reason       string
	State        string
	RequestedAt  time.Time
	LastEventAt  time.Time
	LastError    string
}

type BillingExitSource struct {
	ID          string
	ExitID      string
	AccountID   string
	Source      domain.Channel
	LocalID     string
	RemoteID    string
	OfferID     string
	Provider    domain.Provider
	Environment domain.Environment
	Merchant    string
	State       string
	Attempts    int
	LastError   string
	LastEventAt time.Time
}

type BillingHistoryRow struct {
	OrderID         string
	OfferID         string
	Channel         domain.Channel
	Amount          domain.Money
	OrderState      string
	OrderedAt       time.Time
	SettlementID    string
	RemotePaymentID string
	SettlementState string
	SettledAt       time.Time
	GrantID         string
	GrantStart      time.Time
	GrantEnd        time.Time
	GrantState      string
}

type BillingExitStatus struct {
	Exit    *BillingExit
	Sources []BillingExitSource
	Active  []BillingSource
}

type BillingExitStore interface {
	CreateBillingExit(context.Context, string, string, string, time.Time) (BillingExit, []BillingExitSource, bool, error)
	LatestBillingExit(context.Context, string) (BillingExit, bool, error)
	BillingExit(context.Context, string) (BillingExit, error)
	BillingExitSources(context.Context, string) ([]BillingExitSource, error)
	UpdateBillingExit(context.Context, BillingExit) error
	ClaimBillingExitSource(context.Context, string, time.Time) (bool, error)
	ResolveBillingExitSource(context.Context, BillingExitSource) error
	BillingSources(context.Context, string) ([]BillingSource, error)
	BillingHistory(context.Context, string, int) ([]BillingHistoryRow, error)
	ContractForAccount(context.Context, string, string) (ContractRecord, error)
	UpsertContract(context.Context, ContractRecord) error
	MandateForAccount(context.Context, string, string) (MandateRecord, error)
	UpsertMandate(context.Context, MandateRecord) error
}
