package main

import (
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	payadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/adapters"
	paywebhook "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/adapters/http"
	paymentsapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
	paydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/config"
)

const (
	// payCatalogVersion is the frozen catalog lineage. Production builds
	// carry zero sellable offers: PAY-01 keeps production prices unresolved
	// and every sale channel disabled, so checkout always fails closed.
	payCatalogVersion = "pay-catalog-v1"
	// payConsentVersion mirrors the tested Pix Automático consent text.
	payConsentVersion = "consent-v1"
	// payMandateMonths is the default Pix Automático mandate window.
	payMandateMonths = 12
	// payServiceLease bounds one provider write attempt.
	payServiceLease = time.Minute
)

// paymentsComposition is the entire PAY-19 API runtime: config-gated real
// provider clients, durable services, webhook intake and billing exits.
// Unconfigured providers stay nil so every dependent command fails closed;
// owner and operator proofs are not implemented by any account surface yet,
// so proof-gated commands stay unavailable by construction. No Play
// credential, cookie session or redirect is consulted anywhere here.
type paymentsComposition struct {
	Store      *payadapters.Store
	Catalog    paydomain.Catalog
	Stripe     *payadapters.StripeProvider
	Asaas      *payadapters.AsaasProvider
	Checkout   *paymentsapp.Service
	Pix        *paymentsapp.PixService
	PixAuto    *paymentsapp.PixAutoService
	Cycles     *paymentsapp.PixAutoCycleService
	Settlement *paymentsapp.SettlementService
	Lifecycle  *paymentsapp.LifecycleService
	Dispute    *paymentsapp.DisputeService
	Refunds    *paymentsapp.RefundService
	Switch     *paymentsapp.SwitchService
	Portal     *paymentsapp.PortalService
	Billing    *paymentsapp.BillingExitService
	StripeHook paywebhook.StripeWebhookHandler
	AsaasHook  paywebhook.AsaasWebhookHandler
	Preflight  config.PaymentsPreflight
}

func stripeEnvironment(mode string) paydomain.Environment {
	if mode == "live" {
		return paydomain.EnvironmentProduction
	}
	return paydomain.EnvironmentTest
}

func asaasEnvironment(mode string) paydomain.Environment {
	if mode == "production" {
		return paydomain.EnvironmentProduction
	}
	return paydomain.EnvironmentTest
}

// composePayments builds the payment runtime from validated configuration.
// Sales stay off unless flags, catalog, return origin and credentials all
// agree; maintenance intake, reconciliation, cancellation and refunds never
// depend on the sales switches.
func composePayments(payments config.PaymentsConfig, pool *pgxpool.Pool, clock func() time.Time) paymentsComposition {
	if clock == nil {
		clock = time.Now
	}
	store := payadapters.NewStore(pool)
	catalog, err := paydomain.NewCatalog(payCatalogVersion)
	if err != nil {
		catalog = paydomain.Catalog{}
	}
	preflight := payments.Preflight()
	comp := paymentsComposition{Store: store, Catalog: catalog, Preflight: preflight}

	stripeReady := preflight.Stripe.MaintenanceReady
	asaasReady := preflight.PixManual.MaintenanceReady
	stripeEnv := stripeEnvironment(payments.Stripe.Mode)
	asaasEnv := asaasEnvironment(payments.Asaas.Mode)
	if stripeReady {
		comp.Stripe = payadapters.NewStripeProvider(payments.Stripe.SecretKey, stripeEnv, nil)
		comp.Stripe.Merchant = payments.Stripe.MerchantID
		comp.Checkout = &paymentsapp.Service{
			Store: store, Provider: comp.Stripe, Catalog: catalog, Clock: clock,
			Config: paymentsapp.Config{
				Environment: stripeEnv, MerchantID: payments.Stripe.MerchantID,
				PriceID: payments.Stripe.PriceID, CatalogVersion: catalog.Version,
				ReturnOrigin:    payments.ReturnOrigin,
				NewSalesEnabled: payments.Stripe.NewSalesEnabled,
				LeaseTTL:        payServiceLease,
			},
		}
	}
	if asaasReady {
		comp.Asaas = payadapters.NewAsaasProvider(payments.Asaas.APIKey, asaasEnv, nil)
		comp.Pix = &paymentsapp.PixService{
			Store: store, Provider: comp.Asaas, Catalog: catalog, Clock: clock,
			Config: paymentsapp.PixConfig{
				Environment: asaasEnv, MerchantID: payments.Asaas.MerchantID,
				CatalogVersion:  catalog.Version,
				NewSalesEnabled: payments.Asaas.PixManualNewSalesEnabled,
				LeaseTTL:        payServiceLease,
			},
		}
		comp.PixAuto = &paymentsapp.PixAutoService{
			Store: store, Provider: comp.Asaas, Catalog: catalog, Clock: clock,
			Config: paymentsapp.PixAutoConfig{
				Environment: asaasEnv, MerchantID: payments.Asaas.MerchantID,
				CatalogVersion:  catalog.Version,
				NewSalesEnabled: payments.Asaas.PixAutomaticNewSalesEnabled,
				// Merchant eligibility is an external gate (PAY-12); it stays
				// closed until the authorized eligibility proof lands.
				MerchantEligible: false,
				ConsentVersion:   payConsentVersion, MandateMonths: payMandateMonths,
				LeaseTTL: payServiceLease,
			},
		}
	}

	mux := payadapters.SettlementMux{}
	if comp.Stripe != nil {
		mux.Stripe = comp.Stripe
	}
	if comp.Asaas != nil {
		mux.Asaas = comp.Asaas
	}
	comp.Settlement = &paymentsapp.SettlementService{Store: store, Catalog: catalog, Verifier: mux}
	// Assign providers only when configured: a typed nil inside an
	// interface would bypass the service nil guards.
	comp.Lifecycle = &paymentsapp.LifecycleService{Store: store}
	if comp.Stripe != nil {
		comp.Lifecycle.Verifier = comp.Stripe
	}
	comp.Dispute = &paymentsapp.DisputeService{Store: store, Clock: clock}
	if comp.Stripe != nil {
		comp.Dispute.Stripe = comp.Stripe
	}
	if comp.Asaas != nil {
		comp.Dispute.Asaas = comp.Asaas
	}
	comp.Refunds = &paymentsapp.RefundService{
		Store: store,
		// No production dual-approval matrix is authorized yet: every
		// refund requires a second approver once operator proofs land.
		Config: paymentsapp.RefundServiceConfig{SingleApprovalAboveCents: 0},
		Clock:  clock,
	}
	if comp.Stripe != nil {
		comp.Refunds.Stripe = comp.Stripe
	}
	if comp.Asaas != nil {
		comp.Refunds.Asaas = comp.Asaas
	}
	if comp.Asaas != nil {
		comp.Cycles = &paymentsapp.PixAutoCycleService{
			Store: store, Provider: comp.Asaas, Settlements: comp.Settlement,
			Clock: clock,
			Config: paymentsapp.PixAutoConfig{
				Environment: asaasEnv, MerchantID: payments.Asaas.MerchantID,
				CatalogVersion: catalog.Version, ConsentVersion: payConsentVersion,
				MandateMonths: payMandateMonths, LeaseTTL: payServiceLease,
			},
		}
	}
	// Cross-merchant switch records need per-target merchant routing that is
	// not designed yet: the merchant stays empty so switch requests fail
	// closed while the PAY-14 fences remain proven at service level.
	switchEnv := paydomain.Environment("")
	if stripeReady {
		switchEnv = stripeEnv
	} else if asaasReady {
		switchEnv = asaasEnv
	}
	comp.Switch = &paymentsapp.SwitchService{
		Store: store, Catalog: catalog, PixAuto: comp.PixAuto,
		Clock: clock, WorkerID: "api-payments", LeaseTTL: payServiceLease,
		Config: paymentsapp.SwitchConfig{
			Environment: switchEnv,
			CardReady:   stripeReady, PixAutoReady: preflight.PixAutomatic.MaintenanceReady,
		},
	}
	if comp.Stripe != nil {
		comp.Switch.Stripe = comp.Stripe
	}
	comp.Portal = &paymentsapp.PortalService{Store: store}
	if comp.Stripe != nil {
		comp.Portal.Provider = comp.Stripe
		// No portal configuration ID is admitted by configuration yet.
		comp.Portal.Config = paymentsapp.PortalConfig{
			Environment: stripeEnv, Merchant: payments.Stripe.MerchantID,
			ReturnOrigin: payments.ReturnOrigin,
		}
	}
	comp.Billing = &paymentsapp.BillingExitService{
		Store: store, PixAuto: comp.PixAuto,
		Clock: clock, WorkerID: "api-payments", LeaseTTL: payServiceLease,
	}
	if comp.Stripe != nil {
		comp.Billing.Stripe = comp.Stripe
	}

	comp.StripeHook = paywebhook.StripeWebhookHandler{
		Inbox: store, Secret: payments.Stripe.WebhookSecret,
		Environment: stripeEnv, Merchant: payments.Stripe.MerchantID,
		ExpectedLivemode: stripeEnv == paydomain.EnvironmentProduction,
	}
	comp.AsaasHook = paywebhook.AsaasWebhookHandler{
		Inbox: store, WebhookToken: payments.Asaas.WebhookToken,
		APIKey: payments.Asaas.APIKey, Environment: asaasEnv,
		Merchant: payments.Asaas.MerchantID, RequireTLS: true,
	}
	return comp
}

// registerPaymentRoutes mounts authenticated callback intake. Webhook
// handlers fail closed with 503 while their channel is unconfigured; no
// owner command, portal or purchase surface is mounted here.
func registerPaymentRoutes(r chi.Router, comp paymentsComposition) {
	comp.StripeHook.RegisterRoutes(r)
	comp.AsaasHook.RegisterRoutes(r)
}
