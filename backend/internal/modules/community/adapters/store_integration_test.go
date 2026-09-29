//go:build integration

package adapters

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	application "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/application"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/domain"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
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

func TestRecordDecisionWithJobCommitsAtomically(t *testing.T) {
	// P04-T05: the VALIDATED decision and its consensus intent commit in
	// one transaction, so no validated fact waits without downstream work.
	s, pool, _ := freshStore(t)
	ctx := context.Background()
	obs := testObs("d6c74c23-63db-4c24-a2e5-408cb23bad26", "sub-1")
	var stub [][]byte
	if _, _, err := s.Submit(ctx, obs, enqueueStub(&stub)); err != nil {
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
	admit, err := domain.Admit(obs, domain.StateValidating, domain.ActorWorker, now, 2)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"version":1,"observation_id":"` + obs.ID + `"}`)
	enqueue := func(ctx context.Context, tx pgx.Tx, kind string, p []byte, dedupe string) error {
		_, err := platformjobs.Enqueue(ctx, tx, kind, p, dedupe, 5, time.Time{})
		return err
	}
	if err := s.RecordDecisionWithJob(ctx, admit, "community-consensus", payload, "consensus:"+obs.ID, enqueue); err != nil {
		t.Fatalf("record with job: %v", err)
	}
	history, err := s.Decisions(ctx, obs.ID)
	if err != nil || len(history) != 2 || history[1].ToState != domain.StateValidated {
		t.Fatalf("decisions = %+v, %v", history, err)
	}
	var n int64
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM job_queue WHERE dedupe_key = $1", "consensus:"+obs.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("consensus jobs = %d, want exactly the downstream intent", n)
	}
}

func TestRecordDecisionWithJobRollsBackOnEnqueueFailure(t *testing.T) {
	// The enqueue failure must roll back the decision too: a retried
	// validation finds VALIDATING with no phantom VALIDATED row.
	s, pool, _ := freshStore(t)
	ctx := context.Background()
	obs := testObs("d6c74c23-63db-4c24-a2e5-408cb23bad26", "sub-1")
	var stub [][]byte
	if _, _, err := s.Submit(ctx, obs, enqueueStub(&stub)); err != nil {
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
	admit, err := domain.Admit(obs, domain.StateValidating, domain.ActorWorker, now, 2)
	if err != nil {
		t.Fatal(err)
	}
	failEnqueue := func(context.Context, pgx.Tx, string, []byte, string) error {
		return fmt.Errorf("queue down")
	}
	if err := s.RecordDecisionWithJob(ctx, admit, "community-consensus", []byte(`{}`), "consensus:"+obs.ID, failEnqueue); err == nil {
		t.Fatal("enqueue failure accepted")
	}
	history, err := s.Decisions(ctx, obs.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].ToState != domain.StateValidating {
		t.Errorf("decisions after rollback = %+v, want only the claim", history)
	}
	var n int64
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM job_queue WHERE dedupe_key = $1", "consensus:"+obs.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("consensus jobs = %d after rollback, want 0", n)
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

func testBands() application.Bands {
	return application.Bands{
		Proximity: application.ProximityUnknown, Recency: application.RecencyFresh,
		Capture: application.CaptureFresh, Photo: application.PhotoPresent,
		Regional:  application.RegionalConsistent,
		RiskCodes: []string{application.RiskMissingClaim}, NeedsReview: true,
		PolicyVersion: application.SignalsV1,
	}
}

func TestUpsertSignalsConvergesRecomputation(t *testing.T) {
	s, _, stationID := freshStore(t)
	ctx := context.Background()
	obs := testObs(stationID, "sub-1")
	var jobs [][]byte
	id, _, err := s.Submit(ctx, obs, enqueueStub(&jobs))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := s.UpsertSignals(ctx, id, testBands(), now); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	loaded, err := s.LoadSignals(ctx, id)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Photo != application.PhotoPresent || !loaded.NeedsReview ||
		len(loaded.RiskCodes) != 1 || loaded.PolicyVersion != application.SignalsV1 {
		t.Errorf("loaded = %+v", loaded)
	}
	// Recomputation with new inputs converges on one row, no fork.
	evolved := testBands()
	evolved.Photo = application.PhotoDuplicate
	evolved.DuplicateCount = 2
	evolved.RiskCodes = []string{application.RiskMissingClaim, application.RiskDuplicateImage}
	if err := s.UpsertSignals(ctx, id, evolved, now); err != nil {
		t.Fatalf("recompute: %v", err)
	}
	loaded, err = s.LoadSignals(ctx, id)
	if err != nil || loaded.Photo != application.PhotoDuplicate || loaded.DuplicateCount != 2 {
		t.Fatalf("recomputed = %+v, %v", loaded, err)
	}
	if _, err := s.LoadSignals(ctx, "d6c74c23-63db-4c24-a2e5-000000000000"); err == nil {
		t.Error("missing signals accepted")
	}
}

func TestSignalsSchemaHoldsNoExactPayload(t *testing.T) {
	// Privacy by schema: the owned signals migration and queries must
	// never gain coordinate, meter-level or payload columns. Exact
	// inputs live in memory during derivation and never persist.
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}
	base := filepath.Join(filepath.Dir(file), "..", "..", "..", "..")
	for _, rel := range []string{
		"db/migrations/000012_community_signals.sql",
		"db/queries/community/signals.sql",
	} {
		raw, err := os.ReadFile(filepath.Join(base, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		// Scan code, not comments: strip line comments first so the
		// guard judges stored columns rather than documentation.
		var code strings.Builder
		for _, line := range strings.Split(string(raw), "\n") {
			if idx := strings.Index(line, "--"); idx >= 0 {
				line = line[:idx]
			}
			code.WriteString(line + "\n")
		}
		upper := strings.ToUpper(code.String())
		for _, token := range []string{
			"LATITUDE", "LONGITUDE", "GEOGRAPHY", "POINT(",
			"ACCURACY_M", "DISTANCE_M", "COORD", "PAYLOAD",
			"DOUBLE PRECISION",
		} {
			if strings.Contains(upper, token) {
				t.Errorf("%s stores exact payload: %s", rel, token)
			}
		}
	}
}

var fixtureSeq atomic.Int64

func validatedFixture(t *testing.T, s *Store, stationID, submission string) domain.Observation {
	t.Helper()
	ctx := context.Background()
	obs := testObs(stationID, submission)
	obs.ID = fmt.Sprintf("d6c74c23-63db-4c24-a2e5-408cb23bad%02d", fixtureSeq.Add(1))
	var jobs [][]byte
	if _, _, err := s.Submit(ctx, obs, enqueueStub(&jobs)); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	claim, err := domain.ClaimValidation(obs, domain.StateReceived, "job-9", domain.ActorWorker, now, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RecordDecision(ctx, claim); err != nil {
		t.Fatal(err)
	}
	admit, err := domain.Admit(obs, domain.StateValidating, domain.ActorWorker, now, 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RecordDecision(ctx, admit); err != nil {
		t.Fatal(err)
	}
	return obs
}

func confirmEnqueue(jobs *[][]byte) func(context.Context, pgx.Tx, string, []byte, string) error {
	return func(_ context.Context, _ pgx.Tx, kind string, payload []byte, dedupe string) error {
		if kind != "community-consensus" {
			return fmt.Errorf("unexpected job kind %q", kind)
		}
		*jobs = append(*jobs, payload)
		return nil
	}
}

func TestConfirmPersistsVoteAndJob(t *testing.T) {
	s, pool, stationID := freshStore(t)
	ctx := context.Background()
	obs := validatedFixture(t, s, stationID, "sub-1")
	vote, _, err := domain.NewConfirmation(domain.ConfirmationParams{
		ID: "c6c74c23-63db-4c24-a2e5-408cb23bad26", ObservationID: obs.ID,
		ContributorRef: "ref-2", AuthorRef: obs.ContributorRef,
		ClientSubmissionID: "cfm-1", ReceivedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	var jobs [][]byte
	id, replayed, err := s.Confirm(ctx, vote, confirmEnqueue(&jobs))
	if err != nil || replayed || id != vote.ID {
		t.Fatalf("confirm = %q %v %v", id, replayed, err)
	}
	if len(jobs) != 1 {
		t.Fatalf("jobs = %d, want exactly the recompute intent", len(jobs))
	}
	var n int64
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM community_confirmations").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("rows = %d, want one vote", n)
	}
	// Identical retry converges; divergent payload conflicts.
	again, replayed, err := s.Confirm(ctx, vote, confirmEnqueue(&jobs))
	if err != nil || !replayed || again != id {
		t.Fatalf("retry = %q %v %v", again, replayed, err)
	}
	if len(jobs) != 1 {
		t.Errorf("retry enqueued again: %d jobs", len(jobs))
	}
	diverged := vote
	diverged.ObservationID = "d6c74c23-63db-4c24-a2e5-408cb23bad99"
	if _, _, err := s.Confirm(ctx, diverged, confirmEnqueue(&jobs)); !errors.Is(err, domain.ErrConflict) {
		t.Errorf("divergent retry = %v, want conflict", err)
	}
}

func TestConfirmRefusesAmplificationConcurrently(t *testing.T) {
	s, pool, stationID := freshStore(t)
	ctx := context.Background()
	obs := validatedFixture(t, s, stationID, "sub-1")
	const workers = 8
	errs := make([]error, workers)
	var wg sync.WaitGroup
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			vote, _, err := domain.NewConfirmation(domain.ConfirmationParams{
				ID:            fmt.Sprintf("c6c74c23-63db-4c24-a2e5-408cb23bad%02d", i),
				ObservationID: obs.ID, ContributorRef: "ref-2", AuthorRef: obs.ContributorRef,
				ClientSubmissionID: fmt.Sprintf("cfm-%d", i), ReceivedAt: time.Now(),
			})
			if err != nil {
				errs[i] = err
				return
			}
			var jobs [][]byte
			_, _, errs[i] = s.Confirm(ctx, vote, confirmEnqueue(&jobs))
		}(i)
	}
	wg.Wait()
	wins := 0
	for _, err := range errs {
		if err == nil {
			wins++
		} else if !errors.Is(err, domain.ErrAlreadyConfirmed) {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if wins != 1 {
		t.Fatalf("winners = %d, want exactly one vote", wins)
	}
	var n int64
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM community_confirmations").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("rows = %d, want one vote", n)
	}
}

func TestConfirmRollsBackOnEnqueueFailure(t *testing.T) {
	s, pool, stationID := freshStore(t)
	ctx := context.Background()
	obs := validatedFixture(t, s, stationID, "sub-1")
	vote, _, err := domain.NewConfirmation(domain.ConfirmationParams{
		ID: "c6c74c23-63db-4c24-a2e5-408cb23bad26", ObservationID: obs.ID,
		ContributorRef: "ref-2", AuthorRef: obs.ContributorRef,
		ClientSubmissionID: "cfm-1", ReceivedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	failEnqueue := func(context.Context, pgx.Tx, string, []byte, string) error {
		return fmt.Errorf("queue down")
	}
	if _, _, err := s.Confirm(ctx, vote, failEnqueue); err == nil {
		t.Fatal("enqueue failure accepted")
	}
	var n int64
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM community_confirmations").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("rows = %d after rollback, want 0", n)
	}
}

func TestDisputeDedupesActiveReports(t *testing.T) {
	s, pool, stationID := freshStore(t)
	ctx := context.Background()
	obs := validatedFixture(t, s, stationID, "sub-1")
	mkReport := func(id, client, reason string) domain.Dispute {
		d, _, err := domain.NewDispute(domain.DisputeParams{
			ID: id, TargetObservationID: obs.ID,
			ContributorRef: "ref-2", ClientSubmissionID: client,
			Reason: reason, ReceivedAt: time.Now(),
		})
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	var jobs [][]byte
	first, replayed, err := s.Report(ctx, mkReport("e6c74c23-63db-4c24-a2e5-408cb23bad26", "dsp-1", domain.ReasonWrongProduct), confirmEnqueue(&jobs))
	if err != nil || replayed {
		t.Fatalf("report = %q %v %v", first, replayed, err)
	}
	if len(jobs) != 1 {
		t.Fatalf("jobs = %d, want the recompute intent", len(jobs))
	}
	// Identical retry converges.
	again, replayed, err := s.Report(ctx, mkReport("e6c74c23-63db-4c24-a2e5-408cb23bad26", "dsp-1", domain.ReasonWrongProduct), confirmEnqueue(&jobs))
	if err != nil || !replayed || again != first {
		t.Fatalf("retry = %q %v %v", again, replayed, err)
	}
	// Fresh client key on the same open triple converges too: active
	// repeated reports deduplicate instead of flooding.
	dupe, replayed, err := s.Report(ctx, mkReport("e6c74c23-63db-4c24-a2e5-408cb23bad27", "dsp-2", domain.ReasonWrongProduct), confirmEnqueue(&jobs))
	if err != nil || !replayed || dupe != first {
		t.Fatalf("active repeat = %q %v %v", dupe, replayed, err)
	}
	// A different reason is a different report.
	other, replayed, err := s.Report(ctx, mkReport("e6c74c23-63db-4c24-a2e5-408cb23bad28", "dsp-3", domain.ReasonEvidenceMismatch), confirmEnqueue(&jobs))
	if err != nil || replayed || other == first {
		t.Fatalf("other reason = %q %v %v", other, replayed, err)
	}
	// Same client key on a divergent report conflicts.
	second := validatedFixture(t, s, stationID, "sub-2")
	bad, _, err := domain.NewDispute(domain.DisputeParams{
		ID: "e6c74c23-63db-4c24-a2e5-408cb23bad29", TargetObservationID: second.ID,
		ContributorRef: "ref-2", ClientSubmissionID: "dsp-1",
		Reason: domain.ReasonWrongProduct, ReceivedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Report(ctx, bad, confirmEnqueue(&jobs)); !errors.Is(err, domain.ErrConflict) {
		t.Errorf("divergent retry = %v, want conflict", err)
	}
	var n int64
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM community_disputes").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("rows = %d, want exactly two reports", n)
	}
}

func TestVotesStayInsertOnly(t *testing.T) {
	// Votes and reports are facts: any UPDATE, DELETE or TRUNCATE in the
	// owned votes queries fails this test. Resolution history lands with
	// moderation, never by editing these rows.
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "db", "queries", "community", "votes.sql"))
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
