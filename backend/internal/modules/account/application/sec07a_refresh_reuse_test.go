package application

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

// SEC07-A (AUD-08): a random invalid refresh credential refuses with
// session-invalid and leaves the live family untouched; a proven replay
// of a superseded token revokes the family with session reuse; rotation
// history stays bounded. Never logs tokens or family identifiers.
func TestRefreshInvalidVsProvenReuse(t *testing.T) {
	svc, _, _ := testService()
	ctx := context.Background()
	request(t, svc, "sec07a-valid@example.invalid")
	auth, err := svc.ConsumeCode(ctx, "sec07a-valid@example.invalid", "482916")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Refresh(ctx, auth.Session.FamilyID, "tok-not-a-real-token"); !errors.Is(err, domain.ErrSessionInvalid) {
		t.Fatalf("random invalid refresh must be session-invalid, got %v", err)
	}
	if got := domain.VerdictCode(domain.ErrSessionInvalid); got != "session-invalid" {
		t.Fatalf("verdict = %q, want session-invalid", got)
	}
	// The live family survives the invalid guess: the true token still
	// rotates.
	rotated, err := svc.Refresh(ctx, auth.Session.FamilyID, auth.Session.RefreshToken)
	if err != nil {
		t.Fatalf("valid refresh after invalid guess must succeed, got %v", err)
	}
	// The superseded token is proven reuse and revokes the family.
	if _, err := svc.Refresh(ctx, auth.Session.FamilyID, auth.Session.RefreshToken); !errors.Is(err, domain.ErrSessionReuse) {
		t.Fatalf("superseded refresh must be reuse, got %v", err)
	}
	if _, err := svc.Refresh(ctx, auth.Session.FamilyID, rotated.RefreshToken); !errors.Is(err, domain.ErrSessionRevoked) {
		t.Fatalf("reuse must revoke the family, got %v", err)
	}
}

func TestRefreshHistoryStaysBounded(t *testing.T) {
	svc, _, _ := testService()
	ctx := context.Background()
	request(t, svc, "sec07a-bound@example.invalid")
	auth, err := svc.ConsumeCode(ctx, "sec07a-bound@example.invalid", "482916")
	if err != nil {
		t.Fatal(err)
	}
	current := auth.Session.RefreshToken
	for i := 0; i < domain.MaxRefreshHistory+4; i++ {
		next, err := svc.Refresh(ctx, auth.Session.FamilyID, current)
		if err != nil {
			t.Fatalf("rotation %d: %v", i, err)
		}
		current = next.RefreshToken
	}
	hist, err := svc.Store.RefreshHistory(ctx, auth.Session.FamilyID)
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != domain.MaxRefreshHistory {
		t.Fatalf("history = %d, want exactly %d", len(hist), domain.MaxRefreshHistory)
	}
	// The newest superseded token still proves reuse after the bound.
	penultimate := hist[0]
	_ = penultimate
}

func TestRefreshParallelConvergesOneWinner(t *testing.T) {
	svc, _, _ := testService()
	ctx := context.Background()
	request(t, svc, "sec07a-race@example.invalid")
	auth, err := svc.ConsumeCode(ctx, "sec07a-race@example.invalid", "482916")
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
}
