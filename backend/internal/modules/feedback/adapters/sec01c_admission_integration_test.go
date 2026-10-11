//go:build integration

package adapters

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/feedback/domain"
)

// SEC01-C real-PostGIS comment admission (B-BR-F03/F04): per-account
// live budget and atomic insert hold on real SQL; parallel floods
// cannot overshoot and deletes free budget.

func TestSEC01CCommentBudgetOnRealSQL(t *testing.T) {
	svc, station, acc1, acc2 := commentSetup(t)
	ctx := context.Background()
	for i := 0; i < domain.MaxLiveCommentsPerAccount; i++ {
		if _, err := svc.SubmitComment(ctx, acc1, station, "GASOLINE_REGULAR", fmt.Sprintf("flood %d", i)); err != nil {
			t.Fatalf("comment %d: %v", i, err)
		}
	}
	if _, err := svc.SubmitComment(ctx, acc1, station, "GASOLINE_REGULAR", "one more"); !errors.Is(err, domain.ErrQuotaExceeded) {
		t.Fatalf("101st comment must refuse quota-exceeded, got %v", err)
	}
	if _, err := svc.SubmitComment(ctx, acc2, station, "GASOLINE_REGULAR", "legit second"); err != nil {
		t.Fatalf("second account must stay free: %v", err)
	}
}

func TestSEC01CParallelCommentsBoundedOnRealSQL(t *testing.T) {
	svc, station, acc1, _ := commentSetup(t)
	ctx := context.Background()
	const racers = 140
	var wg sync.WaitGroup
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _ = svc.SubmitComment(ctx, acc1, station, "GASOLINE_REGULAR", fmt.Sprintf("par %d", i))
		}(i)
	}
	wg.Wait()
	n, err := svc.Store.(interface {
		CountLiveCommentsByAccount(ctx context.Context, accountID string) (int, error)
	}).CountLiveCommentsByAccount(ctx, acc1)
	if err != nil || n != domain.MaxLiveCommentsPerAccount {
		t.Fatalf("parallel live comments = %d (%v), want %d", n, err, domain.MaxLiveCommentsPerAccount)
	}
}
