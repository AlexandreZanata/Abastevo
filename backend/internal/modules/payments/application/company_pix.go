package application

import (
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
)

// CompanyPixProof is a read-only view of a provider-retrieved charge. It does
// not decide the company, approved price or subscription term.
type CompanyPixProof struct {
	ID, CustomerID, SubscriptionID, AuthorizationID, ExternalReference string
	Method, Status                                                     string
	Environment                                                        domain.Environment
	Amount                                                             domain.Money
	PaidOn                                                             time.Time
	Adjusted, RefundedOrDisputed, Deleted                              bool
}
