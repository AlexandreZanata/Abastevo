package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters/oidc"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

func testStatusService(t *testing.T) (*Service, *fakeClock) {
	t.Helper()
	svc, clock, _ := testService()
	google, err := oidc.NewStubIssuer("https://accounts.google.com", "RS256")
	if err != nil {
		t.Fatal(err)
	}
	svc.Verifier = &oidc.Verifier{
		Clock:    func() time.Time { return time.Unix(1_700_000_000, 0) },
		Issuers:  map[string]string{"google": "https://accounts.google.com", "apple": "https://appleid.apple.com"},
		Audience: "anpfuel-backend",
		Keys: oidc.NewMemKeys(map[string]oidc.StubIssuer{
			"https://accounts.google.com": google,
		}),
		Nonces:   oidc.NewMemNonces(),
		Skew:     120 * time.Second,
		CacheTTL: time.Hour,
	}
	return svc, clock
}

func statusSignup(t *testing.T, svc *Service, address, code string) AuthResult {
	t.Helper()
	ctx := context.Background()
	if err := svc.RequestCode(ctx, address); err != nil {
		t.Fatalf("RequestCode: %v", err)
	}
	auth, err := svc.ConsumeCode(ctx, address, code)
	if err != nil {
		t.Fatalf("ConsumeCode: %v", err)
	}
	return auth
}

func TestSuspendBlocksConsumeWithoutBurningCode(t *testing.T) {
	svc, clock := testStatusService(t)
	ctx := context.Background()
	auth := statusSignup(t, svc, "susp-01@example.invalid", "482916")

	clock.advance(61 * time.Second)
	request(t, svc, "susp-01@example.invalid")
	if err := svc.SuspendAccount(ctx, auth.Account.ID); err != nil {
		t.Fatalf("SuspendAccount: %v", err)
	}
	if _, err := svc.ConsumeCode(ctx, "susp-01@example.invalid", "111111"); !errors.Is(err, domain.ErrAccountSuspended) {
		t.Errorf("suspended consume must refuse, got %v", err)
	}
	// The code was not burned: after reactivation the same code logs in.
	if err := svc.ReactivateAccount(ctx, auth.Account.ID); err != nil {
		t.Fatalf("ReactivateAccount: %v", err)
	}
	if _, err := svc.ConsumeCode(ctx, "susp-01@example.invalid", "111111"); err != nil {
		t.Errorf("reactivated consume must succeed, got %v", err)
	}
}

func TestSuspendRevokesLiveSessions(t *testing.T) {
	svc, _ := testStatusService(t)
	ctx := context.Background()
	auth := statusSignup(t, svc, "susp-02@example.invalid", "482916")

	if _, err := svc.Refresh(ctx, auth.Session.FamilyID, auth.Session.RefreshToken); err != nil {
		t.Fatalf("pre-suspend refresh: %v", err)
	}
	if err := svc.SuspendAccount(ctx, auth.Account.ID); err != nil {
		t.Fatal(err)
	}
	// Status leads revocation: the verdict names the suspension.
	if _, err := svc.Refresh(ctx, auth.Session.FamilyID, auth.Session.RefreshToken); !errors.Is(err, domain.ErrAccountSuspended) {
		t.Errorf("post-suspend refresh must be suspended, got %v", err)
	}
	if _, err := svc.ValidateAccess(ctx, auth.Session.FamilyID, auth.Session.AccessToken); !errors.Is(err, domain.ErrAccountSuspended) {
		t.Errorf("post-suspend access must be suspended, got %v", err)
	}
	// Reactivation does not resurrect revoked families.
	if err := svc.ReactivateAccount(ctx, auth.Account.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Refresh(ctx, auth.Session.FamilyID, auth.Session.RefreshToken); !errors.Is(err, domain.ErrSessionRevoked) {
		t.Errorf("revoked family must stay revoked after reactivation, got %v", err)
	}
}

func TestReactivateDeletedRefuses(t *testing.T) {
	svc, _ := testStatusService(t)
	ctx := context.Background()
	auth := statusSignup(t, svc, "del-01@example.invalid", "482916")

	if err := svc.DeleteAccount(ctx, auth.Account.ID); err != nil {
		t.Fatalf("DeleteAccount: %v", err)
	}
	if err := svc.ReactivateAccount(ctx, auth.Account.ID); !errors.Is(err, domain.ErrAccountDeleted) {
		t.Errorf("reactivating a deleted account must refuse, got %v", err)
	}
}

func TestDeleteDropsBindingsAndSessions(t *testing.T) {
	svc, _ := testStatusService(t)
	ctx := context.Background()
	auth := statusSignup(t, svc, "del-02@example.invalid", "482916")

	if err := svc.SuspendAccount(ctx, "missing-id"); !errors.Is(err, domain.ErrAccountNotFound) {
		t.Errorf("suspend unknown must be not-found, got %v", err)
	}
	if err := svc.DeleteAccount(ctx, "missing-id"); !errors.Is(err, domain.ErrAccountNotFound) {
		t.Errorf("delete unknown must be not-found, got %v", err)
	}
	if err := svc.DeleteAccount(ctx, ""); !errors.Is(err, domain.ErrAccountNotFound) {
		t.Errorf("delete empty must be not-found, got %v", err)
	}

	if err := svc.DeleteAccount(ctx, auth.Account.ID); err != nil {
		t.Fatalf("DeleteAccount: %v", err)
	}
	if _, err := svc.ValidateAccess(ctx, auth.Session.FamilyID, auth.Session.AccessToken); !errors.Is(err, domain.ErrAccountDeleted) {
		t.Errorf("post-delete access must be deleted, got %v", err)
	}
	if _, err := svc.Refresh(ctx, auth.Session.FamilyID, auth.Session.RefreshToken); !errors.Is(err, domain.ErrAccountDeleted) {
		t.Errorf("post-delete refresh must be deleted, got %v", err)
	}
	if _, found, _ := svc.Store.FindAccount(ctx, domain.AddressHash("del-02@example.invalid")); found {
		t.Error("deleted address binding must be gone")
	}
	if got, found, err := svc.Store.GetAccount(ctx, auth.Account.ID); err != nil || !found || got.Status != domain.StatusDeleted {
		t.Errorf("audit row must stay deleted, got %+v found=%v err=%v", got, found, err)
	}
	// Re-signup with the same address mints a NEW account: no reputation
	// carryover from the deleted row.
	request(t, svc, "del-02@example.invalid")
	again, err := svc.ConsumeCode(ctx, "del-02@example.invalid", "111111")
	if err != nil {
		t.Fatalf("re-signup: %v", err)
	}
	if !again.Created || again.Account.ID == auth.Account.ID {
		t.Errorf("re-signup must create a new account, got %+v", again.Account)
	}
}

func TestLinkRefusedOnDeadAccounts(t *testing.T) {
	svc, _ := testStatusService(t)
	ctx := context.Background()

	auth := statusSignup(t, svc, "link-dead@example.invalid", "482916")
	if err := svc.SuspendAccount(ctx, auth.Account.ID); err != nil {
		t.Fatal(err)
	}
	// The pre-check fires before verification, so the nonce is not
	// burned: it stays consumable afterwards.
	if _, err := svc.LinkProvider(ctx, auth.Account.ID, "google", "dummy.token.parts", "anpfuel-backend", "n-dead-1"); !errors.Is(err, domain.ErrAccountSuspended) {
		t.Errorf("link on suspended must refuse, got %v", err)
	}
	if ok, _ := svc.Store.TryConsumeNonce(ctx, "n-dead-1", 1_700_000_000); !ok {
		t.Error("refused link must not burn the nonce")
	}
	if err := svc.UnlinkProvider(ctx, auth.Account.ID, "google"); !errors.Is(err, domain.ErrAccountSuspended) {
		t.Errorf("unlink on suspended must refuse suspended, got %v", err)
	}

	if err := svc.DeleteAccount(ctx, auth.Account.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.LinkProvider(ctx, auth.Account.ID, "google", "dummy.token.parts", "anpfuel-backend", "n-dead-2"); !errors.Is(err, domain.ErrAccountDeleted) {
		t.Errorf("link on deleted must refuse, got %v", err)
	}
}
