//go:build integration

package adapters

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

// SEC01-C real-PostGIS admission (B-BR-A01/A02/A04/A05): aggregate mail
// ceiling, per-account session cap and retention purges hold on real
// SQL; parallel issuers cannot overshoot.

func TestSEC01CAggregateMailBoundedOnRealSQL(t *testing.T) {
	svc, clock, mail := freshService(t)
	ctx := context.Background()
	const racers = 40
	var wg sync.WaitGroup
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = svc.RequestCode(ctx, fmt.Sprintf("flood-%d@example.invalid", i))
		}(i)
	}
	wg.Wait()
	if got := mail.count(); got != domain.MaxAggregateCodesPerHour {
		t.Fatalf("parallel mail issued = %d, want exactly %d", got, domain.MaxAggregateCodesPerHour)
	}
	store := svc.Store.(interface {
		CountAllCodesSince(ctx context.Context, sinceUnix int64) (int, error)
	})
	n, err := store.CountAllCodesSince(ctx, clock.NowUnix()-3600)
	if err != nil || n != domain.MaxAggregateCodesPerHour {
		t.Fatalf("real-SQL aggregate rows = %d (%v), want %d", n, err, domain.MaxAggregateCodesPerHour)
	}
	// Retention purge removes only expired rows and reports exactly.
	purged, err := svc.Store.(interface {
		PurgeExpiredCodes(ctx context.Context, beforeUnix int64) (int, error)
	}).PurgeExpiredCodes(ctx, clock.NowUnix()+domain.CodeTTLSeconds+domain.CodePurgeGraceSeconds+1)
	if err != nil || purged != domain.MaxAggregateCodesPerHour {
		t.Fatalf("purge = %d (%v), want %d", purged, err, domain.MaxAggregateCodesPerHour)
	}
}

func TestSEC01CSessionCapOnRealSQL(t *testing.T) {
	svc, _, _ := freshService(t)
	ctx := context.Background()
	created, err := svc.CreateKeyAccount(ctx, "realcap")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < domain.MaxLiveSessionsPerAccount; i++ {
		if _, err := svc.LoginWithKey(ctx, created.Key); err != nil {
			t.Fatalf("login %d: %v", i, err)
		}
	}
	if _, err := svc.LoginWithKey(ctx, created.Key); err != domain.ErrSessionBudgetExhausted {
		t.Fatalf("over-budget login must refuse session-budget-exhausted, got %v", err)
	}
	// Second legitimate account still logs in.
	other, err := svc.CreateKeyAccount(ctx, "realsecond")
	if err != nil {
		t.Fatalf("second account must stay free: %v", err)
	}
	if _, err := svc.LoginWithKey(ctx, other.Key); err != nil {
		t.Fatalf("second login must succeed: %v", err)
	}
	_ = time.Now
}
