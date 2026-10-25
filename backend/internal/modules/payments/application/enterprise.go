package application

import (
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
)

// authorizeEnterprisePurchase binds an enterprise-family sale to its single
// accountable payer before any order exists. The billed entity, the
// authenticated payer and the service/account target must be the same account,
// exactly one unit is allowed, and the offer must be the catalog-approved
// enterprise offer with no app capabilities. Anything else is refused without
// creating an order, a mandate, a charge or a grant.
func AuthorizeEnterprisePurchase(accountID, payerID string, quote domain.Quote, offer domain.Offer) error {
	if !offer.Enterprise && quote.Family != domain.EnterpriseFamily {
		return nil
	}
	decision, err := domain.DecideEnterpriseBinding(domain.EnterpriseBindingRequest{
		Offer: offer, AccountID: accountID, PayerID: payerID, Quantity: 1,
		Approved: offer.SaleEnabled,
	})
	if err != nil {
		return err
	}
	if !decision.Allowed {
		if decision.Reason == domain.EnterpriseUnavailable {
			return domain.ErrSaleDisabled
		}
		return domain.ErrBinding
	}
	return nil
}
