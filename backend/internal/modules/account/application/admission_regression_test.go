package application

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

// SEC01-C admission bounds (AUD-01, B-BR-A01/A02/A04/A05): aggregate
// mail/username ceilings and per-account session caps admit atomically
// before any row exists; malformed shapes refuse before any store IO.
// Formerly skipped SEC01-A regressions, now enforced (GREEN).

func TestDistinctAddressesExceedAggregateBudget(t *testing.T) {
	svc, _, mail := testService()
	ctx := context.Background()
	for i := 0; i < 100; i++ {
		if err := svc.RequestCode(ctx, fmt.Sprintf("flood-%d@example.invalid", i)); err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
	}
	if got := mail.count(); got > domain.MaxAggregateCodesPerHour {
		t.Fatalf("aggregate mail issued = %d, want <= %d", got, domain.MaxAggregateCodesPerHour)
	}
	if got := mail.count(); got != domain.MaxAggregateCodesPerHour {
		t.Fatalf("aggregate mail issued = %d, want exactly the frozen ceiling %d", got, domain.MaxAggregateCodesPerHour)
	}
}

func TestDistinctKeyUsernamesUnbounded(t *testing.T) {
	svc := testKeyService()
	ctx := context.Background()
	for i := 0; i < domain.MaxKeyAccountsPerHour; i++ {
		if _, err := svc.CreateKeyAccount(ctx, fmt.Sprintf("flooduser%d", i)); err != nil {
			t.Fatalf("account %d: %v", i, err)
		}
	}
	if _, err := svc.CreateKeyAccount(ctx, "flooduseroverflow"); !errors.Is(err, domain.ErrAggregateExhausted) {
		t.Fatalf("101st distinct username must refuse aggregate-exhausted, got %v", err)
	}
	// A malformed username still refuses as invalid, not aggregate.
	if _, err := svc.CreateKeyAccount(ctx, "ab"); !errors.Is(err, domain.ErrUsernameInvalid) {
		t.Fatalf("invalid username must refuse username-invalid, got %v", err)
	}
}

func TestSessionsMultiplyWithoutBound(t *testing.T) {
	svc := testKeyService()
	ctx := context.Background()
	created, err := svc.CreateKeyAccount(ctx, "floodsessions")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < domain.MaxLiveSessionsPerAccount; i++ {
		if _, err := svc.LoginWithKey(ctx, created.Key); err != nil {
			t.Fatalf("login %d: %v", i, err)
		}
	}
	if _, err := svc.LoginWithKey(ctx, created.Key); !errors.Is(err, domain.ErrSessionBudgetExhausted) {
		t.Fatalf("over-budget login must refuse session-budget-exhausted, got %v", err)
	}
	if n, err := svc.Store.CountLiveFamiliesByAccount(ctx, created.Account.ID, svc.Clock.NowUnix()); err != nil || n != domain.MaxLiveSessionsPerAccount {
		t.Fatalf("live families = %d (%v), want exactly %d", n, err, domain.MaxLiveSessionsPerAccount)
	}
	// A second legitimate account still logs in: per-account budget,
	// not a global lockout.
	other, err := svc.CreateKeyAccount(ctx, "secondlegit")
	if err != nil {
		t.Fatalf("second account must stay free: %v", err)
	}
	if _, err := svc.LoginWithKey(ctx, other.Key); err != nil {
		t.Fatalf("second account login must succeed: %v", err)
	}
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
	svc, _, _ := testService()
	var calls atomic.Int64
	svc.Store = countingStore{Store: svc.Store, calls: &calls}
	if err := svc.RequestCode(context.Background(), "not-an-address"); err != nil {
		t.Fatalf("invalid address errored instead of cheap refuse: %v", err)
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("invalid address cost %d store lookups, want zero", got)
	}
	// Empty and over-long shapes also refuse cheaply with nil (no oracle).
	for _, bad := range []string{"", "   ", "no-at-sign", "a@b c.invalid"} {
		if err := svc.RequestCode(context.Background(), bad); err != nil {
			t.Fatalf("bad address %q errored: %v", bad, err)
		}
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("malformed addresses cost %d store lookups, want zero", got)
	}
}
