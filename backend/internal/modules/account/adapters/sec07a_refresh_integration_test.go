//go:build integration

package adapters

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

// SEC07-A (AUD-08) real-PostGIS proof: random invalid refresh refuses
// with session-invalid and leaves the family live; proven replay revokes;
// parallel refresh converges to one winner; rotation history stays
// bounded. Never logs tokens or family identifiers.
func TestPGRefreshInvalidVsProvenReuse(t *testing.T) {
	svc, _, _ := freshService(t)
	ctx := context.Background()
	pgRequest(t, svc, "sec07a-pg-invalid@example.invalid")
	auth, err := svc.ConsumeCode(ctx, "sec07a-pg-invalid@example.invalid", "482916")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Refresh(ctx, auth.Session.FamilyID, "tok-not-a-real-token"); !errors.Is(err, domain.ErrSessionInvalid) {
		t.Fatalf("random invalid refresh must be session-invalid, got %v", err)
	}
	rotated, err := svc.Refresh(ctx, auth.Session.FamilyID, auth.Session.RefreshToken)
	if err != nil {
		t.Fatalf("valid refresh after invalid guess must succeed, got %v", err)
	}
	if _, err := svc.Refresh(ctx, auth.Session.FamilyID, auth.Session.RefreshToken); !errors.Is(err, domain.ErrSessionReuse) {
		t.Fatalf("superseded refresh must be reuse, got %v", err)
	}
	if _, err := svc.Refresh(ctx, auth.Session.FamilyID, rotated.RefreshToken); !errors.Is(err, domain.ErrSessionRevoked) {
		t.Fatalf("reuse must revoke the family, got %v", err)
	}
}

func TestPGRefreshParallelAndHistoryBound(t *testing.T) {
	svc, _, _ := freshService(t)
	ctx := context.Background()
	pgRequest(t, svc, "sec07a-pg-race@example.invalid")
	auth, err := svc.ConsumeCode(ctx, "sec07a-pg-race@example.invalid", "482916")
	if err != nil {
		t.Fatal(err)
	}
	const racers = 8
	var wg sync.WaitGroup
	wins := make([]string, racers)
	errs := make([]error, racers)
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sess, err := svc.Refresh(ctx, auth.Session.FamilyID, auth.Session.RefreshToken)
			if err == nil {
				wins[i] = sess.RefreshToken
			} else {
				errs[i] = err
			}
		}(i)
	}
	wg.Wait()
	ok := 0
	for _, w := range wins {
		if w != "" {
			ok++
		}
	}
	if ok != 1 {
		t.Fatalf("parallel refresh must mint exactly one winner, got %d", ok)
	}
	for _, err := range errs {
		if err == nil {
			continue
		}
		if !errors.Is(err, domain.ErrSessionReuse) && !errors.Is(err, domain.ErrSessionRevoked) && !errors.Is(err, domain.ErrSessionInvalid) {
			t.Fatalf("loser verdict must stay fail-closed, got %v", err)
		}
	}

	pgRequest(t, svc, "sec07a-pg-bound@example.invalid")
	auth2, err := svc.ConsumeCode(ctx, "sec07a-pg-bound@example.invalid", "111111")
	if err != nil {
		t.Fatal(err)
	}
	current := auth2.Session.RefreshToken
	for i := 0; i < domain.MaxRefreshHistory+4; i++ {
		next, err := svc.Refresh(ctx, auth2.Session.FamilyID, current)
		if err != nil {
			t.Fatalf("rotation %d: %v", i, err)
		}
		current = next.RefreshToken
	}
	store, isPG := svc.Store.(*PGStore)
	if !isPG {
		t.Fatal("integration service must use PGStore")
	}
	hist, err := store.RefreshHistory(ctx, auth2.Session.FamilyID)
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != domain.MaxRefreshHistory {
		t.Fatalf("history = %d, want exactly %d", len(hist), domain.MaxRefreshHistory)
	}
}
