package application

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

// SEC04-B deletion linkage (AUD-05, B-BR-A05/B-BR-016): revocation
// and durable erasure intents commit atomically; scope cleanup
// enqueues through the explicit port with the captured IDs; a nil
// port refuses closed before any mutation; crashes replay from
// durable intents.

type erasureRecorder struct {
	mu           sync.Mutex
	accountIDs   []string
	contributors [][]string
	err          error
}

func (r *erasureRecorder) port() func(context.Context, string, []string) error {
	return func(_ context.Context, accountID string, contributorIDs []string) error {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.accountIDs = append(r.accountIDs, accountID)
		r.contributors = append(r.contributors, append([]string{}, contributorIDs...))
		return r.err
	}
}

func deletionService() (*Service, *fakeClock, *erasureRecorder) {
	svc, clock, _ := testService()
	rec := &erasureRecorder{}
	svc.RequestErasure = rec.port()
	return svc, clock, rec
}

func signupBound(t *testing.T, svc *Service, address, code, contributor string) AuthResult {
	t.Helper()
	ctx := context.Background()
	request(t, svc, address)
	auth, err := svc.ConsumeCode(ctx, address, code)
	if err != nil {
		t.Fatalf("signup: %v", err)
	}
	// Bind through the store port directly (key-proof ceremony is
	// covered elsewhere); the linkage under test starts at deletion.
	if err := svc.Store.BindContributor(ctx, domain.ContributorBinding{
		AccountID: auth.Account.ID, ContributorID: contributor,
		KeyFingerprint: "fp:" + contributor, BoundAt: svc.Clock.NowUnix(),
	}, "audit-"+contributor); err != nil {
		t.Fatalf("bind: %v", err)
	}
	return auth
}

func TestDeleteCapturesIntentsAndEnqueues(t *testing.T) {
	svc, _, rec := deletionService()
	ctx := context.Background()
	auth := signupBound(t, svc, "erase-a@example.invalid", "482916", "contrib-erase-a")

	receipt, err := svc.DeleteAccount(ctx, auth.Account.ID)
	if err != nil {
		t.Fatalf("DeleteAccount: %v", err)
	}
	if !receipt.ErasureRequested || receipt.AccountID != auth.Account.ID {
		t.Errorf("receipt = %+v, want requested intent", receipt)
	}
	if len(receipt.ContributorIDs) != 1 || receipt.ContributorIDs[0] != "contrib-erase-a" {
		t.Errorf("receipt contributors = %v, want captured binding", receipt.ContributorIDs)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.accountIDs) != 1 || rec.accountIDs[0] != auth.Account.ID {
		t.Errorf("enqueued accounts = %v", rec.accountIDs)
	}
	if len(rec.contributors) != 1 || len(rec.contributors[0]) != 1 || rec.contributors[0][0] != "contrib-erase-a" {
		t.Errorf("enqueued contributors = %v", rec.contributors)
	}
	// Sessions revoked, bindings gone, intents durable.
	if _, err := svc.ValidateAccess(ctx, auth.Session.FamilyID, auth.Session.AccessToken); !errors.Is(err, domain.ErrAccountDeleted) {
		t.Errorf("post-delete access = %v, want deleted", err)
	}
	if _, found, _ := svc.Store.FindBindingOwner(ctx, "contrib-erase-a"); found {
		t.Error("bindings must drop after capture")
	}
	intents, err := svc.Store.ListErasureIntents(ctx, auth.Account.ID)
	if err != nil || len(intents) != 1 || intents[0] != "contrib-erase-a" {
		t.Errorf("intents = %v (%v), want captured contributor", intents, err)
	}
}

func TestDeleteWithoutWiringRefusesBeforeMutation(t *testing.T) {
	svc, _, _ := testService()
	ctx := context.Background()
	auth := signupBound(t, svc, "erase-b@example.invalid", "482916", "contrib-erase-b")
	if _, err := svc.DeleteAccount(ctx, auth.Account.ID); !errors.Is(err, domain.ErrErasureWiringMissing) {
		t.Fatalf("unwired deletion = %v, want wiring-missing", err)
	}
	// Nothing mutated: account live, sessions live, bindings kept, no
	// intents.
	if _, err := svc.ValidateAccess(ctx, auth.Session.FamilyID, auth.Session.AccessToken); err != nil {
		t.Errorf("account mutated by refused deletion: %v", err)
	}
	if _, found, _ := svc.Store.FindBindingOwner(ctx, "contrib-erase-b"); !found {
		t.Error("bindings dropped by refused deletion")
	}
	if intents, _ := svc.Store.ListErasureIntents(ctx, auth.Account.ID); len(intents) != 0 {
		t.Errorf("intents = %v after refused deletion", intents)
	}
}

func TestDeleteCrashReplaysFromIntents(t *testing.T) {
	svc, _, rec := deletionService()
	ctx := context.Background()
	auth := signupBound(t, svc, "erase-c@example.invalid", "482916", "contrib-erase-c")
	// Crash between revocation and enqueue: enqueue fails, bindings
	// already dropped, but intents stay durable.
	rec.err = errors.New("queue down")
	if _, err := svc.DeleteAccount(ctx, auth.Account.ID); err == nil {
		t.Fatal("failing enqueue must error")
	}
	if _, found, _ := svc.Store.FindBindingOwner(ctx, "contrib-erase-c"); found {
		t.Fatal("bindings must drop in the atomic transaction")
	}
	intents, err := svc.Store.ListErasureIntents(ctx, auth.Account.ID)
	if err != nil || len(intents) != 1 {
		t.Fatalf("intents = %v (%v), want durable capture", intents, err)
	}
	// Retry replays from intents (bindings are gone) and converges.
	rec.err = nil
	receipt, err := svc.DeleteAccount(ctx, auth.Account.ID)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if len(receipt.ContributorIDs) != 1 || receipt.ContributorIDs[0] != "contrib-erase-c" {
		t.Errorf("retry contributors = %v, want intent replay", receipt.ContributorIDs)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	last := rec.contributors[len(rec.contributors)-1]
	if len(last) != 1 || last[0] != "contrib-erase-c" {
		t.Errorf("replayed enqueue = %v", rec.contributors)
	}
}

func TestDeleteBindingFreeAccountEnqueuesEmpty(t *testing.T) {
	svc, _, rec := deletionService()
	ctx := context.Background()
	request(t, svc, "erase-d@example.invalid")
	auth, err := svc.ConsumeCode(ctx, "erase-d@example.invalid", "482916")
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := svc.DeleteAccount(ctx, auth.Account.ID)
	if err != nil {
		t.Fatalf("DeleteAccount: %v", err)
	}
	if !receipt.ErasureRequested || len(receipt.ContributorIDs) != 0 {
		t.Errorf("receipt = %+v, want empty requested intent", receipt)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.contributors) != 1 || len(rec.contributors[0]) != 0 {
		t.Errorf("enqueued = %v, want one empty call", rec.contributors)
	}
}

// SEC04-C credential minimization: key verifiers never survive the
// owner. The old key authenticates nothing after deletion, and the
// username is free for a new account without reputation carryover.
func TestDeleteMinimizesKeyCredentials(t *testing.T) {
	svc := testKeyService()
	svc.RequestErasure = func(_ context.Context, _ string, _ []string) error { return nil }
	ctx := context.Background()
	created, err := svc.CreateKeyAccount(ctx, "erasablekey")
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
	if _, found, _ := svc.Store.FindKeyCredentialByUsername(ctx, domain.UsernameHash("erasablekey")); found {
		t.Error("username verifier survived deletion")
	}
	// The handle is free again for a new account.
	if _, err := svc.CreateKeyAccount(ctx, "erasablekey"); err != nil {
		t.Fatalf("username reuse: %v", err)
	}
}
