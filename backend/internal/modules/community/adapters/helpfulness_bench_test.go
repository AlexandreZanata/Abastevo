//go:build integration

package adapters

// CS07A helpfulness benchmark: the exact application paths (Cast,
// TallyFor) against a disposable database at safe local rates with a
// separate warmup. Runs only with -bench; use -benchtime=1x -v (inner
// loops are fixed). Staging guard runs (5 x 120s windows, separate
// warmup, <=5 req/s ceiling) reuse these scenarios under the frozen
// staging protocol recorded in the CS07A evidence file.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/domain"
)

// benchMirrorTallySQL mirrors GetHelpfulnessTally for EXPLAIN capture.
// TestBenchHelpfulnessMirrors fails on drift against helpfulness.sql.
const benchMirrorTallySQL = `EXPLAIN (ANALYZE, BUFFERS)
SELECT observation_id, decision_sequence, helpful, not_helpful
FROM community_helpfulness_tallies
WHERE observation_id = $1 AND decision_sequence = $2`

func TestBenchHelpfulnessMirrors(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller unavailable")
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "db", "queries", "community", "helpfulness.sql"))
	if err != nil {
		t.Fatalf("read helpfulness.sql: %v", err)
	}
	app := string(raw)
	for _, fragment := range []string{
		"FROM community_helpfulness_tallies",
		"WHERE observation_id = @observation_id AND decision_sequence = @decision_sequence",
	} {
		if !strings.Contains(app, fragment) {
			t.Errorf("bench mirror drifted, missing %q", fragment)
		}
	}
}

func benchPercentiles(ms []float64) (p50, p95, p99 float64) {
	cp := append([]float64{}, ms...)
	sort.Float64s(cp)
	at := func(q float64) float64 {
		if len(cp) == 0 {
			return 0
		}
		i := int(q * float64(len(cp)-1))
		return cp[i]
	}
	return at(0.5), at(0.95), at(0.99)
}

type benchVoter struct {
	ref string
}

func (v benchVoter) caller() application.Caller {
	return application.Caller{ContributorID: v.ref, Fingerprint: "fp-bench", KeyID: "k", Token: "tok-" + v.ref}
}

func benchmarkHelpfulness(b *testing.B, workers, iters int) {
	b.Helper()
	s, pool, stationID := freshStore(b)
	hs := NewHelpfulnessStore(pool)
	ctx := context.Background()
	obs := validatedFixture(b, s, stationID, "help-bench-1")
	ports := helpfulPorts(hs)

	// Separate warmup, discarded: pools, plans and caches settle
	// before any measured window.
	for i := 0; i < 50; i++ {
		v := benchVoter{ref: fmt.Sprintf("warm-%d", i)}
		if _, err := application.Cast(ctx, ports, v.caller(), application.CastDTO{
			ObservationID: obs.ID, DecisionSequence: 2,
			Choice: domain.ChoiceHelpful, ClientSubmissionID: fmt.Sprintf("warm-%d", i),
		}); err != nil {
			b.Fatalf("warmup: %v", err)
		}
	}

	measure := func(name string, op func(i int) error) {
		b.Helper()
		timings := make([]float64, 0, iters)
		var mu sync.Mutex
		jobs := make(chan int, iters)
		for i := 0; i < iters; i++ {
			jobs <- i
		}
		close(jobs)
		start := time.Now()
		var wg sync.WaitGroup
		errs := make(chan error, iters)
		for w := 0; w < workers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := range jobs {
					t0 := time.Now()
					if err := op(i); err != nil {
						errs <- err
						return
					}
					mu.Lock()
					timings = append(timings, float64(time.Since(t0).Microseconds())/1000)
					mu.Unlock()
				}
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			b.Fatalf("%s: %v", name, err)
		}
		wall := time.Since(start).Seconds()
		p50, p95, p99 := benchPercentiles(timings)
		var max float64
		for _, v := range timings {
			if v > max {
				max = v
			}
		}
		b.Logf("%s workers=%d iters=%d p50=%.2fms p95=%.2fms p99=%.2fms max=%.2fms wall=%.1fop/s",
			name, workers, len(timings), p50, p95, p99, max, float64(len(timings))/wall)
	}

	measure("tally-read", func(i int) error {
		_, _, err := hs.Tally(ctx, obs.ID, 2)
		return err
	})
	measure("cast-fresh-voter", func(i int) error {
		v := benchVoter{ref: fmt.Sprintf("bench-%d", i)}
		_, err := application.Cast(ctx, ports, v.caller(), application.CastDTO{
			ObservationID: obs.ID, DecisionSequence: 2,
			Choice: domain.ChoiceHelpful, ClientSubmissionID: fmt.Sprintf("bench-%d", i),
		})
		return err
	})
	measure("flip-same-voter", func(i int) error {
		v := benchVoter{ref: "bench-flipper"}
		choice := domain.ChoiceNotHelpful
		if i%2 == 0 {
			choice = domain.ChoiceHelpful
		}
		_, err := application.Cast(ctx, ports, v.caller(), application.CastDTO{
			ObservationID: obs.ID, DecisionSequence: 2,
			Choice: choice, ClientSubmissionID: fmt.Sprintf("flip-%d", i),
		})
		return err
	})
}

func BenchmarkHelpfulness(b *testing.B) { benchmarkHelpfulness(b, 8, 200) }
