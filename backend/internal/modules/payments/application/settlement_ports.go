package application

import (
	"context"
	"errors"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
)

var (
	ErrIgnored    = errors.New("payments: event ignored")
	ErrNotSettled = errors.New("payments: event is not a settled payment")
)

type SettlementCandidate struct {
	OrderID         string
	Provider        domain.Provider
	Environment     domain.Environment
	Merchant        string
	Channel         domain.Channel
	CustomerID      string
	RemotePaymentID string
	ProviderStatus  string
	Amount          domain.Money
	SettledAt       time.Time
}

type SettlementVerifier interface {
	Verify(context.Context, InboxRecord) (SettlementCandidate, error)
}
