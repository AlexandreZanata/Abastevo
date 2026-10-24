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
	subadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/adapters"
)

type fakePixAutoProvider struct {
	mu             sync.Mutex
	auths          map[string]app.PixAutoAuthorization
	byContract     map[string]string
	ensureCount    int
	createCount    int
	findCount      int
	retrieveCount  int
	cancelCount    int
	ambiguousOnce  bool
	findDefinitive bool
	badBinding     bool
	last           app.PixAutoProviderRequest
}

func newFakePixAutoProvider() *fakePixAutoProvider {
	return &fakePixAutoProvider{auths: map[string]app.PixAutoAuthorization{}, byContract: map[string]string{}, findDefinitive: true}
}

func (p *fakePixAutoProvider) EnsureCustomer(_ context.Context, request app.PixCustomerRequest) (app.Customer, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ensureCount++
	return app.Customer{ID: "cus_" + request.AccountID}, nil
}

func (p *fakePixAutoProvider) CreatePixAutoAuthorization(_ context.Context, request app.PixAutoProviderRequest) (app.PixAutoAuthorization, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.last = request
	p.createCount++
	auth := app.PixAutoAuthorization{
		ID: "auth_" + request.ContractID, CustomerID: request.CustomerID, ContractID: request.ContractID,
		RemoteSubscriptionID: "sub_" + request.ContractID, Status: "CREATED", Value: request.Value,
		StartDate: request.StartDate, FinishDate: request.FinishDate, QRCode: "qr_" + request.ContractID,
		QRImage: "image_" + request.ContractID,
	}
	if p.badBinding {
		auth.CustomerID = "cus_foreign"
	}
	p.auths[auth.ID] = auth
	p.byContract[request.ContractID] = auth.ID
	if p.ambiguousOnce {
		p.ambiguousOnce = false
		return auth, app.ErrAmbiguous
	}
	return auth, nil
}

func (p *fakePixAutoProvider) RetrievePixAutoAuthorization(_ context.Context, id string) (app.PixAutoAuthorization, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.retrieveCount++
	auth, ok := p.auths[id]
	if !ok {
		return app.PixAutoAuthorization{}, paydomain.ErrUnavailable
	}
	return auth, nil
}

func (p *fakePixAutoProvider) FindPixAutoAuthorization(_ context.Context, _, contractID string) (app.PixAutoAuthorization, bool, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.findCount++
	if !p.findDefinitive {
		return app.PixAutoAuthorization{}, false, false, nil
	}
	id, ok := p.byContract[contractID]
	if !ok {
		return app.PixAutoAuthorization{}, false, true, nil
	}
	return p.auths[id], true, true, nil
}

func (p *fakePixAutoProvider) CancelPixAutoAuthorization(_ context.Context, id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cancelCount++
	auth, ok := p.auths[id]
	if !ok {
		return paydomain.ErrUnavailable
	}
	auth.Status = "CANCELLED"
	p.auths[id] = auth
	return nil
}

func (p *fakePixAutoProvider) setStatus(id, status string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	auth := p.auths[id]
	auth.Status = status
	p.auths[id] = auth
}

func pixAutoCatalog(t *testing.T) paydomain.Catalog {
	t.Helper()
	catalog, err := paydomain.NewCatalog("pay-catalog-v1", paydomain.Offer{
		ID: testOffer, Family: "premium", Term: paydomain.TermMonthly, Price: paydomain.BRL(1234),
		Capabilities: paydomain.PremiumCapabilityList(),
		Channels:     map[paydomain.Channel]bool{paydomain.ChannelPixAutomatic: true}, SaleEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func newPixAutoService(t *testing.T, store *payadapters.Store, provider *fakePixAutoProvider) *app.PixAutoService {
	t.Helper()
	return &app.PixAutoService{
		Store: store, Provider: provider, Catalog: pixAutoCatalog(t),
		Config: app.PixAutoConfig{
			Environment: paydomain.EnvironmentTest, MerchantID: "11111111-1111-4111-8111-111111111111",
			CatalogVersion: "pay-catalog-v1", NewSalesEnabled: true, MerchantEligible: true,
			ConsentVersion: "consent-v1", MandateMonths: 12, LeaseTTL: time.Minute,
		},
	}
}

func pixAutoRequest() app.PixAutoStartRequest {
	return app.PixAutoStartRequest{
		OperationID: testOperation, OfferID: testOffer,
		PayerName: "Synthetic Payer", PayerTaxID: "000.000.000-00", Consent: true,
	}
}

func TestPixAutoMandateRefusedKeepsInitialPaidPeriod(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	provider := newFakePixAutoProvider()
	service := newPixAutoService(t, store, provider)
	auth, err := service.Start(context.Background(), app.Principal{AccountID: owner}, pixAutoRequest())
	if err != nil {
		t.Fatal(err)
	}
	if auth.Status != "CREATED" || auth.QRCode == "" || auth.RemoteSubscriptionID == "" {
		t.Fatalf("auth=%+v", auth)
	}
	mandate, err := store.MandateForAccount(context.Background(), owner, testOffer)
	if err != nil || mandate.Status != "CREATED" || mandate.ConsentVersion != "consent-v1" ||
		mandate.ConsentAt.IsZero() || mandate.RemoteCustomerID != "cus_"+owner {
		t.Fatalf("mandate=%+v err=%v", mandate, err)
	}
	provider.setStatus(auth.ID, "REFUSED")
	status, err := service.Status(context.Background(), app.Principal{AccountID: owner}, testOffer)
	if err != nil || status.Status != "REFUSED" {
		t.Fatalf("status=%+v err=%v", status, err)
	}
	mandate, _ = store.MandateForAccount(context.Background(), owner, testOffer)
	if mandate.Status != "REFUSED" {
		t.Fatalf("mandate status=%s", mandate.Status)
	}

	order, err := store.GetOrder(context.Background(), mandate.OrderID)
	if err != nil {
		t.Fatal(err)
	}
	candidate := app.SettlementCandidate{
		OrderID: "", Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
		Merchant: "11111111-1111-4111-8111-111111111111", Channel: paydomain.ChannelPixAutomatic,
		CustomerID: "cus_" + owner, RemotePaymentID: "pay_auto_initial", RemoteSubscriptionID: auth.RemoteSubscriptionID,
		ProviderStatus: "RECEIVED", Amount: order.Amount, SettledAt: time.Now().UTC(),
	}
	settlement := settlementInbox(t, store, "evt_auto_paid", "pay_auto_initial")
	settlementService := settlementService(store, &fakeSettlementVerifier{candidate: candidate})
	if err := settlementService.Process(context.Background(), settlement); err != nil {
		t.Fatal(err)
	}
	subStore := subadapters.NewStore(pool)
	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if err := subStore.RequireCapability(context.Background(), tx, owner, "cloud.sync"); err != nil {
		t.Fatalf("initial paid period lost: %v", err)
	}
	mandate, _ = store.MandateForAccount(context.Background(), owner, testOffer)
	if mandate.Status != "REFUSED" {
		t.Fatal("refused mandate gained future authority")
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_grants WHERE order_id=$1 AND state='ACTIVE'", order.ID) != 1 {
		t.Fatal("initial period not funded")
	}
}

func TestPixAutoCancelAndExpiryKeepPaidPeriod(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	provider := newFakePixAutoProvider()
	service := newPixAutoService(t, store, provider)
	auth, err := service.Start(context.Background(), app.Principal{AccountID: owner}, pixAutoRequest())
	if err != nil {
		t.Fatal(err)
	}
	provider.setStatus(auth.ID, "ACTIVE")
	if _, err := service.Status(context.Background(), app.Principal{AccountID: owner}, testOffer); err != nil {
		t.Fatal(err)
	}
	if err := service.Cancel(context.Background(), app.Principal{AccountID: owner}, testOffer); err != nil {
		t.Fatal(err)
	}
	mandate, _ := store.MandateForAccount(context.Background(), owner, testOffer)
	if mandate.Status != "CANCELLED" || provider.cancelCount != 1 {
		t.Fatalf("cancel mandate=%+v count=%d", mandate, provider.cancelCount)
	}
	provider.setStatus(auth.ID, "EXPIRED")
	status, err := service.Status(context.Background(), app.Principal{AccountID: owner}, testOffer)
	if err != nil || status.Status != "EXPIRED" {
		t.Fatalf("expired status=%+v err=%v", status, err)
	}
	mandate, _ = store.MandateForAccount(context.Background(), owner, testOffer)
	if mandate.Status != "EXPIRED" {
		t.Fatalf("expired mandate=%s", mandate.Status)
	}
}

func TestPixAutoLostReplyAndInconclusiveSearch(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	provider := newFakePixAutoProvider()
	service := newPixAutoService(t, store, provider)
	provider.ambiguousOnce = true
	if _, err := service.Start(context.Background(), app.Principal{AccountID: owner}, pixAutoRequest()); !errors.Is(err, app.ErrAmbiguous) {
		t.Fatalf("first err=%v", err)
	}
	opKey := "asaas-pix-auto:" + testOperation
	if countRows(t, pool, "SELECT count(*) FROM payment_operations WHERE operation_key=$1 AND state='OUTCOME_UNKNOWN'", opKey) != 1 {
		t.Fatal("ambiguous authorization not persisted")
	}
	if _, err := pool.Exec(context.Background(), "UPDATE payment_operations SET lease_until=clock_timestamp()-interval '1 second' WHERE operation_key=$1", opKey); err != nil {
		t.Fatal(err)
	}
	recovered, err := service.Start(context.Background(), app.Principal{AccountID: owner}, pixAutoRequest())
	if err != nil || !recovered.Replayed || provider.createCount != 1 || provider.findCount != 1 {
		t.Fatalf("recovered=%+v err=%v create=%d find=%d", recovered, err, provider.createCount, provider.findCount)
	}

	other := seedCheckoutOwner(t, pool)
	provider2 := newFakePixAutoProvider()
	provider2.ambiguousOnce = true
	service2 := newPixAutoService(t, store, provider2)
	if _, err := service2.Start(context.Background(), app.Principal{AccountID: other}, pixAutoRequest()); !errors.Is(err, app.ErrAmbiguous) {
		t.Fatal("second ambiguous setup failed")
	}
	opKey2 := "asaas-pix-auto:" + testOperation
	if _, err := pool.Exec(context.Background(), "UPDATE payment_operations SET lease_until=clock_timestamp()-interval '1 second' WHERE operation_key=$1", opKey2); err != nil {
		t.Fatal(err)
	}
	provider2.findDefinitive = false
	if _, err := service2.Start(context.Background(), app.Principal{AccountID: other}, pixAutoRequest()); !errors.Is(err, app.ErrAmbiguous) {
		t.Fatalf("inconclusive err=%v", err)
	}
}

func TestPixAutoConsentForeignAndWrongCustomer(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	provider := newFakePixAutoProvider()
	service := newPixAutoService(t, store, provider)
	noConsent := pixAutoRequest()
	noConsent.Consent = false
	if _, err := service.Start(context.Background(), app.Principal{AccountID: owner}, noConsent); !errors.Is(err, paydomain.ErrInvalid) {
		t.Fatalf("consent err=%v", err)
	}
	service.Config.MerchantEligible = false
	if _, err := service.Start(context.Background(), app.Principal{AccountID: owner}, pixAutoRequest()); !errors.Is(err, paydomain.ErrUnavailable) {
		t.Fatalf("eligibility err=%v", err)
	}
	service.Config.MerchantEligible = true
	auth, err := service.Start(context.Background(), app.Principal{AccountID: owner}, pixAutoRequest())
	if err != nil {
		t.Fatal(err)
	}
	foreign := seedCheckoutOwner(t, pool)
	if _, err := service.Status(context.Background(), app.Principal{AccountID: foreign}, testOffer); !errors.Is(err, paydomain.ErrNotFound) {
		t.Fatalf("foreign status err=%v", err)
	}
	if err := service.Cancel(context.Background(), app.Principal{AccountID: foreign}, testOffer); !errors.Is(err, paydomain.ErrNotFound) {
		t.Fatalf("foreign cancel err=%v", err)
	}
	provider.setStatus(auth.ID, "REFUSED")
	if err := service.Cancel(context.Background(), app.Principal{AccountID: owner}, testOffer); err != nil {
		t.Fatalf("refused cancel err=%v", err)
	}
	badProvider := newFakePixAutoProvider()
	badProvider.badBinding = true
	badService := newPixAutoService(t, store, badProvider)
	if _, err := badService.Start(context.Background(), app.Principal{AccountID: owner}, app.PixAutoStartRequest{
		OperationID: "55555555-5555-4555-8555-555555555555", OfferID: testOffer,
		PayerName: "Payer", PayerTaxID: "00000000000", Consent: true,
	}); !errors.Is(err, paydomain.ErrBinding) {
		t.Fatalf("wrong customer err=%v", err)
	}
}
