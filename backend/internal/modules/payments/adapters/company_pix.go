package adapters

import (
	"context"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
)

func (p *AsaasProvider) RetrieveCompanyPix(ctx context.Context, id string) (app.CompanyPixProof, error) {
	if !validAsaasID(id) {
		return app.CompanyPixProof{}, domain.ErrInvalid
	}
	payment, err := p.RetrieveAsaasPayment(ctx, id)
	if err != nil {
		return app.CompanyPixProof{}, err
	}
	location, err := time.LoadLocation(domain.BillingTimezone)
	if err != nil {
		return app.CompanyPixProof{}, domain.ErrUnavailable
	}
	paidOn, _ := time.ParseInLocation(time.DateOnly, payment.PaymentDate, location)
	amount, _ := parseBRLMinor(payment.Value)
	interest, interestErr := parseBRLMinor(payment.InterestValue)
	adjusted := payment.OriginalValue.String() != "" || payment.InterestValue.String() != "" && (interestErr != nil || interest != 0)
	return app.CompanyPixProof{
		ID: payment.ID, CustomerID: payment.Customer, SubscriptionID: payment.Subscription,
		AuthorizationID: payment.PixAutomaticAuthorizationID, ExternalReference: payment.ExternalReference,
		Method: payment.BillingType, Status: payment.Status, Environment: p.Environment,
		Amount: domain.BRL(amount), PaidOn: paidOn, Adjusted: adjusted, Deleted: payment.Deleted,
		RefundedOrDisputed: len(payment.Refunds) != 0 || payment.Chargeback != nil,
	}, nil
}
