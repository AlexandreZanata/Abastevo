package application

import (
	"context"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
)

type ContractRecord struct {
	ID                   string
	AccountID            string
	OfferID              string
	Source               domain.Channel
	Environment          domain.Environment
	Merchant             string
	Provider             domain.Provider
	RemoteSubscriptionID string
	RemoteCustomerID     string
	ProviderStatus       string
	CancelAtPeriodEnd    bool
	LastEventAt          time.Time
}

type LifecycleFact struct {
	AccountID            string
	OfferID              string
	Environment          domain.Environment
	Merchant             string
	Provider             domain.Provider
	RemoteSubscriptionID string
	RemoteCustomerID     string
	ProviderStatus       string
	CancelAtPeriodEnd    bool
	LastEventAt          time.Time
}

type LifecycleVerifier interface {
	VerifyLifecycle(context.Context, InboxRecord) (LifecycleFact, error)
	RetrieveLifecycle(context.Context, string) (LifecycleFact, error)
}

type LifecycleStore interface {
	Inbox(context.Context, domain.Provider, domain.Environment, string, string) (InboxRecord, error)
	PendingInbox(context.Context, domain.Provider, domain.Environment, string, int) ([]InboxRecord, error)
	MarkInbox(context.Context, InboxEvent, string, string) error
	UpsertContract(context.Context, ContractRecord) error
	ContractForAccount(context.Context, string, string) (ContractRecord, error)
	ContractByRemote(context.Context, domain.Environment, string, string) (ContractRecord, error)
}

type PortalConfig struct {
	Environment     domain.Environment
	Merchant        string
	ReturnOrigin    string
	ConfigurationID string
}

type PortalRequest struct {
	CustomerID      string
	ReturnURL       string
	ConfigurationID string
}

type PortalProvider interface {
	CreatePortalSession(context.Context, PortalRequest) (string, error)
}
