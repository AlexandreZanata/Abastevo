package application

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

// SEC01-C storage accounting and retention (B-BR-A02/A04/A05): counts
// report hashed totals only; purges remove only expired rows and
// report exact removals; parallel issuance cannot overshoot ceilings.

func TestAggregateAccountingAndPurge(t *testing.T) {
	svc, clock, _ := testService()
	ctx := context.Background()
	store := svc.Store.(*MemStore)

	for i := 0; i < 3; i++ {
		if err := svc.RequestCode(ctx, fmt.Sprintf("acc-%d@example.invalid", i)); err != nil {
			t.Fatal(err)
		}
		clock.advance(61 * 1_000_000_000)
	}
	if n, err := store.CountAllCodesSince(ctx, clock.NowUnix()-3600); err != nil || n != 3 {
		t.Fatalf("aggregate count = %d (%v), want 3", n, err)
	}
	if n, err := store.CountCodesSince(ctx, domain.AddressHash("acc-0@example.invalid"), clock.NowUnix()-3600); err != nil || n != 1 {
		t.Fatalf("per-address count = %d (%v), want 1", n, err)
	}
	// Expired rows purge exactly; live rows stay.
	purged, err := store.PurgeExpiredCodes(ctx, clock.NowUnix()+domain.CodeTTLSeconds+domain.CodePurgeGraceSeconds+1)
	if err != nil || purged != 3 {
		t.Fatalf("purge = %d (%v), want 3", purged, err)
	}
	if n, _ := store.CountAllCodesSince(ctx, 0); n != 0 {
		t.Fatalf("post-purge aggregate = %d, want 0", n)
	}
}

func TestSessionAccountingAndPurge(t *testing.T) {
	svc := testKeyService()
	ctx := context.Background()
	created, err := svc.CreateKeyAccount(ctx, "budgetprobe")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := svc.LoginWithKey(ctx, created.Key); err != nil {
			t.Fatal(err)
		}
	}
	store := svc.Store.(*MemStore)
	if n, err := store.CountLiveFamiliesByAccount(ctx, created.Account.ID, svc.Clock.NowUnix()); err != nil || n != 3 {
		t.Fatalf("live families = %d (%v), want 3", n, err)
	}
	// Absolutely-expired families purge; live ones stay.
	future := svc.Clock.NowUnix() + int64(domain.RefreshAbsoluteLifetimeDays)*24*3600 + int64(domain.SessionPurgeGraceSeconds) + 1
	purged, err := store.PurgeExpiredFamilies(ctx, future)
	if err != nil || purged != 3 {
		t.Fatalf("family purge = %d (%v), want 3", purged, err)
	}
}

func TestParallelIssueCannotOvershootAggregate(t *testing.T) {
	svc, _, mail := testService()
	ctx := context.Background()
	const racers = 40
	var wg sync.WaitGroup
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = svc.RequestCode(ctx, fmt.Sprintf("par-%d@example.invalid", i))
		}(i)
	}
	wg.Wait()
	if got := mail.count(); got != domain.MaxAggregateCodesPerHour {
		t.Fatalf("parallel mail issued = %d, want exactly %d", got, domain.MaxAggregateCodesPerHour)
	}
	if n, err := svc.Store.(*MemStore).CountAllCodesSince(ctx, 0); err != nil || n != domain.MaxAggregateCodesPerHour {
		t.Fatalf("parallel code rows = %d (%v), want %d", n, err, domain.MaxAggregateCodesPerHour)
	}
}
