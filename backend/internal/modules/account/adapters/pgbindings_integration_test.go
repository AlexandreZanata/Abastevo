//go:build integration

package adapters

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

// stubKeyProver models the identity key verifier for binding tests:
// known contributors prove with "proof-<id>".
type stubKeyProver struct {
	mu    sync.Mutex
	known map[string]bool
}

func newStubKeyProver(ids ...string) *stubKeyProver {
	known := map[string]bool{}
	for _, id := range ids {
		known[id] = true
	}
	return &stubKeyProver{known: known}
}

func (p *stubKeyProver) VerifyKeyProof(_ context.Context, contributorID, proof string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.known[contributorID] || proof != "proof-"+contributorID {
		return "", errors.New("account: key proof denied")
	}
	return "fp:" + contributorID, nil
}

func bindSession(t *testing.T, f *linkService, address, code string) application.Session {
	t.Helper()
	ctx := context.Background()
	if err := f.svc.RequestCode(ctx, address); err != nil {
		t.Fatalf("RequestCode: %v", err)
	}
	auth, err := f.svc.ConsumeCode(ctx, address, code)
	if err != nil {
		t.Fatalf("ConsumeCode: %v", err)
	}
	return auth.Session
}

func TestPGBindHappyAndAudit(t *testing.T) {
	f := freshLinkService(t)
	f.svc.KeyProver = newStubKeyProver("contrib-1")
	ctx := context.Background()
	sess := bindSession(t, f, "bind-a@example.invalid", "482916")

	got, err := f.svc.BindContributor(ctx, sess.FamilyID, sess.AccessToken, "contrib-1", "proof-contrib-1")
	if err != nil {
		t.Fatalf("BindContributor: %v", err)
	}
	if got.KeyFingerprint != "fp:contrib-1" {
		t.Errorf("binding must carry the proven fingerprint, got %+v", got)
	}
	if _, err := f.svc.BindContributor(ctx, sess.FamilyID, sess.AccessToken, "contrib-1", "proof-contrib-1"); err != nil {
		t.Fatalf("relink: %v", err)
	}
	listed, err := f.svc.ListBindings(ctx, sess.FamilyID, sess.AccessToken)
	if err != nil || len(listed) != 1 {
		t.Errorf("want 1 binding, got %+v err=%v", listed, err)
	}
	accID := listed[0].AccountID
	audit, err := f.svc.Store.ListBindingAudit(ctx, accID)
	if err != nil || len(audit) != 2 {
		t.Fatalf("audit must record both binds, got %+v err=%v", audit, err)
	}
	for _, entry := range audit {
		if entry.Action != domain.BindingActionBind || entry.ContributorID != "contrib-1" {
			t.Errorf("audit entry wrong: %+v", entry)
		}
	}
	if err := f.svc.UnbindContributor(ctx, sess.FamilyID, sess.AccessToken, "contrib-1"); err != nil {
		t.Fatalf("UnbindContributor: %v", err)
	}
	audit, err = f.svc.Store.ListBindingAudit(ctx, accID)
	if err != nil || len(audit) != 3 || audit[2].Action != domain.BindingActionUnbind {
		t.Errorf("audit must record the unbind, got %+v err=%v", audit, err)
	}
	if err := f.svc.UnbindContributor(ctx, sess.FamilyID, sess.AccessToken, "contrib-1"); !errors.Is(err, domain.ErrBindingNotFound) {
		t.Errorf("second unbind must be not-found, got %v", err)
	}
}

func TestPGBindStolenRefused(t *testing.T) {
	f := freshLinkService(t)
	f.svc.KeyProver = newStubKeyProver("shared-1")
	ctx := context.Background()
	sessA := bindSession(t, f, "bind-a@example.invalid", "482916")
	sessB := bindSession(t, f, "bind-b@example.invalid", "111111")

	if _, err := f.svc.BindContributor(ctx, sessA.FamilyID, sessA.AccessToken, "shared-1", "proof-shared-1"); err != nil {
		t.Fatalf("bind A: %v", err)
	}
	if _, err := f.svc.BindContributor(ctx, sessB.FamilyID, sessB.AccessToken, "shared-1", "proof-shared-1"); !errors.Is(err, domain.ErrBindingCrossAccount) {
		t.Errorf("stolen bind must refuse, got %v", err)
	}
}

func TestPGBindRaceAdmitsExactlyOneOwner(t *testing.T) {
	f := freshLinkService(t)
	f.svc.KeyProver = newStubKeyProver("race-1")
	ctx := context.Background()
	sessA := bindSession(t, f, "race-a@example.invalid", "482916")
	sessB := bindSession(t, f, "race-b@example.invalid", "111111")

	const racers = 16
	var wg sync.WaitGroup
	errs := make([]error, racers)
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sess := sessA
			if i%2 == 1 {
				sess = sessB
			}
			_, errs[i] = f.svc.BindContributor(ctx, sess.FamilyID, sess.AccessToken, "race-1", "proof-race-1")
		}(i)
	}
	wg.Wait()
	ok, refused := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, domain.ErrBindingCrossAccount):
			refused++
		default:
			t.Errorf("unexpected bind error: %v", err)
		}
	}
	// Same-account relinks converge (nil) while the losing account
	// refuses: every racer lands on exactly one of the two outcomes,
	// and the contributor keeps a single owner.
	if ok == 0 || refused == 0 {
		t.Errorf("race must split winners and refusals, got %d ok %d refused", ok, refused)
	}
	owner, found, err := f.svc.Store.FindBindingOwner(ctx, "race-1")
	if err != nil || !found || owner == "" {
		t.Fatalf("contributor must keep one owner, found=%v err=%v", found, err)
	}
}

func TestPGDeleteDropsBindings(t *testing.T) {
	f := freshLinkService(t)
	f.svc.KeyProver = newStubKeyProver("contrib-6")
	ctx := context.Background()
	sess := bindSession(t, f, "bind-f@example.invalid", "482916")
	accID, err := f.svc.ValidateAccess(ctx, sess.FamilyID, sess.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.BindContributor(ctx, sess.FamilyID, sess.AccessToken, "contrib-6", "proof-contrib-6"); err != nil {
		t.Fatal(err)
	}
	// Advance past access expiry so the delete path (not the session) is
	// what we exercise below; deletion itself needs no session.
	f.clock.advance(901 * time.Second)
	if err := f.svc.DeleteAccount(ctx, accID); err != nil {
		t.Fatalf("DeleteAccount: %v", err)
	}
	if _, found, _ := f.svc.Store.FindBindingOwner(ctx, "contrib-6"); found {
		t.Error("deleted bindings must be gone")
	}
}
