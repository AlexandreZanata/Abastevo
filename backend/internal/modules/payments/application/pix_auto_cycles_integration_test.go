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
)

func newPixAutoCycleService(t *testing.T, store *payadapters.Store, provider *fakePixAutoProvider) *app.PixAutoCycleService {
	t.Helper()
	return &app.PixAutoCycleService{
		Store: store, Provider: provider,
		Settlements: &app.SettlementService{Store: store, Catalog: pixAutoCatalog(t), Verifier: &fakeSettlementVerifier{}},
		Config: app.PixAutoConfig{
			Environment: paydomain.EnvironmentTest, MerchantID: "11111111-1111-4111-8111-111111111111",
			CatalogVersion: "pay-catalog-v1", NewSalesEnabled: true, MerchantEligible: true,
			ConsentVersion: "consent-v1", MandateMonths: 12, LeaseTTL: time.Minute,
		},
	}
}

func pixAutoInbox(t *testing.T, store *payadapters.Store, eventID, eventType, resourceType, resourceID, status string) app.InboxRecord {
	t.Helper()
	event := app.InboxEvent{
		Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
		Merchant: "11111111-1111-4111-8111-111111111111", EventID: eventID,
		PayloadHash: strings64("f"), EventType: eventType, ResourceType: resourceType,
		ResourceID: resourceID, ResourceStatus: status,
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

func strings64(value string) string {
	out := ""
	for len(out) < 64 {
		out += value
	}
	return out[:64]
}

func activePixAuto(t *testing.T, service *app.PixAutoService, provider *fakePixAutoProvider, owner string) app.PixAutoAuthorization {
	t.Helper()
	auth, err := service.Start(context.Background(), app.Principal{AccountID: owner}, pixAutoRequest())
	if err != nil {
		t.Fatal(err)
	}
	provider.setStatus(auth.ID, "ACTIVE")
	auth, err = service.Status(context.Background(), app.Principal{AccountID: owner}, testOffer)
	if err != nil || auth.Status != "ACTIVE" {
		t.Fatalf("active auth=%+v err=%v", auth, err)
	}
	return auth
}

func pixAutoPayment(owner, subscriptionID, paymentID string, settledAt time.Time) app.PixPayment {
	return app.PixPayment{
		ID: paymentID, CustomerID: "cus_" + owner, SubscriptionID: subscriptionID,
		Amount: paydomain.BRL(1234), Status: "RECEIVED", ProviderStatus: "RECEIVED",
		Class: app.PixSettled, SettledAt: settledAt,
	}
}

func TestPixAutoCycleInstructionAndSettlementConverge(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	provider := newFakePixAutoProvider()
	pixService := newPixAutoService(t, store, provider)
	auth := activePixAuto(t, pixService, provider, owner)
	provider.addInstruction(app.PixAutoInstruction{
		ID: "ins_1", AuthorizationID: auth.ID, PaymentID: "pay_cycle_1",
		Status: "SCHEDULED", Purpose: "SCHEDULE", DueDate: time.Now().Add(24 * time.Hour),
	})
	provider.addPayment(auth.RemoteSubscriptionID, pixAutoPayment(owner, auth.RemoteSubscriptionID, "pay_cycle_1", time.Now().Add(-time.Minute)))
	cycle := newPixAutoCycleService(t, store, provider)
	event := pixAutoInbox(t, store, "evt_ins_1", "PIX_AUTOMATIC_RECURRING_PAYMENT_INSTRUCTION_SCHEDULED",
		"payment_instruction", "ins_1", "")
	if err := cycle.Process(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_pix_auto_instructions WHERE remote_instruction_id='ins_1' AND status='SCHEDULED'") != 1 {
		t.Fatal("instruction not persisted")
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_grants") != 0 ||
		countRows(t, pool, "SELECT count(*) FROM payment_settlements") != 0 {
		t.Fatal("instruction alone granted access")
	}

	mandate, err := store.MandateForAccount(context.Background(), owner, testOffer)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _, errs[i] = cycle.Reconcile(context.Background(), mandate)
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("reconcile err=%v", err)
		}
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_grants WHERE state='ACTIVE'") != 1 ||
		countRows(t, pool, "SELECT count(*) FROM payment_settlements") != 1 ||
		countRows(t, pool, "SELECT count(*) FROM payment_cycles WHERE settlement_id IS NOT NULL") != 1 ||
		countRows(t, pool, "SELECT count(*) FROM payment_orders") != 2 {
		t.Fatal("concurrent cycle settlement duplicated or lost an effect")
	}

	second := time.Now().Add(-30 * time.Second)
	provider.addPayment(auth.RemoteSubscriptionID, pixAutoPayment(owner, auth.RemoteSubscriptionID, "pay_cycle_2", second))
	if _, _, err := cycle.Reconcile(context.Background(), mandate); err != nil {
		t.Fatal(err)
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_grants WHERE state='ACTIVE'") != 2 ||
		countRows(t, pool, "SELECT count(*) FROM payment_settlements") != 2 ||
		countRows(t, pool, "SELECT count(*) FROM payment_cycles WHERE settlement_id IS NOT NULL") != 2 ||
		countRows(t, pool, "SELECT count(*) FROM payment_orders") != 3 {
		t.Fatal("second cycle did not grant exactly one new period")
	}
}

func TestPixAutoCycleRefusedInstructionAndOutOfOrder(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	provider := newFakePixAutoProvider()
	pixService := newPixAutoService(t, store, provider)
	auth := activePixAuto(t, pixService, provider, owner)
	provider.addInstruction(app.PixAutoInstruction{
		ID: "ins_refused", AuthorizationID: auth.ID, PaymentID: "pay_refused",
		Status: "REFUSED", Purpose: "SCHEDULE", DueDate: time.Now().Add(24 * time.Hour),
		RefusalReason: "INSUFFICIENT_FUNDS",
	})
	cycle := newPixAutoCycleService(t, store, provider)
	event := pixAutoInbox(t, store, "evt_ins_refused", "PIX_AUTOMATIC_RECURRING_PAYMENT_INSTRUCTION_REFUSED",
		"payment_instruction", "ins_refused", "")
	if err := cycle.Process(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_pix_auto_instructions WHERE remote_instruction_id='ins_refused' AND status='REFUSED'") != 1 ||
		countRows(t, pool, "SELECT count(*) FROM payment_grants") != 0 {
		t.Fatal("refused instruction granted access or was not recorded")
	}
	provider.addInstruction(app.PixAutoInstruction{
		ID: "ins_refused", AuthorizationID: auth.ID, PaymentID: "pay_refused",
		Status: "SCHEDULED", Purpose: "SCHEDULE", DueDate: time.Now().Add(24 * time.Hour),
	})
	stale := pixAutoInbox(t, store, "evt_ins_stale", "PIX_AUTOMATIC_RECURRING_PAYMENT_INSTRUCTION_SCHEDULED",
		"payment_instruction", "ins_refused", "")
	if err := cycle.Process(context.Background(), stale); err != nil {
		t.Fatal(err)
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_pix_auto_instructions WHERE remote_instruction_id='ins_refused' AND status='REFUSED'") != 1 {
		t.Fatal("terminal instruction regressed to scheduled")
	}
}

func TestPixAutoCycleCancellationRaceStaysPendingThenResolves(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	provider := newFakePixAutoProvider()
	pixService := newPixAutoService(t, store, provider)
	activePixAuto(t, pixService, provider, owner)
	provider.cancelPending = true
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = pixService.Cancel(context.Background(), app.Principal{AccountID: owner}, testOffer)
		}(i)
	}
	wg.Wait()
	pending := 0
	for _, err := range errs {
		if errors.Is(err, app.ErrAmbiguous) {
			pending++
		}
	}
	if pending != 2 {
		t.Fatalf("cancel errs=%v", errs)
	}
	mandate, err := store.MandateForAccount(context.Background(), owner, testOffer)
	if err != nil {
		t.Fatal(err)
	}
	if mandate.CancelRequestedAt == nil || mandate.Status != "ACTIVE" {
		t.Fatalf("unknown cancellation not explicit: %+v", mandate)
	}
	provider.cancelPending = false
	provider.setStatus(mandate.RemoteAuthorizationID, "CANCELLED")
	cycle := newPixAutoCycleService(t, store, provider)
	if _, _, err := cycle.Reconcile(context.Background(), mandate); err != nil {
		t.Fatal(err)
	}
	mandate, err = store.MandateForAccount(context.Background(), owner, testOffer)
	if err != nil || mandate.Status != "CANCELLED" || mandate.CancelledAt == nil {
		t.Fatalf("cancellation not resolved: %+v err=%v", mandate, err)
	}
	if err := pixService.Cancel(context.Background(), app.Principal{AccountID: owner}, testOffer); err != nil {
		t.Fatalf("terminal cancel err=%v", err)
	}
}

func TestPixAutoEligibilityStopsActiveMandatesAndKeepsPaidPeriod(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	provider := newFakePixAutoProvider()
	pixService := newPixAutoService(t, store, provider)
	auth := activePixAuto(t, pixService, provider, owner)
	provider.addPayment(auth.RemoteSubscriptionID, pixAutoPayment(owner, auth.RemoteSubscriptionID, "pay_paid", time.Now().Add(-time.Minute)))
	cycle := newPixAutoCycleService(t, store, provider)
	mandate, err := store.MandateForAccount(context.Background(), owner, testOffer)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := cycle.Reconcile(context.Background(), mandate); err != nil {
		t.Fatal(err)
	}
	event := pixAutoInbox(t, store, "evt_ineligible", "PIX_AUTOMATIC_RECURRING_ELIGIBILITY_UPDATED",
		"event", "evt_ineligible", "INELIGIBLE")
	if err := cycle.Process(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	eligibility, found, err := store.Eligibility(context.Background(), paydomain.EnvironmentTest, "11111111-1111-4111-8111-111111111111")
	if err != nil || !found || eligibility.Status != "INELIGIBLE" {
		t.Fatalf("eligibility=%+v found=%v err=%v", eligibility, found, err)
	}
	mandate, _ = store.MandateForAccount(context.Background(), owner, testOffer)
	if mandate.Status != "CANCELLED" || provider.cancelCount == 0 {
		t.Fatalf("active mandate not canceled: %+v count=%d", mandate, provider.cancelCount)
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_grants WHERE state='ACTIVE'") != 1 {
		t.Fatal("paid period was erased")
	}
	if _, err := pixService.Start(context.Background(), app.Principal{AccountID: owner}, app.PixAutoStartRequest{
		OperationID: "77777777-7777-4777-8777-777777777777", OfferID: testOffer,
		PayerName: "Payer", PayerTaxID: "00000000000", Consent: true,
	}); !errors.Is(err, paydomain.ErrUnavailable) {
		t.Fatalf("ineligible start err=%v", err)
	}
}

func TestPixAutoNoSecondActiveMandate(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	provider := newFakePixAutoProvider()
	pixService := newPixAutoService(t, store, provider)
	activePixAuto(t, pixService, provider, owner)
	if _, err := pixService.Start(context.Background(), app.Principal{AccountID: owner}, app.PixAutoStartRequest{
		OperationID: "88888888-8888-4888-8888-888888888888", OfferID: testOffer,
		PayerName: "Payer", PayerTaxID: "00000000000", Consent: true,
	}); !errors.Is(err, paydomain.ErrConflict) {
		t.Fatalf("second source err=%v", err)
	}
	if provider.createCount != 1 {
		t.Fatalf("second remote authorization created: %d", provider.createCount)
	}
}
