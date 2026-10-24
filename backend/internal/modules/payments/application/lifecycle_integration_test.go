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
)

type fakeLifecycleVerifier struct {
	fact     app.LifecycleFact
	err      error
	retrieve app.LifecycleFact
}

func (f *fakeLifecycleVerifier) VerifyLifecycle(context.Context, app.InboxRecord) (app.LifecycleFact, error) {
	return f.fact, f.err
}
func (f *fakeLifecycleVerifier) RetrieveLifecycle(context.Context, string) (app.LifecycleFact, error) {
	return f.retrieve, nil
}

type fakePortalProvider struct {
	request app.PortalRequest
	url     string
	err     error
}

func (f *fakePortalProvider) CreatePortalSession(_ context.Context, request app.PortalRequest) (string, error) {
	f.request = request
	return f.url, f.err
}

func lifecycleEvent(t *testing.T, store *payadapters.Store, eventID, eventType, resourceType, resourceID string) app.InboxRecord {
	t.Helper()
	event := app.InboxEvent{
		Provider: paydomain.ProviderStripe, Environment: paydomain.EnvironmentTest,
		Merchant: "acct_synthetic", EventID: eventID, PayloadHash: strings.Repeat("f", 64),
		EventType: eventType, ResourceType: resourceType, ResourceID: resourceID,
	}
	if _, err := store.RecordInbox(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	record, err := store.Inbox(context.Background(), event.Provider, event.Environment, event.Merchant, event.EventID)
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func TestLifecycleContractPortalReconcileAndAccessDistinction(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	subStore := subadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	now := time.Now().UTC()
	verifier := &fakeLifecycleVerifier{
		fact: app.LifecycleFact{
			AccountID: owner, OfferID: testOffer, Environment: paydomain.EnvironmentTest,
			Merchant: "acct_synthetic", Provider: paydomain.ProviderStripe,
			RemoteSubscriptionID: "sub_1", RemoteCustomerID: "cus_" + owner,
			ProviderStatus: "active", CancelAtPeriodEnd: true, LastEventAt: now,
		},
	}
	service := &app.LifecycleService{Store: store, Verifier: verifier}
	event := lifecycleEvent(t, store, "evt_lifecycle", "customer.subscription.updated", "subscription", "sub_1")
	if err := service.Process(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	contract, err := store.ContractForAccount(context.Background(), owner, testOffer)
	if err != nil || contract.ProviderStatus != "active" || !contract.CancelAtPeriodEnd {
		t.Fatalf("contract=%+v err=%v", contract, err)
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_grants") != 0 {
		t.Fatal("lifecycle event granted access")
	}

	portalProvider := &fakePortalProvider{url: "https://billing.example.invalid/session"}
	portal := &app.PortalService{
		Store: store, Provider: portalProvider,
		Config: app.PortalConfig{
			Environment: paydomain.EnvironmentTest, Merchant: "acct_synthetic",
			ReturnOrigin: "https://app.example.invalid", ConfigurationID: "bpc_1",
		},
	}
	url, err := portal.Manage(context.Background(), app.Principal{AccountID: owner}, testOffer)
	if err != nil || url != portalProvider.url || portalProvider.request.CustomerID != "cus_"+owner ||
		portalProvider.request.ConfigurationID != "bpc_1" ||
		portalProvider.request.ReturnURL != "https://app.example.invalid/billing/manage" {
		t.Fatalf("portal url=%q request=%+v err=%v", url, portalProvider.request, err)
	}
	foreign := seedCheckoutOwner(t, pool)
	if _, err := portal.Manage(context.Background(), app.Principal{AccountID: foreign}, testOffer); !errors.Is(err, paydomain.ErrNotFound) {
		t.Fatalf("foreign portal err=%v", err)
	}
	portal.Config.ConfigurationID = ""
	if _, err := portal.Manage(context.Background(), app.Principal{AccountID: owner}, testOffer); !errors.Is(err, paydomain.ErrUnavailable) {
		t.Fatalf("unconstrained portal err=%v", err)
	}

	verifier.retrieve = verifier.fact
	verifier.retrieve.ProviderStatus = "canceled"
	verifier.retrieve.CancelAtPeriodEnd = true
	verifier.retrieve.LastEventAt = now.Add(time.Minute)
	if err := service.Reconcile(context.Background(), contract); err != nil {
		t.Fatal(err)
	}
	contract, err = store.ContractForAccount(context.Background(), owner, testOffer)
	if err != nil || contract.ProviderStatus != "canceled" {
		t.Fatalf("reconciled contract=%+v err=%v", contract, err)
	}
	// A stale upsert must not overwrite the newer canceled state.
	old := contract
	old.ProviderStatus = "active"
	old.LastEventAt = now
	if err := store.UpsertContract(context.Background(), old); err != nil {
		t.Fatal(err)
	}
	contract, _ = store.ContractForAccount(context.Background(), owner, testOffer)
	if contract.ProviderStatus != "canceled" {
		t.Fatalf("stale upsert won: %+v", contract)
	}

	// Canceled renewal keeps an already funded paid-through grant.
	order, _, customerID := settlementOrder(t, store, owner, "lifecycle-grant")
	settlement := settlementInbox(t, store, "evt_lifecycle_grant", "pay_lifecycle_grant")
	settlementService := settlementService(store, &fakeSettlementVerifier{candidate: settlementCandidate(order, owner, customerID, "pay_lifecycle_grant")})
	if err := settlementService.Process(context.Background(), settlement); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if err := subStore.RequireCapability(context.Background(), tx, owner, "cloud.sync"); err != nil {
		t.Fatalf("canceled renewal lost paid-through: %v", err)
	}
}

func TestLifecycleSCAAndInvoiceCreationHaveNoNewRights(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	now := time.Now().UTC()
	verifier := &fakeLifecycleVerifier{
		fact: app.LifecycleFact{
			AccountID: owner, OfferID: testOffer, Environment: paydomain.EnvironmentTest,
			Merchant: "acct_synthetic", Provider: paydomain.ProviderStripe,
			RemoteSubscriptionID: "sub_sca", RemoteCustomerID: "cus_" + owner,
			ProviderStatus: "past_due", LastEventAt: now,
		},
	}
	service := &app.LifecycleService{Store: store, Verifier: verifier}
	event := lifecycleEvent(t, store, "evt_sca", "invoice.payment_action_required", "invoice", "in_sca")
	if err := service.Process(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	contract, err := store.ContractForAccount(context.Background(), owner, testOffer)
	if err != nil || contract.ProviderStatus != "past_due" {
		t.Fatalf("contract=%+v err=%v", contract, err)
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_grants") != 0 {
		t.Fatal("SCA/action-required granted access")
	}

	other := seedCheckoutOwner(t, pool)
	ignored := &fakeLifecycleVerifier{err: app.ErrIgnored}
	service = &app.LifecycleService{Store: store, Verifier: ignored}
	event = lifecycleEvent(t, store, "evt_invoice_created", "invoice.created", "invoice", "in_created")
	if err := service.Process(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ContractForAccount(context.Background(), other, testOffer); !errors.Is(err, paydomain.ErrNotFound) {
		t.Fatalf("invoice.created created contract err=%v", err)
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_grants") != 0 {
		t.Fatal("invoice.created granted access")
	}
}
