//go:build integration

package jobs

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

func testDSNJobs(t *testing.T) string {
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

func freshDispatcher(t *testing.T, handlers map[string]Handler) (*Dispatcher, *pgxpool.Pool) {
	t.Helper()
	adminDSN := testDSNJobs(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("dispatch_test_%d", time.Now().UnixNano())
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
	q := NewQueue(pool)
	return &Dispatcher{Queue: q, Handlers: handlers, WorkerID: "test-worker", LeaseTTL: time.Minute, RetryDelay: time.Second}, pool
}

type scriptHandler struct {
	kind    string
	version int
	calls   *int
	fail    error
}

func (s scriptHandler) Kind() string { return s.kind }
func (s scriptHandler) Version() int { return s.version }
func (s scriptHandler) Handle(_ context.Context, _ Job) error {
	*s.calls++
	return s.fail
}

func enqueueRaw(t *testing.T, pool *pgxpool.Pool, kind, payload, dedupe string) {
	t.Helper()
	if _, err := Enqueue(context.Background(), pool, kind, []byte(payload), dedupe, 1, time.Time{}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
}

func jobStatus(t *testing.T, pool *pgxpool.Pool, dedupe string) (string, int) {
	t.Helper()
	var status string
	var attempts int
	err := pool.QueryRow(context.Background(),
		`SELECT status, attempts FROM job_queue WHERE dedupe_key = $1`, dedupe).Scan(&status, &attempts)
	if err != nil {
		t.Fatal(err)
	}
	return status, attempts
}

func TestUnknownKindAndVersionParkDead(t *testing.T) {
	var calls int
	d, pool := freshDispatcher(t, map[string]Handler{
		"known": scriptHandler{kind: "known", version: 2, calls: &calls},
	})
	ctx := context.Background()
	enqueueRaw(t, pool, "mystery", `{"version":1}`, "u1")
	enqueueRaw(t, pool, "known", `{"version":1}`, "u2")
	enqueueRaw(t, pool, "known", `{"version":2}`, "u3")
	for i := 0; i < 3; i++ {
		found, err := d.RunOnce(ctx)
		if err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
		if !found {
			t.Fatalf("run %d found nothing", i)
		}
	}
	if status, _ := jobStatus(t, pool, "u1"); status != "dead" {
		t.Errorf("unknown kind = %q, want dead", status)
	}
	if status, _ := jobStatus(t, pool, "u2"); status != "dead" {
		t.Errorf("version mismatch = %q, want dead", status)
	}
	if status, _ := jobStatus(t, pool, "u3"); status != "done" {
		t.Errorf("valid = %q, want done", status)
	}
	if calls != 1 {
		t.Errorf("handler calls = %d, want exactly the valid job", calls)
	}
}

func TestHandlerErrorRetriesThenParks(t *testing.T) {
	var calls int
	d, pool := freshDispatcher(t, map[string]Handler{
		"flaky": scriptHandler{kind: "flaky", version: 1, calls: &calls, fail: errors.New("boom")},
	})
	ctx := context.Background()
	enqueueRaw(t, pool, "flaky", `{"version":1}`, "f1")
	found, err := d.RunOnce(ctx)
	if err != nil || !found {
		t.Fatalf("run = %v, %v", found, err)
	}
	if status, attempts := jobStatus(t, pool, "f1"); status != "dead" || attempts != 1 {
		t.Errorf("after cap = %q attempts %d, want dead/1", status, attempts)
	}
	if calls != 1 {
		t.Errorf("calls = %d", calls)
	}
}

func TestRunStopsOnCancel(t *testing.T) {
	d, _ := freshDispatcher(t, map[string]Handler{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := d.Run(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("run = %v, want canceled", err)
	}
}

func TestSchedulerDedupeAndDisabled(t *testing.T) {
	d, pool := freshDispatcher(t, map[string]Handler{})
	_ = d
	ctx := context.Background()
	now := time.Date(2026, 9, 28, 15, 4, 0, 0, time.UTC)
	sched := NewScheduler(pool, []Schedule{
		{Name: "daily", Kind: "import", Version: 1, Interval: 24 * time.Hour, Enabled: true,
			Build: func(period string) map[string]any { return map[string]any{"period": period} }},
		{Name: "off", Kind: "import", Version: 1, Interval: time.Hour, Enabled: false, Reason: "D05 pending",
			Build: func(period string) map[string]any { return map[string]any{"period": period} }},
	})
	sched.now = func() time.Time { return now }
	if _, err := sched.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := sched.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	var n int64
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM job_queue").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("rows = %d, want exactly one (restart-safe dedupe)", n)
	}
	sched.now = func() time.Time { return now.Add(25 * time.Hour) }
	if _, err := sched.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM job_queue").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("rows = %d, want one per period", n)
	}
}
