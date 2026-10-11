package application

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

// SEC01-A admission regressions (AUD-01): per-subject fairness
// exists (per-address quota, cooldown), but no global item/byte/
// concurrency admission bounds distinct subjects, and invalid
// requests cost store IO before refusing. Each test asserts the
// future bound and skips until SEC01-C implements it — committed
// skipped (not blessed, not breaking CI), mirroring the audit
// characterization without permanently asserting the flaw.

func TestDistinctAddressesExceedAggregateBudget(t *testing.T) {
	t.Skip("SEC01-C: aggregate email admission pending")
	svc, _, mail := testService()
	ctx := context.Background()
	for i := 0; i < 100; i++ {
		if err := svc.RequestCode(ctx, fmt.Sprintf("flood-%d@example.invalid", i)); err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
	}
	if got := mail.count(); got > 20 {
		t.Fatalf("aggregate mail issued = %d, want a global bound", got)
	}
}

func TestDistinctKeyUsernamesUnbounded(t *testing.T) {
	t.Skip("SEC01-C: aggregate username admission pending")
	svc := testKeyService()
	ctx := context.Background()
	for i := 0; i < 100; i++ {
		if _, err := svc.CreateKeyAccount(ctx, fmt.Sprintf("flooduser%d", i)); err != nil {
			t.Fatalf("account %d: %v", i, err)
		}
	}
	if _, err := svc.CreateKeyAccount(ctx, "flooduser100"); err == nil {
		t.Fatal("101st distinct username accepted without aggregate bound")
	}
}

func TestSessionsMultiplyWithoutBound(t *testing.T) {
	t.Skip("SEC01-C: per-account session cap pending")
	svc := testKeyService()
	ctx := context.Background()
	created, err := svc.CreateKeyAccount(ctx, "floodsessions")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		if _, err := svc.LoginWithKey(ctx, created.Key); err != nil {
			t.Fatalf("login %d: %v", i, err)
		}
	}
	t.Fatal("50 sessions for one account accepted without a cap")
}

// countingStore decorates the account Store to prove invalid
// requests cost backend IO before refusing.
type countingStore struct {
	Store
	calls *atomic.Int64
}

func (s countingStore) FindAccount(ctx context.Context, addressHash string) (domain.Account, bool, error) {
	s.calls.Add(1)
	return s.Store.FindAccount(ctx, addressHash)
}

func TestInvalidAddressCostsStoreAccess(t *testing.T) {
	t.Skip("SEC01-C: validate address format before store IO")
	svc, _, _ := testService()
	var calls atomic.Int64
	svc.Store = countingStore{Store: svc.Store, calls: &calls}
	if err := svc.RequestCode(context.Background(), "not-an-address"); err != nil {
		t.Fatalf("invalid address errored instead of cheap refuse: %v", err)
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("invalid address cost %d store lookups, want zero", got)
	}
}
