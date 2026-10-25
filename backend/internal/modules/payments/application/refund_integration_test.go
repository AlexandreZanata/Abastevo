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
	subdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fakeOperatorProof struct {
	mu   sync.Mutex
	seen []string
}

func (f *fakeOperatorProof) VerifyOperator(_ context.Context, actor, proof, action string) error {
	if proof != "proof-"+actor {
		return paydomain.ErrUnauthorized
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seen = append(f.seen, actor+":"+action)
	return nil
}

type fakeRefundProvider struct {
	mu          sync.Mutex
	createCalls int
	findCalls   int
	createErr   error
	createState paydomain.RefundState
	findState   paydomain.RefundState
	found       bool
	complete    bool
	findErr     error
	findID      string
}

func (f *fakeRefundProvider) CreateRefund(_ context.Context, request app.RefundProviderRequest) (app.RefundProviderResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createCalls++
	if f.createErr != nil {
		return app.RefundProviderResult{}, f.createErr
	}
	state := f.createState
	if state == "" {
		state = paydomain.RefundSucceeded
	}
	return app.RefundProviderResult{
		ID: "re_" + request.RefundID, State: state, RawStatus: string(state),
		Amount: request.Amount, At: time.Now().UTC(),
	}, nil
}

func (f *fakeRefundProvider) FindRefund(_ context.Context, request app.RefundProviderRequest) (app.RefundProviderResult, bool, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.findCalls++
	if f.findErr != nil {
		return app.RefundProviderResult{}, false, false, f.findErr
	}
	if !f.found {
		return app.RefundProviderResult{}, false, f.complete, nil
	}
	state := f.findState
	if state == "" {
		state = paydomain.RefundSucceeded
	}
	id := f.findID
	if id == "" {
		id = "re_" + request.RefundID
	}
	return app.RefundProviderResult{
		ID: id, State: state, RawStatus: string(state),
		Amount: request.Amount, At: time.Now().UTC(),
	}, true, true, nil
}

const refundMerchant = "11111111-1111-4111-8111-111111111111"

func refundService(store *payadapters.Store, provider *fakeRefundProvider, proof *fakeOperatorProof) *app.RefundService {
	return &app.RefundService{
		Store: store, Asaas: provider, Proof: proof,
		Config: app.RefundServiceConfig{SingleApprovalAboveCents: 1000000},
	}
}

func refundGrant(t *testing.T, pool *pgxpool.Pool, store *payadapters.Store, owner, paymentID string) {
	t.Helper()
	order, _, customerID := settlementOrder(t, store, owner, "refund-"+paymentID)
	event := settlementInbox(t, store, "evt_"+paymentID, paymentID)
	verifier := &fakeSettlementVerifier{candidate: settlementCandidate(order, owner, customerID, paymentID)}
	if err := settlementService(store, verifier).Process(context.Background(), event); err != nil {
		t.Fatal(err)
	}
}

func requireAccess(t *testing.T, pool *pgxpool.Pool, owner string, want bool) {
	t.Helper()
	subStore := subadapters.NewStore(pool)
	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	err = subStore.RequireCapability(context.Background(), tx, owner, "cloud.sync")
	if want && err != nil {
		t.Fatalf("expected access, got %v", err)
	}
	if !want && !errors.Is(err, subdomain.ErrDenied) {
		t.Fatalf("expected denial, got %v", err)
	}
}

func TestRefundFullRevokesMatchedUnexpiredGrant(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	refundGrant(t, pool, store, owner, "pay_refund_full")
	provider := &fakeRefundProvider{}
	proof := &fakeOperatorProof{}
	service := refundService(store, provider, proof)

	record, err := service.Request(context.Background(), app.RefundRequest{
		Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
		Merchant: refundMerchant, RemotePaymentID: "pay_refund_full",
		Reason: "duplicate charge", ActorID: "op_1", Proof: "proof-op_1",
		OperationKey: "op_refund_full",
	})
	if err != nil || record.State != paydomain.RefundSucceeded || record.RemoteRefundID == "" {
		t.Fatalf("record=%+v err=%v", record, err)
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_grants WHERE state='REVOKED'") != 1 {
		t.Fatal("full refund did not revoke the matched grant")
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_settlements WHERE state='REFUNDED'") != 1 {
		t.Fatal("settlement not marked REFUNDED")
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_operator_actions WHERE action='refund.request'") != 1 {
		t.Fatal("operator action not audited")
	}
	requireAccess(t, pool, owner, false)
}

func TestRefundPartialKeepsAccess(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	refundGrant(t, pool, store, owner, "pay_refund_partial")
	provider := &fakeRefundProvider{}
	service := refundService(store, provider, &fakeOperatorProof{})
	amount := paydomain.BRL(500)

	record, err := service.Request(context.Background(), app.RefundRequest{
		Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
		Merchant: refundMerchant, RemotePaymentID: "pay_refund_partial", Amount: &amount,
		Reason: "partial goodwill", ActorID: "op_1", Proof: "proof-op_1",
		OperationKey: "op_refund_partial",
	})
	if err != nil || record.State != paydomain.RefundSucceeded {
		t.Fatalf("record=%+v err=%v", record, err)
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_grants WHERE state='ACTIVE'") != 1 {
		t.Fatal("partial refund revoked access")
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_settlements WHERE state='PARTIALLY_REFUNDED'") != 1 {
		t.Fatal("settlement not marked PARTIALLY_REFUNDED")
	}
	requireAccess(t, pool, owner, true)

	rest := paydomain.BRL(734)
	record, err = service.Request(context.Background(), app.RefundRequest{
		Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
		Merchant: refundMerchant, RemotePaymentID: "pay_refund_partial", Amount: &rest,
		Reason: "rest", ActorID: "op_1", Proof: "proof-op_1",
		OperationKey: "op_refund_rest",
	})
	if err != nil || record.State != paydomain.RefundSucceeded {
		t.Fatalf("record=%+v err=%v", record, err)
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_grants WHERE state='REVOKED'") != 1 {
		t.Fatal("cumulative full refund did not revoke")
	}
	requireAccess(t, pool, owner, false)
}

func TestRefundConcurrentRequestsRespectPrincipal(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	refundGrant(t, pool, store, owner, "pay_refund_race")
	provider := &fakeRefundProvider{}
	service := refundService(store, provider, &fakeOperatorProof{})

	amount := paydomain.BRL(800)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			op := "op_refund_race_a"
			if i == 1 {
				op = "op_refund_race_b"
			}
			_, errs[i] = service.Request(context.Background(), app.RefundRequest{
				Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
				Merchant: refundMerchant, RemotePaymentID: "pay_refund_race", Amount: &amount,
				Reason: "race", ActorID: "op_1", Proof: "proof-op_1", OperationKey: op,
			})
		}(i)
	}
	wg.Wait()
	succeeded, bounded := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, paydomain.ErrRefundBounds):
			bounded++
		default:
			t.Fatalf("unexpected err=%v", err)
		}
	}
	if succeeded != 1 || bounded != 1 {
		t.Fatalf("succeeded=%d bounded=%d", succeeded, bounded)
	}
	if provider.createCalls != 1 {
		t.Fatalf("provider writes=%d", provider.createCalls)
	}
}

func TestRefundDuplicateOperationReplaysWithoutSecondWrite(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	refundGrant(t, pool, store, owner, "pay_refund_dupe")
	provider := &fakeRefundProvider{}
	service := refundService(store, provider, &fakeOperatorProof{})
	amount := paydomain.BRL(400)
	input := app.RefundRequest{
		Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
		Merchant: refundMerchant, RemotePaymentID: "pay_refund_dupe", Amount: &amount,
		Reason: "dupe", ActorID: "op_1", Proof: "proof-op_1", OperationKey: "op_refund_dupe",
	}
	first, err := service.Request(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Request(context.Background(), input)
	if err != nil || second.ID != first.ID {
		t.Fatalf("second=%+v err=%v", second, err)
	}
	if provider.createCalls != 1 {
		t.Fatalf("provider writes=%d", provider.createCalls)
	}
}

func TestRefundLostReplyReconcilesWithoutSecondWrite(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	refundGrant(t, pool, store, owner, "pay_refund_lost")
	provider := &fakeRefundProvider{createErr: app.ErrAmbiguous, found: true}
	proof := &fakeOperatorProof{}
	service := refundService(store, provider, proof)
	amount := paydomain.BRL(1234)

	record, err := service.Request(context.Background(), app.RefundRequest{
		Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
		Merchant: refundMerchant, RemotePaymentID: "pay_refund_lost", Amount: &amount,
		Reason: "lost reply", ActorID: "op_1", Proof: "proof-op_1",
		OperationKey: "op_refund_lost",
	})
	if !errors.Is(err, app.ErrAmbiguous) || record.State != paydomain.RefundPending {
		t.Fatalf("record=%+v err=%v", record, err)
	}
	reconciled, err := service.Reconcile(context.Background(), "op_1", "proof-op_1", record.ID)
	if err != nil || reconciled.State != paydomain.RefundSucceeded {
		t.Fatalf("reconciled=%+v err=%v", reconciled, err)
	}
	if provider.createCalls != 1 {
		t.Fatalf("provider writes=%d, want exactly one remote write", provider.createCalls)
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_refunds") != 1 {
		t.Fatal("duplicate local refund row created")
	}
}

func TestRefundInsufficientBalanceFailsWithoutRevoking(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	refundGrant(t, pool, store, owner, "pay_refund_short")
	provider := &fakeRefundProvider{createErr: paydomain.ErrInvalid}
	service := refundService(store, provider, &fakeOperatorProof{})
	amount := paydomain.BRL(1234)

	record, err := service.Request(context.Background(), app.RefundRequest{
		Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
		Merchant: refundMerchant, RemotePaymentID: "pay_refund_short", Amount: &amount,
		Reason: "insufficient", ActorID: "op_1", Proof: "proof-op_1",
		OperationKey: "op_refund_short",
	})
	if err == nil || record.State != paydomain.RefundFailed {
		t.Fatalf("record=%+v err=%v", record, err)
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_grants WHERE state='ACTIVE'") != 1 {
		t.Fatal("failed refund revoked access")
	}
	requireAccess(t, pool, owner, true)

	// Failed refunds free the reserved balance: a smaller retry succeeds.
	provider.createErr = nil
	retry := paydomain.BRL(100)
	if _, err := service.Request(context.Background(), app.RefundRequest{
		Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
		Merchant: refundMerchant, RemotePaymentID: "pay_refund_short", Amount: &retry,
		Reason: "retry smaller", ActorID: "op_1", Proof: "proof-op_1",
		OperationKey: "op_refund_short_retry",
	}); err != nil {
		t.Fatalf("retry err=%v", err)
	}
}

func TestRefundAboveThresholdRequiresSecondApprover(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	refundGrant(t, pool, store, owner, "pay_refund_approve")
	service := refundService(store, &fakeRefundProvider{}, &fakeOperatorProof{})
	service.Config.SingleApprovalAboveCents = 500
	amount := paydomain.BRL(1000)

	base := app.RefundRequest{
		Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
		Merchant: refundMerchant, RemotePaymentID: "pay_refund_approve", Amount: &amount,
		Reason: "large", ActorID: "op_1", Proof: "proof-op_1",
		OperationKey: "op_refund_approve_1",
	}
	if _, err := service.Request(context.Background(), base); !errors.Is(err, paydomain.ErrUnauthorized) {
		t.Fatalf("missing approval err=%v", err)
	}
	self := base
	self.OperationKey = "op_refund_approve_2"
	self.ApprovedBy, self.ApprovalProof = "op_1", "proof-op_1"
	if _, err := service.Request(context.Background(), self); !errors.Is(err, paydomain.ErrUnauthorized) {
		t.Fatalf("self approval err=%v", err)
	}
	dual := base
	dual.OperationKey = "op_refund_approve_3"
	dual.ApprovedBy, dual.ApprovalProof = "op_2", "proof-op_2"
	record, err := service.Request(context.Background(), dual)
	if err != nil || record.RequestedBy != "op_1" || record.ApprovedBy != "op_2" {
		t.Fatalf("record=%+v err=%v", record, err)
	}
}

func TestRefundUnknownPaymentCreatesNothing(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	seedCheckoutOwner(t, pool)
	service := refundService(store, &fakeRefundProvider{}, &fakeOperatorProof{})
	amount := paydomain.BRL(100)
	_, err := service.Request(context.Background(), app.RefundRequest{
		Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
		Merchant: refundMerchant, RemotePaymentID: "pay_missing", Amount: &amount,
		Reason: "missing", ActorID: "op_1", Proof: "proof-op_1",
		OperationKey: "op_refund_missing",
	})
	if !errors.Is(err, paydomain.ErrNotFound) {
		t.Fatalf("err=%v", err)
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_refunds") != 0 {
		t.Fatal("refund row created for unknown payment")
	}
}
