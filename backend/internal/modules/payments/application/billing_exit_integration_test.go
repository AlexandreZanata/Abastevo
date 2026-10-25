//go:build integration

package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	payadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
	paydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
)

type fakeOwnerProof struct {
	reject map[string]bool
}

func (f *fakeOwnerProof) VerifyFreshOwner(_ context.Context, accountID, proof, action string) error {
	if f.reject[accountID] || proof != "owner-"+accountID {
		return paydomain.ErrUnauthorized
	}
	return nil
}

func newBillingExitService(store *payadapters.Store, proof app.OwnerProof, fencer app.StripeRenewalFencer, pixAuto *app.PixAutoService) *app.BillingExitService {
	return &app.BillingExitService{
		Store: store, Stripe: fencer, PixAuto: pixAuto, Proof: proof,
		WorkerID: "billing-exit-test", LeaseTTL: time.Minute,
	}
}

func TestBillingExitCardCancelPendingThenReconcile(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	seedCardContract(t, store, owner, "sub_1")
	fencer := &fakeStripeRenewal{fact: app.LifecycleFact{
		AccountID: owner, OfferID: testOffer, Environment: paydomain.EnvironmentTest,
		Merchant: switchMerchant, Provider: paydomain.ProviderStripe,
		RemoteSubscriptionID: "sub_1", RemoteCustomerID: "cus_" + owner,
		ProviderStatus: "active", LastEventAt: time.Now().UTC(),
	}}
	proof := &fakeOwnerProof{reject: map[string]bool{}}
	service := newBillingExitService(store, proof, fencer, nil)
	if _, err := service.Cancel(context.Background(), app.Principal{AccountID: owner},
		"61616161-6161-4161-8161-616161616161", "wrong"); !errors.Is(err, paydomain.ErrUnauthorized) {
		t.Fatalf("wrong proof err=%v", err)
	}
	if _, found, _ := store.LatestBillingExit(context.Background(), owner); found {
		t.Fatal("wrong proof created an exit")
	}
	fencer.cancelErr = paydomain.ErrUnavailable
	exit, err := service.Cancel(context.Background(), app.Principal{AccountID: owner},
		"62626262-6262-4262-8262-626262626262", "owner-"+owner)
	if !errors.Is(err, app.ErrAmbiguous) || exit.State != "PENDING" {
		t.Fatalf("outage exit=%+v err=%v", exit, err)
	}
	sources, err := store.BillingExitSources(context.Background(), exit.ID)
	if err != nil || len(sources) != 1 || sources[0].State != "PENDING" || sources[0].LastError == "" {
		t.Fatalf("sources=%+v err=%v", sources, err)
	}
	fencer.cancelErr = nil
	fencer.fenced = true
	exit, err = service.Reconcile(context.Background(), exit.ID)
	if err != nil || exit.State != "DONE" || fencer.cancelCount != 1 {
		t.Fatalf("reconciled=%+v err=%v cancels=%d", exit, err, fencer.cancelCount)
	}
	contract, err := store.ContractForAccount(context.Background(), owner, testOffer)
	if err != nil || !contract.CancelAtPeriodEnd {
		t.Fatalf("contract=%+v err=%v", contract, err)
	}
}

func TestBillingExitPixAutoAndAccountIntent(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	provider := newFakePixAutoProvider()
	pixAuto := newPixAutoService(t, store, provider)
	activePixAuto(t, pixAuto, provider, owner)
	proof := &fakeOwnerProof{reject: map[string]bool{}}
	service := newBillingExitService(store, proof, &fakeStripeRenewal{}, pixAuto)
	if err := service.RequestAccountExit(context.Background(), owner); err != nil {
		t.Fatal(err)
	}
	exit, found, err := store.LatestBillingExit(context.Background(), owner)
	if err != nil || !found || exit.Reason != app.BillingExitAccountDeletion || exit.State != "DONE" {
		t.Fatalf("exit=%+v found=%v err=%v", exit, found, err)
	}
	mandate, err := store.MandateForAccount(context.Background(), owner, testOffer)
	if err != nil || mandate.Status != "CANCELLED" {
		t.Fatalf("mandate=%+v err=%v", mandate, err)
	}
}

func TestBillingExitAccountIntentWithoutProvidersStaysVisible(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	seedCardContract(t, store, owner, "sub_1")
	service := newBillingExitService(store, &fakeOwnerProof{reject: map[string]bool{}}, nil, nil)
	if err := service.RequestAccountExit(context.Background(), owner); err != nil {
		t.Fatal(err)
	}
	exit, found, err := store.LatestBillingExit(context.Background(), owner)
	if err != nil || !found || exit.State != "PENDING" {
		t.Fatalf("exit=%+v found=%v err=%v", exit, found, err)
	}
	sources, err := store.BillingExitSources(context.Background(), exit.ID)
	if err != nil || len(sources) != 1 || sources[0].State != "PENDING" || sources[0].LastError == "" {
		t.Fatalf("sources=%+v err=%v", sources, err)
	}
}

func seedBillingGrant(t *testing.T, pool *pgxpool.Pool, store *payadapters.Store, owner, paymentID string) {
	t.Helper()
	order, _, customerID := settlementOrder(t, store, owner, "billing-grant-"+paymentID)
	event := settlementInbox(t, store, "evt_"+paymentID, paymentID)
	service := settlementService(store, &fakeSettlementVerifier{
		candidate: settlementCandidate(order, owner, customerID, paymentID),
	})
	if err := service.Process(context.Background(), event); err != nil {
		t.Fatal(err)
	}
}

func TestBillingExitStatusHistoryExport(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	seedBillingGrant(t, pool, store, owner, "pay_exit_history")
	seedCardContract(t, store, owner, "sub_1")
	proof := &fakeOwnerProof{reject: map[string]bool{}}
	service := newBillingExitService(store, proof, &fakeStripeRenewal{}, nil)
	status, err := service.Status(context.Background(), app.Principal{AccountID: owner})
	if err != nil || len(status.Active) != 1 || status.Active[0].Source != paydomain.ChannelCard {
		t.Fatalf("status=%+v err=%v", status, err)
	}
	history, err := service.History(context.Background(), app.Principal{AccountID: owner}, 10)
	if err != nil || len(history) == 0 || history[0].OrderID == "" {
		t.Fatalf("history=%+v err=%v", history, err)
	}
	if _, err := service.Export(context.Background(), app.Principal{AccountID: owner}, "wrong", 10); !errors.Is(err, paydomain.ErrUnauthorized) {
		t.Fatalf("export wrong proof err=%v", err)
	}
	exported, err := service.Export(context.Background(), app.Principal{AccountID: owner}, "owner-"+owner, 10)
	if err != nil || len(exported) != len(history) {
		t.Fatalf("export=%+v err=%v", exported, err)
	}
}

func TestBillingExitRejectsForeignAndRevokedProof(t *testing.T) {
	pool := freshCheckoutDB(t)
	store := payadapters.NewStore(pool)
	owner := seedCheckoutOwner(t, pool)
	seedCardContract(t, store, owner, "sub_1")
	proof := &fakeOwnerProof{reject: map[string]bool{owner: true}}
	service := newBillingExitService(store, proof, &fakeStripeRenewal{}, nil)
	if _, err := service.Cancel(context.Background(), app.Principal{AccountID: owner},
		"63636363-6363-4363-8363-636363636363", "owner-"+owner); !errors.Is(err, paydomain.ErrUnauthorized) {
		t.Fatalf("revoked proof err=%v", err)
	}
	if _, err := service.Cancel(context.Background(), app.Principal{AccountID: owner},
		"64646464-6464-4464-8464-646464646464", "owner-other"); !errors.Is(err, paydomain.ErrUnauthorized) {
		t.Fatalf("foreign proof err=%v", err)
	}
	if _, found, _ := store.LatestBillingExit(context.Background(), owner); found {
		t.Fatal("unauthorized proof created an exit")
	}
}
