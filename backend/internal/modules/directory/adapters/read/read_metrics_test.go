//go:build integration

package read

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

type checkpointQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func completedCheckpointCount(ctx context.Context, query checkpointQuerier) (int64, error) {
	var count int64
	if err := query.QueryRow(ctx, `SELECT num_done FROM pg_stat_checkpointer`).Scan(&count); err != nil {
		return 0, fmt.Errorf("collect completed checkpoints (PostgreSQL 18): %w", err)
	}
	return count, nil
}

func aggregateImportRate(latenciesMS []float64, window time.Duration) (float64, error) {
	if len(latenciesMS) == 0 {
		return 0, nil
	}
	if window <= 0 {
		return 0, fmt.Errorf("completed imports require a positive observation window")
	}
	return float64(len(latenciesMS)) / window.Seconds(), nil
}

type checkpointRow struct {
	value int64
	err   error
}

func (r checkpointRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	*dest[0].(*int64) = r.value
	return nil
}

type checkpointProbe struct {
	row checkpointRow
	sql string
}

func (q *checkpointProbe) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	q.sql = sql
	return q.row
}

func TestAggregateImportRateUsesElapsedConcurrentWindow(t *testing.T) {
	got, err := aggregateImportRate([]float64{2000, 2000, 2000, 2000}, 2*time.Second)
	if err != nil || math.Abs(got-2) > 1e-12 {
		t.Fatalf("four overlapping imports in two seconds = %v/s, %v; want 2/s", got, err)
	}
	if _, err := aggregateImportRate([]float64{2000}, 0); err == nil {
		t.Fatal("missing elapsed window must not produce a rate")
	}
	got, err = aggregateImportRate(nil, 0)
	if err != nil || got != 0 {
		t.Fatalf("no completed imports = %v, %v; want zero", got, err)
	}
}

func TestCompletedCheckpointCountPostgres18AndFailure(t *testing.T) {
	probe := &checkpointProbe{row: checkpointRow{value: 7}}
	got, err := completedCheckpointCount(context.Background(), probe)
	if err != nil || got != 7 || !strings.Contains(probe.sql, "num_done") || strings.Contains(probe.sql, "checkpoints_req") {
		t.Fatalf("PG18 count=%d err=%v query=%s", got, err, probe.sql)
	}
	want := errors.New("permission denied")
	probe.row = checkpointRow{err: want}
	if _, err := completedCheckpointCount(context.Background(), probe); !errors.Is(err, want) {
		t.Fatalf("collector must preserve failure, got %v", err)
	}
	probe.row = checkpointRow{}
	if got, err := completedCheckpointCount(context.Background(), probe); err != nil || got != 0 {
		t.Fatalf("valid zero count=%d err=%v", got, err)
	}
}

// This uses a uniquely created/disposable database, never the caller's database
// as a fixture. The PG18 counter itself is cluster-wide and is not reset here.
func TestCompletedCheckpointCountRealPostgres18(t *testing.T) {
	pool := freshPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var version int
	if err := pool.QueryRow(ctx, "SELECT current_setting('server_version_num')::integer").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version < 180000 || version >= 190000 {
		t.Fatalf("expected PostgreSQL 18, got %d", version)
	}
	before, err := completedCheckpointCount(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	after, err := completedCheckpointCount(ctx, pool)
	if err != nil || after < before {
		t.Fatalf("non-monotonic checkpoint count %d -> %d: %v", before, after, err)
	}
	t.Logf("server_version_num=%d completed_checkpoints_before=%d after=%d (shared Abastevo cluster counter)", version, before, after)
}
