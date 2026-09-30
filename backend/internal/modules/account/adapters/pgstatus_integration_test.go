//go:build integration

package adapters

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

func TestPGSuspendBlocksAuthPaths(t *testing.T) {
	f := freshLinkService(t)
	ctx := context.Background()
	accountID := f.signup(t, "susp-a@example.invalid", "482916")

	// Fresh login for session assertions (second code in the cycle).
	f.clock.advance(61 * time.Second)
	if err := f.svc.RequestCode(ctx, "susp-a@example.invalid"); err != nil {
		t.Fatal(err)
	}
	login, err := f.svc.ConsumeCode(ctx, "susp-a@example.invalid", "111111")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if err := f.svc.SuspendAccount(ctx, accountID); err != nil {
		t.Fatalf("SuspendAccount: %v", err)
	}
	if _, err := f.svc.Refresh(ctx, login.Session.FamilyID, login.Session.RefreshToken); !errors.Is(err, domain.ErrAccountSuspended) {
		t.Errorf("post-suspend refresh must be suspended, got %v", err)
	}
	if _, err := f.svc.ValidateAccess(ctx, login.Session.FamilyID, login.Session.AccessToken); !errors.Is(err, domain.ErrAccountSuspended) {
		t.Errorf("post-suspend access must be suspended, got %v", err)
	}
	if _, err := f.svc.LinkProvider(ctx, accountID, "google", "dummy", linkAudience, "n-susp-1"); !errors.Is(err, domain.ErrAccountSuspended) {
		t.Errorf("post-suspend link must be suspended, got %v", err)
	}
	if _, err := f.svc.ConsumeCode(ctx, "susp-a@example.invalid", "222222"); !errors.Is(err, domain.ErrAccountSuspended) {
		t.Errorf("post-suspend consume must be suspended, got %v", err)
	}

	if err := f.svc.ReactivateAccount(ctx, accountID); err != nil {
		t.Fatalf("ReactivateAccount: %v", err)
	}
	if _, err := f.svc.Refresh(ctx, login.Session.FamilyID, login.Session.RefreshToken); !errors.Is(err, domain.ErrSessionRevoked) {
		t.Errorf("revoked family must stay revoked after reactivation, got %v", err)
	}
}

func TestPGDeleteDropsBindingsResetsSignup(t *testing.T) {
	f := freshLinkService(t)
	ctx := context.Background()
	accountID := f.signup(t, "gone-a@example.invalid", "482916")

	if _, err := f.svc.LinkProvider(ctx, accountID, "google", f.googleToken(t, "gone-sub-1", "", "n-pg-del-1"), linkAudience, "n-pg-del-1"); err != nil {
		t.Fatalf("link: %v", err)
	}
	if err := f.svc.DeleteAccount(ctx, accountID); err != nil {
		t.Fatalf("DeleteAccount: %v", err)
	}
	if _, found, _ := f.svc.Store.FindAccount(ctx, domain.AddressHash("gone-a@example.invalid")); found {
		t.Error("deleted address binding must be gone")
	}
	if _, found, _ := f.svc.Store.FindProviderOwner(ctx, "google", "gone-sub-1"); found {
		t.Error("deleted provider binding must be gone")
	}
	if got, found, _ := f.svc.Store.GetAccount(ctx, accountID); !found || got.Status != domain.StatusDeleted {
		t.Errorf("audit row must stay deleted, got %+v found=%v", got, found)
	}
	if err := f.svc.DeleteAccount(ctx, "aaaaaaaa-1111-4111-8111-999999999999"); !errors.Is(err, domain.ErrAccountNotFound) {
		t.Errorf("delete unknown must be not-found, got %v", err)
	}
	if err := f.svc.ReactivateAccount(ctx, accountID); !errors.Is(err, domain.ErrAccountDeleted) {
		t.Errorf("reactivating deleted must refuse, got %v", err)
	}

	f.clock.advance(61 * time.Second)
	if err := f.svc.RequestCode(ctx, "gone-a@example.invalid"); err != nil {
		t.Fatal(err)
	}
	again, err := f.svc.ConsumeCode(ctx, "gone-a@example.invalid", "111111")
	if err != nil {
		t.Fatalf("re-signup: %v", err)
	}
	if !again.Created || again.Account.ID == accountID {
		t.Errorf("re-signup must mint a new account, got %+v", again.Account)
	}
}

func TestPGConcurrentRefreshAfterDeleteFailsClosed(t *testing.T) {
	f := freshLinkService(t)
	ctx := context.Background()
	accountID := f.signup(t, "race-del@example.invalid", "482916")
	f.clock.advance(61 * time.Second)
	if err := f.svc.RequestCode(ctx, "race-del@example.invalid"); err != nil {
		t.Fatal(err)
	}
	login, err := f.svc.ConsumeCode(ctx, "race-del@example.invalid", "111111")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.DeleteAccount(ctx, accountID); err != nil {
		t.Fatal(err)
	}

	const racers = 16
	var wg sync.WaitGroup
	errs := make([]error, racers)
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = f.svc.Refresh(ctx, login.Session.FamilyID, login.Session.RefreshToken)
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		if !errors.Is(err, domain.ErrAccountDeleted) {
			t.Errorf("post-delete refresh must be deleted, got %v", err)
		}
	}
}
