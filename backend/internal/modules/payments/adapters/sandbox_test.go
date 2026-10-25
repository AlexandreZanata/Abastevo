//go:build sandbox

// Sandbox acceptance harness (PAY-21): real Stripe test-mode and Asaas
// sandbox API journeys. Runs ONLY with explicitly supplied test
// credentials; without them every test skips. Live credentials are
// refused hard. Nothing identifying is ever logged: output carries pass
// counts and statuses only, while merchant/environment/resource records
// stay in the operator's private handoff, never in the repository.
//
// Required environment (values never logged, never committed):
//
//	ANPFUEL_SANDBOX_STRIPE_SECRET   sk_test_* restricted key
//	ANPFUEL_SANDBOX_STRIPE_PRICE    price_* test price (BRL, exact amount)
//	ANPFUEL_SANDBOX_STRIPE_MERCHANT acct_* test merchant
//	ANPFUEL_SANDBOX_STRIPE_PORTAL   bpc_* portal configuration (optional)
//	ANPFUEL_SANDBOX_ASAAS_KEY       sandbox API key
//	ANPFUEL_SANDBOX_ASAAS_MERCHANT  sandbox merchant UUID
//
// Run: go test -tags=sandbox -count=1 -run TestSandbox ./internal/modules/payments/adapters/
package adapters

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
)

type sandboxStripe struct {
	secret   string
	price    string
	merchant string
	portal   string
}

type sandboxAsaas struct {
	key      string
	merchant string
}

func sandboxCredentials(t *testing.T) (sandboxStripe, sandboxAsaas, bool) {
	t.Helper()
	stripe := sandboxStripe{
		secret:   os.Getenv("ANPFUEL_SANDBOX_STRIPE_SECRET"),
		price:    os.Getenv("ANPFUEL_SANDBOX_STRIPE_PRICE"),
		merchant: os.Getenv("ANPFUEL_SANDBOX_STRIPE_MERCHANT"),
		portal:   os.Getenv("ANPFUEL_SANDBOX_STRIPE_PORTAL"),
	}
	asaas := sandboxAsaas{
		key:      os.Getenv("ANPFUEL_SANDBOX_ASAAS_KEY"),
		merchant: os.Getenv("ANPFUEL_SANDBOX_ASAAS_MERCHANT"),
	}
	for _, value := range []string{stripe.secret, stripe.price, stripe.merchant, stripe.portal, asaas.key, asaas.merchant} {
		if strings.HasPrefix(value, "sk_live_") || strings.HasPrefix(value, "rk_live_") {
			t.Fatal("live credentials are refused by the sandbox harness")
		}
	}
	stripeOK := stripe.secret != "" && stripe.price != "" && stripe.merchant != ""
	asaasOK := asaas.key != "" && asaas.merchant != ""
	if !stripeOK && !asaasOK {
		t.Skip("sandbox credentials absent; set ANPFUEL_SANDBOX_* test credentials to prove provider journeys")
	}
	return stripe, asaas, true
}

func sandboxStripeProvider(t *testing.T, creds sandboxStripe) *StripeProvider {
	t.Helper()
	if creds.secret == "" || creds.price == "" || creds.merchant == "" {
		t.Skip("stripe sandbox credentials absent")
	}
	if !strings.HasPrefix(creds.secret, "sk_test_") && !strings.HasPrefix(creds.secret, "rk_test_") {
		t.Fatal("stripe sandbox key must be a test-mode key")
	}
	// Test environment is hardcoded: the harness cannot address live.
	return NewStripeProvider(creds.secret, domain.EnvironmentTest, nil)
}

func sandboxAsaasProvider(t *testing.T, creds sandboxAsaas) *AsaasProvider {
	t.Helper()
	if creds.key == "" || creds.merchant == "" {
		t.Skip("asaas sandbox credentials absent")
	}
	// Test environment selects the sandbox base URL by construction.
	return NewAsaasProvider(creds.key, domain.EnvironmentTest, nil)
}

func TestSandboxStripeCustomerCheckoutAndRecovery(t *testing.T) {
	stripeCreds, _, _ := sandboxCredentials(t)
	provider := sandboxStripeProvider(t, stripeCreds)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	accountID := "sandbox-owner-probe"

	customer, err := provider.EnsureCustomer(ctx, app.CustomerRequest{
		AccountID: accountID, Environment: domain.EnvironmentTest, MerchantID: stripeCreds.merchant,
	})
	if err != nil || customer.ID == "" {
		t.Fatalf("customer err=%v", err)
	}
	order := testOrder(t)
	session, err := provider.CreateCheckout(ctx, app.CheckoutRequest{
		Order: order, CustomerID: customer.ID, PriceID: stripeCreds.price,
		MerchantID: stripeCreds.merchant, Environment: domain.EnvironmentTest,
		SuccessURL:     "https://app.example.invalid/billing/checkout/return?session_id={CHECKOUT_SESSION_ID}",
		CancelURL:      "https://app.example.invalid/billing/checkout/cancel",
		IdempotencyKey: "sandbox:checkout:probe",
	})
	if err != nil || session.ID == "" || session.URL == "" || session.Livemode {
		t.Fatalf("session err=%v", err)
	}
	// Lost-reply recovery finds the same open session without a second write.
	found, ok, complete, err := provider.FindCheckout(ctx, customer.ID, order.ID)
	if err != nil || !ok || !complete || found.ID != session.ID {
		t.Fatalf("recovery err=%v", err)
	}
	t.Log("stripe customer/checkout/recovery: 3 calls, 0 charges")
}

func TestSandboxStripeSubscriptionCancelAndPortal(t *testing.T) {
	stripeCreds, _, _ := sandboxCredentials(t)
	provider := sandboxStripeProvider(t, stripeCreds)
	provider.Merchant = stripeCreds.merchant
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	fact, err := provider.RetrieveLifecycle(ctx, "sub_sandbox_probe")
	if err == nil {
		t.Fatalf("probe subscription must not exist: %+v", fact)
	}
	if stripeCreds.portal == "" {
		t.Skip("portal configuration absent; subscription cancel path needs a probe subscription")
	}
	url, err := provider.CreatePortalSession(ctx, app.PortalRequest{
		CustomerID: "cus_sandbox_probe", ReturnURL: "https://app.example.invalid/billing/manage",
		ConfigurationID: stripeCreds.portal,
	})
	if err != nil || url == "" {
		t.Fatalf("portal err=%v", err)
	}
	t.Log("stripe portal: 1 call")
}

func sandboxPixOrder(t *testing.T) domain.Order {
	t.Helper()
	offer := domain.Offer{
		ID: "premium.monthly", Family: "premium", Term: domain.TermMonthly, Price: domain.BRL(1234),
		Capabilities: domain.PremiumCapabilityList(),
		Channels:     map[domain.Channel]bool{domain.ChannelPixManual: true},
		SaleEnabled:  true,
	}
	catalog, err := domain.NewCatalog("pay-catalog-v1", offer)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	quote, err := catalog.Quote(domain.QuoteRequest{OfferID: offer.ID, Channel: domain.ChannelPixManual}, now)
	if err != nil {
		t.Fatal(err)
	}
	order, err := domain.NewOrder("66666666-6666-4666-8666-666666666666", "sandbox-owner-probe", quote, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := order.MarkPending(now); err != nil {
		t.Fatal(err)
	}
	return order
}

func TestSandboxAsaasCustomerPaymentAndQR(t *testing.T) {
	_, asaasCreds, _ := sandboxCredentials(t)
	provider := sandboxAsaasProvider(t, asaasCreds)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	accountID := "sandbox-owner-probe"

	customer, err := provider.EnsureCustomer(ctx, app.PixCustomerRequest{
		AccountID: accountID, Environment: domain.EnvironmentTest, MerchantID: asaasCreds.merchant,
		Name: "Sandbox Probe", TaxID: "12345678909",
	})
	if err != nil || customer.ID == "" {
		t.Fatalf("customer err=%v", err)
	}
	payment, err := provider.CreatePayment(ctx, app.PixPaymentRequest{
		Order: sandboxPixOrder(t), CustomerID: customer.ID,
		Environment: domain.EnvironmentTest, MerchantID: asaasCreds.merchant,
		DueDate: time.Now().Add(24 * time.Hour),
	})
	if err != nil || payment.ID == "" {
		t.Fatalf("payment err=%v", err)
	}
	if !payment.Amount.Equal(domain.BRL(1234)) {
		t.Fatalf("amount=%v", payment.Amount)
	}
	qr, err := provider.GetQR(ctx, payment.ID)
	if err != nil || qr.Payload == "" {
		t.Fatalf("qr err=%v", err)
	}
	// Sandbox Pix cannot be paid headlessly: the payment must stay
	// unsettled, proving no fake grant is possible from creation alone.
	retrieved, err := provider.RetrievePayment(ctx, payment.ID)
	if err != nil {
		t.Fatalf("retrieve err=%v", err)
	}
	if retrieved.ProviderStatus == "RECEIVED" {
		t.Fatal("sandbox payment unexpectedly settled")
	}
	t.Log("asaas customer/payment/qr/retrieve: 4 calls, 0 settlements")
}
