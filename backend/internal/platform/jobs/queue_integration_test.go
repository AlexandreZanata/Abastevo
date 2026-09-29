//go:build integration

package jobs

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("integration database unreachable at %s: %v (start it: docker compose -f infra/compose.dev.yml up -d db)", dsn, err)
	}
	defer conn.Close(ctx)
	return dsn
}

func freshQueue(t *testing.T) (*Queue, *pgxpool.Pool) {
	t.Helper()
	adminDSN := testDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("jobs_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatalf("create database: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		admin, err := pgx.Connect(ctx, adminDSN)
		if err != nil {
			t.Errorf("admin connect for drop: %v", err)
			return
		}
		defer admin.Close(ctx)
		if _, err := admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
			t.Errorf("drop database: %v", err)
		}
	})
	u, err := url.Parse(adminDSN)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	u.Path = "/" + name
	dsn := u.String()
	if _, err := migrate.Apply(ctx, dsn, dbmigrations.Files); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return NewQueue(pool), pool
}

func TestTwoWorkersClaimDisjointly(t *testing.T) {
	q, _ := freshQueue(t)
	ctx := context.Background()
	const jobs = 6
	for i := 0; i < jobs; i++ {
		if _, err := Enqueue(ctx, q.pool, "import", []byte(`{"n":1}`), fmt.Sprintf("job-%d", i), 3, time.Time{}); err != nil {
			t.Fatalf("enqueue %d: %v", i, err)
		}
	}
	claimed := make(map[string]string)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for w := 0; w < 2; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			worker := fmt.Sprintf("w-%d", w)
			for {
				job, err := q.Claim(ctx, "import", worker, time.Minute)
				if errors.Is(err, ErrNoJob) {
					return
				}
				if err != nil {
					t.Errorf("claim: %v", err)
					return
				}
				mu.Lock()
				if owner, dup := claimed[job.ID]; dup {
					t.Errorf("job %s claimed twice (%s and %s)", job.ID, owner, worker)
				}
				claimed[job.ID] = worker
				mu.Unlock()
				if err := q.Complete(ctx, job.ID, job.LeaseToken); err != nil {
					t.Errorf("complete: %v", err)
					return
				}
			}
		}(w)
	}
	wg.Wait()
	if len(claimed) != jobs {
		t.Errorf("claimed %d, want %d", len(claimed), jobs)
	}
}

func TestLeaseExpiryAndStaleAck(t *testing.T) {
	q, pool := freshQueue(t)
	ctx := context.Background()
	if _, err := Enqueue(ctx, pool, "import", []byte(`{}`), "lease-1", 3, time.Time{}); err != nil {
		t.Fatal(err)
	}
	first, err := q.Claim(ctx, "import", "w-1", 50*time.Millisecond)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	time.Sleep(120 * time.Millisecond)
	second, err := q.Claim(ctx, "import", "w-2", time.Minute)
	if err != nil {
		t.Fatalf("reclaim after expiry: %v", err)
	}
	if second.ID != first.ID || second.LeaseToken == first.LeaseToken {
		t.Errorf("lease not fenced: %+v -> %+v", first, second)
	}
	if err := q.Complete(ctx, first.ID, first.LeaseToken); !errors.Is(err, ErrStaleLease) {
		t.Errorf("stale ack = %v, want stale lease", err)
	}
	if err := q.Complete(ctx, second.ID, second.LeaseToken); err != nil {
		t.Errorf("fresh ack: %v", err)
	}
}

func TestCrashAfterEffectRedeliversWithoutDuplicating(t *testing.T) {
	q, _ := freshQueue(t)
	ctx := context.Background()
	// The effect map stands in for an idempotent consumer keyed by the job
	// dedupe key (P03-T04 Runner semantics without crossing packages).
	effects := map[string]int{}
	if _, err := Enqueue(ctx, q.pool, "import", []byte(`{}`), "crash-1", 3, time.Time{}); err != nil {
		t.Fatal(err)
	}
	apply := func(job Job) {
		effects[job.DedupeKey]++
	}
	job, err := q.Claim(ctx, "import", "w-1", 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	apply(job)
	// Crash: no ack. The lease lapses and the job redelivers.
	time.Sleep(120 * time.Millisecond)
	again, err := q.Claim(ctx, "import", "w-2", time.Minute)
	if err != nil {
		t.Fatalf("redelivery: %v", err)
	}
	if again.ID != job.ID {
		t.Fatalf("redelivered different job: %s vs %s", again.ID, job.ID)
	}
	// An idempotent consumer keys the effect and runs it once.
	if effects[job.DedupeKey] != 1 {
		t.Fatalf("effect applied %d times before redelivery", effects[job.DedupeKey])
	}
	if err := q.Complete(ctx, again.ID, again.LeaseToken); err != nil {
		t.Fatal(err)
	}
}

func TestPoisonParksDeadAndReplays(t *testing.T) {
	q, _ := freshQueue(t)
	ctx := context.Background()
	if _, err := Enqueue(ctx, q.pool, "import", []byte(`{}`), "poison-1", 2, time.Time{}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		job, err := q.Claim(ctx, "import", "w-1", time.Minute)
		if err != nil {
			t.Fatalf("claim %d: %v", i, err)
		}
		status, err := q.Fail(ctx, job.ID, job.LeaseToken, 0, "boom")
		if err != nil {
			t.Fatalf("fail %d: %v", i, err)
		}
		if i == 0 && status != "queued" {
			t.Errorf("first failure = %q, want queued", status)
		}
		if i == 1 && status != "dead" {
			t.Errorf("second failure = %q, want dead", status)
		}
	}
	if _, err := q.Claim(ctx, "import", "w-1", time.Minute); !errors.Is(err, ErrNoJob) {
		t.Errorf("dead job claimable: %v", err)
	}
	var reason string
	if err := q.pool.QueryRow(ctx, "SELECT last_error FROM job_queue").Scan(&reason); err != nil || reason != "boom" {
		t.Errorf("diagnosis = %q, %v", reason, err)
	}
	// Audited replay returns it with a fresh budget.
	var id string
	if err := q.pool.QueryRow(ctx, "SELECT id::text FROM job_queue").Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := q.ReplayDead(ctx, id, "operator: fixed handler"); err != nil {
		t.Fatalf("replay: %v", err)
	}
	job, err := q.Claim(ctx, "import", "w-1", time.Minute)
	if err != nil {
		t.Fatalf("replayed claim: %v", err)
	}
	if job.Attempts != 1 {
		t.Errorf("replayed attempts = %d, want reset budget (1 after claim)", job.Attempts)
	}
	if err := q.Complete(ctx, job.ID, job.LeaseToken); err != nil {
		t.Fatal(err)
	}
	if err := q.ReplayDead(ctx, id, "operator: again"); err == nil {
		t.Error("live job replayed as dead")
	}
}

func TestDedupeKeyConverges(t *testing.T) {
	q, _ := freshQueue(t)
	ctx := context.Background()
	first, err := Enqueue(ctx, q.pool, "import", []byte(`{"a":1}`), "same", 3, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Enqueue(ctx, q.pool, "import", []byte(`{"a":2}`), "same", 3, time.Time{})
	if err != nil {
		t.Fatalf("dedupe re-enqueue: %v", err)
	}
	if first != second {
		t.Errorf("dedupe returned different ids: %s vs %s", first, second)
	}
}
