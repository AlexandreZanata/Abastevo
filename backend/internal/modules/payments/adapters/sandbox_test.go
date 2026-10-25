//go:build sandbox

// Sandbox acceptance harness (PAY-21): real Stripe test-mode and Asaas
// sandbox API journeys. Runs ONLY with explicitly supplied test
// credentials; without them every journey skips. Live credentials are
// refused hard before any network use. Nothing identifying is ever
// logged: output carries statuses only, while merchant/environment/
// resource records stay in the operator's private handoff, never in the
// repository.
//
// Required environment (values never logged, never committed):
//
//	ANPFUEL_SANDBOX_STRIPE_SECRET       sk_test_* restricted key
//	ANPFUEL_SANDBOX_STRIPE_PRICE        price_* test price
//	ANPFUEL_SANDBOX_STRIPE_MERCHANT     acct_* test merchant
//	ANPFUEL_SANDBOX_STRIPE_PORTAL       bpc_* portal configuration (optional)
//	ANPFUEL_SANDBOX_STRIPE_SUBSCRIPTION sub_* operator-supplied test subscription (cancel journey)
//	ANPFUEL_SANDBOX_ASAAS_KEY           sandbox API key
//	ANPFUEL_SANDBOX_ASAAS_MERCHANT      sandbox merchant UUID
//
// Run: go test -tags=sandbox -count=1 -run TestSandbox ./internal/modules/payments/adapters/
package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
)

var sandboxRunCounter int64

// sandboxUUID mints a run-unique identifier: separate between runs, while
// the same logical operation reuses its own key for recovery attempts.
func sandboxUUID(t *testing.T, scope string) string {
	t.Helper()
	serial := atomic.AddInt64(&sandboxRunCounter, 1)
	sum := sha256.Sum256([]byte(fmt.Sprintf("sandbox\x00%s\x00%d\x00%d", scope, serial, time.Now().UnixNano())))
	raw := sum[:16]
	raw[6] = raw[6]&0x0f | 0x40
	raw[8] = raw[8]&0x3f | 0x80
	encoded := hex.EncodeToString(raw)
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" +
		encoded[16:20] + "-" + encoded[20:32]
}

func sandboxCredentials(t *testing.T) (sandboxStripeConfig, sandboxAsaasConfig) {
	t.Helper()
	stripe, asaas, err := readSandboxConfig(os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	if !stripe.journeysReady() && !asaas.journeysReady() {
		t.Skip("sandbox credentials absent; set ANPFUEL_SANDBOX_* test credentials to prove provider journeys")
	}
	return stripe, asaas
}

func sandboxStripeProvider(t *testing.T, creds sandboxStripeConfig) *StripeProvider {
	t.Helper()
	if !creds.journeysReady() {
		t.Skip("stripe sandbox credentials absent")
	}
	if !creds.testKey() {
		t.Fatal("stripe sandbox key must be a test-mode key")
	}
	// Test environment is hardcoded: the harness cannot address live.
	return NewStripeProvider(creds.secret, domain.EnvironmentTest, nil)
}

func sandboxAsaasProvider(t *testing.T, creds sandboxAsaasConfig) *AsaasProvider {
	t.Helper()
	if !creds.journeysReady() {
		t.Skip("asaas sandbox credentials absent")
	}
	// Test environment selects the sandbox base URL by construction.
	return NewAsaasProvider(creds.key, domain.EnvironmentTest, nil)
}

func sandboxCheckoutOrder(t *testing.T, accountID string) domain.Order {
	t.Helper()
	offer := domain.Offer{
		ID: "premium.monthly", Family: "premium", Term: domain.TermMonthly, Price: domain.BRL(1234),
		Capabilities: domain.PremiumCapabilityList(),
		Channels:     map[domain.Channel]bool{domain.ChannelCard: true},
		SaleEnabled:  true,
	}
	catalog, err := domain.NewCatalog("pay-catalog-v1", offer)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	quote, err := catalog.Quote(domain.QuoteRequest{OfferID: offer.ID, Channel: domain.ChannelCard}, now)
	if err != nil {
		t.Fatal(err)
	}
	order, err := domain.NewOrder(sandboxUUID(t, "stripe-order"), accountID, quote, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := order.MarkPending(now); err != nil {
		t.Fatal(err)
	}
	return order
}

func sandboxPixOrder(t *testing.T, accountID string) domain.Order {
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
	order, err := domain.NewOrder(sandboxUUID(t, "asaas-order"), accountID, quote, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := order.MarkPending(now); err != nil {
		t.Fatal(err)
	}
	return order
}

func TestSandboxStripeCheckoutRecovery(t *testing.T) {
	stripeCreds, _ := sandboxCredentials(t)
	provider := sandboxStripeProvider(t, stripeCreds)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	accountID := sandboxUUID(t, "stripe-account")

	customer, err := provider.EnsureCustomer(ctx, app.CustomerRequest{
		AccountID: accountID, Environment: domain.EnvironmentTest, MerchantID: stripeCreds.merchant,
	})
	if err != nil || customer.ID == "" {
		t.Fatalf("customer err=%v", classifySandbox(err))
	}
	order := sandboxCheckoutOrder(t, accountID)
	session, err := provider.CreateCheckout(ctx, app.CheckoutRequest{
		Order: order, CustomerID: customer.ID, PriceID: stripeCreds.price,
		MerchantID: stripeCreds.merchant, Environment: domain.EnvironmentTest,
		SuccessURL:     "https://app.example.invalid/billing/checkout/return?session_id={CHECKOUT_SESSION_ID}",
		CancelURL:      "https://app.example.invalid/billing/checkout/cancel",
		IdempotencyKey: "sandbox:checkout:probe",
	})
	if err != nil {
		t.Fatalf("checkout err=%v", classifySandbox(err))
	}
	// Account/customer/order/price/environment binding on the live response.
	if session.CustomerID != customer.ID || session.Livemode ||
		session.Mode != "subscription" || session.ID == "" || session.URL == "" {
		t.Fatal("checkout binding refused")
	}
	// Lost-reply recovery with the same operation key finds the same open
	// session instead of writing a second one.
	found, ok, complete, err := provider.FindCheckout(ctx, customer.ID, order.ID)
	if err != nil || !ok || !complete || found.ID != session.ID {
		t.Fatalf("recovery err=%v", classifySandbox(err))
	}
	t.Log("stripe checkout recovery journey complete")
}

func TestSandboxStripeSubscriptionCancel(t *testing.T) {
	stripeCreds, _ := sandboxCredentials(t)
	provider := sandboxStripeProvider(t, stripeCreds)
	provider.Merchant = stripeCreds.merchant
	if stripeCreds.subscription == "" {
		t.Skip("operator-supplied test subscription required (ANPFUEL_SANDBOX_STRIPE_SUBSCRIPTION); cancel journey not implemented without one")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	before, err := provider.RetrieveLifecycle(ctx, stripeCreds.subscription)
	if err != nil {
		t.Fatalf("retrieve err=%v", classifySandbox(err))
	}
	if before.Environment != domain.EnvironmentTest || before.Merchant != stripeCreds.merchant ||
		before.Provider != domain.ProviderStripe {
		t.Fatal("subscription binding refused")
	}
	if err := provider.CancelRenewal(ctx, stripeCreds.subscription, "sandbox:cancel:probe"); err != nil {
		t.Fatalf("cancel err=%v", classifySandbox(err))
	}
	after, err := provider.RetrieveLifecycle(ctx, stripeCreds.subscription)
	if err != nil {
		t.Fatalf("re-retrieve err=%v", classifySandbox(err))
	}
	if !after.CancelAtPeriodEnd && after.ProviderStatus != "canceled" && after.ProviderStatus != "incomplete_expired" {
		t.Fatal("renewal not fenced after cancel")
	}
	t.Log("stripe subscription cancel journey complete")
}

func TestSandboxStripePortalSession(t *testing.T) {
	stripeCreds, _ := sandboxCredentials(t)
	provider := sandboxStripeProvider(t, stripeCreds)
	if stripeCreds.portal == "" {
		t.Skip("portal configuration absent (ANPFUEL_SANDBOX_STRIPE_PORTAL); portal journey not implemented without one")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	accountID := sandboxUUID(t, "stripe-portal-account")

	customer, err := provider.EnsureCustomer(ctx, app.CustomerRequest{
		AccountID: accountID, Environment: domain.EnvironmentTest, MerchantID: stripeCreds.merchant,
	})
	if err != nil || customer.ID == "" {
		t.Fatalf("customer err=%v", classifySandbox(err))
	}
	url, err := provider.CreatePortalSession(ctx, app.PortalRequest{
		CustomerID: customer.ID, ReturnURL: "https://app.example.invalid/billing/manage",
		ConfigurationID: stripeCreds.portal,
	})
	if err != nil || url == "" {
		t.Fatalf("portal err=%v", classifySandbox(err))
	}
	t.Log("stripe portal journey complete")
}

func TestSandboxAsaasCustomerPaymentAndQR(t *testing.T) {
	_, asaasCreds := sandboxCredentials(t)
	provider := sandboxAsaasProvider(t, asaasCreds)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	accountID := sandboxUUID(t, "asaas-account")

	customer, err := provider.EnsureCustomer(ctx, app.PixCustomerRequest{
		AccountID: accountID, Environment: domain.EnvironmentTest, MerchantID: asaasCreds.merchant,
		Name: "Sandbox Probe", TaxID: "12345678909",
	})
	if err != nil || customer.ID == "" {
		t.Fatalf("customer err=%v", classifySandbox(err))
	}
	order := sandboxPixOrder(t, accountID)
	payment, err := provider.CreatePayment(ctx, app.PixPaymentRequest{
		Order: order, CustomerID: customer.ID,
		Environment: domain.EnvironmentTest, MerchantID: asaasCreds.merchant,
		DueDate: time.Now().Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("payment err=%v", classifySandbox(err))
	}
	if payment.CustomerID != customer.ID || payment.ExternalReference != order.ID ||
		!payment.Amount.Equal(order.Amount) {
		t.Fatal("payment binding refused")
	}
	qr, err := provider.GetQR(ctx, payment.ID)
	if err != nil || qr.Payload == "" {
		t.Fatalf("qr err=%v", classifySandbox(err))
	}
	// Sandbox Pix cannot be paid headlessly: the payment must stay
	// unsettled, proving no fake grant is possible from creation alone.
	retrieved, err := provider.RetrievePayment(ctx, payment.ID)
	if err != nil {
		t.Fatalf("retrieve err=%v", classifySandbox(err))
	}
	if retrieved.ProviderStatus == "RECEIVED" {
		t.Fatal("sandbox payment unexpectedly settled")
	}
	t.Log("asaas customer/payment/qr journey complete")
}

// classifySandbox keeps the failure taxonomy explicit: expected absence,
// authentication, unavailability and anything else stay distinguishable
// instead of collapsing into one opaque error.
func classifySandbox(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return errors.New("expected-absence")
	case errors.Is(err, app.ErrProviderUnauthorized):
		return errors.New("authentication-failed")
	case errors.Is(err, domain.ErrUnavailable):
		return errors.New("provider-unavailable")
	default:
		return err
	}
}
