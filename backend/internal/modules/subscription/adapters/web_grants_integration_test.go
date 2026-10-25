//go:build integration

package adapters

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	payadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/adapters"
	paydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
	subapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/application"
	subdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
)

func TestWebGrantFundsCapabilityWithoutPlay(t *testing.T) {
	pool := freshDB(t)
	payStore := payadapters.NewStore(pool)
	subStore := NewStore(pool)
	owner := seedOwner(t, pool)

	catalog, err := paydomain.NewCatalog("pay-catalog-v1", paydomain.Offer{
		ID: "premium.monthly", Family: "premium", Term: paydomain.TermMonthly, Price: paydomain.BRL(1234),
		Capabilities: paydomain.PremiumCapabilityList(),
		Channels:     map[paydomain.Channel]bool{paydomain.ChannelPixManual: true}, SaleEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	quote, err := catalog.Quote(paydomain.QuoteRequest{OfferID: "premium.monthly", Channel: paydomain.ChannelPixManual}, now)
	if err != nil {
		t.Fatal(err)
	}
	order, err := paydomain.NewOrder(newid(t), owner.AccountID, quote, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := order.MarkPending(now); err != nil {
		t.Fatal(err)
	}
	if err := payStore.CreateOrder(context.Background(), order); err != nil {
		t.Fatal(err)
	}
	customerID := "cus_" + owner.AccountID
	if err := payStore.MapCustomer(context.Background(), payadapters.CustomerMapping{
		AccountID: owner.AccountID, Provider: paydomain.ProviderAsaas,
		Environment: paydomain.EnvironmentTest, Merchant: "merchant_1", RemoteCustomerID: customerID,
	}); err != nil {
		t.Fatal(err)
	}
	claim, err := payStore.ClaimAttempt(context.Background(), payadapters.ClaimAttemptInput{
		AccountID: owner.AccountID, Provider: paydomain.ProviderAsaas,
		Environment: paydomain.EnvironmentTest, Merchant: "merchant_1",
		OperationKey: "settle-web-grant", Kind: "create_attempt",
		PayloadHash: strings.Repeat("a", 64), OrderID: order.ID, LeaseUntil: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := payStore.MarkDispatched(context.Background(), claim); err != nil {
		t.Fatal(err)
	}
	settledAt := time.Now().UTC()
	if _, err := payStore.RecordSettlement(context.Background(), payadapters.SettlementInput{
		Offer: catalog.Offers["premium.monthly"],
		Settlement: paydomain.Settlement{
			ID: newid(t), OrderID: order.ID, AttemptID: claim.AttemptID, AccountID: owner.AccountID,
			OfferID: order.OfferID, Channel: paydomain.ChannelPixManual, Provider: paydomain.ProviderAsaas,
			Environment: paydomain.EnvironmentTest, MerchantID: "merchant_1", RemotePaymentID: "pay_" + newid(t),
			ProviderStatus: "RECEIVED", Amount: paydomain.BRL(1234), Status: paydomain.SettlementSettled,
			Authority: paydomain.AuthorityProviderVerified, SettledAt: settledAt,
		},
	}); err != nil {
		t.Fatal(err)
	}

	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if err := subStore.RequireCapability(context.Background(), tx, owner.AccountID, "cloud.sync"); err != nil {
		t.Fatalf("web capability denied: %v", err)
	}
	if err := subStore.RequireCapability(context.Background(), tx, owner.AccountID, "not.a.capability"); !errors.Is(err, subdomain.ErrDenied) {
		t.Fatalf("unknown capability err=%v", err)
	}
	tx.Rollback(context.Background())

	snapshot, err := subStore.Status(context.Background(), owner)
	if err != nil || snapshot == nil || !slices.Contains(snapshot.WebCapabilities, "cloud.sync") {
		t.Fatalf("snapshot=%+v err=%v", snapshot, err)
	}
	service := &subapp.Service{Store: subStore}
	status, err := service.Status(context.Background(), owner)
	if err != nil || !status.PaymentsConfigured || !slices.Contains(status.Capabilities, "cloud.sync") ||
		!status.RetainedDataAccess {
		t.Fatalf("status=%+v err=%v", status, err)
	}
	if service.Configured() {
		t.Fatal("Play unexpectedly configured")
	}

	if err := payStore.RevokeGrant(context.Background(), order.ID, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	tx, err = pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if err := subStore.RequireCapability(context.Background(), tx, owner.AccountID, "cloud.sync"); !errors.Is(err, subdomain.ErrDenied) {
		t.Fatalf("revoked capability err=%v", err)
	}
	tx.Rollback(context.Background())
	status, err = service.Status(context.Background(), owner)
	if err != nil || status.PaymentsConfigured || len(status.Capabilities) != 0 || !status.RetainedDataAccess {
		t.Fatalf("revoked status=%+v err=%v", status, err)
	}
}

func createWebGrant(t *testing.T, payStore *payadapters.Store, ownerID, operationKey string) string {
	t.Helper()
	catalog, err := paydomain.NewCatalog("pay-catalog-v1", paydomain.Offer{
		ID: "premium.monthly", Family: "premium", Term: paydomain.TermMonthly, Price: paydomain.BRL(1234),
		Capabilities: paydomain.PremiumCapabilityList(),
		Channels:     map[paydomain.Channel]bool{paydomain.ChannelPixManual: true}, SaleEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	quote, err := catalog.Quote(paydomain.QuoteRequest{OfferID: "premium.monthly", Channel: paydomain.ChannelPixManual}, now)
	if err != nil {
		t.Fatal(err)
	}
	order, err := paydomain.NewOrder(newid(t), ownerID, quote, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := order.MarkPending(now); err != nil {
		t.Fatal(err)
	}
	if err := payStore.CreateOrder(context.Background(), order); err != nil {
		t.Fatal(err)
	}
	customerID := "cus_" + ownerID
	if err := payStore.MapCustomer(context.Background(), payadapters.CustomerMapping{
		AccountID: ownerID, Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
		Merchant: "merchant_1", RemoteCustomerID: customerID,
	}); err != nil {
		t.Fatal(err)
	}
	claim, err := payStore.ClaimAttempt(context.Background(), payadapters.ClaimAttemptInput{
		AccountID: ownerID, Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
		Merchant: "merchant_1", OperationKey: operationKey, Kind: "create_attempt",
		PayloadHash: strings.Repeat("b", 64), OrderID: order.ID, LeaseUntil: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := payStore.MarkDispatched(context.Background(), claim); err != nil {
		t.Fatal(err)
	}
	settledAt := time.Now().UTC()
	if _, err := payStore.RecordSettlement(context.Background(), payadapters.SettlementInput{
		Offer: catalog.Offers["premium.monthly"],
		Settlement: paydomain.Settlement{
			ID: newid(t), OrderID: order.ID, AttemptID: claim.AttemptID, AccountID: ownerID,
			OfferID: order.OfferID, Channel: paydomain.ChannelPixManual, Provider: paydomain.ProviderAsaas,
			Environment: paydomain.EnvironmentTest, MerchantID: "merchant_1", RemotePaymentID: "pay_" + newid(t),
			ProviderStatus: "RECEIVED", Amount: paydomain.BRL(1234), Status: paydomain.SettlementSettled,
			Authority: paydomain.AuthorityProviderVerified, SettledAt: settledAt,
		},
	}); err != nil {
		t.Fatal(err)
	}
	return order.ID
}

func TestWebGrantRevocationKeepsUnrelatedGrant(t *testing.T) {
	pool := freshDB(t)
	payStore := payadapters.NewStore(pool)
	subStore := NewStore(pool)
	first := seedOwner(t, pool)
	second := seedOwner(t, pool)
	firstOrder := createWebGrant(t, payStore, first.AccountID, "grant-first")
	createWebGrant(t, payStore, second.AccountID, "grant-second")
	if err := payStore.RevokeGrant(context.Background(), firstOrder, time.Now()); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if err := subStore.RequireCapability(context.Background(), tx, first.AccountID, "cloud.sync"); !errors.Is(err, subdomain.ErrDenied) {
		t.Fatalf("revoked owner capability err=%v", err)
	}
	if err := subStore.RequireCapability(context.Background(), tx, second.AccountID, "cloud.sync"); err != nil {
		t.Fatalf("unrelated grant denied: %v", err)
	}
}
