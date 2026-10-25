package main

import (
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	payadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/adapters"
	payjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/adapters/jobs"
	paymentsapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
	paydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/config"
)

const (
	// payWorkerCatalogVersion mirrors the API frozen catalog lineage:
	// zero sellable offers, settlement binds nothing unverified.
	payWorkerCatalogVersion = "pay-catalog-v1"
	// payWorkerConsentVersion mirrors the Pix Automático consent text.
	payWorkerConsentVersion = "consent-v1"
	// payWorkerMandateMonths is the default mandate window.
	payWorkerMandateMonths = 12
	// payWorkerLease bounds one provider write attempt.
	payWorkerLease = time.Minute
)

func payWorkerStripeEnv(mode string) paydomain.Environment {
	if mode == "live" {
		return paydomain.EnvironmentProduction
	}
	return paydomain.EnvironmentTest
}

func payWorkerAsaasEnv(mode string) paydomain.Environment {
	if mode == "production" {
		return paydomain.EnvironmentProduction
	}
	return paydomain.EnvironmentTest
}

// composePaymentInbox builds the PAY-19 worker runtime: the same durable
// store and provider-gated verifiers as the API, drained through one serial
// payment-inbox slot isolated from safety and map workloads. Unconfigured
// providers stay nil so pending events are preserved, never dropped.
func composePaymentInbox(payments config.PaymentsConfig, pool *pgxpool.Pool, clock func() time.Time) payjobs.Handler {
	if clock == nil {
		clock = time.Now
	}
	store := payadapters.NewStore(pool)
	catalog, err := paydomain.NewCatalog(payWorkerCatalogVersion)
	if err != nil {
		catalog = paydomain.Catalog{}
	}
	preflight := payments.Preflight()
	router := &payjobs.Router{Store: store}
	mux := payadapters.SettlementMux{}
	dispute := &paymentsapp.DisputeService{Store: store, Clock: clock}

	if preflight.Stripe.MaintenanceReady {
		env := payWorkerStripeEnv(payments.Stripe.Mode)
		stripe := payadapters.NewStripeProvider(payments.Stripe.SecretKey, env, nil)
		stripe.Merchant = payments.Stripe.MerchantID
		mux.Stripe = stripe
		dispute.Stripe = stripe
		router.Lifecycle = &paymentsapp.LifecycleService{Store: store, Verifier: stripe}
		router.Channels = append(router.Channels, payjobs.Channel{
			Provider: paydomain.ProviderStripe, Environment: env,
			Merchant: payments.Stripe.MerchantID,
		})
	}
	settlement := &paymentsapp.SettlementService{Store: store, Catalog: catalog}
	router.Settlement = settlement
	if preflight.PixManual.MaintenanceReady {
		env := payWorkerAsaasEnv(payments.Asaas.Mode)
		asaas := payadapters.NewAsaasProvider(payments.Asaas.APIKey, env, nil)
		mux.Asaas = asaas
		dispute.Asaas = asaas
		router.Cycles = &paymentsapp.PixAutoCycleService{
			Store: store, Provider: asaas, Settlements: settlement, Clock: clock,
			Config: paymentsapp.PixAutoConfig{
				Environment: env, MerchantID: payments.Asaas.MerchantID,
				CatalogVersion: catalog.Version, ConsentVersion: payWorkerConsentVersion,
				MandateMonths: payWorkerMandateMonths, LeaseTTL: payWorkerLease,
			},
		}
		router.Channels = append(router.Channels, payjobs.Channel{
			Provider: paydomain.ProviderAsaas, Environment: env,
			Merchant: payments.Asaas.MerchantID,
		})
	}
	// The mux is complete only after both provider gates: bind it last so
	// no configured provider is dropped from settlement verification.
	settlement.Verifier = mux
	// Dispute stays composed even with zero configured channels: its fan-out
	// fails closed per provider and the drain over no channels is a no-op.
	router.Dispute = dispute
	return payjobs.Handler{Router: router}
}
