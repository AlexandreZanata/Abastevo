package application

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

// SEC01-B: unavailable email delivery leaves no retained issuance.
// With delivery off, RequestCode answers nil before any store
// write — identical to quota/cooldown/unknown, so no oracle leaks
// status and no unbounded rows accumulate for a dependency that
// cannot deliver.

type countingCodeStore struct {
	Store
	saves *atomic.Int64
}

func (s countingCodeStore) SaveCode(ctx context.Context, rec domain.EmailCode) error {
	s.saves.Add(1)
	return s.Store.SaveCode(ctx, rec)
}

func TestDisabledDeliveryRetainsNothing(t *testing.T) {
	svc, _, mail := testService()
	var saves atomic.Int64
	svc.Store = countingCodeStore{Store: svc.Store, saves: &saves}
	svc.MailEnabled = false
	if err := svc.RequestCode(context.Background(), "nobody@example.invalid"); err != nil {
		t.Fatalf("disabled delivery errored: %v", err)
	}
	if got := saves.Load(); got != 0 {
		t.Errorf("code rows saved = %d, want zero", got)
	}
	if got := mail.count(); got != 0 {
		t.Errorf("mail issued = %d, want zero", got)
	}
}

func TestEnabledDeliveryUnchanged(t *testing.T) {
	svc, _, mail := testService()
	svc.MailEnabled = true
	if err := svc.RequestCode(context.Background(), "somebody@example.invalid"); err != nil {
		t.Fatalf("enabled delivery errored: %v", err)
	}
	if got := mail.count(); got != 1 {
		t.Errorf("mail issued = %d, want exactly one", got)
	}
}
