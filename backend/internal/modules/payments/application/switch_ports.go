package application

import (
	"context"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
)

const (
	SourceSwitchPending   = "PENDING"
	SourceSwitchCanceling = "CANCELING"
	SourceSwitchReady     = "READY"
	SourceSwitchConsumed  = "CONSUMED"
	SourceSwitchCanceled  = "CANCELED"

	sourceSwitchCancelReason = "SOURCE_SWITCH"
)

type RecurringSource struct {
	Source            domain.Channel
	OfferID           string
	LocalID           string
	RemoteID          string
	ProviderStatus    string
	CancelAtPeriodEnd bool
}

type SourceSwitchRecord struct {
	ID                   string
	AccountID            string
	OfferID              string
	Environment          domain.Environment
	Merchant             string
	OperationKey         string
	OldSource            domain.Channel
	OldOfferID           string
	NewSource            domain.Channel
	OldLocalID           string
	OldRemoteID          string
	TargetCatalogVersion string
	TargetAmount         domain.Money
	TargetCapabilities   []string
	CurrentEnd           time.Time
	FirstDebitAt         time.Time
	State                string
	OldCancellation      domain.CancellationState
	Attempts             int
	LastError            string
	RequestedAt          time.Time
	LastEventAt          time.Time
}

type SourceGateStore interface {
	ActiveRecurringSource(context.Context, string, string) (RecurringSource, bool, error)
	ActiveGrantEnd(context.Context, string, string) (time.Time, error)
	OpenSourceSwitch(context.Context, string, string) (SourceSwitchRecord, bool, error)
}

type SwitchStore interface {
	SourceGateStore
	CreateSourceSwitch(context.Context, SourceSwitchRecord) (SourceSwitchRecord, bool, error)
	ClaimSourceSwitch(context.Context, string, string, time.Time) (bool, error)
	ResolveSourceSwitch(context.Context, SourceSwitchRecord) error
	ConsumeSourceSwitch(context.Context, string) error
	ContractForAccount(context.Context, string, string) (ContractRecord, error)
	UpsertContract(context.Context, ContractRecord) error
	MandateForAccount(context.Context, string, string) (MandateRecord, error)
}

type SwitchConfig struct {
	Environment       domain.Environment
	CardMerchantID    string
	PixAutoMerchantID string
	CardReady         bool
	PixAutoReady      bool
}

type SwitchRequest struct {
	OperationID string
	OfferID     string
	NewChannel  domain.Channel
	Proration   bool
}

type StripeRenewalFencer interface {
	CancelRenewal(context.Context, string, string) error
	RetrieveLifecycle(context.Context, string) (LifecycleFact, error)
}
