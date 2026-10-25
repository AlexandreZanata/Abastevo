package application_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
)

type fakePixProvider struct {
	mu             sync.Mutex
	payments       map[string]app.PixPayment
	byOrder        map[string]string
	qr             map[string]app.PixQR
	ensureCount    int
	createCount    int
	findCount      int
	retrieveCount  int
	getQRCount     int
	ambiguousOnce  bool
	findFound      bool
	findDefinitive bool
	badBinding     bool
	qrTooLarge     bool
	last           app.PixPaymentRequest
}

func newFakePixProvider() *fakePixProvider {
	return &fakePixProvider{
		payments: map[string]app.PixPayment{}, byOrder: map[string]string{}, qr: map[string]app.PixQR{},
		findFound: true, findDefinitive: true,
	}
}

func (p *fakePixProvider) EnsureCustomer(_ context.Context, request app.PixCustomerRequest) (app.Customer, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ensureCount++
	return app.Customer{ID: "cus_" + request.AccountID}, nil
}

func (p *fakePixProvider) CreatePayment(_ context.Context, request app.PixPaymentRequest) (app.PixPayment, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.last = request
	p.createCount++
	payment := app.PixPayment{
		ID: "pay_" + request.Order.ID, CustomerID: request.CustomerID, ExternalReference: request.Order.ID,
		Amount: request.Order.Amount, Status: "PENDING", ProviderStatus: "PENDING", Class: app.PixPending,
	}
	if p.badBinding {
		payment.CustomerID = "cus_foreign"
	}
	p.payments[payment.ID] = payment
	p.byOrder[request.Order.ID] = payment.ID
	p.qr[payment.ID] = app.PixQR{Payload: "qr_" + payment.ID, Image: "img_" + payment.ID, ExpiresAt: time.Now().Add(time.Hour)}
	if p.ambiguousOnce {
		p.ambiguousOnce = false
		return payment, app.ErrAmbiguous
	}
	return payment, nil
}

func (p *fakePixProvider) RetrievePayment(_ context.Context, id string) (app.PixPayment, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.retrieveCount++
	payment, ok := p.payments[id]
	if !ok {
		return app.PixPayment{}, domain.ErrUnavailable
	}
	return payment, nil
}

func (p *fakePixProvider) FindPayment(_ context.Context, externalReference string) (app.PixPayment, bool, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.findCount++
	if !p.findDefinitive {
		return app.PixPayment{}, false, false, nil
	}
	id, ok := p.byOrder[externalReference]
	if !ok || !p.findFound {
		return app.PixPayment{}, false, true, nil
	}
	return p.payments[id], true, true, nil
}

func (p *fakePixProvider) GetQR(_ context.Context, id string) (app.PixQR, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.getQRCount++
	qr, ok := p.qr[id]
	if !ok {
		return app.PixQR{}, domain.ErrUnavailable
	}
	if p.qrTooLarge {
		qr.Payload = string(make([]byte, 5000))
	}
	return qr, nil
}

func (p *fakePixProvider) setStatus(orderID, status, class string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	id, ok := p.byOrder[orderID]
	if !ok {
		return
	}
	payment := p.payments[id]
	payment.Status, payment.ProviderStatus, payment.Class = status, status, class
	p.payments[id] = payment
}

func newPixService(t *testing.T) (*app.PixService, *memoryStore, *fakePixProvider) {
	t.Helper()
	catalog, err := domain.NewCatalog("pay-catalog-v1", domain.Offer{
		ID: testOffer, Family: "premium", Term: domain.TermMonthly, Price: domain.BRL(1234),
		Capabilities: domain.PremiumCapabilityList(),
		Channels:     map[domain.Channel]bool{domain.ChannelPixManual: true},
		SaleEnabled:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	store := newMemoryStore()
	provider := newFakePixProvider()
	service := &app.PixService{
		Store: store, Provider: provider, Catalog: catalog,
		Config: app.PixConfig{
			Environment: domain.EnvironmentTest, MerchantID: "11111111-1111-4111-8111-111111111111",
			CatalogVersion: "pay-catalog-v1", NewSalesEnabled: true, LeaseTTL: time.Minute,
		},
	}
	return service, store, provider
}

func pixRequest() app.PixStartRequest {
	return app.PixStartRequest{OperationID: testOperation, OfferID: testOffer, PayerName: "Synthetic Payer", PayerTaxID: "000.000.000-00"}
}

func TestPixStartCreatesServerPricedQR(t *testing.T) {
	service, _, provider := newPixService(t)
	order, err := service.Start(context.Background(), app.Principal{AccountID: testAccount}, pixRequest())
	if err != nil {
		t.Fatal(err)
	}
	if order.Status != app.PixPending || order.QRCode == "" || order.QRImage == "" || order.PaymentID == "" || order.Replayed {
		t.Fatalf("order = %+v", order)
	}
	if provider.createCount != 1 || provider.ensureCount != 1 || provider.last.Order.Amount != domain.BRL(1234) ||
		provider.last.CustomerID != "cus_"+testAccount || provider.last.DueDate.IsZero() {
		t.Fatalf("provider create=%d ensure=%d last=%+v", provider.createCount, provider.ensureCount, provider.last)
	}
	if provider.last.Order.Channel != domain.ChannelPixManual {
		t.Fatal("non-Pix channel used")
	}
}

func TestPixStartRejectsTamperingAndInvalidPayer(t *testing.T) {
	service, store, provider := newPixService(t)
	amount := int64(1)
	cases := map[string]app.PixStartRequest{
		"amount":   {OperationID: testOperation, OfferID: testOffer, AmountCentavos: &amount},
		"currency": {OperationID: testOperation, OfferID: testOffer, Currency: "USD"},
		"customer": {OperationID: testOperation, OfferID: testOffer, CustomerID: "cus_evil"},
		"order":    {OperationID: testOperation, OfferID: testOffer, OrderID: "44444444-4444-4444-8444-444444444444"},
		"metadata": {OperationID: testOperation, OfferID: testOffer, Metadata: map[string]string{"x": "y"}},
		"taxid":    {OperationID: testOperation, OfferID: testOffer, PayerName: "Payer", PayerTaxID: "123"},
		"name":     {OperationID: testOperation, OfferID: testOffer, PayerName: "", PayerTaxID: "00000000000"},
		"email":    {OperationID: testOperation, OfferID: testOffer, PayerName: "Payer", PayerTaxID: "00000000000", PayerEmail: "bad"},
	}
	for name, request := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := service.Start(context.Background(), app.Principal{AccountID: testAccount}, request); !errors.Is(err, domain.ErrInvalid) {
				t.Fatalf("err = %v", err)
			}
		})
	}
	if len(store.orders) != 0 || provider.createCount != 0 || provider.ensureCount != 0 {
		t.Fatal("tampered Pix request touched state")
	}
}

func TestPixExistingCustomerDoesNotNeedPayerAgain(t *testing.T) {
	service, store, provider := newPixService(t)
	if err := store.MapCustomer(context.Background(), app.CustomerMapping{
		AccountID: testAccount, Provider: domain.ProviderAsaas, Environment: domain.EnvironmentTest,
		Merchant: "11111111-1111-4111-8111-111111111111", RemoteCustomerID: "cus_existing",
	}); err != nil {
		t.Fatal(err)
	}
	request := app.PixStartRequest{OperationID: testOperation, OfferID: testOffer}
	if _, err := service.Start(context.Background(), app.Principal{AccountID: testAccount}, request); err != nil {
		t.Fatal(err)
	}
	if provider.ensureCount != 0 || provider.last.CustomerID != "cus_existing" {
		t.Fatalf("ensure=%d customer=%s", provider.ensureCount, provider.last.CustomerID)
	}
}

func TestPixConcurrentStartsCreateOnePayment(t *testing.T) {
	service, store, provider := newPixService(t)
	const n = 6
	errs := make([]error, n)
	results := make([]app.PixOrder, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = service.Start(context.Background(), app.Principal{AccountID: testAccount}, pixRequest())
		}(i)
	}
	wg.Wait()
	payments := map[string]bool{}
	success := 0
	for i, err := range errs {
		if err == nil {
			success++
			payments[results[i].PaymentID] = true
		} else if !errors.Is(err, domain.ErrBusy) {
			t.Fatalf("unexpected err = %v", err)
		}
	}
	if success == 0 || len(payments) != 1 || provider.createCount != 1 || len(store.orders) != 1 {
		t.Fatalf("success=%d payments=%d create=%d orders=%d", success, len(payments), provider.createCount, len(store.orders))
	}
}

func TestPixLostResponseRecoversWithoutSecondCharge(t *testing.T) {
	service, store, provider := newPixService(t)
	provider.ambiguousOnce = true
	if _, err := service.Start(context.Background(), app.Principal{AccountID: testAccount}, pixRequest()); !errors.Is(err, app.ErrAmbiguous) {
		t.Fatalf("first err = %v", err)
	}
	key := "asaas-pix:" + testOperation
	if store.state(key) != "OUTCOME_UNKNOWN" {
		t.Fatalf("state = %s", store.state(key))
	}
	store.expire(key)
	recovered, err := service.Start(context.Background(), app.Principal{AccountID: testAccount}, pixRequest())
	if err != nil {
		t.Fatal(err)
	}
	if provider.createCount != 1 || provider.findCount != 1 || !recovered.Replayed || recovered.QRCode == "" {
		t.Fatalf("recovered=%+v create=%d find=%d", recovered, provider.createCount, provider.findCount)
	}
}

func TestPixInconclusiveSearchStaysAmbiguous(t *testing.T) {
	service, store, provider := newPixService(t)
	provider.ambiguousOnce = true
	_, _ = service.Start(context.Background(), app.Principal{AccountID: testAccount}, pixRequest())
	key := "asaas-pix:" + testOperation
	store.expire(key)
	provider.findDefinitive = false
	if _, err := service.Start(context.Background(), app.Principal{AccountID: testAccount}, pixRequest()); !errors.Is(err, app.ErrAmbiguous) {
		t.Fatalf("err = %v", err)
	}
	if provider.createCount != 1 || store.state(key) != "OUTCOME_UNKNOWN" {
		t.Fatalf("create=%d state=%s", provider.createCount, store.state(key))
	}
}

func TestPixStatusOwnerAndForeignRead(t *testing.T) {
	service, _, provider := newPixService(t)
	started, err := service.Start(context.Background(), app.Principal{AccountID: testAccount}, pixRequest())
	if err != nil {
		t.Fatal(err)
	}
	status, err := service.Status(context.Background(), app.Principal{AccountID: testAccount}, started.OrderID)
	if err != nil || status.QRCode == "" || status.PaymentID != started.PaymentID {
		t.Fatalf("status=%+v err=%v", status, err)
	}
	if _, err := service.Status(context.Background(), app.Principal{AccountID: testOther}, started.OrderID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("foreign status err = %v", err)
	}
	if _, err := service.Status(context.Background(), app.Principal{AccountID: testAccount}, "44444444-4444-4444-8444-444444444444"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown order err = %v", err)
	}
	provider.setStatus(started.OrderID, "OVERDUE", app.PixExpired)
	status, err = service.Status(context.Background(), app.Principal{AccountID: testAccount}, started.OrderID)
	if err != nil || status.Status != app.PixExpired || status.QRCode != "" {
		t.Fatalf("expired status=%+v err=%v", status, err)
	}
	provider.setStatus(started.OrderID, "RECEIVED", app.PixSettled)
	status, err = service.Status(context.Background(), app.Principal{AccountID: testAccount}, started.OrderID)
	if err != nil || status.Status != app.PixSettled || status.QRCode != "" {
		t.Fatalf("settled status=%+v err=%v", status, err)
	}
	if provider.createCount != 1 {
		t.Fatal("status created another charge")
	}
}

func TestPixWrongPaymentBindingAndOversizedQR(t *testing.T) {
	service, store, provider := newPixService(t)
	provider.badBinding = true
	if _, err := service.Start(context.Background(), app.Principal{AccountID: testAccount}, pixRequest()); !errors.Is(err, domain.ErrBinding) {
		t.Fatalf("binding err = %v", err)
	}
	if store.state("asaas-pix:"+testOperation) != "OUTCOME_UNKNOWN" {
		t.Fatal("bad payment marked succeeded")
	}

	service, store, provider = newPixService(t)
	provider.qrTooLarge = true
	if _, err := service.Start(context.Background(), app.Principal{AccountID: testAccount}, pixRequest()); !errors.Is(err, domain.ErrBinding) {
		t.Fatalf("qr err = %v", err)
	}
	if store.state("asaas-pix:"+testOperation) != "SUCCEEDED" {
		t.Fatal("payment success should persist even if QR response is rejected")
	}
}
