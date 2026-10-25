package adapters

import (
	"context"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
)

// settlementProvider is the narrow provider port the mux fans out to.
type settlementProvider interface {
	VerifySettlement(context.Context, app.InboxRecord) (app.SettlementCandidate, error)
}

// SettlementMux fans settlement verification out to the configured provider
// only. An unconfigured provider fails closed; an impossible provider value
// (rejected by the inbox CHECK constraint) is ignored with no effect.
type SettlementMux struct {
	Stripe settlementProvider
	Asaas  settlementProvider
}

func (m SettlementMux) Verify(ctx context.Context, event app.InboxRecord) (app.SettlementCandidate, error) {
	switch event.Provider {
	case domain.ProviderStripe:
		if m.Stripe == nil {
			return app.SettlementCandidate{}, domain.ErrUnavailable
		}
		return m.Stripe.VerifySettlement(ctx, event)
	case domain.ProviderAsaas:
		if m.Asaas == nil {
			return app.SettlementCandidate{}, domain.ErrUnavailable
		}
		return m.Asaas.VerifySettlement(ctx, event)
	default:
		return app.SettlementCandidate{}, app.ErrIgnored
	}
}
