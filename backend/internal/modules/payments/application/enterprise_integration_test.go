//go:build integration

package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	payadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
	paydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
	subadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/adapters"
	subdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
)

const enterpriseTestOffer = "enterprise.regional_brief.v1"

func candidateCatalog(t *testing.T) paydomain.Catalog {
	t.Helper()
	catalog, err := paydomain.NewCatalog("pay-catalog-v1", paydomain.Offer{
		ID: testOffer, Family: "premium", Term: paydomain.TermMonthly, Price: paydomain.BRL(1234),
		Capabilities: paydomain.PremiumCapabilityList(),
		Channels:     map[paydomain.Channel]bool{paydomain.ChannelCard: true, paydomain.ChannelPixManual: true, paydomain.ChannelPixAutomatic: true},
		SaleEnabled:  true,
	}, paydomain.Offer{
		ID: enterpriseTestOffer, Family: "enterprise", Term: paydomain.TermMonthly,
		Capabilities: []string{},
		Channels:     map[paydomain.Channel]bool{paydomain.ChannelCard: true, paydomain.ChannelPixManual: true, paydomain.ChannelPixAutomatic: true},
		SaleEnabled:  false, Enterprise: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

// approvedEnterpriseCatalog is a synthetic, explicitly approved enterprise
// offer. It can only exist in tests: production catalog validation refuses to
// admit an enterprise offer for sale.
func approvedEnterpriseCatalog() paydomain.Catalog {
	approved := paydomain.Offer{
		ID: "enterprise.regional_brief.approved", Family: "enterprise", Term: paydomain.TermMonthly,
		Price: paydomain.BRL(9900), Capabilities: []string{},
		Channels: map[paydomain.Channel]bool{
			paydomain.ChannelCard: true, paydomain.ChannelPixManual: true, paydomain.ChannelPixAutomatic: true,
		},
		SaleEnabled: true, Enterprise: true,
	}
	return paydomain.Catalog{Version: "pay-catalog-v1", Offers: map[string]paydomain.Offer{
		testOffer: {
			ID: testOffer, Family: "premium", Term: paydomain.TermMonthly, Price: paydomain.BRL(1234),
			Capabilities: paydomain.PremiumCapabilityList(),
			Channels:     map[paydomain.Channel]bool{paydomain.ChannelCard: true, paydomain.ChannelPixManual: true, paydomain.ChannelPixAutomatic: true},
			SaleEnabled:  true,
		},
		approved.ID: approved,
	}}
}

func enterpriseCheckoutService(t *testing.T, store *payadapters.Store, catalog paydomain.Catalog, provider *fakeProvider) *app.Service {
	t.Helper()
	return &app.Service{
		Store: store, Provider: provider, Catalog: catalog,
		Config: app.Config{
			Environment: paydomain.EnvironmentTest, MerchantID: "acct_synthetic", PriceID: "price_synthetic",
			CatalogVersion: "pay-catalog-v1", ReturnOrigin: "https://app.example.invalid",
			NewSalesEnabled: true, LeaseTTL: time.Minute,
		},
	}
}

func TestEnterpriseUnapprovedOfferRefusedEverywhere(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	checkout := enterpriseCheckoutService(t, store, candidateCatalog(t), newFakeProvider())
	if _, err := checkout.Start(context.Background(), app.Principal{AccountID: owner}, app.StartRequest{
		OperationID: settlementTestID(t), OfferID: enterpriseTestOffer,
	}); !errors.Is(err, paydomain.ErrPriceUnresolved) {
		t.Fatalf("checkout err=%v", err)
	}
	pix := &app.PixService{
		Store: store, Provider: newFakePixProvider(), Catalog: candidateCatalog(t),
		Config: app.PixConfig{
			Environment: paydomain.EnvironmentTest, MerchantID: refundMerchant,
			CatalogVersion: "pay-catalog-v1", NewSalesEnabled: true, LeaseTTL: time.Minute,
		},
	}
	if _, err := pix.Start(context.Background(), app.Principal{AccountID: owner}, app.PixStartRequest{
		OperationID: settlementTestID(t), OfferID: enterpriseTestOffer,
	}); !errors.Is(err, paydomain.ErrPriceUnresolved) {
		t.Fatalf("pix err=%v", err)
	}
	autoProvider := newFakePixAutoProvider()
	auto := newPixAutoService(t, store, autoProvider)
	auto.Catalog = candidateCatalog(t)
	if _, err := auto.Start(context.Background(), app.Principal{AccountID: owner}, app.PixAutoStartRequest{
		OperationID: settlementTestID(t), OfferID: enterpriseTestOffer,
		PayerName: "Synthetic Payer", PayerTaxID: "000.000.000-00", Consent: true,
	}); !errors.Is(err, paydomain.ErrPriceUnresolved) {
		t.Fatalf("pix auto err=%v", err)
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_orders") != 0 {
		t.Fatal("refused enterprise candidate created an order")
	}
	if autoProvider.createCount != 0 {
		t.Fatal("refused enterprise candidate reached the provider")
	}
}

func TestEnterpriseApprovedSelfPurchaseEndToEnd(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	catalog := approvedEnterpriseCatalog()
	offerID := "enterprise.regional_brief.approved"
	checkoutProvider := newFakeProvider()
	checkout := enterpriseCheckoutService(t, store, catalog, checkoutProvider)
	checkoutOut, err := checkout.Start(context.Background(), app.Principal{AccountID: owner}, app.StartRequest{
		OperationID: settlementTestID(t), OfferID: offerID,
	})
	if err != nil || checkoutOut.OrderID == "" {
		t.Fatalf("checkout=%+v err=%v", checkoutOut, err)
	}
	if checkoutProvider.createCount != 1 {
		t.Fatalf("provider creates=%d", checkoutProvider.createCount)
	}
	if checkoutProvider.last.Order.OfferID != offerID || !checkoutProvider.last.Order.Amount.Equal(paydomain.BRL(9900)) {
		t.Fatalf("bound order=%+v", checkoutProvider.last.Order)
	}
	autoProvider := newFakePixAutoProvider()
	auto := newPixAutoService(t, store, autoProvider)
	auto.Catalog = catalog
	auth, err := auto.Start(context.Background(), app.Principal{AccountID: owner}, app.PixAutoStartRequest{
		OperationID: settlementTestID(t), OfferID: offerID,
		PayerName: "Synthetic Payer", PayerTaxID: "000.000.000-00", Consent: true,
	})
	if err != nil || !auth.Value.Equal(paydomain.BRL(9900)) {
		t.Fatalf("auth=%+v err=%v", auth, err)
	}
	if autoProvider.createCount != 1 {
		t.Fatalf("auto provider creates=%d", autoProvider.createCount)
	}
}

func TestEnterprisePremiumProductConfusionRejected(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	catalog := approvedEnterpriseCatalog()
	offerID := "enterprise.regional_brief.approved"
	now := time.Now().UTC().Truncate(time.Microsecond)
	quote, err := catalog.Quote(paydomain.QuoteRequest{OfferID: offerID, Channel: paydomain.ChannelPixManual}, now)
	if err != nil {
		t.Fatal(err)
	}
	order, err := paydomain.NewOrder(settlementTestID(t), owner, quote, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := order.MarkPending(now); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateOrder(context.Background(), order); err != nil {
		t.Fatal(err)
	}
	if err := store.MapCustomer(context.Background(), payadapters.CustomerMapping{
		AccountID: owner, Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
		Merchant: refundMerchant, RemoteCustomerID: "cus_" + owner,
	}); err != nil {
		t.Fatal(err)
	}
	claim, err := store.ClaimAttempt(context.Background(), payadapters.ClaimAttemptInput{
		AccountID: owner, Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
		Merchant: refundMerchant, OperationKey: "enterprise-confusion", Kind: "create_attempt",
		PayloadHash: strings.Repeat("d", 64), OrderID: order.ID, LeaseUntil: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MarkDispatched(context.Background(), claim); err != nil {
		t.Fatal(err)
	}
	service := &app.SettlementService{Store: store, Catalog: catalog}
	err = service.Settle(context.Background(), app.SettlementCandidate{
		OrderID: order.ID, Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
		Merchant: refundMerchant, Channel: paydomain.ChannelPixManual,
		CustomerID: "cus_" + owner, RemotePaymentID: "pay_enterprise_confusion",
		ProviderStatus: "RECEIVED", Amount: paydomain.BRL(1234), SettledAt: now.Add(time.Minute),
	})
	if !errors.Is(err, paydomain.ErrBinding) {
		t.Fatalf("premium payment into enterprise order err=%v", err)
	}
}

func TestEnterpriseSettlementRequiresCatalogApproval(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	catalog := approvedEnterpriseCatalog()
	offerID := "enterprise.regional_brief.approved"
	now := time.Now().UTC().Truncate(time.Microsecond)
	quote, err := catalog.Quote(paydomain.QuoteRequest{OfferID: offerID, Channel: paydomain.ChannelPixManual}, now)
	if err != nil {
		t.Fatal(err)
	}
	order, err := paydomain.NewOrder(settlementTestID(t), owner, quote, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(order.Capabilities) != 0 {
		t.Fatalf("enterprise order must carry no capabilities, got %v", order.Capabilities)
	}
	if err := order.MarkPending(now); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateOrder(context.Background(), order); err != nil {
		t.Fatal(err)
	}
	if err := store.MapCustomer(context.Background(), payadapters.CustomerMapping{
		AccountID: owner, Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
		Merchant: refundMerchant, RemoteCustomerID: "cus_" + owner,
	}); err != nil {
		t.Fatal(err)
	}
	claim, err := store.ClaimAttempt(context.Background(), payadapters.ClaimAttemptInput{
		AccountID: owner, Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
		Merchant: refundMerchant, OperationKey: "enterprise-grant", Kind: "create_attempt",
		PayloadHash: strings.Repeat("d", 64), OrderID: order.ID, LeaseUntil: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MarkDispatched(context.Background(), claim); err != nil {
		t.Fatal(err)
	}
	service := &app.SettlementService{Store: store, Catalog: catalog}
	err = service.Settle(context.Background(), app.SettlementCandidate{
		OrderID: order.ID, Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
		Merchant: refundMerchant, Channel: paydomain.ChannelPixManual,
		CustomerID: "cus_" + owner, RemotePaymentID: "pay_enterprise_grant",
		ProviderStatus: "RECEIVED", Amount: paydomain.BRL(9900), SettledAt: now.Add(time.Minute),
	})
	if !errors.Is(err, paydomain.ErrSaleDisabled) {
		t.Fatalf("enterprise settlement must stay refused until catalog approval, got %v", err)
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_grants WHERE order_id=$1", order.ID) != 0 {
		t.Fatal("refused enterprise settlement created a grant")
	}
	subStore := subadapters.NewStore(pool)
	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if err := subStore.RequireCapability(context.Background(), tx, owner, "cloud.sync"); !errors.Is(err, subdomain.ErrDenied) {
		t.Fatalf("enterprise order must not unlock premium capability, err=%v", err)
	}
}

func TestBillingHistoryExposesProviderReferenceScopedToOwner(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	foreign := seedCheckoutOwner(t, pool)
	refundGrant(t, pool, store, owner, "pay_history_ref")
	service := &app.BillingExitService{Store: store}
	rows, err := service.History(context.Background(), app.Principal{AccountID: owner}, 10)
	if err != nil || len(rows) == 0 {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
	found := false
	for _, row := range rows {
		if row.RemotePaymentID == "pay_history_ref" && row.OfferID == testOffer && row.SettlementState == "SETTLED" {
			found = true
		}
	}
	if !found {
		t.Fatal("provider payment reference missing from owner history")
	}
	foreignRows, err := service.History(context.Background(), app.Principal{AccountID: foreign}, 10)
	if err != nil || len(foreignRows) != 0 {
		t.Fatalf("foreign rows=%+v err=%v", foreignRows, err)
	}
}
