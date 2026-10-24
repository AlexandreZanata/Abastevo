//go:build integration

package application_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	payadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
	paydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
	subadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/adapters"
	subdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fakeSettlementVerifier struct {
	candidate app.SettlementCandidate
	err       error
}

func (f *fakeSettlementVerifier) Verify(context.Context, app.InboxRecord) (app.SettlementCandidate, error) {
	return f.candidate, f.err
}

func settlementTestID(t *testing.T) string {
	t.Helper()
	id, err := jobs.NewUUIDv4()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func settlementOrder(t *testing.T, store *payadapters.Store, owner, operationKey string) (paydomain.Order, app.Claim, string) {
	t.Helper()
	catalog := pixCatalogForSettlement()
	now := time.Now().UTC().Truncate(time.Microsecond)
	quote, err := catalog.Quote(paydomain.QuoteRequest{OfferID: testOffer, Channel: paydomain.ChannelPixManual}, now)
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
	customerID := "cus_" + owner
	if err := store.MapCustomer(context.Background(), payadapters.CustomerMapping{
		AccountID: owner, Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
		Merchant: "11111111-1111-4111-8111-111111111111", RemoteCustomerID: customerID,
	}); err != nil {
		t.Fatal(err)
	}
	claim, err := store.ClaimAttempt(context.Background(), payadapters.ClaimAttemptInput{
		AccountID: owner, Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
		Merchant: "11111111-1111-4111-8111-111111111111", OperationKey: operationKey,
		Kind: "create_attempt", PayloadHash: strings.Repeat("d", 64), OrderID: order.ID,
		LeaseUntil: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MarkDispatched(context.Background(), claim); err != nil {
		t.Fatal(err)
	}
	return order, claim, customerID
}

func settlementInbox(t *testing.T, store *payadapters.Store, eventID, paymentID string) app.InboxRecord {
	t.Helper()
	event := app.InboxEvent{
		Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
		Merchant: "11111111-1111-4111-8111-111111111111", EventID: eventID,
		PayloadHash: strings.Repeat("e", 64), EventType: "PAYMENT_RECEIVED",
		ResourceType: "payment", ResourceID: paymentID,
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

func settlementCandidate(order paydomain.Order, owner, customerID, paymentID string) app.SettlementCandidate {
	return app.SettlementCandidate{
		OrderID: order.ID, Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
		Merchant: "11111111-1111-4111-8111-111111111111", Channel: paydomain.ChannelPixManual,
		CustomerID: customerID, RemotePaymentID: paymentID, ProviderStatus: "RECEIVED",
		Amount: paydomain.BRL(1234), SettledAt: time.Now().UTC(),
	}
}

func countRows(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func settlementService(store *payadapters.Store, verifier app.SettlementVerifier) *app.SettlementService {
	return &app.SettlementService{Store: store, Catalog: pixCatalogForSettlement(), Verifier: verifier}
}

func pixCatalogForSettlement() paydomain.Catalog {
	catalog, _ := paydomain.NewCatalog("pay-catalog-v1", paydomain.Offer{
		ID: testOffer, Family: "premium", Term: paydomain.TermMonthly, Price: paydomain.BRL(1234),
		Capabilities: paydomain.PremiumCapabilityList(),
		Channels:     map[paydomain.Channel]bool{paydomain.ChannelPixManual: true}, SaleEnabled: true,
	})
	return catalog
}

func TestSettlementServiceGrantsCycleAndAccess(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	subStore := subadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	order, _, customerID := settlementOrder(t, store, owner, "settle-order-1")
	event := settlementInbox(t, store, "evt_settle_1", "pay_settle_1")
	verifier := &fakeSettlementVerifier{candidate: settlementCandidate(order, owner, customerID, "pay_settle_1")}
	service := settlementService(store, verifier)
	if err := service.Process(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_grants WHERE order_id=$1 AND state='ACTIVE'", order.ID) != 1 ||
		countRows(t, pool, "SELECT count(*) FROM payment_cycles WHERE settlement_id IS NOT NULL") != 1 ||
		countRows(t, pool, "SELECT count(*) FROM payment_settlements") != 1 {
		t.Fatal("settlement/cycle/grant not committed")
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_inbox WHERE event_id='evt_settle_1' AND state='PROCESSED'") != 1 {
		t.Fatal("inbox not processed")
	}
	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if err := subStore.RequireCapability(context.Background(), tx, owner, "cloud.sync"); err != nil {
		t.Fatalf("web capability denied: %v", err)
	}
	tx.Rollback(context.Background())

	duplicate := settlementInbox(t, store, "evt_settle_duplicate", "pay_settle_1")
	verifier.candidate = settlementCandidate(order, owner, customerID, "pay_settle_1")
	if err := service.Process(context.Background(), duplicate); err != nil {
		t.Fatal(err)
	}
	if err := store.RevokeGrant(context.Background(), order.ID, time.Now()); err != nil {
		t.Fatal(err)
	}
	third := settlementInbox(t, store, "evt_settle_after_revoke", "pay_settle_1")
	if err := service.Process(context.Background(), third); err != nil {
		t.Fatal(err)
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_grants WHERE order_id=$1 AND state='ACTIVE'", order.ID) != 0 ||
		countRows(t, pool, "SELECT count(*) FROM payment_grants WHERE order_id=$1", order.ID) != 1 {
		t.Fatal("revoked grant was resurrected or duplicated")
	}
	tx, err = pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if err := subStore.RequireCapability(context.Background(), tx, owner, "cloud.sync"); !errors.Is(err, subdomain.ErrDenied) {
		t.Fatalf("revoked capability err=%v", err)
	}
}

func TestSettlementProviderOutageLeavesPendingThenRecovers(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	order, _, customerID := settlementOrder(t, store, owner, "settle-outage")
	event := settlementInbox(t, store, "evt_outage", "pay_outage")
	verifier := &fakeSettlementVerifier{err: app.ErrAmbiguous}
	service := settlementService(store, verifier)
	if err := service.Process(context.Background(), event); err == nil {
		t.Fatal("provider outage was accepted")
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_grants") != 0 ||
		countRows(t, pool, "SELECT count(*) FROM payment_inbox WHERE event_id='evt_outage' AND state='PENDING'") != 1 {
		t.Fatal("outage changed durable state")
	}
	verifier.err = nil
	verifier.candidate = settlementCandidate(order, owner, customerID, "pay_outage")
	processed, err := service.ProcessPending(context.Background(), paydomain.ProviderAsaas, paydomain.EnvironmentTest,
		"11111111-1111-4111-8111-111111111111", 10)
	if err != nil || processed != 1 {
		t.Fatalf("processed=%d err=%v", processed, err)
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_grants WHERE order_id=$1 AND state='ACTIVE'", order.ID) != 1 ||
		countRows(t, pool, "SELECT count(*) FROM payment_inbox WHERE event_id='evt_outage' AND state='PROCESSED'") != 1 {
		t.Fatal("reconciliation did not complete")
	}
}

func TestSettlementWrongAmountAndNonSettlementHaveNoRights(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	order, _, customerID := settlementOrder(t, store, owner, "settle-invalid")
	event := settlementInbox(t, store, "evt_invalid", "pay_invalid")
	candidate := settlementCandidate(order, owner, customerID, "pay_invalid")
	candidate.Amount = paydomain.BRL(999)
	service := settlementService(store, &fakeSettlementVerifier{candidate: candidate})
	if err := service.Process(context.Background(), event); !errors.Is(err, paydomain.ErrConflict) {
		t.Fatalf("wrong amount err=%v", err)
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_grants") != 0 ||
		countRows(t, pool, "SELECT count(*) FROM payment_inbox WHERE event_id='evt_invalid' AND state='CONFLICT'") != 1 {
		t.Fatal("wrong amount changed rights")
	}

	event = settlementInbox(t, store, "evt_not_settled", "pay_invalid")
	service = settlementService(store, &fakeSettlementVerifier{err: app.ErrNotSettled})
	if err := service.Process(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_grants") != 0 ||
		countRows(t, pool, "SELECT count(*) FROM payment_inbox WHERE event_id='evt_not_settled' AND state='PROCESSED'") != 1 {
		t.Fatal("non-settlement changed rights")
	}
}

func TestSettlementConcurrentReceiptOneGrant(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	order, _, customerID := settlementOrder(t, store, owner, "settle-concurrent")
	event := settlementInbox(t, store, "evt_concurrent", "pay_concurrent")
	service := settlementService(store, &fakeSettlementVerifier{
		candidate: settlementCandidate(order, owner, customerID, "pay_concurrent"),
	})
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = service.Process(context.Background(), event)
		}(i)
	}
	wg.Wait()
	success := 0
	for _, err := range errs {
		if err == nil {
			success++
		}
	}
	if success == 0 {
		t.Fatal("no concurrent receipt succeeded")
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_grants WHERE order_id=$1 AND state='ACTIVE'", order.ID) != 1 ||
		countRows(t, pool, "SELECT count(*) FROM payment_cycles") != 1 {
		t.Fatal("concurrent receipt duplicated economic effect")
	}
}
