package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
)

// SEC03-A atomic admission (AUD-02/04, B-BR-005): daily count plus
// live-byte budgets admit with the row in one step; identical replays
// converge on the winner even at full quota; divergent bodies
// conflict; parallel fresh floods cannot overshoot either ceiling.

func distinctPorts(store *fakeStore, base time.Time) Ports {
	n := 0
	var mu sync.Mutex
	return Ports{
		Clock: func() time.Time { return base },
		NewID: func() (string, error) {
			mu.Lock()
			defer mu.Unlock()
			n++
			return fmt.Sprintf("e0000000-0000-4000-8000-%012d", n), nil
		},
		NewKey: func() (string, error) {
			mu.Lock()
			defer mu.Unlock()
			return fmt.Sprintf("q/%032d", n), nil
		},
		CheckQuota: func(context.Context, string, string) (time.Duration, error) { return 0, nil },
		Presign: func(_ context.Context, key, mime string, maxBytes int64, _ time.Time) (string, map[string]string, time.Time, error) {
			return "https://storage.example.invalid/" + key + "?sig=test",
				map[string]string{"Content-Type": mime},
				base.Add(5 * time.Minute), nil
		},
		Store: store,
	}
}

func fillReserves(t *testing.T, p Ports, caller Caller, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		intent := testIntent()
		intent.ClientSessionID = fmt.Sprintf("fill-%d", i)
		if _, err := Reserve(context.Background(), p, caller, intent); err != nil {
			t.Fatalf("fill %d: %v", i, err)
		}
	}
}

func TestReserveParallelCannotOvershoot(t *testing.T) {
	base := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	store := newFakeStore()
	p := distinctPorts(store, base)
	caller := testCaller()
	fillReserves(t, p, caller, 9)
	const racers = 12
	var wg sync.WaitGroup
	var mu sync.Mutex
	denied := 0
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			intent := testIntent()
			intent.ClientSessionID = fmt.Sprintf("race-%d", i)
			_, err := Reserve(context.Background(), p, caller, intent)
			if errors.Is(err, ErrQuotaDenied) {
				mu.Lock()
				denied++
				mu.Unlock()
			} else if err != nil {
				t.Errorf("race %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	if store.saves != MaxReservesPerDay {
		t.Fatalf("parallel saves = %d, want exactly %d", store.saves, MaxReservesPerDay)
	}
	if denied != racers-1 {
		t.Fatalf("denied = %d, want %d", denied, racers-1)
	}
}

func TestReserveReplayReadableAtFullQuota(t *testing.T) {
	base := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	store := newFakeStore()
	p := distinctPorts(store, base)
	caller := testCaller()
	first, err := Reserve(context.Background(), p, caller, testIntent())
	if err != nil {
		t.Fatal(err)
	}
	fillReserves(t, p, caller, MaxReservesPerDay-1)
	// At full quota the identical retry still reads the winner.
	replay, err := Reserve(context.Background(), p, caller, testIntent())
	if err != nil || !replay.Replayed || replay.SessionID != first.SessionID {
		t.Fatalf("replay at full quota = %+v, %v", replay, err)
	}
	// A divergent body under the same key conflicts (not quota).
	diverged := testIntent()
	diverged.DeclaredBytes++
	if _, err := Reserve(context.Background(), p, caller, diverged); !errors.Is(err, ErrConflict) {
		t.Fatalf("divergent = %v, want conflict", err)
	}
	// A fresh intent at full quota denies with a next-midnight delay.
	fresh := testIntent()
	fresh.ClientSessionID = "upl-fresh-overflow"
	_, err = Reserve(context.Background(), p, caller, fresh)
	var deniedErr *QuotaDeniedError
	if !errors.As(err, &deniedErr) || deniedErr.RetryAfter <= 0 {
		t.Fatalf("overflow = %v, want dated quota denial", err)
	}
}

func TestReserveByteBudgetBindsAndReleases(t *testing.T) {
	base := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	store := newFakeStore()
	budget := domain.ReserveBudget{
		Now: base, DayStart: base.Truncate(24 * time.Hour),
		MaxCount: 100, MaxBytes: 5 << 20,
	}
	mk := func(client string, bytes int64) domain.Session {
		s, _, err := domain.NewSession(domain.Params{
			ID:             "e0000000-0000-4000-8000-0000000000" + client[len(client)-1:],
			ContributorRef: "tok-c1", ClientSessionID: client,
			MIME: "image/jpeg", DeclaredBytes: bytes,
			ClaimedSHA256: strings.Repeat("a", 64),
			QuarantineKey: "q/" + client, CreatedAt: base,
		})
		if err != nil {
			panic(err)
		}
		return s
	}
	ctx := context.Background()
	if _, _, err := store.ReserveSession(ctx, mk("upl-a", 3<<20), budget); err != nil {
		t.Fatalf("first 3MiB: %v", err)
	}
	// 3 + 3 > 5MiB: byte budget denies while count stays at 1.
	if _, _, err := store.ReserveSession(ctx, mk("upl-b", 3<<20), budget); !errors.Is(err, domain.ErrQuotaDenied) {
		t.Fatalf("byte overflow = %v, want quota denial", err)
	}
	// Expired sessions release their bytes: the same retry admits
	// once the earlier row no longer holds live bytes.
	store.mu.Lock()
	for k, s := range store.rows {
		s.ExpiresAt = base.Add(-time.Hour)
		store.rows[k] = s
	}
	store.mu.Unlock()
	if _, _, err := store.ReserveSession(ctx, mk("upl-b", 3<<20), budget); err != nil {
		t.Fatalf("post-expiry retry: %v", err)
	}
}
