//go:build integration

package adapters

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
)

// SEC03-A real-PostGIS atomic admission (AUD-02/04, B-BR-005): count
// plus live-byte budgets admit with the row in one transaction;
// parallel fresh floods cannot overshoot; replays read the winner at
// full quota; divergent bodies conflict.

func sec03aBudget(now time.Time) domain.ReserveBudget {
	y, m, d := now.UTC().Date()
	return domain.ReserveBudget{
		Now: now, DayStart: time.Date(y, m, d, 0, 0, 0, 0, time.UTC),
		MaxCount: 10, MaxBytes: 10 * (3 << 20),
	}
}

func sec03aSession(id, client string, bytes int64, created time.Time) domain.Session {
	s, _, err := domain.NewSession(domain.Params{
		ID: id, ContributorRef: "tok-c1",
		ClientSessionID: client, MIME: "image/jpeg", DeclaredBytes: bytes,
		ClaimedSHA256: strings.Repeat("a", 64),
		QuarantineKey: "q/" + strings.ReplaceAll(id, "-", ""),
		CreatedAt:     created,
	})
	if err != nil {
		panic(err)
	}
	return s
}

func TestSEC03AParallelCannotOvershootOnRealSQL(t *testing.T) {
	s, _ := freshStore(t)
	ctx := context.Background()
	now := time.Now()
	budget := sec03aBudget(now)
	for i := 0; i < 9; i++ {
		sess := sec03aSession(fmt.Sprintf("e0000000-0000-4000-8000-%012d", i+1), fmt.Sprintf("fill-%d", i), 512<<10, now)
		if _, _, err := s.ReserveSession(ctx, sess, budget); err != nil {
			t.Fatalf("fill %d: %v", i, err)
		}
	}
	const racers = 12
	var wg sync.WaitGroup
	var mu sync.Mutex
	denied := 0
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sess := sec03aSession(fmt.Sprintf("e0000000-0000-4000-8000-%012d", 100+i), fmt.Sprintf("race-%d", i), 512<<10, now)
			_, _, err := s.ReserveSession(ctx, sess, budget)
			if errors.Is(err, domain.ErrQuotaDenied) {
				mu.Lock()
				denied++
				mu.Unlock()
			} else if err != nil {
				t.Errorf("race %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	n, err := s.CountSince(ctx, "tok-c1", budget.DayStart)
	if err != nil || n != 10 {
		t.Fatalf("real-SQL rows = %d (%v), want exactly 10", n, err)
	}
	if denied != racers-1 {
		t.Fatalf("denied = %d, want %d", denied, racers-1)
	}
}

func TestSEC03AReplayAndConflictOnRealSQL(t *testing.T) {
	s, _ := freshStore(t)
	ctx := context.Background()
	now := time.Now()
	budget := sec03aBudget(now)
	first := sec03aSession("e0000000-0000-4000-8000-000000009001", "upl-1", 512<<10, now)
	stored, replayed, err := s.ReserveSession(ctx, first, budget)
	if err != nil || replayed {
		t.Fatalf("fresh = %+v, %v", stored, err)
	}
	// Fill to the ceiling with distinct keys.
	for i := 1; i < 10; i++ {
		sess := sec03aSession(fmt.Sprintf("e0000000-0000-4000-8000-%012d", 9002+i), fmt.Sprintf("fill-%d", i), 512<<10, now)
		if _, _, err := s.ReserveSession(ctx, sess, budget); err != nil {
			t.Fatalf("fill %d: %v", i, err)
		}
	}
	// The identical retry still reads the winner at full quota.
	winner, replayed, err := s.ReserveSession(ctx, first, budget)
	if err != nil || !replayed || winner.ID != stored.ID {
		t.Fatalf("replay at full quota = %+v, %v", winner, err)
	}
	// A divergent body under the same key conflicts.
	diverged := first
	diverged.DeclaredBytes++
	if _, _, err := s.ReserveSession(ctx, diverged, budget); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("divergent = %v, want conflict", err)
	}
	// A fresh intent at full quota denies.
	overflow := sec03aSession("e0000000-0000-4000-8000-000000009099", "upl-overflow", 512<<10, now)
	if _, _, err := s.ReserveSession(ctx, overflow, budget); !errors.Is(err, domain.ErrQuotaDenied) {
		t.Fatalf("overflow = %v, want quota denial", err)
	}
}
