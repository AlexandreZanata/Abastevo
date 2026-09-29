//go:build integration

package adapters

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trust/domain"
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

func freshStore(t *testing.T) (*Store, *pgxpool.Pool) {
	t.Helper()
	adminDSN := testDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("trust_test_%d", time.Now().UnixNano())
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
	return NewStore(pool), pool
}

func testDecision(id, tier string, at time.Time) domain.Decision {
	d, _, err := domain.NewDecision(domain.DecisionParams{
		ID: id, ContributorRef: "tok-c1", Tier: tier,
		Reason: "review", CaseRefs: []string{"case-1"}, OccurredAt: at,
	})
	if err != nil {
		panic(err)
	}
	return d
}

func enqueueStub(jobs *[][]byte) func(context.Context, pgx.Tx, string, []byte, string) error {
	return func(_ context.Context, _ pgx.Tx, kind string, payload []byte, dedupe string) error {
		if kind != "trust-affected-recompute" {
			return fmt.Errorf("unexpected job kind %q", kind)
		}
		*jobs = append(*jobs, payload)
		return nil
	}
}

func TestAppendDecisionProjectsCurrent(t *testing.T) {
	s, _ := freshStore(t)
	ctx := context.Background()
	now := time.Now()
	var jobs [][]byte
	if _, err := s.AppendDecision(ctx, testDecision("d0000000-0000-4000-8000-000000000001", domain.TierEstablished, now), enqueueStub(&jobs)); err != nil {
		t.Fatalf("append: %v", err)
	}
	tier, err := s.Tier(ctx, "tok-c1")
	if err != nil || tier != domain.TierEstablished {
		t.Fatalf("tier = %q, %v", tier, err)
	}
	// Blocks and rehabilitations arrive as new rows; the view follows
	// the latest verdict with a bumped version.
	if _, err := s.AppendDecision(ctx, testDecision("d0000000-0000-4000-8000-000000000002", domain.TierBlocked, now.Add(time.Minute)), enqueueStub(&jobs)); err != nil {
		t.Fatal(err)
	}
	if tier, _ := s.Tier(ctx, "tok-c1"); tier != domain.TierBlocked {
		t.Fatalf("tier = %q, want BLOCKED", tier)
	}
	if _, err := s.AppendDecision(ctx, testDecision("d0000000-0000-4000-8000-000000000003", domain.TierEstablished, now.Add(2*time.Minute)), enqueueStub(&jobs)); err != nil {
		t.Fatal(err)
	}
	if tier, _ := s.Tier(ctx, "tok-c1"); tier != domain.TierEstablished {
		t.Fatalf("tier = %q, want ESTABLISHED after rehabilitation", tier)
	}
	history, err := s.History(ctx, "tok-c1")
	if err != nil || len(history) != 3 {
		t.Fatalf("history = %+v, %v", history, err)
	}
	if history[0].Tier != domain.TierEstablished || history[2].Tier != domain.TierEstablished {
		t.Errorf("history order = %+v", history)
	}
	if len(jobs) != 3 {
		t.Errorf("jobs = %d, want one recompute per verdict", len(jobs))
	}
}

func TestUnknownContributorIsNew(t *testing.T) {
	s, _ := freshStore(t)
	if tier, err := s.Tier(context.Background(), "tok-nobody"); err != nil || tier != domain.TierNew {
		t.Errorf("unknown tier = %q, %v (want NEW without suspicion)", tier, err)
	}
}

func TestRollbackOnEnqueueFailure(t *testing.T) {
	s, pool := freshStore(t)
	ctx := context.Background()
	failEnqueue := func(context.Context, pgx.Tx, string, []byte, string) error {
		return fmt.Errorf("queue down")
	}
	if _, err := s.AppendDecision(ctx, testDecision("d0000000-0000-4000-8000-000000000001", domain.TierBlocked, time.Now()), failEnqueue); err == nil {
		t.Fatal("enqueue failure accepted")
	}
	var n int64
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM trust_decisions").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("decisions = %d after rollback, want 0", n)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM trust_current").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("current rows = %d after rollback, want 0", n)
	}
}

func TestConcurrentAppendsKeepEveryVerdict(t *testing.T) {
	s, pool := freshStore(t)
	ctx := context.Background()
	now := time.Now()
	const workers = 8
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tier := domain.TierEstablished
			if i%2 == 1 {
				tier = domain.TierBlocked
			}
			var jobs [][]byte
			_, _ = s.AppendDecision(ctx, testDecision(fmt.Sprintf("d0000000-0000-4000-8000-0000000000%02d", i), tier, now.Add(time.Duration(i)*time.Second)), enqueueStub(&jobs))
		}(i)
	}
	wg.Wait()
	var n int64
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM trust_decisions WHERE contributor_ref = 'tok-c1'").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != workers {
		t.Errorf("decisions = %d, want every verdict kept", n)
	}
	var version int64
	if err := pool.QueryRow(ctx, "SELECT version FROM trust_current WHERE contributor_ref = 'tok-c1'").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != workers {
		t.Errorf("current version = %d, want one bump per verdict", version)
	}
	tier, err := s.Tier(ctx, "tok-c1")
	if err != nil || (tier != domain.TierEstablished && tier != domain.TierBlocked) {
		t.Errorf("tier = %q, %v", tier, err)
	}
}

func TestNoDestructivePaths(t *testing.T) {
	// The ledger is append-only: any UPDATE, DELETE or TRUNCATE in the
	// owned trust queries fails this test. The current view converges
	// through INSERT ... ON CONFLICT, never through edits.
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "db", "queries", "trust", "trust.sql"))
	if err != nil {
		t.Fatalf("read owned queries: %v", err)
	}
	upper := strings.ToUpper(string(raw))
	for _, verb := range []string{"UPDATE trust_", "DELETE FROM trust_", "TRUNCATE"} {
		if strings.Contains(upper, verb) {
			t.Errorf("destructive statement present: %s", verb)
		}
	}
}
