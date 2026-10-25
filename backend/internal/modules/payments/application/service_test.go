package application_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
)

const (
	testAccount   = "11111111-1111-4111-8111-111111111111"
	testOther     = "22222222-2222-4222-8222-222222222222"
	testOperation = "33333333-3333-4333-8333-333333333333"
	testOffer     = "premium.monthly"
)

type memoryStore struct {
	mu         sync.Mutex
	orders     map[string]domain.Order
	customers  map[string]string
	operations map[string]*memoryOperation
	// beforeCreate, when set, runs before CreateOrder takes the lock so
	// tests can rendezvous racers on the build-then-store boundary.
	beforeCreate func()
}

type memoryOperation struct {
	claim       app.Claim
	payloadHash string
	kind        string
	orderID     string
	state       string
	remoteID    string
	lease       time.Time
}

func newMemoryStore() *memoryStore {
	return &memoryStore{orders: map[string]domain.Order{}, customers: map[string]string{}, operations: map[string]*memoryOperation{}}
}

func (s *memoryStore) CreateOrder(_ context.Context, order domain.Order) error {
	if s.beforeCreate != nil {
		s.beforeCreate()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.orders[order.ID]; ok {
		// Mirror Store.CreateOrder: a repeated creation converges only
		// for the identical durable row, timestamps included. Anything
		// else conflicts so the service refetches the winner instead of
		// claiming with a divergent payload.
		if existing.AccountID != order.AccountID || existing.OfferID != order.OfferID ||
			existing.Channel != order.Channel || !existing.Amount.Equal(order.Amount) ||
			existing.CatalogVersion != order.CatalogVersion || !equalStrings(existing.Capabilities, order.Capabilities) ||
			!existing.QuotedAt.Equal(order.QuotedAt) || !existing.ExpiresAt.Equal(order.ExpiresAt) {
			return domain.ErrConflict
		}
		return nil
	}
	s.orders[order.ID] = order
	return nil
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func (s *memoryStore) GetOrder(_ context.Context, id string) (domain.Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	order, ok := s.orders[id]
	if !ok {
		return domain.Order{}, domain.ErrNotFound
	}
	return order, nil
}

func (s *memoryStore) Customer(_ context.Context, accountID string, provider domain.Provider, environment domain.Environment, merchant string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.customers[customerKey(accountID, provider, environment, merchant)]
	return value, ok, nil
}

func (s *memoryStore) MapCustomer(_ context.Context, mapping app.CustomerMapping) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := customerKey(mapping.AccountID, mapping.Provider, mapping.Environment, mapping.Merchant)
	if existing, ok := s.customers[key]; ok && existing != mapping.RemoteCustomerID {
		return domain.ErrConflict
	}
	s.customers[key] = mapping.RemoteCustomerID
	return nil
}

func (s *memoryStore) ClaimAttempt(_ context.Context, input app.ClaimAttemptInput) (app.Claim, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := operationKey(input.AccountID, input.Provider, input.Environment, input.Merchant, input.OperationKey)
	now := time.Now().UTC()
	if op, ok := s.operations[key]; ok {
		if op.payloadHash != input.PayloadHash || op.kind != input.Kind || op.orderID != input.OrderID {
			return app.Claim{}, domain.ErrConflict
		}
		if op.state == "SUCCEEDED" || op.state == "FAILED" || op.state == "CANCELED" || op.state == "QUARANTINED" {
			return app.Claim{OperationID: key, Generation: op.claim.Generation, AttemptNo: op.claim.AttemptNo, State: op.state, RemoteID: op.remoteID, Replay: true}, nil
		}
		if op.lease.After(now) {
			return app.Claim{}, domain.ErrBusy
		}
		op.claim.Generation++
		op.claim.AttemptNo++
		op.state = "PENDING"
		op.lease = input.LeaseUntil
		op.claim.State = "PENDING"
		op.claim.LeaseUntil = input.LeaseUntil
		return op.claim, nil
	}
	claim := app.Claim{OperationID: key, Generation: 1, AttemptNo: 1, State: "PENDING", LeaseUntil: input.LeaseUntil}
	s.operations[key] = &memoryOperation{claim: claim, payloadHash: input.PayloadHash, kind: input.Kind, orderID: input.OrderID, state: "PENDING", lease: input.LeaseUntil}
	return claim, nil
}

func (s *memoryStore) MarkDispatched(_ context.Context, claim app.Claim) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	op, ok := s.operations[claim.OperationID]
	if !ok || op.claim.Generation != claim.Generation {
		return domain.ErrStale
	}
	op.state = "DISPATCHED"
	op.claim.State = "DISPATCHED"
	return nil
}

func (s *memoryStore) MarkOutcome(_ context.Context, claim app.Claim, state domain.AttemptState, remoteID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	op, ok := s.operations[claim.OperationID]
	if !ok || op.claim.Generation != claim.Generation {
		return domain.ErrStale
	}
	switch state {
	case domain.AttemptSucceeded:
		op.state, op.remoteID = "SUCCEEDED", remoteID
	case domain.AttemptFailed:
		op.state = "FAILED"
	case domain.AttemptUnknown:
		op.state = "OUTCOME_UNKNOWN"
		op.remoteID = remoteID
	case domain.AttemptQuarantined:
		op.state = "QUARANTINED"
	case domain.AttemptCanceled:
		op.state = "CANCELED"
	default:
		return domain.ErrInvalid
	}
	op.claim.State = op.state
	op.claim.RemoteID = op.remoteID
	return nil
}

func (s *memoryStore) expire(operationKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, op := range s.operations {
		if strings.HasSuffix(key, "|"+operationKey) {
			op.lease = time.Now().UTC().Add(-time.Second)
		}
	}
}

func (s *memoryStore) state(operationKey string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, op := range s.operations {
		if strings.HasSuffix(key, "|"+operationKey) {
			return op.state
		}
	}
	return ""
}

func operationKey(accountID string, provider domain.Provider, environment domain.Environment, merchant, key string) string {
	return accountID + "|" + string(provider) + "|" + string(environment) + "|" + merchant + "|" + key
}

func customerKey(accountID string, provider domain.Provider, environment domain.Environment, merchant string) string {
	return accountID + "|" + string(provider) + "|" + string(environment) + "|" + merchant
}

type fakeProvider struct {
	mu             sync.Mutex
	sessions       map[string]app.CheckoutSession
	byOrder        map[string]string
	last           app.CheckoutRequest
	ensureCount    int
	createCount    int
	retrieveCount  int
	findCount      int
	ambiguousOnce  bool
	findFound      bool
	findDefinitive bool
	badAmount      bool
}

func newFakeProvider() *fakeProvider {
	return &fakeProvider{sessions: map[string]app.CheckoutSession{}, byOrder: map[string]string{}, findFound: true, findDefinitive: true}
}

func (p *fakeProvider) EnsureCustomer(_ context.Context, request app.CustomerRequest) (app.Customer, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ensureCount++
	return app.Customer{ID: "cus_" + request.AccountID}, nil
}

func (p *fakeProvider) CreateCheckout(_ context.Context, request app.CheckoutRequest) (app.CheckoutSession, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.last = request
	p.createCount++
	id := "cs_test_" + request.Order.ID
	session := app.CheckoutSession{
		ID: id, URL: "https://checkout.example.invalid/" + id, Status: "open", Mode: "subscription",
		CustomerID: request.CustomerID, Livemode: request.Environment == domain.EnvironmentProduction,
		AmountTotal: request.Order.Amount.Centavos, Currency: "brl",
	}
	if p.badAmount {
		session.AmountTotal++
	}
	p.sessions[id] = session
	p.byOrder[request.Order.ID] = id
	if p.ambiguousOnce {
		p.ambiguousOnce = false
		return session, app.ErrAmbiguous
	}
	return session, nil
}

func (p *fakeProvider) RetrieveCheckout(_ context.Context, id string) (app.CheckoutSession, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.retrieveCount++
	session, ok := p.sessions[id]
	if !ok {
		return app.CheckoutSession{}, domain.ErrUnavailable
	}
	return session, nil
}

func (p *fakeProvider) FindCheckout(_ context.Context, _, orderID string) (app.CheckoutSession, bool, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.findCount++
	if !p.findDefinitive {
		return app.CheckoutSession{}, false, false, nil
	}
	id, ok := p.byOrder[orderID]
	if !ok || !p.findFound {
		return app.CheckoutSession{}, false, true, nil
	}
	return p.sessions[id], true, true, nil
}

func (p *fakeProvider) complete(orderID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if id, ok := p.byOrder[orderID]; ok {
		session := p.sessions[id]
		session.Status = "complete"
		p.sessions[id] = session
	}
}

func newService(t *testing.T) (*app.Service, *memoryStore, *fakeProvider) {
	t.Helper()
	offer := domain.Offer{
		ID: testOffer, Family: "premium", Term: domain.TermMonthly, Price: domain.BRL(1234),
		Capabilities: domain.PremiumCapabilityList(),
		Channels: map[domain.Channel]bool{
			domain.ChannelCard: true, domain.ChannelPixManual: true, domain.ChannelPixAutomatic: true,
		},
		SaleEnabled: true,
	}
	catalog, err := domain.NewCatalog("pay-catalog-v1", offer)
	if err != nil {
		t.Fatal(err)
	}
	store := newMemoryStore()
	provider := newFakeProvider()
	service := &app.Service{
		Store: store, Provider: provider, Catalog: catalog,
		Config: app.Config{
			Environment: domain.EnvironmentTest, MerchantID: "acct_synthetic", PriceID: "price_synthetic",
			CatalogVersion: "pay-catalog-v1", ReturnOrigin: "https://app.example.invalid",
			NewSalesEnabled: true, LeaseTTL: time.Minute,
		},
	}
	return service, store, provider
}

func request() app.StartRequest {
	return app.StartRequest{OperationID: testOperation, OfferID: testOffer}
}

func TestStartCreatesServerPricedCheckout(t *testing.T) {
	service, store, provider := newService(t)
	checkout, err := service.Start(context.Background(), app.Principal{AccountID: testAccount}, request())
	if err != nil {
		t.Fatal(err)
	}
	if checkout.OrderID == "" || checkout.SessionID == "" || checkout.URL == "" || checkout.Replayed {
		t.Fatalf("checkout = %+v", checkout)
	}
	if provider.createCount != 1 || provider.ensureCount != 1 {
		t.Fatalf("provider calls create=%d ensure=%d", provider.createCount, provider.ensureCount)
	}
	if provider.last.Order.Amount != domain.BRL(1234) || provider.last.CustomerID != "cus_"+testAccount ||
		provider.last.PriceID != "price_synthetic" || provider.last.IdempotencyKey != "pay:checkout:"+testOperation {
		t.Fatalf("checkout request = %+v", provider.last)
	}
	if provider.last.SuccessURL != "https://app.example.invalid/billing/checkout/return?session_id={CHECKOUT_SESSION_ID}" ||
		provider.last.CancelURL != "https://app.example.invalid/billing/checkout/cancel" {
		t.Fatalf("return URLs = %q %q", provider.last.SuccessURL, provider.last.CancelURL)
	}
	if store.state("stripe-checkout:"+testOperation) != "SUCCEEDED" {
		t.Fatal("operation not succeeded")
	}
	if got, ok, _ := store.Customer(context.Background(), testAccount, domain.ProviderStripe, domain.EnvironmentTest, "acct_synthetic"); !ok || got != "cus_"+testAccount {
		t.Fatalf("customer mapping = %q %v", got, ok)
	}
}

func TestStartRejectsClientTampering(t *testing.T) {
	service, store, provider := newService(t)
	amount := int64(1)
	cases := map[string]app.StartRequest{
		"amount":      {OperationID: testOperation, OfferID: testOffer, AmountCentavos: &amount},
		"currency":    {OperationID: testOperation, OfferID: testOffer, Currency: "USD"},
		"customer":    {OperationID: testOperation, OfferID: testOffer, CustomerID: "cus_evil"},
		"order":       {OperationID: testOperation, OfferID: testOffer, OrderID: "44444444-4444-4444-8444-444444444444"},
		"success_url": {OperationID: testOperation, OfferID: testOffer, SuccessURL: "https://evil.example"},
		"cancel_url":  {OperationID: testOperation, OfferID: testOffer, CancelURL: "https://evil.example"},
		"metadata":    {OperationID: testOperation, OfferID: testOffer, Metadata: map[string]string{"amount": "1"}},
	}
	for name, request := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := service.Start(context.Background(), app.Principal{AccountID: testAccount}, request); !errors.Is(err, domain.ErrInvalid) {
				t.Fatalf("err = %v", err)
			}
		})
	}
	if len(store.orders) != 0 || provider.createCount != 0 || provider.ensureCount != 0 {
		t.Fatal("tampered request touched payment state")
	}
}

func TestStartRefusesDisabledConfigAndServerOffer(t *testing.T) {
	service, _, provider := newService(t)
	service.Config.NewSalesEnabled = false
	if _, err := service.Start(context.Background(), app.Principal{AccountID: testAccount}, request()); !errors.Is(err, domain.ErrSaleDisabled) {
		t.Fatalf("disabled err = %v", err)
	}
	service.Config.NewSalesEnabled = true
	service.Config.CatalogVersion = "other"
	if _, err := service.Start(context.Background(), app.Principal{AccountID: testAccount}, request()); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("catalog mismatch err = %v", err)
	}
	service.Config.CatalogVersion = "pay-catalog-v1"
	if _, err := service.Start(context.Background(), app.Principal{AccountID: testAccount}, app.StartRequest{OperationID: testOperation, OfferID: "unknown"}); !errors.Is(err, domain.ErrUnknownOffer) {
		t.Fatalf("unknown offer err = %v", err)
	}
	if provider.createCount != 0 {
		t.Fatal("provider called for refused offer")
	}
}

func TestConcurrentStartsCreateOneCheckout(t *testing.T) {
	service, store, provider := newService(t)
	const n = 6
	errs := make([]error, n)
	results := make([]app.Checkout, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = service.Start(context.Background(), app.Principal{AccountID: testAccount}, request())
		}(i)
	}
	wg.Wait()
	sessions := map[string]bool{}
	success := 0
	for i, err := range errs {
		if err == nil {
			success++
			if results[i].SessionID == "" {
				t.Fatal("missing session")
			}
			sessions[results[i].SessionID] = true
		} else if !errors.Is(err, domain.ErrBusy) {
			t.Fatalf("unexpected err = %v", err)
		}
	}
	if success == 0 || len(sessions) != 1 {
		t.Fatalf("success=%d unique sessions=%d", success, len(sessions))
	}
	if provider.createCount != 1 {
		t.Fatalf("create count = %d", provider.createCount)
	}
	if len(store.orders) != 1 {
		t.Fatalf("orders = %d", len(store.orders))
	}
}

func TestLostResponseRecoversWithoutSecondCheckout(t *testing.T) {
	service, store, provider := newService(t)
	provider.ambiguousOnce = true
	if _, err := service.Start(context.Background(), app.Principal{AccountID: testAccount}, request()); !errors.Is(err, app.ErrAmbiguous) {
		t.Fatalf("first err = %v", err)
	}
	key := "stripe-checkout:" + testOperation
	if store.state(key) != "OUTCOME_UNKNOWN" {
		t.Fatalf("state = %s", store.state(key))
	}
	store.expire(key)
	checkout, err := service.Start(context.Background(), app.Principal{AccountID: testAccount}, request())
	if err != nil {
		t.Fatal(err)
	}
	if !checkout.Replayed || provider.createCount != 1 || provider.findCount != 1 {
		t.Fatalf("recovery checkout=%+v create=%d find=%d", checkout, provider.createCount, provider.findCount)
	}
	if store.state(key) != "SUCCEEDED" {
		t.Fatal("recovery did not persist success")
	}
	checkout, err = service.Start(context.Background(), app.Principal{AccountID: testAccount}, request())
	if err != nil || !checkout.Replayed || provider.createCount != 1 || provider.retrieveCount != 1 {
		t.Fatalf("replay checkout=%+v err=%v create=%d retrieve=%d", checkout, err, provider.createCount, provider.retrieveCount)
	}
}

func TestInconclusiveRecoveryDoesNotCreateCheckout(t *testing.T) {
	service, store, provider := newService(t)
	provider.ambiguousOnce = true
	_, _ = service.Start(context.Background(), app.Principal{AccountID: testAccount}, request())
	key := "stripe-checkout:" + testOperation
	store.expire(key)
	provider.findDefinitive = false
	if _, err := service.Start(context.Background(), app.Principal{AccountID: testAccount}, request()); !errors.Is(err, app.ErrAmbiguous) {
		t.Fatalf("err = %v", err)
	}
	if provider.createCount != 1 || provider.findCount != 1 || store.state(key) != "OUTCOME_UNKNOWN" {
		t.Fatalf("create=%d find=%d state=%s", provider.createCount, provider.findCount, store.state(key))
	}
}

func TestSessionBindingMismatchIsRefused(t *testing.T) {
	service, store, provider := newService(t)
	provider.badAmount = true
	if _, err := service.Start(context.Background(), app.Principal{AccountID: testAccount}, request()); !errors.Is(err, domain.ErrBinding) {
		t.Fatalf("err = %v", err)
	}
	if store.state("stripe-checkout:"+testOperation) != "OUTCOME_UNKNOWN" {
		t.Fatal("bad session was marked successful")
	}
}

func TestCompletedSessionOnRecoveryIsStale(t *testing.T) {
	service, store, provider := newService(t)
	provider.ambiguousOnce = true
	_, _ = service.Start(context.Background(), app.Principal{AccountID: testAccount}, request())
	key := "stripe-checkout:" + testOperation
	store.expire(key)
	orderID := ""
	for id := range store.orders {
		orderID = id
	}
	provider.complete(orderID)
	if _, err := service.Start(context.Background(), app.Principal{AccountID: testAccount}, request()); !errors.Is(err, domain.ErrStale) {
		t.Fatalf("err = %v", err)
	}
	if provider.createCount != 1 || store.state(key) != "SUCCEEDED" {
		t.Fatalf("create=%d state=%s", provider.createCount, store.state(key))
	}
}

func TestOwnerIsolationUsesAccountBoundOrder(t *testing.T) {
	service, _, provider := newService(t)
	first, err := service.Start(context.Background(), app.Principal{AccountID: testAccount}, request())
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Start(context.Background(), app.Principal{AccountID: testOther}, request())
	if err != nil {
		t.Fatal(err)
	}
	if first.OrderID == second.OrderID || provider.createCount != 2 {
		t.Fatalf("orders %s %s create=%d", first.OrderID, second.OrderID, provider.createCount)
	}
}

func (s *memoryStore) ActiveRecurringSource(context.Context, string, string) (app.RecurringSource, bool, error) {
	return app.RecurringSource{}, false, nil
}

func (s *memoryStore) ActiveGrantEnd(context.Context, string, string) (time.Time, error) {
	return time.Time{}, nil
}

func (s *memoryStore) OpenSourceSwitch(context.Context, string, string) (app.SourceSwitchRecord, bool, error) {
	return app.SourceSwitchRecord{}, false, nil
}

func TestMemoryStoreCreateOrderRequiresIdenticalRow(t *testing.T) {
	service, store, _ := newPixService(t)
	started, err := service.Start(context.Background(), app.Principal{AccountID: testAccount}, pixRequest())
	if err != nil {
		t.Fatal(err)
	}
	stored, err := store.GetOrder(context.Background(), started.OrderID)
	if err != nil {
		t.Fatal(err)
	}
	// Identical repeat converges like the durable insert.
	if err := store.CreateOrder(context.Background(), stored); err != nil {
		t.Fatalf("identical repeat err = %v", err)
	}
	for name, mutate := range map[string]func(*domain.Order){
		"quoted-at":    func(o *domain.Order) { o.QuotedAt = o.QuotedAt.Add(time.Second) },
		"expires-at":   func(o *domain.Order) { o.ExpiresAt = o.ExpiresAt.Add(time.Second) },
		"capabilities": func(o *domain.Order) { o.Capabilities = append(append([]string(nil), o.Capabilities...), "extra.cap") },
		"offer":        func(o *domain.Order) { o.OfferID = "premium.other" },
		"account":      func(o *domain.Order) { o.AccountID = testOther },
	} {
		t.Run(name, func(t *testing.T) {
			divergent := stored
			mutate(&divergent)
			if err := store.CreateOrder(context.Background(), divergent); !errors.Is(err, domain.ErrConflict) {
				t.Fatalf("err = %v", err)
			}
		})
	}
	if len(store.orders) != 1 {
		t.Fatalf("orders = %d", len(store.orders))
	}
}
