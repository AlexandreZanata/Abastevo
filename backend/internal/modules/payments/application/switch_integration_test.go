//go:build integration

package application_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	payadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
	paydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

const switchMerchant = "11111111-1111-4111-8111-111111111111"

type fakeStripeRenewal struct {
	mu          sync.Mutex
	cancelErr   error
	fact        app.LifecycleFact
	fenced      bool
	cancelCount int
}

func (f *fakeStripeRenewal) CancelRenewal(_ context.Context, _ string, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cancelCount++
	if f.cancelErr != nil {
		return f.cancelErr
	}
	f.fenced = true
	return nil
}

func (f *fakeStripeRenewal) RetrieveLifecycle(_ context.Context, _ string) (app.LifecycleFact, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fact.RemoteSubscriptionID == "" {
		return app.LifecycleFact{}, paydomain.ErrUnavailable
	}
	fact := f.fact
	fact.CancelAtPeriodEnd = f.fenced
	return fact, nil
}

func switchCatalog(t *testing.T) paydomain.Catalog {
	t.Helper()
	catalog, err := paydomain.NewCatalog("pay-catalog-v1", paydomain.Offer{
		ID: testOffer, Family: "premium", Term: paydomain.TermMonthly, Price: paydomain.BRL(1234),
		Capabilities: paydomain.PremiumCapabilityList(),
		Channels: map[paydomain.Channel]bool{
			paydomain.ChannelCard: true, paydomain.ChannelPixManual: true,
			paydomain.ChannelPixAutomatic: true,
		},
		SaleEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func seedCardContract(t *testing.T, store *payadapters.Store, owner, remoteID string) {
	t.Helper()
	now := time.Now().UTC()
	if err := store.UpsertContract(context.Background(), app.ContractRecord{
		ID: deterministicTestUUID("contract-" + remoteID), AccountID: owner, OfferID: testOffer,
		Source: paydomain.ChannelCard, Environment: paydomain.EnvironmentTest, Merchant: switchMerchant,
		Provider: paydomain.ProviderStripe, RemoteSubscriptionID: remoteID,
		RemoteCustomerID: "cus_" + owner, ProviderStatus: "active", LastEventAt: now,
	}); err != nil {
		t.Fatal(err)
	}
}

func deterministicTestUUID(value string) string {
	// Fixed, test-only UUIDs are acceptable in disposable fixtures.
	switch value {
	case "contract-sub_1":
		return "11111111-2222-4333-8444-555555555555"
	case "contract-sub_2":
		return "66666666-7777-4888-8999-000000000000"
	default:
		return "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
	}
}

func newSwitchService(t *testing.T, store *payadapters.Store, pixAuto *app.PixAutoService, fencer *fakeStripeRenewal) *app.SwitchService {
	t.Helper()
	return &app.SwitchService{
		Store: store, Catalog: switchCatalog(t),
		Config: app.SwitchConfig{
			Environment:    paydomain.EnvironmentTest,
			CardMerchantID: switchMerchant, PixAutoMerchantID: switchMerchant,
			CardReady: true, PixAutoReady: true,
		},
		Stripe: fencer, PixAuto: pixAuto, WorkerID: "switch-test", LeaseTTL: time.Minute,
	}
}

func seedManualGrant(t *testing.T, pool *pgxpool.Pool, store *payadapters.Store, owner string) (time.Time, paydomain.Order) {
	t.Helper()
	order, _, customerID := settlementOrder(t, store, owner, "manual-grant")
	event := settlementInbox(t, store, "evt_manual_grant", "pay_manual_grant")
	service := settlementService(store, &fakeSettlementVerifier{
		candidate: settlementCandidate(order, owner, customerID, "pay_manual_grant"),
	})
	if err := service.Process(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	end, err := store.ActiveGrantEnd(context.Background(), owner, testOffer)
	if err != nil || end.IsZero() {
		t.Fatalf("grant end=%v err=%v", end, err)
	}
	_ = pool
	return end, order
}

func TestSourceSwitchCardToAutomaticFencesAndWaitsForBoundary(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	manualEnd, _ := seedManualGrant(t, pool, store, owner)
	seedCardContract(t, store, owner, "sub_1")
	provider := newFakePixAutoProvider()
	pixAuto := newPixAutoService(t, store, provider)
	pixAuto.Catalog = switchCatalog(t)
	fencer := &fakeStripeRenewal{fact: app.LifecycleFact{
		AccountID: owner, OfferID: testOffer, Environment: paydomain.EnvironmentTest,
		Merchant: switchMerchant, Provider: paydomain.ProviderStripe,
		RemoteSubscriptionID: "sub_1", RemoteCustomerID: "cus_" + owner,
		ProviderStatus: "active", LastEventAt: time.Now().UTC(),
	}}
	switcher := newSwitchService(t, store, pixAuto, fencer)
	record, err := switcher.Request(context.Background(), app.Principal{AccountID: owner}, app.SwitchRequest{
		OperationID: "99999999-9999-4999-8999-999999999999", OfferID: testOffer,
		NewChannel: paydomain.ChannelPixAutomatic,
	})
	if err != nil || record.State != app.SourceSwitchReady || record.OldCancellation != paydomain.CancellationConfirmed ||
		!record.FirstDebitAt.Equal(manualEnd) {
		t.Fatalf("record=%+v err=%v", record, err)
	}
	contract, err := store.ContractForAccount(context.Background(), owner, testOffer)
	if err != nil || !contract.CancelAtPeriodEnd {
		t.Fatalf("contract=%+v err=%v", contract, err)
	}
	if _, err := pixAuto.Start(context.Background(), app.Principal{AccountID: owner}, pixAutoRequest()); !errors.Is(err, paydomain.ErrBusy) {
		t.Fatalf("early start err=%v", err)
	}
	if provider.createCount != 0 {
		t.Fatal("early activation called provider")
	}
	pixAuto.Clock = func() time.Time { return manualEnd.Add(time.Second) }
	if _, err := pixAuto.Start(context.Background(), app.Principal{AccountID: owner}, pixAutoRequest()); err != nil {
		t.Fatal(err)
	}
	if provider.createCount != 1 {
		t.Fatalf("provider creates=%d", provider.createCount)
	}
}

func TestSourceSwitchAutomaticToCardFencesBeforeCheckout(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	provider := newFakePixAutoProvider()
	pixAuto := newPixAutoService(t, store, provider)
	pixAuto.Catalog = switchCatalog(t)
	activePixAuto(t, pixAuto, provider, owner)

	checkoutProvider := newFakeProvider()
	checkout := &app.Service{
		Store: store, Provider: checkoutProvider, Catalog: switchCatalog(t),
		Config: app.Config{
			Environment: paydomain.EnvironmentTest, MerchantID: switchMerchant,
			PriceID: "price_synthetic", CatalogVersion: "pay-catalog-v1",
			ReturnOrigin: "https://app.example.invalid", NewSalesEnabled: true, LeaseTTL: time.Minute,
		},
	}
	if _, err := checkout.Start(context.Background(), app.Principal{AccountID: owner}, app.StartRequest{
		OperationID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", OfferID: testOffer,
	}); !errors.Is(err, paydomain.ErrConflict) {
		t.Fatalf("active pix auto checkout err=%v", err)
	}
	if checkoutProvider.createCount != 0 {
		t.Fatal("checkout ran while Pix Automatic was active")
	}
	switcher := newSwitchService(t, store, pixAuto, &fakeStripeRenewal{})
	record, err := switcher.Request(context.Background(), app.Principal{AccountID: owner}, app.SwitchRequest{
		OperationID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", OfferID: testOffer,
		NewChannel: paydomain.ChannelCard,
	})
	if err != nil || record.State != app.SourceSwitchReady || record.OldSource != paydomain.ChannelPixAutomatic {
		t.Fatalf("record=%+v err=%v", record, err)
	}
	mandate, err := store.MandateForAccount(context.Background(), owner, testOffer)
	if err != nil || mandate.Status != "CANCELLED" {
		t.Fatalf("mandate=%+v err=%v", mandate, err)
	}
	if _, err := checkout.Start(context.Background(), app.Principal{AccountID: owner}, app.StartRequest{
		OperationID: "cccccccc-cccc-4ccc-8ccc-cccccccccccc", OfferID: testOffer,
	}); err != nil {
		t.Fatal(err)
	}
	if checkoutProvider.createCount != 1 {
		t.Fatalf("checkout creates=%d", checkoutProvider.createCount)
	}
}

func TestSourceSwitchUnknownCancellationBlocksAndReconciles(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	seedCardContract(t, store, owner, "sub_2")
	provider := newFakePixAutoProvider()
	pixAuto := newPixAutoService(t, store, provider)
	pixAuto.Catalog = switchCatalog(t)
	fencer := &fakeStripeRenewal{cancelErr: app.ErrAmbiguous, fact: app.LifecycleFact{
		AccountID: owner, OfferID: testOffer, Environment: paydomain.EnvironmentTest,
		Merchant: switchMerchant, Provider: paydomain.ProviderStripe,
		RemoteSubscriptionID: "sub_2", RemoteCustomerID: "cus_" + owner,
		ProviderStatus: "active", LastEventAt: time.Now().UTC(),
	}}
	switcher := newSwitchService(t, store, pixAuto, fencer)
	record, err := switcher.Request(context.Background(), app.Principal{AccountID: owner}, app.SwitchRequest{
		OperationID: "dddddddd-dddd-4ddd-8ddd-dddddddddddd", OfferID: testOffer,
		NewChannel: paydomain.ChannelPixAutomatic,
	})
	if !errors.Is(err, app.ErrAmbiguous) || record.State != app.SourceSwitchPending ||
		record.OldCancellation != paydomain.CancellationUnknown {
		t.Fatalf("record=%+v err=%v", record, err)
	}
	if _, err := pixAuto.Start(context.Background(), app.Principal{AccountID: owner}, pixAutoRequest()); !errors.Is(err, paydomain.ErrConflict) {
		t.Fatalf("unknown cancellation allowed start: %v", err)
	}
	if provider.createCount != 0 {
		t.Fatal("provider created a second source while cancellation was unknown")
	}
	fencer.cancelErr = nil
	record, err = switcher.Reconcile(context.Background(), record)
	if err != nil || record.State != app.SourceSwitchReady || record.OldCancellation != paydomain.CancellationConfirmed {
		t.Fatalf("reconciled=%+v err=%v", record, err)
	}
}

func TestSourceSwitchConcurrentRequestAndStaleQuote(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	seedCardContract(t, store, owner, "sub_1")
	provider := newFakePixAutoProvider()
	pixAuto := newPixAutoService(t, store, provider)
	pixAuto.Catalog = switchCatalog(t)
	fencer := &fakeStripeRenewal{fact: app.LifecycleFact{
		AccountID: owner, OfferID: testOffer, Environment: paydomain.EnvironmentTest,
		Merchant: switchMerchant, Provider: paydomain.ProviderStripe,
		RemoteSubscriptionID: "sub_1", RemoteCustomerID: "cus_" + owner,
		ProviderStatus: "active", LastEventAt: time.Now().UTC(),
	}}
	switcher := newSwitchService(t, store, pixAuto, fencer)
	var wg sync.WaitGroup
	records := make([]app.SourceSwitchRecord, 2)
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			records[i], errs[i] = switcher.Request(context.Background(), app.Principal{AccountID: owner}, app.SwitchRequest{
				OperationID: "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee", OfferID: testOffer,
				NewChannel: paydomain.ChannelPixAutomatic,
			})
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("concurrent request err=%v", err)
		}
	}
	if fencer.cancelCount != 1 {
		t.Fatalf("cancel calls=%d", fencer.cancelCount)
	}
	if records[0].ID != records[1].ID {
		t.Fatal("concurrent request created two switches")
	}
	open, found, err := store.OpenSourceSwitch(context.Background(), owner, testOffer)
	if err != nil || !found || open.State != app.SourceSwitchReady {
		t.Fatalf("open=%+v found=%v err=%v", open, found, err)
	}
	if _, err := switcher.Request(context.Background(), app.Principal{AccountID: owner}, app.SwitchRequest{
		OperationID: "abababab-abab-4bab-8bab-abababababab", OfferID: testOffer,
		NewChannel: paydomain.ChannelPixAutomatic,
	}); !errors.Is(err, paydomain.ErrConflict) {
		t.Fatalf("second device err=%v", err)
	}

	staleCatalog, err := paydomain.NewCatalog("pay-catalog-v2", paydomain.Offer{
		ID: testOffer, Family: "premium", Term: paydomain.TermMonthly, Price: paydomain.BRL(1234),
		Capabilities: paydomain.PremiumCapabilityList(),
		Channels:     map[paydomain.Channel]bool{paydomain.ChannelPixAutomatic: true},
		SaleEnabled:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	pixAuto.Catalog = staleCatalog
	pixAuto.Config.CatalogVersion = "pay-catalog-v2"
	if _, err := pixAuto.Start(context.Background(), app.Principal{AccountID: owner}, pixAutoRequest()); !errors.Is(err, paydomain.ErrStale) {
		t.Fatalf("stale quote err=%v", err)
	}
}

func TestSimultaneousManualAndAutomaticSettlementOneGrantPerPayment(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	provider := newFakePixAutoProvider()
	pixAuto := newPixAutoService(t, store, provider)
	pixAuto.Catalog = switchCatalog(t)
	auth, err := pixAuto.Start(context.Background(), app.Principal{AccountID: owner}, pixAutoRequest())
	if err != nil {
		t.Fatal(err)
	}
	manualOrder, _, customerID := settlementOrder(t, store, owner, "manual-settle")
	settlements := &app.SettlementService{Store: store, Catalog: switchCatalog(t)}
	now := time.Now().UTC()
	candidates := []app.SettlementCandidate{
		settlementCandidate(manualOrder, owner, customerID, "pay_manual"),
		{
			Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
			Merchant: switchMerchant, Channel: paydomain.ChannelPixAutomatic,
			CustomerID: "cus_" + owner, RemotePaymentID: "pay_auto_1",
			RemoteSubscriptionID: auth.RemoteSubscriptionID, ProviderStatus: "RECEIVED",
			Amount: paydomain.BRL(1234), SettledAt: now,
		},
		{
			Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
			Merchant: switchMerchant, Channel: paydomain.ChannelPixAutomatic,
			CustomerID: "cus_" + owner, RemotePaymentID: "pay_auto_2",
			RemoteSubscriptionID: auth.RemoteSubscriptionID, ProviderStatus: "RECEIVED",
			Amount: paydomain.BRL(1234), SettledAt: now,
		},
	}
	var wg sync.WaitGroup
	errs := make([]error, len(candidates))
	for i := range candidates {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = settlements.Settle(context.Background(), candidates[i])
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("settle err=%v", err)
		}
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_grants WHERE account_id=$1 AND offer_id=$2 AND state='ACTIVE'", owner, testOffer) != 2 {
		t.Fatal("simultaneous payments produced an extra or missing period")
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_settlements WHERE outcome='GRANTED'") != 2 ||
		countRows(t, pool, "SELECT count(*) FROM payment_settlements WHERE outcome='EXCESS'") != 1 {
		t.Fatal("excess money was not recorded explicitly")
	}
}

func planChangeCatalog(t *testing.T) paydomain.Catalog {
	t.Helper()
	v1 := paydomain.Offer{
		ID: testOffer, Family: "premium", Term: paydomain.TermMonthly, Price: paydomain.BRL(1234),
		Capabilities: paydomain.PremiumCapabilityList(),
		Channels: map[paydomain.Channel]bool{
			paydomain.ChannelCard: true, paydomain.ChannelPixManual: true,
			paydomain.ChannelPixAutomatic: true,
		},
		SaleEnabled: true,
	}
	v2 := v1
	v2.ID = "premium.monthly.v2"
	v2.Price = paydomain.BRL(1490)
	catalog, err := paydomain.NewCatalog("pay-catalog-v1", v1, v2)
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func TestSourceSwitchNextTermPlanChangeSameFamily(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	manualEnd, _ := seedManualGrant(t, pool, store, owner)
	seedCardContract(t, store, owner, "sub_1")
	provider := newFakePixAutoProvider()
	pixAuto := newPixAutoService(t, store, provider)
	pixAuto.Catalog = planChangeCatalog(t)
	fencer := &fakeStripeRenewal{fact: app.LifecycleFact{
		AccountID: owner, OfferID: testOffer, Environment: paydomain.EnvironmentTest,
		Merchant: switchMerchant, Provider: paydomain.ProviderStripe,
		RemoteSubscriptionID: "sub_1", RemoteCustomerID: "cus_" + owner,
		ProviderStatus: "active", LastEventAt: time.Now().UTC(),
	}}
	switcher := newSwitchService(t, store, pixAuto, fencer)
	switcher.Catalog = planChangeCatalog(t)
	record, err := switcher.Request(context.Background(), app.Principal{AccountID: owner}, app.SwitchRequest{
		OperationID: "ffffffff-ffff-4fff-8fff-ffffffffffff", OfferID: "premium.monthly.v2",
		NewChannel: paydomain.ChannelPixAutomatic,
	})
	if err != nil || record.State != app.SourceSwitchReady || record.OfferID != "premium.monthly.v2" ||
		record.OldOfferID != testOffer || !record.TargetAmount.Equal(paydomain.BRL(1490)) ||
		!record.FirstDebitAt.Equal(manualEnd) {
		t.Fatalf("record=%+v err=%v", record, err)
	}
	request := pixAutoRequest()
	request.OfferID = "premium.monthly.v2"
	if _, err := pixAuto.Start(context.Background(), app.Principal{AccountID: owner}, request); !errors.Is(err, paydomain.ErrBusy) {
		t.Fatalf("early plan change err=%v", err)
	}
	pixAuto.Clock = func() time.Time { return manualEnd.Add(time.Second) }
	if _, err := pixAuto.Start(context.Background(), app.Principal{AccountID: owner}, request); err != nil {
		t.Fatal(err)
	}
	if provider.createCount != 1 || provider.last.Value != paydomain.BRL(1490) {
		t.Fatalf("create=%d value=%v", provider.createCount, provider.last.Value)
	}
}

func TestSourceSwitchRejectsProrationBeforeFencing(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	seedCardContract(t, store, owner, "sub_1")
	provider := newFakePixAutoProvider()
	pixAuto := newPixAutoService(t, store, provider)
	pixAuto.Catalog = switchCatalog(t)
	fencer := &fakeStripeRenewal{}
	switcher := newSwitchService(t, store, pixAuto, fencer)
	if _, err := switcher.Request(context.Background(), app.Principal{AccountID: owner}, app.SwitchRequest{
		OperationID: "12312312-1231-4123-8123-123123123123", OfferID: testOffer,
		NewChannel: paydomain.ChannelPixAutomatic, Proration: true,
	}); !errors.Is(err, paydomain.ErrFeatureUnsupported) {
		t.Fatalf("proration err=%v", err)
	}
	if fencer.cancelCount != 0 {
		t.Fatal("proration attempt canceled old source")
	}
	if _, found, err := store.OpenSourceSwitch(context.Background(), owner, testOffer); err != nil || found {
		t.Fatalf("proration created switch found=%v err=%v", found, err)
	}
}

func TestSourceSwitchSameChannelNextTermPriceChange(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	seedCardContract(t, store, owner, "sub_1")
	provider := newFakePixAutoProvider()
	pixAuto := newPixAutoService(t, store, provider)
	pixAuto.Catalog = planChangeCatalog(t)
	fencer := &fakeStripeRenewal{fact: app.LifecycleFact{
		AccountID: owner, OfferID: testOffer, Environment: paydomain.EnvironmentTest,
		Merchant: switchMerchant, Provider: paydomain.ProviderStripe,
		RemoteSubscriptionID: "sub_1", RemoteCustomerID: "cus_" + owner,
		ProviderStatus: "active", LastEventAt: time.Now().UTC(),
	}}
	switcher := newSwitchService(t, store, pixAuto, fencer)
	switcher.Catalog = planChangeCatalog(t)
	record, err := switcher.Request(context.Background(), app.Principal{AccountID: owner}, app.SwitchRequest{
		OperationID: "12121212-1212-4212-8212-121212121212", OfferID: "premium.monthly.v2",
		NewChannel: paydomain.ChannelCard,
	})
	if err != nil || record.State != app.SourceSwitchReady || record.OldSource != paydomain.ChannelCard ||
		record.NewSource != paydomain.ChannelCard || record.FirstDebitAt.IsZero() {
		t.Fatalf("record=%+v err=%v", record, err)
	}
	checkoutProvider := newFakeProvider()
	checkout := &app.Service{
		Store: store, Provider: checkoutProvider, Catalog: planChangeCatalog(t),
		Config: app.Config{
			Environment: paydomain.EnvironmentTest, MerchantID: switchMerchant,
			PriceID: "price_synthetic", CatalogVersion: "pay-catalog-v1",
			ReturnOrigin: "https://app.example.invalid", NewSalesEnabled: true, LeaseTTL: time.Minute,
		},
	}
	if _, err := checkout.Start(context.Background(), app.Principal{AccountID: owner}, app.StartRequest{
		OperationID: "34343434-3434-4434-8434-343434343434", OfferID: "premium.monthly.v2",
	}); err != nil {
		t.Fatal(err)
	}
	if checkoutProvider.createCount != 1 || checkoutProvider.last.Order.OfferID != "premium.monthly.v2" ||
		checkoutProvider.last.Order.Amount != paydomain.BRL(1490) {
		t.Fatalf("checkout=%+v", checkoutProvider.last)
	}
}

func TestSourceSwitchStampsOldSourceMerchant(t *testing.T) {
	const cardMerchant = "acct_card_merchant_synthetic"
	seedMerchantContract := func(t *testing.T, store *payadapters.Store, owner string) time.Time {
		t.Helper()
		now := time.Now().UTC()
		if err := store.UpsertContract(context.Background(), app.ContractRecord{
			ID: deterministicTestUUID("contract-sub_merchant"), AccountID: owner, OfferID: testOffer,
			Source: paydomain.ChannelCard, Environment: paydomain.EnvironmentTest, Merchant: cardMerchant,
			Provider: paydomain.ProviderStripe, RemoteSubscriptionID: "sub_merchant",
			RemoteCustomerID: "cus_" + owner, ProviderStatus: "active", LastEventAt: now,
		}); err != nil {
			t.Fatal(err)
		}
		return now
	}
	newMerchantSwitcher := func(t *testing.T, store *payadapters.Store, fencer *fakeStripeRenewal) *app.SwitchService {
		t.Helper()
		provider := newFakePixAutoProvider()
		pixAuto := newPixAutoService(t, store, provider)
		pixAuto.Catalog = switchCatalog(t)
		return &app.SwitchService{
			Store: store, Catalog: switchCatalog(t),
			Config: app.SwitchConfig{
				Environment:    paydomain.EnvironmentTest,
				CardMerchantID: cardMerchant, PixAutoMerchantID: switchMerchant,
				CardReady: true, PixAutoReady: true,
			},
			Stripe: fencer, PixAuto: pixAuto, WorkerID: "switch-test", LeaseTTL: time.Minute,
		}
	}
	merchantFact := func(owner string, merchant string, now time.Time) *fakeStripeRenewal {
		return &fakeStripeRenewal{fact: app.LifecycleFact{
			AccountID: owner, OfferID: testOffer, Environment: paydomain.EnvironmentTest,
			Merchant: merchant, Provider: paydomain.ProviderStripe,
			RemoteSubscriptionID: "sub_merchant", RemoteCustomerID: "cus_" + owner,
			ProviderStatus: "active", LastEventAt: now,
		}}
	}
	merchantOp := func(tag string) app.SwitchRequest {
		return app.SwitchRequest{
			OperationID: tag, OfferID: testOffer,
			NewChannel: paydomain.ChannelPixAutomatic,
		}
	}

	t.Run("matching", func(t *testing.T) {
		pool := freshCheckoutDB(t)
		store := payadapters.NewStore(pool)
		owner := seedCheckoutOwner(t, pool)
		now := seedMerchantContract(t, store, owner)
		// Matching old-source merchant: the card fence binds and the
		// record carries the card merchant, not the Pix one.
		fencer := merchantFact(owner, cardMerchant, now)
		record, err := newMerchantSwitcher(t, store, fencer).Request(context.Background(),
			app.Principal{AccountID: owner}, merchantOp("55555555-5555-4555-8555-555555555555"))
		if err != nil || record.OldSource != paydomain.ChannelCard || record.Merchant != cardMerchant {
			t.Fatalf("record=%+v err=%v", record, err)
		}
		if fencer.cancelCount != 1 {
			t.Fatalf("card renewal cancels=%d", fencer.cancelCount)
		}
	})

	t.Run("foreign", func(t *testing.T) {
		pool := freshCheckoutDB(t)
		store := payadapters.NewStore(pool)
		owner := seedCheckoutOwner(t, pool)
		now := seedMerchantContract(t, store, owner)
		// Foreign old-source merchant: binding refuses before any
		// provider write.
		foreign := merchantFact(owner, "acct_foreign_merchant", now)
		denied, err := newMerchantSwitcher(t, store, foreign).Request(context.Background(),
			app.Principal{AccountID: owner}, merchantOp("66666666-6666-4666-8666-666666666666"))
		if !errors.Is(err, paydomain.ErrBinding) || denied.State != app.SourceSwitchPending {
			t.Fatalf("record=%+v err=%v", denied, err)
		}
		if foreign.cancelCount != 0 {
			t.Fatal("foreign merchant reached the provider")
		}
	})

	t.Run("unconfigured", func(t *testing.T) {
		pool := freshCheckoutDB(t)
		store := payadapters.NewStore(pool)
		owner := seedCheckoutOwner(t, pool)
		now := seedMerchantContract(t, store, owner)
		// Missing per-channel merchant fails closed at admission.
		unconfigured := newMerchantSwitcher(t, store, merchantFact(owner, cardMerchant, now))
		unconfigured.Config.CardMerchantID = ""
		if _, err := unconfigured.Request(context.Background(), app.Principal{AccountID: owner}, app.SwitchRequest{
			OperationID: "77777777-7777-4777-8777-777777777777", OfferID: testOffer,
			NewChannel: paydomain.ChannelCard,
		}); !errors.Is(err, paydomain.ErrUnavailable) {
			t.Fatalf("err=%v", err)
		}
	})
}
