//go:build integration

package application_test

import (
	"context"
	"testing"
	"time"

	payadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
	paydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
)

type fakeDisputeVerifier struct {
	fact app.DisputeFact
	err  error
}

func (f *fakeDisputeVerifier) VerifyDispute(_ context.Context, _ app.InboxRecord) (app.DisputeFact, error) {
	return f.fact, f.err
}

func disputeInbox(t *testing.T, store *payadapters.Store, eventID, eventType, resourceType, resourceID string) app.InboxRecord {
	t.Helper()
	event := app.InboxEvent{
		Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
		Merchant: refundMerchant, EventID: eventID, PayloadHash: "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		EventType: eventType, ResourceType: resourceType, ResourceID: resourceID,
	}
	if _, err := store.RecordInbox(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	return app.InboxRecord{InboxEvent: event, State: "PENDING", ReceivedAt: time.Now().UTC()}
}

func TestDisputeLostRevokesMatchedUnexpiredGrant(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	refundGrant(t, pool, store, owner, "pay_dispute_lost")
	service := &app.DisputeService{Store: store, Asaas: &fakeDisputeVerifier{fact: app.DisputeFact{
		Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
		Merchant: refundMerchant, RemoteDisputeID: "cb_lost_1", RemotePaymentID: "pay_dispute_lost",
		Amount: paydomain.BRL(1234), State: paydomain.DisputeLost,
		OpenedAt: time.Now().UTC(), LastEventAt: time.Now().UTC(),
	}}}
	event := disputeInbox(t, store, "evt_dispute_lost", "PAYMENT_CHARGEBACK_DISPUTE", "payment", "pay_dispute_lost")
	if err := service.Process(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_disputes WHERE remote_dispute_id='cb_lost_1' AND state='LOST'") != 1 {
		t.Fatal("lost dispute not stored")
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_grants WHERE state='REVOKED'") != 1 {
		t.Fatal("lost dispute did not revoke matched grant")
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_settlements WHERE state='REVERSED'") != 1 {
		t.Fatal("settlement not marked REVERSED")
	}
	requireAccess(t, pool, owner, false)
}

func TestDisputeWonKeepsAccessAndDuplicateReplays(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	refundGrant(t, pool, store, owner, "pay_dispute_won")
	fact := app.DisputeFact{
		Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
		Merchant: refundMerchant, RemoteDisputeID: "cb_won_1", RemotePaymentID: "pay_dispute_won",
		Amount: paydomain.BRL(1234), State: paydomain.DisputeWon,
		OpenedAt: time.Now().UTC(), LastEventAt: time.Now().UTC(),
	}
	service := &app.DisputeService{Store: store, Asaas: &fakeDisputeVerifier{fact: fact}, Proof: &fakeOperatorProof{}}
	event := disputeInbox(t, store, "evt_dispute_won", "PAYMENT_CHARGEBACK_DISPUTE", "payment", "pay_dispute_won")
	if err := service.Process(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	requireAccess(t, pool, owner, true)
	again := disputeInbox(t, store, "evt_dispute_won_again", "PAYMENT_CHARGEBACK_DISPUTE", "payment", "pay_dispute_won")
	if err := service.Process(context.Background(), again); err != nil {
		t.Fatal(err)
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_disputes WHERE remote_dispute_id='cb_won_1'") != 1 {
		t.Fatal("duplicate dispute created a second row")
	}

	// A reversed outcome after a terminal state cannot re-grant or revoke.
	lost := fact
	lost.State = paydomain.DisputeLost
	lost.LastEventAt = fact.LastEventAt.Add(time.Minute)
	reversed := &app.DisputeService{Store: store, Asaas: &fakeDisputeVerifier{fact: lost}}
	third := disputeInbox(t, store, "evt_dispute_won_lost", "PAYMENT_CHARGEBACK_DISPUTE", "payment", "pay_dispute_won")
	if err := reversed.Process(context.Background(), third); err != nil {
		t.Fatal(err)
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_disputes WHERE remote_dispute_id='cb_won_1' AND state='WON'") != 1 {
		t.Fatal("terminal dispute state was overwritten")
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_grants WHERE state='REVOKED'") != 0 {
		t.Fatal("reversed terminal dispute revoked the grant")
	}
	requireAccess(t, pool, owner, true)
}

func TestDisputeUnmatchedPaymentCreatesHold(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	refundGrant(t, pool, store, owner, "pay_dispute_hold_grant")
	fact := app.DisputeFact{
		Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
		Merchant: refundMerchant, RemoteDisputeID: "cb_unknown_1", RemotePaymentID: "pay_unknown",
		Amount: paydomain.BRL(100), State: paydomain.DisputePending,
		OpenedAt: time.Now().UTC(), LastEventAt: time.Now().UTC(),
	}
	service := &app.DisputeService{Store: store, Asaas: &fakeDisputeVerifier{fact: fact}, Proof: &fakeOperatorProof{}}
	event := disputeInbox(t, store, "evt_hold", "PAYMENT_CHARGEBACK_REQUESTED", "payment", "pay_unknown")
	if err := service.Process(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_provider_holds WHERE state='OPEN'") != 1 {
		t.Fatal("unmatched money did not create an explicit hold")
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_grants WHERE state='REVOKED'") != 0 {
		t.Fatal("unmatched hold revoked a grant")
	}
	requireAccess(t, pool, owner, true)

	var holdID string
	if err := pool.QueryRow(context.Background(),
		"SELECT id FROM payment_provider_holds WHERE remote_hold_id='unmatched:cb_unknown_1'").Scan(&holdID); err != nil {
		t.Fatal(err)
	}
	reviewed, err := service.ReviewHold(context.Background(), "op_1", "proof-op_1", holdID, "REVIEWED", "manual review")
	if err != nil || reviewed.State != "REVIEWED" || reviewed.ReviewedBy != "op_1" {
		t.Fatalf("reviewed=%+v err=%v", reviewed, err)
	}
	if countRows(t, pool, "SELECT count(*) FROM payment_operator_actions WHERE action='hold.review'") != 1 {
		t.Fatal("hold review not audited")
	}
}
