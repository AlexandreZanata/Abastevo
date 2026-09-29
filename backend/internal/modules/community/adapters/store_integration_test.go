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
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/domain"
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

func freshStore(t *testing.T) (*Store, *pgxpool.Pool, string) {
	t.Helper()
	adminDSN := testDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("community_test_%d", time.Now().UnixNano())
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
	stationID := "d6c74c23-63db-4c24-a2e5-408cb23bad26"
	if _, err := pool.Exec(ctx, `INSERT INTO directory_stations (id, display_name) VALUES ($1, 'Posto T')`, stationID); err != nil {
		t.Fatalf("station: %v", err)
	}
	return NewStore(pool), pool, stationID
}

func testObs(stationID, submission string) domain.Observation {
	now := time.Now()
	obs, _, err := domain.NewObservation(domain.Params{
		ID: "d6c74c23-63db-4c24-a2e5-408cb23bad27", ContributorRef: "ref-1",
		ClientSubmissionID: submission, StationID: stationID,
		Product: "GASOLINE_REGULAR", Unit: "L", AmountMilli: 5999,
		RawText: "5,999", ConditionKind: "STANDARD",
		ClaimedCapturedAt: now.Add(-time.Hour), ReceivedAt: now,
	})
	if err != nil {
		panic(err)
	}
	return obs
}

func enqueueStub(jobs *[][]byte) func(context.Context, pgx.Tx, string, []byte, string) error {
	return func(_ context.Context, _ pgx.Tx, kind string, payload []byte, dedupe string) error {
		if kind != "validate-observation" {
			return fmt.Errorf("unexpected job kind %q", kind)
		}
		*jobs = append(*jobs, payload)
		return nil
	}
}

func TestSubmitPersistsFactAndJob(t *testing.T) {
	s, _, _ := freshStore(t)
	ctx := context.Background()
	obs := testObs("d6c74c23-63db-4c24-a2e5-408cb23bad26", "sub-1")
	var jobs [][]byte
	id, existed, err := s.Submit(ctx, obs, enqueueStub(&jobs))
	if err != nil || existed || id != obs.ID {
		t.Fatalf("submit = %q %v %v", id, existed, err)
	}
	if len(jobs) != 1 {
		t.Fatalf("jobs = %d, want exactly the validation job", len(jobs))
	}
	loaded, err := s.Observation(ctx, id)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.AmountMilli != 5999 || loaded.QualifierKey != domain.StandardQualifier {
		t.Errorf("loaded = %+v", loaded)
	}
}

func TestRetryYieldsSameObservation(t *testing.T) {
	s, _, _ := freshStore(t)
	ctx := context.Background()
	obs := testObs("d6c74c23-63db-4c24-a2e5-408cb23bad26", "sub-1")
	var jobs [][]byte
	first, existed, err := s.Submit(ctx, obs, enqueueStub(&jobs))
	if err != nil || existed {
		t.Fatalf("first = %q %v %v", first, existed, err)
	}
	second, existed, err := s.Submit(ctx, obs, enqueueStub(&jobs))
	if err != nil || !existed || second != first {
		t.Fatalf("retry = %q %v %v", second, existed, err)
	}
	if len(jobs) != 1 {
		t.Errorf("retry enqueued again: %d jobs", len(jobs))
	}
	diverged := obs
	diverged.AmountMilli = 6099
	if _, _, err := s.Submit(ctx, diverged, enqueueStub(&jobs)); err != domain.ErrConflict {
		t.Errorf("divergent retry = %v, want conflict", err)
	}
}

func TestRollbackOnEnqueueFailure(t *testing.T) {
	s, pool, _ := freshStore(t)
	ctx := context.Background()
	obs := testObs("d6c74c23-63db-4c24-a2e5-408cb23bad26", "sub-1")
	failEnqueue := func(context.Context, pgx.Tx, string, []byte, string) error {
		return fmt.Errorf("queue down")
	}
	if _, _, err := s.Submit(ctx, obs, failEnqueue); err == nil {
		t.Fatal("enqueue failure accepted")
	}
	var n int64
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM community_observations").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("observations = %d after rollback, want 0", n)
	}
	// The key was never consumed: a retry proceeds fresh.
	var jobs [][]byte
	if _, existed, err := s.Submit(ctx, obs, enqueueStub(&jobs)); err != nil || existed {
		t.Errorf("retry after rollback = %v %v", existed, err)
	}
}

func TestConcurrentSubmissionIDOneRow(t *testing.T) {
	s, pool, _ := freshStore(t)
	ctx := context.Background()
	const workers = 8
	ids := make([]string, workers)
	errs := make([]error, workers)
	var wg sync.WaitGroup
	for i := range ids {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var jobs [][]byte
			id, _, err := s.Submit(ctx, testObs("d6c74c23-63db-4c24-a2e5-408cb23bad26", "sub-race"), enqueueStub(&jobs))
			ids[i], errs[i] = id, err
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("worker %d: %v", i, err)
		}
		if ids[i] != ids[0] {
			t.Fatalf("divergent ids: %v", ids)
		}
	}
	var n int64
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM community_observations").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("rows = %d, want exactly one fact", n)
	}
}

func TestRecordDecisionEnforcesMachine(t *testing.T) {
	s, _, _ := freshStore(t)
	ctx := context.Background()
	obs := testObs("d6c74c23-63db-4c24-a2e5-408cb23bad26", "sub-1")
	var jobs [][]byte
	id, _, err := s.Submit(ctx, obs, enqueueStub(&jobs))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	claim, err := domain.ClaimValidation(obs, domain.StateReceived, "job-9", domain.ActorWorker, now, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RecordDecision(ctx, claim); err != nil {
		t.Fatalf("record claim: %v", err)
	}
	// Wrong sequence and skipped states fail before any row exists.
	bad, err := domain.Admit(obs, domain.StateValidating, domain.ActorWorker, now, 9)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RecordDecision(ctx, bad); err == nil {
		t.Error("out-of-order sequence recorded")
	}
	admit, err := domain.Admit(obs, domain.StateValidating, domain.ActorWorker, now, 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RecordDecision(ctx, admit); err != nil {
		t.Fatalf("record admit: %v", err)
	}
	// Wrong FromState with the right sequence must still fail: the store
	// enforces the machine, not just ordering. (An unknown pair would die
	// earlier at the known-transition check.)
	stale, err := domain.Admit(obs, domain.StateValidating, domain.ActorWorker, now, 3)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RecordDecision(ctx, stale); err == nil {
		t.Error("stale from-state recorded")
	}
	history, err := s.Decisions(ctx, id)
	if err != nil || len(history) != 2 {
		t.Fatalf("decisions = %+v, %v", history, err)
	}
	if history[0].ToState != domain.StateValidating || history[1].ToState != domain.StateValidated {
		t.Errorf("history = %+v", history)
	}
}

func TestNoDestructivePaths(t *testing.T) {
	// The owned queries must stay insert/select-only: any UPDATE or DELETE
	// against community tables fails this test, keeping forward fixes
	// append-only by construction. Least-privilege database roles land with
	// provisioning (P08); until then the repository offers no mutation
	// method at all.
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "db", "queries", "community", "observations.sql"))
	if err != nil {
		t.Fatalf("read owned queries: %v", err)
	}
	upper := strings.ToUpper(string(raw))
	for _, verb := range []string{"UPDATE community_", "DELETE FROM community_", "TRUNCATE"} {
		if strings.Contains(upper, verb) {
			t.Errorf("destructive statement present: %s", verb)
		}
	}
}
