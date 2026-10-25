//go:build integration

package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	payadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/adapters"
	payjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/adapters/jobs"
	paymentsapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
	paydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
	subadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/adapters"
	subdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/config"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

const (
	pay19Merchant   = "11111111-1111-4111-8111-111111111111"
	pay19Token      = "asaas_webhook_token_synthetic_1234567890"
	pay19APIKey     = "asaas_api_key_synthetic_1234567890"
	pay19Owner      = "22222222-2222-4222-8222-222222222222"
	pay19Order      = "33333333-3333-4333-8333-333333333333"
	pay19OpCheckout = "44444444-4444-4444-8444-444444444444"
)

func pay19TestConfig() config.PaymentsConfig {
	return config.PaymentsConfig{
		CatalogVersion: payCatalogVersion,
		Asaas: config.AsaasPaymentConfig{
			Mode: "test", ExpectedMode: "test", APIKey: pay19APIKey,
			WebhookToken: pay19Token, MerchantID: pay19Merchant,
			MaintenanceEnabled: true, Configured: true,
		},
	}
}

func pay19FreshPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("explicit disposable PostGIS DSN required")
	}
	u, err := url.Parse(dsn)
	if err != nil || u.Hostname() != "127.0.0.1" {
		t.Fatal("literal loopback test DSN required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("payapi_test_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		admin.Close(ctx)
		t.Fatal(err)
	}
	admin.Close(ctx)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		admin, err := pgx.Connect(ctx, dsn)
		if err != nil {
			t.Error(err)
			return
		}
		defer admin.Close(ctx)
		if _, err = admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
			t.Error(err)
		}
	})
	u.Path = "/" + name
	if _, err := migrate.Apply(ctx, u.String(), dbmigrations.Files); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(context.Background(),
		"INSERT INTO accounts(id,alias,status) VALUES($1,$2,'active')", pay19Owner, "synthetic-"+pay19Owner); err != nil {
		t.Fatal(err)
	}
	return pool
}

func pay19Count(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func pay19RequireAccess(t *testing.T, pool *pgxpool.Pool, want bool) {
	t.Helper()
	subStore := subadapters.NewStore(pool)
	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	err = subStore.RequireCapability(context.Background(), tx, pay19Owner, "cloud.sync")
	if want && err != nil {
		t.Fatalf("expected access, got %v", err)
	}
	if !want && !errors.Is(err, subdomain.ErrDenied) {
		t.Fatalf("expected denial, got %v", err)
	}
}

func pay19AsaasBody(id, eventType string) []byte {
	return []byte(fmt.Sprintf(`{"id":%q,"event":%q,"dateCreated":"2026-10-25 12:00:00","account":{"id":%q},"payment":{"id":"pay_p19_hook"}}`,
		id, eventType, pay19Merchant))
}

func pay19PostAsaas(t *testing.T, router chi.Router, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/v1/payments/asaas/webhook", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("asaas-access-token", pay19Token)
	request.TLS = &tls.ConnectionState{}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func TestPAY19CallbackToDrainJourney(t *testing.T) {
	pool := pay19FreshPool(t)
	comp := composePayments(pay19TestConfig(), pool, time.Now)
	if comp.Asaas == nil {
		t.Fatal("asaas provider not composed")
	}
	// Services exist for maintenance, but sales stay disabled by flag.
	principal := paymentsapp.Principal{AccountID: pay19Owner}
	if _, err := comp.Pix.Start(context.Background(), principal, paymentsapp.PixStartRequest{
		OperationID: pay19OpCheckout, OfferID: "premium.monthly",
	}); !errors.Is(err, paydomain.ErrSaleDisabled) {
		t.Fatalf("pix start err=%v", err)
	}
	if comp.Preflight.PixManual.NewSalesReady || comp.Preflight.PixAutomatic.NewSalesReady {
		t.Fatal("sales ready despite disabled flags")
	}
	router := chi.NewRouter()
	registerPaymentRoutes(router, comp)

	// Authenticated callback lands durably before any worker runs.
	body := pay19AsaasBody("evt_p19_hook", "SYNTHETIC_UNKNOWN")
	if response := pay19PostAsaas(t, router, body); response.Code != http.StatusOK {
		t.Fatalf("webhook status=%d body=%s", response.Code, response.Body.String())
	}
	record, err := comp.Store.Inbox(context.Background(), paydomain.ProviderAsaas,
		paydomain.EnvironmentTest, pay19Merchant, "evt_p19_hook")
	if err != nil || record.State != "PENDING" {
		t.Fatalf("record=%+v err=%v", record, err)
	}

	// The worker drain from the same composition retains the unknown event
	// minimally with no grant and no provider call.
	drain := &payjobs.Router{
		Store: comp.Store, Settlement: comp.Settlement, Lifecycle: comp.Lifecycle,
		Dispute: comp.Dispute, Cycles: comp.Cycles,
		Channels: []payjobs.Channel{{Provider: paydomain.ProviderAsaas,
			Environment: paydomain.EnvironmentTest, Merchant: pay19Merchant}},
	}
	processed, err := drain.Drain(context.Background())
	if err != nil || processed != 1 {
		t.Fatalf("drain=%d err=%v", processed, err)
	}
	if pay19Count(t, pool, "SELECT count(*) FROM payment_grants") != 0 {
		t.Fatal("unknown event granted access")
	}
	// Restart converges with no second effect.
	again, err := drain.Drain(context.Background())
	if err != nil || again != 0 {
		t.Fatalf("restart drain=%d err=%v", again, err)
	}

	// The unconfigured Stripe channel fails closed on the same router.
	stripeBody := []byte(`{"id":"evt_p19_stripe","object":"event","api_version":"2026-09-30.endive","created":1700000000,"livemode":false,"type":"invoice.paid","data":{"object":{"id":"in_1","object":"invoice"}}}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/payments/stripe/webhook", bytes.NewReader(stripeBody))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Stripe-Signature", "synthetic")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code/100 == 2 {
		t.Fatalf("stripe acknowledged without configuration: %d", response.Code)
	}
}

func TestPAY19OutagePreservesPendingWithoutGrant(t *testing.T) {
	pool := pay19FreshPool(t)
	// Empty configuration: no provider client exists anywhere.
	comp := composePayments(config.PaymentsConfig{}, pool, time.Now)
	if comp.Stripe != nil || comp.Asaas != nil {
		t.Fatal("providers exist without credentials")
	}
	event := paymentsapp.InboxEvent{
		Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
		Merchant: "merchant-test", EventID: "evt_p19_outage",
		PayloadHash: strings.Repeat("e", 64), EventType: "PAYMENT_RECEIVED",
		ResourceType: "payment", ResourceID: "pay_p19_outage",
	}
	if _, err := comp.Store.RecordInbox(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	drain := &payjobs.Router{
		Store: comp.Store, Settlement: comp.Settlement, Lifecycle: comp.Lifecycle,
		Dispute: comp.Dispute,
		Channels: []payjobs.Channel{{Provider: paydomain.ProviderAsaas,
			Environment: paydomain.EnvironmentTest, Merchant: "merchant-test"}},
	}
	processed, err := drain.Drain(context.Background())
	if !errors.Is(err, paydomain.ErrUnavailable) || processed != 0 {
		t.Fatalf("drain=%d err=%v", processed, err)
	}
	record, err := comp.Store.Inbox(context.Background(), paydomain.ProviderAsaas,
		paydomain.EnvironmentTest, "merchant-test", "evt_p19_outage")
	if err != nil || record.State != "PENDING" {
		t.Fatalf("record=%+v err=%v", record, err)
	}
	if pay19Count(t, pool, "SELECT count(*) FROM payment_grants") != 0 {
		t.Fatal("outage granted access")
	}
	pay19RequireAccess(t, pool, false)
}

type pay19SettlementVerifier struct {
	candidate paymentsapp.SettlementCandidate
}

func (f *pay19SettlementVerifier) Verify(context.Context, paymentsapp.InboxRecord) (paymentsapp.SettlementCandidate, error) {
	return f.candidate, nil
}

type pay19OperatorProof struct{}

func (pay19OperatorProof) VerifyOperator(_ context.Context, actor, proof, _ string) error {
	if proof != "proof-"+actor {
		return paydomain.ErrUnauthorized
	}
	return nil
}

type pay19RefundProvider struct {
	calls int
}

func (f *pay19RefundProvider) CreateRefund(_ context.Context, request paymentsapp.RefundProviderRequest) (paymentsapp.RefundProviderResult, error) {
	f.calls++
	return paymentsapp.RefundProviderResult{
		ID: "re_" + request.RefundID, State: paydomain.RefundSucceeded,
		RawStatus: string(paydomain.RefundSucceeded), Amount: request.Amount, At: time.Now().UTC(),
	}, nil
}

func (f *pay19RefundProvider) FindRefund(_ context.Context, request paymentsapp.RefundProviderRequest) (paymentsapp.RefundProviderResult, bool, bool, error) {
	return paymentsapp.RefundProviderResult{
		ID: "re_" + request.RefundID, State: paydomain.RefundSucceeded,
		RawStatus: string(paydomain.RefundSucceeded), Amount: request.Amount, At: time.Now().UTC(),
	}, true, true, nil
}

type pay19OwnerProof struct{}

func (pay19OwnerProof) VerifyFreshOwner(_ context.Context, accountID, proof, _ string) error {
	if proof != "owner-"+accountID {
		return paydomain.ErrUnauthorized
	}
	return nil
}

func pay19TestCatalog(t *testing.T) paydomain.Catalog {
	t.Helper()
	catalog, err := paydomain.NewCatalog(payCatalogVersion, paydomain.Offer{
		ID: "premium.monthly", Family: "premium", Term: paydomain.TermMonthly,
		Price: paydomain.BRL(1234), Capabilities: paydomain.PremiumCapabilityList(),
		Channels:    map[paydomain.Channel]bool{paydomain.ChannelPixManual: true},
		SaleEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func TestPAY19ComposedSettlementRevocationAndExit(t *testing.T) {
	pool := pay19FreshPool(t)
	comp := composePayments(pay19TestConfig(), pool, time.Now)
	now := time.Now().UTC().Truncate(time.Microsecond)

	// The composed store binds the synthetic order exactly like the
	// service-level suites; provider verification itself stays PAY-21
	// sandbox proof, so a fixture verifier stands in for the network.
	catalog := pay19TestCatalog(t)
	quote, err := catalog.Quote(paydomain.QuoteRequest{OfferID: "premium.monthly", Channel: paydomain.ChannelPixManual}, now)
	if err != nil {
		t.Fatal(err)
	}
	order, err := paydomain.NewOrder(pay19Order, pay19Owner, quote, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := order.MarkPending(now); err != nil {
		t.Fatal(err)
	}
	if err := comp.Store.CreateOrder(context.Background(), order); err != nil {
		t.Fatal(err)
	}
	if err := comp.Store.MapCustomer(context.Background(), payadapters.CustomerMapping{
		AccountID: pay19Owner, Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
		Merchant: pay19Merchant, RemoteCustomerID: "cus_" + pay19Owner,
	}); err != nil {
		t.Fatal(err)
	}
	claim, err := comp.Store.ClaimAttempt(context.Background(), payadapters.ClaimAttemptInput{
		AccountID: pay19Owner, Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
		Merchant: pay19Merchant, OperationKey: "pay19-journey", Kind: "create_attempt",
		PayloadHash: strings.Repeat("d", 64), OrderID: order.ID, LeaseUntil: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := comp.Store.MarkDispatched(context.Background(), claim); err != nil {
		t.Fatal(err)
	}

	event := paymentsapp.InboxEvent{
		Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
		Merchant: pay19Merchant, EventID: "evt_p19_grant",
		PayloadHash: strings.Repeat("e", 64), EventType: "PAYMENT_RECEIVED",
		ResourceType: "payment", ResourceID: "pay_p19_grant",
	}
	if _, err := comp.Store.RecordInbox(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	settlement := &paymentsapp.SettlementService{
		Store: comp.Store, Catalog: catalog,
		Verifier: &pay19SettlementVerifier{candidate: paymentsapp.SettlementCandidate{
			OrderID: order.ID, Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
			Merchant: pay19Merchant, Channel: paydomain.ChannelPixManual,
			CustomerID: "cus_" + pay19Owner, RemotePaymentID: "pay_p19_grant",
			// Settle strictly after dispatch: read the clock after setup.
			ProviderStatus: "RECEIVED", Amount: paydomain.BRL(1234), SettledAt: time.Now().UTC(),
		}},
	}
	drain := &payjobs.Router{
		Store: comp.Store, Settlement: settlement, Lifecycle: comp.Lifecycle,
		Dispute: comp.Dispute, Cycles: comp.Cycles,
		Channels: []payjobs.Channel{{Provider: paydomain.ProviderAsaas,
			Environment: paydomain.EnvironmentTest, Merchant: pay19Merchant}},
	}
	processed, err := drain.Drain(context.Background())
	if err != nil || processed != 1 {
		t.Fatalf("drain=%d err=%v", processed, err)
	}
	if pay19Count(t, pool, "SELECT count(*) FROM payment_grants WHERE state='ACTIVE'") != 1 {
		t.Fatal("composed drain granted nothing")
	}
	pay19RequireAccess(t, pool, true)

	// Revocation runs on the composed store with an operator proof; the
	// production proof surface stays unimplemented by construction.
	refunds := &paymentsapp.RefundService{
		Store: comp.Store, Asaas: &pay19RefundProvider{}, Proof: pay19OperatorProof{},
		Config: paymentsapp.RefundServiceConfig{SingleApprovalAboveCents: 0}, Clock: time.Now,
	}
	record, err := refunds.Request(context.Background(), paymentsapp.RefundRequest{
		Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
		Merchant: pay19Merchant, RemotePaymentID: "pay_p19_grant",
		Reason: "runtime acceptance", ActorID: "op_1", Proof: "proof-op_1",
		ApprovedBy: "op_2", ApprovalProof: "proof-op_2", OperationKey: "op_p19_runtime",
	})
	if err != nil || record.State != paydomain.RefundSucceeded {
		t.Fatalf("record=%+v err=%v", record, err)
	}
	if pay19Count(t, pool, "SELECT count(*) FROM payment_grants WHERE state='REVOKED'") != 1 {
		t.Fatal("composed refund revoked nothing")
	}
	pay19RequireAccess(t, pool, false)

	// Owner cancellation records its durable exit on the composed billing
	// service with an owner proof; history stays owner-scoped.
	comp.Billing.Proof = pay19OwnerProof{}
	principal := paymentsapp.Principal{AccountID: pay19Owner}
	exit, err := comp.Billing.Cancel(context.Background(), principal, pay19OpCheckout, "owner-"+pay19Owner)
	if err != nil || exit.State != "DONE" {
		t.Fatalf("exit=%+v err=%v", exit, err)
	}
	status, err := comp.Billing.Status(context.Background(), principal)
	if err != nil || status.Exit == nil || status.Exit.ID != exit.ID {
		t.Fatalf("status=%+v err=%v", status, err)
	}
	history, err := comp.Billing.History(context.Background(), principal, 100)
	if err != nil || len(history) != 1 || history[0].RemotePaymentID != "pay_p19_grant" {
		t.Fatalf("history=%+v err=%v", history, err)
	}
}

func TestPAY19FlagsAloneNeverEnableSales(t *testing.T) {
	pool := pay19FreshPool(t)
	// Flags on, credentials missing: the switches alone must not conjure
	// providers, sales or grants.
	cfg := config.PaymentsConfig{
		CatalogVersion: payCatalogVersion,
		Stripe:         config.StripePaymentConfig{NewSalesEnabled: true},
		Asaas: config.AsaasPaymentConfig{
			PixManualNewSalesEnabled: true, PixAutomaticNewSalesEnabled: true,
		},
	}
	comp := composePayments(cfg, pool, time.Now)
	// Flags without credentials change nothing: no provider, no sales.
	if comp.Stripe != nil || comp.Asaas != nil {
		t.Fatal("flags conjured providers")
	}
	principal := paymentsapp.Principal{AccountID: pay19Owner}
	if _, err := comp.Checkout.Start(context.Background(), principal, paymentsapp.StartRequest{
		OperationID: pay19OpCheckout, OfferID: "premium.monthly",
	}); err == nil {
		t.Fatal("flag-only config started checkout")
	}
	for channel, ready := range map[string]bool{
		"stripe": comp.Preflight.Stripe.NewSalesReady,
		"manual": comp.Preflight.PixManual.NewSalesReady,
		"auto":   comp.Preflight.PixAutomatic.NewSalesReady,
	} {
		if ready {
			t.Fatalf("%s sales ready without credentials", channel)
		}
	}
	if comp.Preflight.Stripe.Reason == "" || comp.Preflight.PixManual.Reason == "" ||
		comp.Preflight.PixAutomatic.Reason == "" {
		t.Fatal("preflight hides the blocking reason")
	}
	// Free owner reads keep working with every sale disabled.
	if _, err := comp.Billing.History(context.Background(), principal, 100); err != nil {
		t.Fatalf("history err=%v", err)
	}
	if pay19Count(t, pool, "SELECT count(*) FROM accounts") != 1 {
		t.Fatal("free data disturbed")
	}
}
