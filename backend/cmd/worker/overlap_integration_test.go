//go:build integration

package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5/pgxpool"
)

type overlapHandler struct {
	pool *pgxpool.Pool
}

func (overlapHandler) Kind() string { return "overlap-test" }
func (overlapHandler) Version() int { return 1 }
func (h overlapHandler) Handle(ctx context.Context, job platformjobs.Job) error {
	// Primary-key insertion is the observable effect. A second worker that
	// claimed the same job would fail here and be retried, exposing a broken
	// SKIP LOCKED/lease fence as attempts > 1 and a missing effect.
	if _, err := h.pool.Exec(ctx,
		"INSERT INTO overlap_effects(id, worker_id) VALUES ($1, $2)",
		job.ID, job.WorkerID); err != nil {
		return err
	}
	time.Sleep(2 * time.Millisecond)
	return nil
}

// TestOverlappingWorkersDoNotDuplicateEffects runs two independent dispatcher
// loops (distinct worker IDs) over the same queue and database. The queue
// claim uses FOR UPDATE SKIP LOCKED plus a monotonic lease token
// (backend/db/queries/platform/jobs.sql:10-47); the handler writes one
// primary-key effect per job. The test fails if any job executes twice.
func TestOverlappingWorkersDoNotDuplicateEffects(t *testing.T) {
	pool := freshNativeWorkerDB(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `
		CREATE TABLE overlap_effects (
			id TEXT PRIMARY KEY,
			worker_id TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`); err != nil {
		t.Fatal(err)
	}

	const jobs = 40
	for i := 0; i < jobs; i++ {
		if _, err := platformjobs.Enqueue(ctx, pool, "overlap-test",
			[]byte(`{"version":1}`), fmt.Sprintf("overlap-%d", i), 3, time.Time{}); err != nil {
			t.Fatal(err)
		}
	}

	queue := platformjobs.NewQueue(pool)
	handlers := map[string]platformjobs.Handler{"overlap-test": overlapHandler{pool: pool}}
	dispatchers := []*platformjobs.Dispatcher{
		{Queue: queue, Handlers: handlers, WorkerID: "overlap-a", Kind: "overlap-test", LeaseTTL: 30 * time.Second, RetryDelay: time.Millisecond},
		{Queue: queue, Handlers: handlers, WorkerID: "overlap-b", Kind: "overlap-test", LeaseTTL: 30 * time.Second, RetryDelay: time.Millisecond},
	}

	runCtx, cancel := context.WithCancel(ctx)
	stopped := make(chan error, 1)
	go func() { stopped <- runDispatchers(runCtx, dispatchers) }()

	deadline := time.Now().Add(30 * time.Second)
	for {
		var pending int
		if err := pool.QueryRow(ctx,
			"SELECT count(*) FROM job_queue WHERE kind='overlap-test' AND status <> 'done'").Scan(&pending); err != nil {
			cancel()
			t.Fatal(err)
		}
		if pending == 0 {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatalf("overlap jobs did not drain (pending=%d)", pending)
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	select {
	case <-stopped:
	case <-time.After(10 * time.Second):
		t.Fatal("dispatchers did not stop")
	}

	var done, attempts, effects, workers int
	if err := pool.QueryRow(ctx,
		"SELECT count(*) FROM job_queue WHERE kind='overlap-test' AND status='done'").Scan(&done); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx,
		"SELECT count(*) FROM job_queue WHERE kind='overlap-test' AND attempts <> 1").Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM overlap_effects").Scan(&effects); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(DISTINCT worker_id) FROM overlap_effects").Scan(&workers); err != nil {
		t.Fatal(err)
	}
	if done != jobs || attempts != 0 || effects != jobs || workers != 2 {
		t.Fatalf("overlap effects done=%d attempts>1=%d effects=%d workers=%d want %d/0/%d/2", done, attempts, effects, workers, jobs, jobs)
	}
}
