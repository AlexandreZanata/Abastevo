package application

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/feedback/domain"
)

// Hot-path metering (P22-T04). These benchmarks measure the
// application-layer cost of the comment/vote/rating writes against
// the in-memory store with all gates open: no database, no network,
// synthetic accounts only. Results are recorded in the phase
// acceptance evidence as relative costs, never as capacity or SLO
// claims; real-PostGIS behavior stays covered by the integration
// suites.

func benchService() *Service {
	store := NewMemStore()
	var mu sync.Mutex
	var n int
	return &Service{
		Clock:         &fakeClock{now: 1_700_000_000},
		Store:         store,
		Comments:      store,
		Votes:         store,
		CheckAccount:  allowAll,
		StationExists: knownStations,
		IDGen: func() (string, error) {
			mu.Lock()
			defer mu.Unlock()
			n++
			return fmt.Sprintf("bench-%d", n), nil
		},
	}
}

func BenchmarkSubmitComment(b *testing.B) {
	svc := benchService()
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := svc.SubmitComment(ctx, fmt.Sprintf("author-%d", i), "station-1", "GASOLINE_REGULAR", "Preço bom ⛽"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkVote(b *testing.B) {
	svc := benchService()
	ctx := context.Background()
	top, err := svc.SubmitComment(ctx, "author-0", "station-1", "GASOLINE_REGULAR", "Thread")
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := svc.Vote(ctx, fmt.Sprintf("voter-%d", i), top.ID, domain.VoteValid); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRate(b *testing.B) {
	svc := benchService()
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := svc.Rate(ctx, fmt.Sprintf("rater-%d", i), "station-1", "GASOLINE_REGULAR", 5); err != nil {
			b.Fatal(err)
		}
	}
}
