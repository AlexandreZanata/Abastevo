package adapters

import (
	"context"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
	"github.com/stripe/stripe-go/v87"
)

// VerifyCompanyRefund reads the authenticated provider account's current charge.
func (p *StripeProvider) VerifyCompanyRefund(ctx context.Context, event app.InboxRecord) (app.ChargeRefundFact, error) {
	if p == nil || p.Client == nil {
		return app.ChargeRefundFact{}, domain.ErrUnavailable
	}
	if event.EventType != "charge.refunded" || event.ResourceType != "charge" ||
		event.ResourceID == "" || event.Environment != p.Environment || event.Merchant != p.Merchant {
		return app.ChargeRefundFact{}, domain.ErrBinding
	}
	charge, err := p.Client.V1Charges.Retrieve(ctx, event.ResourceID, nil)
	if err != nil {
		return app.ChargeRefundFact{}, stripeError(err)
	}
	if charge == nil || charge.ID != event.ResourceID || !charge.Paid || !charge.Captured ||
		charge.Currency != stripe.CurrencyBRL || charge.Livemode != (p.Environment == domain.EnvironmentProduction) ||
		charge.AmountCaptured <= 0 || charge.AmountRefunded < 0 || charge.AmountRefunded > charge.AmountCaptured {
		return app.ChargeRefundFact{}, domain.ErrBinding
	}
	return app.ChargeRefundFact{ChargeID: charge.ID, Amount: domain.BRL(charge.AmountCaptured),
		Full: charge.Refunded && charge.AmountRefunded == charge.AmountCaptured}, nil
}

func (p *AsaasProvider) VerifyCompanyRefund(ctx context.Context, event app.InboxRecord) (app.ChargeRefundFact, error) {
	if event.EventType != "PAYMENT_REFUNDED" && event.EventType != "PAYMENT_PARTIALLY_REFUNDED" ||
		event.ResourceType != "payment" || !validAsaasID(event.ResourceID) || event.Environment != p.Environment {
		return app.ChargeRefundFact{}, domain.ErrBinding
	}
	pix, err := p.RetrieveCompanyPix(ctx, event.ResourceID)
	if err != nil {
		return app.ChargeRefundFact{}, err
	}
	if pix.ID != event.ResourceID || pix.Method != "PIX" || pix.Deleted || pix.Adjusted || pix.Amount.Positive() != nil {
		return app.ChargeRefundFact{}, domain.ErrBinding
	}
	return app.ChargeRefundFact{ChargeID: pix.ID, ExternalReference: pix.ExternalReference, Amount: pix.Amount, Full: pix.Status == "REFUNDED"}, nil
}
