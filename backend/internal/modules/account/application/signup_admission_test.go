package application

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

func TestSignupDeletionCannotRefundAdmission(t *testing.T) {
	svc := testKeyService()
	ctx := context.Background()
	svc.RequestErasure = func(context.Context, string, []string) error { return nil }
	svc.RequestBillingExit = func(context.Context, string) error { return nil }
	for i := 0; i < domain.MaxKeyAccountsPerHour; i++ {
		acc, err := svc.CreateKeyAccount(ctx, fmt.Sprintf("cycleuser%d", i))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.DeleteAccount(ctx, acc.Account.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.CreateKeyAccount(ctx, "onebeyond"); !errors.Is(err, domain.ErrAggregateExhausted) {
		t.Fatalf("erasure refunded budget: %v", err)
	}
	if n, err := svc.Store.CountKeyAccountsSince(ctx, svc.Clock.NowUnix()-3600); err != nil || n != domain.MaxKeyAccountsPerHour {
		t.Fatalf("admission count %d %v", n, err)
	}
	svc.Clock.(*fakeClock).advance(3601 * time.Second)
	if _, err := svc.CreateKeyAccount(ctx, "afterexpiry"); err != nil {
		t.Fatal(err)
	}
	if n, err := svc.Store.CountKeyAccountsSince(ctx, svc.Clock.NowUnix()-3600); err != nil || n != 1 {
		t.Fatalf("expired admission count %d %v", n, err)
	}
}
