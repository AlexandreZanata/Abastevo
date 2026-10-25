//go:build integration

// Provider credential failures preserve recoverability: a rejected key
// never marks conflicts, grants, revokes or fails intents, and fixing
// the credential converges exactly once. User authorization refusals
// stay terminal.
package application_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	payadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
	paydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
	"github.com/stripe/stripe-go/v87"
)

func pay22AsaasProvider(t *testing.T, handler http.Handler) *payadapters.AsaasProvider {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	provider := payadapters.NewAsaasProvider(strings.Repeat("k", 24), paydomain.EnvironmentTest, server.Client())
	provider.BaseURL = server.URL
	return provider
}

func pay22StripeProvider(t *testing.T, handler http.Handler) *payadapters.StripeProvider {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := stripe.NewClient("sk_test_"+strings.Repeat("s", 24), stripe.WithBackends(stripe.NewBackendsWithConfig(&stripe.BackendConfig{
		HTTPClient:        server.Client(),
		URL:               stripe.String(server.URL),
		EnableTelemetry:   stripe.Bool(false),
		MaxNetworkRetries: stripe.Int64(0),
		LeveledLogger:     &stripe.LeveledLogger{Level: stripe.LevelNull},
	})))
	return payadapters.NewStripeProviderWithClient(client, paydomain.EnvironmentTest)
}

func pay22RefusedTransport() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		// Both providers answer credential refusals with a parseable
		// body; the SDK mapping relies on it.
		if strings.HasPrefix(r.URL.Path, "/v1/") {
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{
				"type": "invalid_request_error", "message": "revoked",
			}})
		}
	})
}

func pay22InboxState(t *testing.T, store *payadapters.Store, provider paydomain.Provider, merchant, eventID string) string {
	t.Helper()
	record, err := store.Inbox(context.Background(), provider, paydomain.EnvironmentTest, merchant, eventID)
	if err != nil {
		t.Fatal(err)
	}
	return record.State
}

type pay22DisputeRefused struct{ err error }

func (f *pay22DisputeRefused) VerifyDispute(context.Context, app.InboxRecord) (app.DisputeFact, error) {
	return app.DisputeFact{}, f.err
}

type pay22LifecycleFact struct{ fact app.LifecycleFact }

func (f *pay22LifecycleFact) VerifyLifecycle(context.Context, app.InboxRecord) (app.LifecycleFact, error) {
	return f.fact, nil
}

func (f *pay22LifecycleFact) RetrieveLifecycle(context.Context, string) (app.LifecycleFact, error) {
	return f.fact, nil
}

func TestProviderAuthFailurePreservesSettlement(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	order, _, customerID := settlementOrder(t, store, owner, "auth-settle")
	event := settlementInbox(t, store, "evt_auth_settle", "pay_auth_settle")

	// Real adapter, refused credential: no grant, no conflict marking,
	// event stays pending with an empty diagnosis.
	mux := payadapters.SettlementMux{}
	mux.Asaas = pay22AsaasProvider(t, pay22RefusedTransport())
	settlement := &app.SettlementService{
		Store: store, Catalog: pixCatalogForSettlement(), Verifier: mux,
	}
	if err := settlement.Process(context.Background(), event); !errors.Is(err, app.ErrProviderUnauthorized) {
		t.Fatalf("err = %v", err)
	}
	state := pay22InboxState(t, store, paydomain.ProviderAsaas, "11111111-1111-4111-8111-111111111111", "evt_auth_settle")
	if state != "PENDING" {
		t.Fatalf("state = %q", state)
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_grants") != 0 {
		t.Fatal("refused credential granted access")
	}
	var lastError string
	if err := pool.QueryRow(context.Background(),
		"SELECT coalesce(last_error,'') FROM payment_inbox WHERE event_id='evt_auth_settle'").Scan(&lastError); err != nil {
		t.Fatal(err)
	}
	if lastError != "" {
		t.Fatalf("diagnosis = %q", lastError)
	}

	// Fixed credential converges exactly once; replay is a no-op.
	settlement.Verifier = &fakeSettlementVerifier{candidate: settlementCandidate(order, owner, customerID, "pay_auth_settle")}
	if err := settlement.Process(context.Background(), event); err != nil {
		t.Fatalf("recovery err = %v", err)
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_grants WHERE state='ACTIVE'") != 1 {
		t.Fatal("recovery granted nothing")
	}
	refetched, err := store.Inbox(context.Background(), paydomain.ProviderAsaas,
		paydomain.EnvironmentTest, "11111111-1111-4111-8111-111111111111", "evt_auth_settle")
	if err != nil {
		t.Fatal(err)
	}
	if err := settlement.Process(context.Background(), refetched); err != nil {
		t.Fatalf("replay err = %v", err)
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_grants") != 1 {
		t.Fatal("replay duplicated the grant")
	}
}

func TestProviderAuthFailurePreservesLifecycleAndDispute(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)

	lifecycleEvent := app.InboxRecord{InboxEvent: app.InboxEvent{
		Provider: paydomain.ProviderStripe, Environment: paydomain.EnvironmentTest,
		Merchant: "acct_synthetic", EventID: "evt_auth_lifecycle",
		PayloadHash: strings.Repeat("e", 64), EventType: "customer.subscription.updated",
		ResourceType: "subscription", ResourceID: "sub_auth",
	}, State: "PENDING", ReceivedAt: time.Now().UTC()}
	if _, err := store.RecordInbox(context.Background(), lifecycleEvent.InboxEvent); err != nil {
		t.Fatal(err)
	}
	stripeRefused := pay22StripeProvider(t, pay22RefusedTransport())
	stripeRefused.Merchant = "acct_synthetic"
	lifecycle := &app.LifecycleService{Store: store, Verifier: stripeRefused}
	if err := lifecycle.Process(context.Background(), lifecycleEvent); !errors.Is(err, app.ErrProviderUnauthorized) {
		t.Fatalf("lifecycle err = %v", err)
	}

	disputeEvent := app.InboxRecord{InboxEvent: app.InboxEvent{
		Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
		Merchant: "11111111-1111-4111-8111-111111111111", EventID: "evt_auth_dispute",
		PayloadHash: strings.Repeat("e", 64), EventType: "PAYMENT_CHARGEBACK_DISPUTE",
		ResourceType: "payment", ResourceID: "pay_auth_dispute",
	}, State: "PENDING", ReceivedAt: time.Now().UTC()}
	if _, err := store.RecordInbox(context.Background(), disputeEvent.InboxEvent); err != nil {
		t.Fatal(err)
	}
	dispute := &app.DisputeService{
		Store: store, Stripe: pay22StripeProvider(t, pay22RefusedTransport()),
		Asaas: pay22AsaasProvider(t, pay22RefusedTransport()),
	}
	if err := dispute.Process(context.Background(), disputeEvent); !errors.Is(err, app.ErrProviderUnauthorized) {
		t.Fatalf("dispute err = %v", err)
	}

	for _, check := range []struct {
		provider paydomain.Provider
		merchant string
		eventID  string
	}{
		{paydomain.ProviderStripe, "acct_synthetic", "evt_auth_lifecycle"},
		{paydomain.ProviderAsaas, "11111111-1111-4111-8111-111111111111", "evt_auth_dispute"},
	} {
		record, err := store.Inbox(context.Background(), check.provider,
			paydomain.EnvironmentTest, check.merchant, check.eventID)
		if err != nil || record.State != "PENDING" {
			t.Fatalf("%s = %+v err = %v", check.eventID, record, err)
		}
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_contracts") != 0 {
		t.Fatal("refused credential upserted a contract")
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_disputes") != 0 {
		t.Fatal("refused credential recorded a dispute")
	}

	// Fixed credential converges the lifecycle exactly once.
	fact := app.LifecycleFact{
		AccountID: owner, OfferID: testOffer, Provider: paydomain.ProviderStripe,
		Environment: paydomain.EnvironmentTest, Merchant: "acct_synthetic",
		RemoteSubscriptionID: "sub_auth", RemoteCustomerID: "cus_" + owner,
		ProviderStatus: "active", LastEventAt: time.Now().UTC(),
	}
	lifecycle.Verifier = &pay22LifecycleFact{fact: fact}
	if err := lifecycle.Process(context.Background(), lifecycleEvent); err != nil {
		t.Fatalf("lifecycle recovery err = %v", err)
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_contracts") != 1 {
		t.Fatal("lifecycle recovery upserted nothing")
	}
}

func TestProviderAuthFailureKeepsRefundPending(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	order, _, customerID := settlementOrder(t, store, owner, "auth-refund")
	grantEvent := settlementInbox(t, store, "evt_auth_refund", "pay_auth_refund")
	granter := settlementService(store, &fakeSettlementVerifier{
		candidate: settlementCandidate(order, owner, customerID, "pay_auth_refund"),
	})
	if err := granter.Process(context.Background(), grantEvent); err != nil {
		t.Fatal(err)
	}
	requireAccess(t, pool, owner, true)

	refunds := &app.RefundService{
		Store: store, Asaas: pay22AsaasProvider(t, pay22RefusedTransport()),
		Proof: &fakeOperatorProof{}, Config: app.RefundServiceConfig{SingleApprovalAboveCents: 1000000},
	}
	record, err := refunds.Request(context.Background(), app.RefundRequest{
		Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
		Merchant: refundMerchant, RemotePaymentID: "pay_auth_refund",
		Reason: "credential outage", ActorID: "op_1", Proof: "proof-op_1",
		OperationKey: "op_auth_refund",
	})
	if !errors.Is(err, app.ErrAmbiguous) || record.State != paydomain.RefundPending {
		t.Fatalf("record=%+v err=%v", record, err)
	}
	// Neither granted extra access nor revoked what exists.
	requireAccess(t, pool, owner, true)
	if countRows(t, pool, "SELECT count(*) FROM payment_grants WHERE state='ACTIVE'") != 1 {
		t.Fatal("refused credential disturbed the grant")
	}

	// Fixed credential revokes exactly once with a single provider write.
	fixed := &fakeRefundProvider{found: true}
	refunds.Asaas = fixed
	resolved, err := refunds.Reconcile(context.Background(), "op_1", "proof-op_1", record.ID)
	if err != nil || resolved.State != paydomain.RefundSucceeded {
		t.Fatalf("resolved=%+v err=%v", resolved, err)
	}
	if fixed.createCalls != 0 {
		t.Fatalf("reconcile wrote %d times", fixed.createCalls)
	}
	requireAccess(t, pool, owner, false)
}

func TestUserAuthorizationRefusalsStayTerminal(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	refunds := &app.RefundService{
		Store: store, Asaas: &fakeRefundProvider{}, Proof: &fakeOperatorProof{},
		Config: app.RefundServiceConfig{SingleApprovalAboveCents: 1000000},
	}
	_, err := refunds.Request(context.Background(), app.RefundRequest{
		Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
		Merchant: refundMerchant, RemotePaymentID: "pay_terminal",
		Reason: "bad proof", ActorID: "op_1", Proof: "stale-proof",
		OperationKey: "op_terminal",
	})
	if !errors.Is(err, paydomain.ErrUnauthorized) || errors.Is(err, app.ErrProviderUnauthorized) {
		t.Fatalf("proof refusal err = %v", err)
	}

	billing := &app.BillingExitService{Store: store, Proof: &fakeOwnerProof{}}
	_, err = billing.Cancel(context.Background(), app.Principal{AccountID: owner},
		"44444444-4444-4444-8444-444444444444", "wrong-proof")
	if !errors.Is(err, paydomain.ErrUnauthorized) || errors.Is(err, app.ErrProviderUnauthorized) {
		t.Fatalf("owner refusal err = %v", err)
	}
}
