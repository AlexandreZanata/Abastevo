package application

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

// stubProver models the identity key verifier: known contributors prove
// with "proof-<id>", anything else fails closed.
type stubProver struct {
	mu    sync.Mutex
	known map[string]string
	calls int
}

func newStubProver(pairs ...string) *stubProver {
	known := map[string]string{}
	for _, id := range pairs {
		known[id] = "fp:" + id
	}
	return &stubProver{known: known}
}

func (p *stubProver) VerifyKeyProof(_ context.Context, _, contributorID, proof string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	fp, ok := p.known[contributorID]
	if !ok || proof != "proof-"+contributorID {
		return "", errors.New("account: key proof denied")
	}
	return fp, nil
}

func testBindingService(t *testing.T, contributors ...string) (*Service, *fakeClock, *stubProver) {
	t.Helper()
	svc, clock, _ := testService()
	prover := newStubProver(contributors...)
	svc.KeyProver = prover
	return svc, clock, prover
}

func bindSignup(t *testing.T, svc *Service, address, code string) Session {
	t.Helper()
	ctx := context.Background()
	request(t, svc, address)
	auth, err := svc.ConsumeCode(ctx, address, code)
	if err != nil {
		t.Fatalf("ConsumeCode: %v", err)
	}
	return auth.Session
}

func TestBindHappyAndIdempotent(t *testing.T) {
	svc, _, _ := testBindingService(t, "contrib-1")
	ctx := context.Background()
	sess := bindSignup(t, svc, "bind-a@example.invalid", "482916")

	got, err := svc.BindContributor(ctx, sess.FamilyID, sess.AccessToken, "contrib-1", "proof-contrib-1")
	if err != nil {
		t.Fatalf("BindContributor: %v", err)
	}
	if got.KeyFingerprint != "fp:contrib-1" {
		t.Errorf("binding must carry the proven fingerprint, got %+v", got)
	}
	// Same-account relink converges without a second audit row… no:
	// relinks are still audited, but the binding stays single.
	again, err := svc.BindContributor(ctx, sess.FamilyID, sess.AccessToken, "contrib-1", "proof-contrib-1")
	if err != nil {
		t.Fatalf("relink: %v", err)
	}
	if again.ContributorID != got.ContributorID {
		t.Errorf("relink must converge, got %+v", again)
	}
	listed, err := svc.ListBindings(ctx, sess.FamilyID, sess.AccessToken)
	if err != nil || len(listed) != 1 {
		t.Errorf("want 1 binding, got %+v err=%v", listed, err)
	}
	owner, found, err := svc.Store.FindBindingOwner(ctx, "contrib-1")
	if err != nil || !found {
		t.Fatalf("owner must resolve, found=%v err=%v", found, err)
	}
	_ = owner
}

func TestBindStolenContributorRefused(t *testing.T) {
	svc, _, _ := testBindingService(t, "shared-1")
	ctx := context.Background()
	sessA := bindSignup(t, svc, "bind-a@example.invalid", "482916")
	sessB := bindSignup(t, svc, "bind-b@example.invalid", "111111")

	if _, err := svc.BindContributor(ctx, sessA.FamilyID, sessA.AccessToken, "shared-1", "proof-shared-1"); err != nil {
		t.Fatalf("bind A: %v", err)
	}
	// B presents a VALID key proof for A's contributor: ownership, not
	// proof validity, decides — recovery never transfers reputation.
	_, err := svc.BindContributor(ctx, sessB.FamilyID, sessB.AccessToken, "shared-1", "proof-shared-1")
	if !errors.Is(err, domain.ErrBindingCrossAccount) {
		t.Errorf("stolen bind must refuse, got %v", err)
	}
	if got := domain.VerdictCode(err); got != "binding-cross-account-refused" {
		t.Errorf("verdict = %q, want binding-cross-account-refused", got)
	}
	if listed, _ := svc.ListBindings(ctx, sessB.FamilyID, sessB.AccessToken); len(listed) != 0 {
		t.Errorf("refused bind must persist nothing, got %+v", listed)
	}
}

func TestBindRequiresFreshSessionAndProof(t *testing.T) {
	svc, clock, _ := testBindingService(t, "contrib-9")
	ctx := context.Background()
	sess := bindSignup(t, svc, "bind-c@example.invalid", "482916")

	clock.advance(901 * time.Second)
	if _, err := svc.BindContributor(ctx, sess.FamilyID, sess.AccessToken, "contrib-9", "proof-contrib-9"); !errors.Is(err, domain.ErrSessionExpired) {
		t.Errorf("expired session must refuse, got %v", err)
	}
	if _, err := svc.BindContributor(ctx, "missing", "bad", "contrib-9", "proof-contrib-9"); !errors.Is(err, domain.ErrCodeUnknown) {
		t.Errorf("forged session must refuse, got %v", err)
	}

	// Fresh session for proof-shape cases.
	clock.advance(61 * time.Second)
	request(t, svc, "bind-c@example.invalid")
	fresh, err := svc.ConsumeCode(ctx, "bind-c@example.invalid", "111111")
	if err != nil {
		t.Fatalf("fresh login: %v", err)
	}
	if _, err := svc.BindContributor(ctx, fresh.Session.FamilyID, fresh.Session.AccessToken, "unknown-1", "proof-unknown-1"); err == nil {
		t.Error("unknown contributor proof must fail closed")
	}
	if _, err := svc.BindContributor(ctx, fresh.Session.FamilyID, fresh.Session.AccessToken, "", "proof"); !errors.Is(err, domain.ErrBindingInvalid) {
		t.Errorf("empty contributor must be invalid, got %v", err)
	}
	if _, err := svc.BindContributor(ctx, fresh.Session.FamilyID, fresh.Session.AccessToken, "contrib-9", ""); !errors.Is(err, domain.ErrBindingInvalid) {
		t.Errorf("empty proof must be invalid, got %v", err)
	}
	svc.KeyProver = nil
	if _, err := svc.BindContributor(ctx, fresh.Session.FamilyID, fresh.Session.AccessToken, "contrib-9", "proof-contrib-9"); !errors.Is(err, domain.ErrKeyUnavailable) {
		t.Errorf("missing prover must fail closed, got %v", err)
	}
}

func TestBindRefusedOnDeadAccounts(t *testing.T) {
	svc, _, _ := testBindingService(t, "contrib-8")
	ctx := context.Background()
	sess := bindSignup(t, svc, "bind-d@example.invalid", "482916")
	accID := mustBindAccountID(t, svc, "bind-d@example.invalid")

	if err := svc.SuspendAccount(ctx, accID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.BindContributor(ctx, sess.FamilyID, sess.AccessToken, "contrib-8", "proof-contrib-8"); !errors.Is(err, domain.ErrAccountSuspended) {
		t.Errorf("bind on suspended must refuse, got %v", err)
	}
	if err := svc.DeleteAccount(ctx, accID); err != nil {
		t.Fatal(err)
	}
	// Status leads revocation: the verdict names the deletion.
	if _, err := svc.BindContributor(ctx, sess.FamilyID, sess.AccessToken, "contrib-8", "proof-contrib-8"); !errors.Is(err, domain.ErrAccountDeleted) {
		t.Errorf("bind on deleted must refuse deleted, got %v", err)
	}
}

func mustBindAccountID(t *testing.T, svc *Service, address string) string {
	t.Helper()
	acc, found, err := svc.Store.FindAccount(context.Background(), domain.AddressHash(address))
	if err != nil || !found {
		t.Fatalf("FindAccount: %+v found=%v err=%v", acc, found, err)
	}
	return acc.ID
}

func TestUnbindAndAuditTrail(t *testing.T) {
	svc, _, _ := testBindingService(t, "contrib-7")
	ctx := context.Background()
	sess := bindSignup(t, svc, "bind-e@example.invalid", "482916")
	accID := mustBindAccountID(t, svc, "bind-e@example.invalid")

	if _, err := svc.BindContributor(ctx, sess.FamilyID, sess.AccessToken, "contrib-7", "proof-contrib-7"); err != nil {
		t.Fatal(err)
	}
	if err := svc.UnbindContributor(ctx, sess.FamilyID, sess.AccessToken, "contrib-7"); err != nil {
		t.Fatalf("UnbindContributor: %v", err)
	}
	if err := svc.UnbindContributor(ctx, sess.FamilyID, sess.AccessToken, "contrib-7"); !errors.Is(err, domain.ErrBindingNotFound) {
		t.Errorf("second unbind must be not-found, got %v", err)
	}
	if err := svc.UnbindContributor(ctx, sess.FamilyID, sess.AccessToken, ""); !errors.Is(err, domain.ErrBindingInvalid) {
		t.Errorf("empty unbind must be invalid, got %v", err)
	}
	audit, err := svc.Store.ListBindingAudit(ctx, accID)
	if err != nil {
		t.Fatal(err)
	}
	if len(audit) != 2 || audit[0].Action != domain.BindingActionBind || audit[1].Action != domain.BindingActionUnbind {
		t.Errorf("audit must record bind then unbind, got %+v", audit)
	}
	for _, entry := range audit {
		if entry.AccountID != accID || entry.ContributorID != "contrib-7" {
			t.Errorf("audit entry misattributed: %+v", entry)
		}
	}
}

func TestBindingBlockedMatrix(t *testing.T) {
	svc, _, _ := testBindingService(t, "contrib-g")
	ctx := context.Background()

	if blocked, err := BindingBlocked(ctx, svc.Store, ""); blocked || err != nil {
		t.Errorf("empty contributor must not block, got %v %v", blocked, err)
	}
	if blocked, err := BindingBlocked(ctx, svc.Store, "ghost-1"); blocked || err != nil {
		t.Errorf("unbound contributor keeps the baseline, got %v %v", blocked, err)
	}
	sess := bindSignup(t, svc, "bind-g@example.invalid", "482916")
	accID := mustBindAccountID(t, svc, "bind-g@example.invalid")
	if _, err := svc.BindContributor(ctx, sess.FamilyID, sess.AccessToken, "contrib-g", "proof-contrib-g"); err != nil {
		t.Fatal(err)
	}
	if blocked, err := BindingBlocked(ctx, svc.Store, "contrib-g"); blocked || err != nil {
		t.Errorf("bound-active must not block, got %v %v", blocked, err)
	}
	if err := svc.SuspendAccount(ctx, accID); err != nil {
		t.Fatal(err)
	}
	if blocked, err := BindingBlocked(ctx, svc.Store, "contrib-g"); !blocked || err != nil {
		t.Errorf("bound-suspended must block, got %v %v", blocked, err)
	}
	if err := svc.DeleteAccount(ctx, accID); err != nil {
		t.Fatal(err)
	}
	if blocked, err := BindingBlocked(ctx, svc.Store, "contrib-g"); blocked || err != nil {
		t.Errorf("deleted bindings read as unbound baseline, got %v %v", blocked, err)
	}
}

func TestDeleteDropsBindings(t *testing.T) {
	svc, _, _ := testBindingService(t, "contrib-6")
	ctx := context.Background()
	sess := bindSignup(t, svc, "bind-f@example.invalid", "482916")
	accID := mustBindAccountID(t, svc, "bind-f@example.invalid")

	if _, err := svc.BindContributor(ctx, sess.FamilyID, sess.AccessToken, "contrib-6", "proof-contrib-6"); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteAccount(ctx, accID); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := svc.Store.FindBindingOwner(ctx, "contrib-6"); found {
		t.Error("deleted bindings must be gone")
	}
	if listed, _ := svc.Store.ListBindings(ctx, accID); len(listed) != 0 {
		t.Errorf("deleted bindings must list empty, got %+v", listed)
	}
}
