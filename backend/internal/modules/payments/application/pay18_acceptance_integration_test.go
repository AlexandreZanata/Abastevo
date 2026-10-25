//go:build integration

// PAY-18 adversarial privacy and performance acceptance: operator proof
// rotation fails closed, restore/stale replays create no financial effect,
// and callback intake stays bounded with indexed hot queries.
package application_test

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	payadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
	paydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
)

// Frozen PAY-18 ceilings: accepted callback intake stays far below these
// bounds on disposable PostGIS; the evidence records the measured values.
const (
	pay18InboxCount      = 200
	pay18MaxInboxTotal   = 120 * time.Second
	pay18MaxInboxMean    = time.Second
	pay18MaxInboxP95     = 2 * time.Second
	pay18FloodRacers     = 64
	pay18MaxFloodElapsed = 60 * time.Second
	pay18MerchantA       = "11111111-1111-4111-8111-111111111111"
	pay18MerchantB       = "22222222-2222-4222-8222-222222222222"
)

type rotatingOperatorProof struct {
	mu      sync.Mutex
	current string
	seen    []string
}

func (f *rotatingOperatorProof) VerifyOperator(_ context.Context, actor, proof, action string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if proof == "" || proof != f.current {
		return paydomain.ErrUnauthorized
	}
	f.seen = append(f.seen, actor+":"+action)
	return nil
}

func (f *rotatingOperatorProof) rotate(proof string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.current = proof
}

func pay18RefundService(store *payadapters.Store, provider *fakeRefundProvider, proof *rotatingOperatorProof) *app.RefundService {
	return &app.RefundService{
		Store: store, Asaas: provider, Proof: proof,
		Config: app.RefundServiceConfig{SingleApprovalAboveCents: 1000000},
	}
}

func TestPAY18RotatedOperatorProofRefusesStaleRefund(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	refundGrant(t, pool, store, owner, "pay_p18_rotation")
	provider := &fakeRefundProvider{}
	proof := &rotatingOperatorProof{current: "rot-proof-1"}
	service := pay18RefundService(store, provider, proof)

	first, err := service.Request(context.Background(), app.RefundRequest{
		Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
		Merchant: refundMerchant, RemotePaymentID: "pay_p18_rotation",
		Reason: "rotation acceptance", ActorID: "op_1", Proof: "rot-proof-1",
		OperationKey: "op_p18_rotation_a",
	})
	if err != nil || first.State != paydomain.RefundSucceeded {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	if provider.createCalls != 1 {
		t.Fatalf("provider writes=%d", provider.createCalls)
	}

	proof.rotate("rot-proof-2")

	// Stale proof on a fresh operation key fails closed before any store
	// mutation or provider write.
	if _, err := service.Request(context.Background(), app.RefundRequest{
		Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
		Merchant: refundMerchant, RemotePaymentID: "pay_p18_rotation",
		Reason: "stale proof replay", ActorID: "op_1", Proof: "rot-proof-1",
		OperationKey: "op_p18_rotation_b",
	}); !errors.Is(err, paydomain.ErrUnauthorized) {
		t.Fatalf("stale proof err=%v", err)
	}
	// Stale proof cannot even replay the completed operation.
	if _, err := service.Request(context.Background(), app.RefundRequest{
		Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
		Merchant: refundMerchant, RemotePaymentID: "pay_p18_rotation",
		Reason: "rotation acceptance", ActorID: "op_1", Proof: "rot-proof-1",
		OperationKey: "op_p18_rotation_a",
	}); !errors.Is(err, paydomain.ErrUnauthorized) {
		t.Fatalf("stale replay err=%v", err)
	}
	// Stale proof cannot drive reconciliation either.
	if _, err := service.Reconcile(context.Background(), "op_1", "rot-proof-1", first.ID); !errors.Is(err, paydomain.ErrUnauthorized) {
		t.Fatalf("stale reconcile err=%v", err)
	}
	if provider.createCalls != 1 {
		t.Fatalf("provider writes after rotation=%d", provider.createCalls)
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_operator_actions WHERE action='refund.request'") != 1 {
		t.Fatal("stale proof recorded an operator action")
	}
}

func TestPAY18RestoreReplayCreatesNoFinancialEffect(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	refundGrant(t, pool, store, owner, "pay_p18_restore")
	provider := &fakeRefundProvider{}
	proof := &rotatingOperatorProof{current: "rot-proof-1"}
	service := pay18RefundService(store, provider, proof)

	partial := paydomain.BRL(400)
	record, err := service.Request(context.Background(), app.RefundRequest{
		Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
		Merchant: refundMerchant, RemotePaymentID: "pay_p18_restore",
		Amount: &partial, Reason: "restore acceptance", ActorID: "op_1", Proof: "rot-proof-1",
		OperationKey: "op_p18_restore",
	})
	if err != nil || record.State != paydomain.RefundSucceeded {
		t.Fatalf("record=%+v err=%v", record, err)
	}
	// A partial refund keeps paid-through access: the restore must preserve
	// exactly this state, neither revoking nor resurrecting anything.
	requireAccess(t, pool, owner, true)

	// Simulate a restored worker re-driving the original settlement event:
	// the durable inbox state makes it a no-op with no resurrected grant.
	event := settlementInbox(t, store, "evt_pay_p18_restore", "pay_p18_restore")
	verifier := &fakeSettlementVerifier{}
	if err := settlementService(store, verifier).Process(context.Background(), event); err != nil {
		t.Fatalf("restore settlement replay err=%v", err)
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_grants") != 1 {
		t.Fatal("restore replay created a grant")
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_grants WHERE state='ACTIVE'") != 1 {
		t.Fatal("restore replay changed grant state")
	}
	requireAccess(t, pool, owner, true)

	// Replaying the completed refund intent converges without a second
	// provider write.
	second, err := service.Request(context.Background(), app.RefundRequest{
		Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
		Merchant: refundMerchant, RemotePaymentID: "pay_p18_restore",
		Amount: &partial, Reason: "restore acceptance", ActorID: "op_1", Proof: "rot-proof-1",
		OperationKey: "op_p18_restore",
	})
	if err != nil || second.ID != record.ID {
		t.Fatalf("second=%+v err=%v", second, err)
	}
	if provider.createCalls != 1 {
		t.Fatalf("provider writes after restore=%d", provider.createCalls)
	}

	// A stale state transition on the terminal record is refused with no
	// mutation and no provider write.
	if _, err := store.UpdateRefundState(context.Background(), record.ID,
		paydomain.RefundPending, paydomain.RefundSucceeded, "re_stale", "", time.Now().UTC()); !errors.Is(err, paydomain.ErrConflict) {
		t.Fatalf("stale transition err=%v", err)
	}
	kept, err := store.GetRefund(context.Background(), record.ID)
	if err != nil || kept.State != paydomain.RefundSucceeded {
		t.Fatalf("kept=%+v err=%v", kept, err)
	}
	if provider.createCalls != 1 {
		t.Fatalf("provider writes after stale transition=%d", provider.createCalls)
	}
}

func TestPAY18ProviderOutageStaysPendingWithoutDuplicating(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	refundGrant(t, pool, store, owner, "pay_p18_outage")
	provider := &fakeRefundProvider{createErr: app.ErrAmbiguous}
	proof := &rotatingOperatorProof{current: "rot-proof-1"}
	service := pay18RefundService(store, provider, proof)

	pending, err := service.Request(context.Background(), app.RefundRequest{
		Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
		Merchant: refundMerchant, RemotePaymentID: "pay_p18_outage",
		Reason: "outage acceptance", ActorID: "op_1", Proof: "rot-proof-1",
		OperationKey: "op_p18_outage",
	})
	if !errors.Is(err, app.ErrAmbiguous) || pending.State != paydomain.RefundPending {
		t.Fatalf("pending=%+v err=%v", pending, err)
	}
	// The outage revoked nothing and the owner keeps paid-through access.
	requireAccess(t, pool, owner, true)

	// Recovery reconciles the one durable intent exactly once.
	provider.createErr = nil
	provider.found = true
	resolved, err := service.Reconcile(context.Background(), "op_1", "rot-proof-1", pending.ID)
	if err != nil || resolved.State != paydomain.RefundSucceeded {
		t.Fatalf("resolved=%+v err=%v", resolved, err)
	}
	if provider.createCalls != 1 {
		t.Fatalf("provider writes=%d", provider.createCalls)
	}
	requireAccess(t, pool, owner, false)
}

func TestPAY18BoundedCallbackIntakeAndHotQueryPlans(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)

	latencies := make([]time.Duration, 0, pay18InboxCount)
	start := time.Now()
	for i := 0; i < pay18InboxCount; i++ {
		event := app.InboxEvent{
			Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
			Merchant: pay18MerchantA, EventID: fmt.Sprintf("evt_p18_perf_%04d", i),
			PayloadHash: strings.Repeat("a", 64), EventType: "PAYMENT_RECEIVED",
			ResourceType: "payment", ResourceID: fmt.Sprintf("pay_p18_perf_%04d", i),
		}
		call := time.Now()
		result, err := store.RecordInbox(context.Background(), event)
		latencies = append(latencies, time.Since(call))
		if err != nil || !result.Created {
			t.Fatalf("insert %d err=%v result=%+v", i, err, result)
		}
	}
	total := time.Since(start)
	if total > pay18MaxInboxTotal {
		t.Fatalf("inbox total=%v exceeds %v", total, pay18MaxInboxTotal)
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	mean := total / time.Duration(pay18InboxCount)
	p95 := latencies[int(float64(len(latencies))*0.95)]
	if mean > pay18MaxInboxMean {
		t.Fatalf("inbox mean=%v exceeds %v", mean, pay18MaxInboxMean)
	}
	if p95 > pay18MaxInboxP95 {
		t.Fatalf("inbox p95=%v exceeds %v", p95, pay18MaxInboxP95)
	}
	t.Logf("PAY-18 intake: n=%d total=%v mean=%v p95=%v", pay18InboxCount, total, mean, p95)

	// Duplicate flood converges on one durable effect with no grant.
	floodStart := time.Now()
	created := 0
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < pay18FloodRacers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := store.RecordInbox(context.Background(), app.InboxEvent{
				Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
				Merchant: pay18MerchantB, EventID: "evt_p18_flood",
				PayloadHash: strings.Repeat("b", 64), EventType: "PAYMENT_RECEIVED",
				ResourceType: "payment", ResourceID: "pay_p18_flood",
			})
			if err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			if result.Created {
				created++
			}
			mu.Unlock()
		}()
	}
	wg.Wait()
	if elapsed := time.Since(floodStart); elapsed > pay18MaxFloodElapsed {
		t.Fatalf("flood elapsed=%v exceeds %v", elapsed, pay18MaxFloodElapsed)
	}
	if created != 1 {
		t.Fatalf("flood created=%d want 1", created)
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_inbox WHERE event_id='evt_p18_flood'") != 1 {
		t.Fatal("flood duplicated the inbox row")
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_grants") != 0 {
		t.Fatal("flood granted access without settlement")
	}

	// Hot queries use indexes, never sequential scans.
	plans := map[string]struct {
		query string
		args  []any
	}{
		"inbox-pk": {
			"EXPLAIN SELECT * FROM payment_inbox WHERE provider=$1 AND environment=$2 AND merchant=$3 AND event_id=$4",
			[]any{"asaas", "test", pay18MerchantA, "evt_p18_perf_0001"},
		},
		"grants-owner": {
			"EXPLAIN SELECT * FROM payment_grants WHERE account_id=$1::uuid AND offer_id=$2 ORDER BY start_at,id",
			[]any{owner, "premium.monthly"},
		},
		"orders-owner": {
			"EXPLAIN SELECT * FROM payment_orders WHERE account_id=$1::uuid",
			[]any{owner},
		},
		"pending-inbox": {
			"EXPLAIN SELECT * FROM payment_inbox WHERE provider=$1 AND environment=$2 AND merchant=$3 AND state='PENDING' ORDER BY received_at,event_id LIMIT 100",
			[]any{"asaas", "test", pay18MerchantA},
		},
	}
	for name, plan := range plans {
		rows, err := pool.Query(context.Background(), plan.query, plan.args...)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var sb strings.Builder
		for rows.Next() {
			var line string
			if err := rows.Scan(&line); err != nil {
				rows.Close()
				t.Fatalf("%s: %v", name, err)
			}
			sb.WriteString(line + "\n")
		}
		rows.Close()
		text := sb.String()
		t.Logf("PAY-18 plan %s:\n%s", name, text)
		if !strings.Contains(text, "Index") {
			t.Fatalf("%s: no index in plan:\n%s", name, text)
		}
		if strings.Contains(text, "Seq Scan") {
			t.Fatalf("%s: sequential scan in plan:\n%s", name, text)
		}
	}
}
