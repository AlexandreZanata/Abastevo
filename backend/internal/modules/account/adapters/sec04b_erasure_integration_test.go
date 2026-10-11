//go:build integration

package adapters

import (
	"context"
	"sync"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

// SEC04-B real-PostGIS deletion linkage (AUD-05, B-BR-A05/B-BR-016):
// bindings capture into durable intents atomically with revocation;
// scope cleanup enqueues with the captured IDs; retries replay from
// intents after the bindings are gone.

func TestSEC04BDeleteCapturesIntentsOnRealSQL(t *testing.T) {
	svc, _, _ := freshService(t)
	ctx := context.Background()
	var mu sync.Mutex
	var enqueued [][]string
	svc.RequestErasure = func(_ context.Context, _ string, contributorIDs []string) error {
		mu.Lock()
		defer mu.Unlock()
		enqueued = append(enqueued, append([]string{}, contributorIDs...))
		return nil
	}
	// Email signup, then bind two contributors through the store port
	// (key-proof ceremony is covered elsewhere).
	if err := svc.RequestCode(ctx, "sec04b@example.invalid"); err != nil {
		t.Fatal(err)
	}
	auth, err := svc.ConsumeCode(ctx, "sec04b@example.invalid", "482916")
	if err != nil {
		t.Fatal(err)
	}
	for i, c := range []string{"contrib-sec04b-1", "contrib-sec04b-2"} {
		if err := svc.Store.BindContributor(ctx, domain.ContributorBinding{
			AccountID: auth.Account.ID, ContributorID: c,
			KeyFingerprint: "fp:" + c, BoundAt: svc.Clock.NowUnix(),
		}, testUUID(9100+i)); err != nil {
			t.Fatalf("bind %s: %v", c, err)
		}
	}
	receipt, err := svc.DeleteAccount(ctx, auth.Account.ID)
	if err != nil {
		t.Fatalf("DeleteAccount: %v", err)
	}
	if len(receipt.ContributorIDs) != 2 || !receipt.ErasureRequested {
		t.Fatalf("receipt = %+v, want both captured contributors", receipt)
	}
	mu.Lock()
	if len(enqueued) != 1 || len(enqueued[0]) != 2 {
		mu.Unlock()
		t.Fatalf("enqueued = %v, want captured IDs", enqueued)
	}
	mu.Unlock()
	// Bindings dropped, sessions revoked, intents durable.
	if _, found, _ := svc.Store.FindBindingOwner(ctx, "contrib-sec04b-1"); found {
		t.Error("bindings must drop after capture")
	}
	intents, err := svc.Store.ListErasureIntents(ctx, auth.Account.ID)
	if err != nil || len(intents) != 2 {
		t.Fatalf("intents = %v (%v), want both contributors", intents, err)
	}
	if _, err := svc.ValidateAccess(ctx, auth.Session.FamilyID, auth.Session.AccessToken); err != domain.ErrAccountDeleted {
		t.Fatalf("post-delete access = %v, want deleted", err)
	}
	// Retry replays from intents (bindings already gone).
	again, err := svc.DeleteAccount(ctx, auth.Account.ID)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if len(again.ContributorIDs) != 2 {
		t.Fatalf("retry contributors = %v, want intent replay", again.ContributorIDs)
	}
}

// SEC04-C credential minimization on real PostGIS: key verifiers
// never survive the owner; the handle is free for a new account.
func TestSEC04CKeyMinimizationOnRealSQL(t *testing.T) {
	svc, _, _ := freshService(t)
	ctx := context.Background()
	created, err := svc.CreateKeyAccount(ctx, "minimizekey")
	if err != nil {
		t.Fatal(err)
	}
	oldKey := created.Key
	if _, err := svc.LoginWithKey(ctx, oldKey); err != nil {
		t.Fatalf("pre-delete login: %v", err)
	}
	if _, err := svc.DeleteAccount(ctx, created.Account.ID); err != nil {
		t.Fatalf("DeleteAccount: %v", err)
	}
	if _, err := svc.LoginWithKey(ctx, oldKey); err != domain.ErrKeyInvalid {
		t.Fatalf("post-delete login = %v, want key-invalid", err)
	}
	if _, found, _ := svc.Store.FindKeyCredentialByUsername(ctx, domain.UsernameHash("minimizekey")); found {
		t.Fatal("username verifier survived deletion")
	}
	if _, found, _ := svc.Store.FindKeyCredentialByLookup(ctx, domain.KeyLookup(oldKey)); found {
		t.Fatal("key lookup survived deletion")
	}
}
