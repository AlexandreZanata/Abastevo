//go:build integration

package adapters

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
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
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
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
	name := fmt.Sprintf("evidence_test_%d", time.Now().UnixNano())
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

// newUUIDv4 mints test identifiers from crypto/rand.
func newUUIDv4() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	hexed := hex.EncodeToString(b[:])
	return hexed[0:8] + "-" + hexed[8:12] + "-" + hexed[12:16] + "-" +
		hexed[16:20] + "-" + hexed[20:32], nil
}

func testSession(id, client string) domain.Session {
	now := time.Now()
	s, _, err := domain.NewSession(domain.Params{
		ID: id, ContributorRef: "tok-c1",
		ClientSessionID: client, MIME: "image/jpeg", DeclaredBytes: 512 << 10,
		ClaimedSHA256: strings.Repeat("a", 64), QuarantineKey: "q/" + strings.ReplaceAll(id, "-", ""),
		CreatedAt: now,
	})
	if err != nil {
		panic(err)
	}
	return s
}

func enqueueStub(jobs *[][]byte) func(context.Context, pgx.Tx, string, []byte, string) error {
	return func(_ context.Context, _ pgx.Tx, kind string, payload []byte, dedupe string) error {
		if kind != "verify-evidence" {
			return fmt.Errorf("unexpected job kind %q", kind)
		}
		*jobs = append(*jobs, payload)
		return nil
	}
}

func testObject(sessionID string) ObjectData {
	sum := sha256.Sum256([]byte(sessionID))
	id, err := newUUIDv4()
	if err != nil {
		panic(err)
	}
	return ObjectData{
		ID: id, FinalKey: "f/" + hex.EncodeToString(sum[:]),
		SourceSHA256: strings.Repeat("a", 64), SanitizedSHA256: strings.Repeat("b", 64),
		Width: 4, Height: 4, DHash: 123,
	}
}

func TestReserveConvergesAndConflicts(t *testing.T) {
	s, _ := freshStore(t)
	ctx := context.Background()
	sess := testSession("e0000000-0000-4000-8000-000000000001", "upl-1")
	stored, replayed, err := s.ReserveSession(ctx, sess)
	if err != nil || replayed || stored.ID != sess.ID {
		t.Fatalf("reserve = %+v %v %v", stored, replayed, err)
	}
	again, replayed, err := s.ReserveSession(ctx, sess)
	if err != nil || !replayed || again.ID != stored.ID {
		t.Fatalf("retry = %+v %v %v", again, replayed, err)
	}
	diverged := sess
	diverged.DeclaredBytes++
	if _, _, err := s.ReserveSession(ctx, diverged); !errors.Is(err, domain.ErrConflict) {
		t.Errorf("divergent retry = %v, want conflict", err)
	}
}

func TestCompleteClaimsAndEnqueuesAtomically(t *testing.T) {
	s, _ := freshStore(t)
	ctx := context.Background()
	sess := testSession("e0000000-0000-4000-8000-000000000001", "upl-1")
	if _, _, err := s.ReserveSession(ctx, sess); err != nil {
		t.Fatal(err)
	}
	var jobs [][]byte
	if err := s.CompleteSession(ctx, sess.ID, enqueueStub(&jobs)); err != nil {
		t.Fatalf("complete: %v", err)
	}
	got, err := s.Session(ctx, sess.ID)
	if err != nil || got.Status != domain.StateVerifying {
		t.Fatalf("session = %+v, %v", got, err)
	}
	if len(jobs) != 1 {
		t.Fatalf("jobs = %d, want exactly the verify job", len(jobs))
	}
	// Idempotent replay converges without error.
	if err := s.CompleteSession(ctx, sess.ID, enqueueStub(&jobs)); err != nil {
		t.Fatalf("replay complete: %v", err)
	}
}

func TestCompleteRollsBackOnEnqueueFailure(t *testing.T) {
	s, pool := freshStore(t)
	ctx := context.Background()
	sess := testSession("e0000000-0000-4000-8000-000000000001", "upl-1")
	if _, _, err := s.ReserveSession(ctx, sess); err != nil {
		t.Fatal(err)
	}
	failEnqueue := func(context.Context, pgx.Tx, string, []byte, string) error {
		return fmt.Errorf("queue down")
	}
	if err := s.CompleteSession(ctx, sess.ID, failEnqueue); err == nil {
		t.Fatal("enqueue failure accepted")
	}
	got, err := s.Session(ctx, sess.ID)
	if err != nil || got.Status != domain.StateIssued {
		t.Errorf("status after rollback = %+v, %v (want ISSUED)", got, err)
	}
	var n int64
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM job_queue").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("jobs = %d after rollback, want 0", n)
	}
}

func readyFixture(t *testing.T, s *Store, id, client string) ObjectData {
	t.Helper()
	ctx := context.Background()
	sess := testSession(id, client)
	if _, _, err := s.ReserveSession(ctx, sess); err != nil {
		t.Fatal(err)
	}
	var jobs [][]byte
	if err := s.CompleteSession(ctx, sess.ID, enqueueStub(&jobs)); err != nil {
		t.Fatal(err)
	}
	obj := testObject(sess.ID)
	id, err := s.RecordVerified(ctx, sess.ID, obj)
	if err != nil {
		t.Fatal(err)
	}
	if id != obj.ID {
		t.Fatalf("object id = %q, want %q", id, obj.ID)
	}
	return obj
}

func TestRecordVerifiedBindsFactAtomically(t *testing.T) {
	s, _ := freshStore(t)
	ctx := context.Background()
	obj := readyFixture(t, s, "e0000000-0000-4000-8000-000000000001", "upl-1")
	view, err := s.ForCommunity(ctx, obj.ID)
	if err != nil || !view.Found || !view.Ready || view.OwnerRef != "tok-c1" {
		t.Fatalf("community view = %+v, %v", view, err)
	}
	// Same outcome replays with the existing object id; a different
	// outcome for the same session refuses.
	id, err := s.RecordVerified(ctx, "e0000000-0000-4000-8000-000000000001", obj)
	if err != nil || id != obj.ID {
		t.Fatalf("replay verified = %q, %v", id, err)
	}
	other := obj
	other.FinalKey = "f/" + strings.Repeat("c", 64)
	other.ID, _ = newUUIDv4()
	if _, err := s.RecordVerified(ctx, "e0000000-0000-4000-8000-000000000001", other); !errors.Is(err, domain.ErrBadTransition) {
		t.Errorf("second outcome = %v, want bad transition", err)
	}
}

func TestRecordVerifiedConvergesDuplicateContentKey(t *testing.T) {
	s, _ := freshStore(t)
	ctx := context.Background()
	first := readyFixture(t, s, "e0000000-0000-4000-8000-000000000001", "upl-1")
	sess := testSession("e0000000-0000-4000-8000-000000000002", "upl-2")
	if _, _, err := s.ReserveSession(ctx, sess); err != nil {
		t.Fatal(err)
	}
	var jobs [][]byte
	if err := s.CompleteSession(ctx, sess.ID, enqueueStub(&jobs)); err != nil {
		t.Fatal(err)
	}
	// Byte-identical resubmission converges on the existing object
	// instead of dying on the unique constraint; the session is READY.
	clash := testObject(sess.ID)
	clash.FinalKey = first.FinalKey
	clash.SourceSHA256 = first.SourceSHA256
	id, err := s.RecordVerified(ctx, sess.ID, clash)
	if err != nil || id != first.ID {
		t.Fatalf("converged verified = %q, %v", id, err)
	}
	got, err := s.Session(ctx, sess.ID)
	if err != nil || got.Status != domain.StateReady {
		t.Errorf("status after converge = %+v, %v (want READY)", got, err)
	}
	// Same key with divergent bytes refuses instead of forking history.
	sess2 := testSession("e0000000-0000-4000-8000-000000000003", "upl-3")
	if _, _, err := s.ReserveSession(ctx, sess2); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteSession(ctx, sess2.ID, enqueueStub(&jobs)); err != nil {
		t.Fatal(err)
	}
	divergent := testObject(sess2.ID)
	divergent.FinalKey = first.FinalKey
	divergent.SourceSHA256 = strings.Repeat("c", 64)
	if _, err := s.RecordVerified(ctx, sess2.ID, divergent); !errors.Is(err, domain.ErrBadTransition) {
		t.Errorf("divergent outcome = %v, want bad transition", err)
	}
}

func TestRecordRejectedKeepsReasons(t *testing.T) {
	s, _ := freshStore(t)
	ctx := context.Background()
	sess := testSession("e0000000-0000-4000-8000-000000000001", "upl-1")
	if _, _, err := s.ReserveSession(ctx, sess); err != nil {
		t.Fatal(err)
	}
	var jobs [][]byte
	if err := s.CompleteSession(ctx, sess.ID, enqueueStub(&jobs)); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordRejected(ctx, sess.ID, []string{"invalid-image"}); err != nil {
		t.Fatalf("reject: %v", err)
	}
	got, err := s.Session(ctx, sess.ID)
	if err != nil || got.Status != domain.StateRejected {
		t.Fatalf("session = %+v, %v", got, err)
	}
	if err := s.RecordRejected(ctx, sess.ID, []string{"invalid-image"}); !errors.Is(err, domain.ErrBadTransition) {
		t.Errorf("second reject = %v, want bad transition", err)
	}
}

func TestTryBindObjectClaimsOnce(t *testing.T) {
	s, _ := freshStore(t)
	ctx := context.Background()
	obj := readyFixture(t, s, "e0000000-0000-4000-8000-000000000001", "upl-1")
	obs1, _ := newUUIDv4()
	obs2, _ := newUUIDv4()
	if err := s.TryBindObject(ctx, obj.ID, obs1, "tok-c1"); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if err := s.TryBindObject(ctx, obj.ID, obs1, "tok-c1"); err != nil {
		t.Fatalf("replay bind: %v", err)
	}
	if err := s.TryBindObject(ctx, obj.ID, obs2, "tok-c1"); !errors.Is(err, ErrAlreadyBound) {
		t.Errorf("second bind = %v, want already-bound", err)
	}
	if err := s.TryBindObject(ctx, obj.ID, obs2, "tok-other"); !errors.Is(err, ErrNotOwner) {
		t.Errorf("foreign bind = %v, want not-owner", err)
	}
	missing, _ := newUUIDv4()
	if err := s.TryBindObject(ctx, missing, obs1, "tok-c1"); !errors.Is(err, ErrUnknownObject) {
		t.Errorf("missing bind = %v, want unknown-object", err)
	}
}

func TestCountSinceScopesContributor(t *testing.T) {
	s, _ := freshStore(t)
	ctx := context.Background()
	if _, _, err := s.ReserveSession(ctx, testSession("e0000000-0000-4000-8000-000000000001", "upl-1")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ReserveSession(ctx, testSession("e0000000-0000-4000-8000-000000000002", "upl-2")); err != nil {
		t.Fatal(err)
	}
	n, err := s.CountSince(ctx, "tok-c1", time.Now().Add(-time.Hour))
	if err != nil || n != 2 {
		t.Errorf("count = %d, %v (want 2)", n, err)
	}
	n, err = s.CountSince(ctx, "tok-c1", time.Now().Add(time.Hour))
	if err != nil || n != 0 {
		t.Errorf("future count = %d, %v (want 0)", n, err)
	}
}

func TestNoDestructivePaths(t *testing.T) {
	// Sessions mutate only through guarded conditional updates; objects
	// stay insert-only apart from the exclusive observation/capture bind claims, the
	// final-deleted marker and the 90-day hash purge. Any TRUNCATE,
	// session DELETE or further objects mutation fails this test.
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "db", "queries", "evidence", "evidence.sql"))
	if err != nil {
		t.Fatalf("read owned queries: %v", err)
	}
	upper := strings.ToUpper(string(raw))
	if strings.Contains(upper, "TRUNCATE") {
		t.Error("destructive statement present: TRUNCATE")
	}
	if strings.Contains(upper, "DELETE FROM EVIDENCE_SESSIONS") {
		t.Error("destructive statement present: session deletes")
	}
	if n := strings.Count(upper, "UPDATE EVIDENCE_OBJECTS"); n != 3 {
		t.Errorf("objects updates = %d, want exactly the two exclusive bind claims plus final-deleted marker", n)
	}
	for _, guard := range []string{
		"BOUND_CAPTURE_ID IS NULL AND (BOUND_OBSERVATION_ID IS NULL",
		"S.PHOTO_CAPTURE_ID = @CAPTURE_ID AND S.STATUS = 'READY' AND S.EXPIRES_AT > @NOW_AT",
		"O.FINAL_DELETED_AT IS NULL AND O.BOUND_OBSERVATION_ID IS NULL",
		"(O.BOUND_CAPTURE_ID IS NULL OR O.BOUND_CAPTURE_ID = @CAPTURE_ID)",
	} {
		if !strings.Contains(upper, guard) {
			t.Errorf("missing exclusive owner/capture/expiry guard: %s", guard)
		}
	}
	if n := strings.Count(upper, "DELETE FROM EVIDENCE_OBJECTS"); n != 1 {
		t.Errorf("objects deletes = %d, want exactly the 90-day purge", n)
	}
}

func testSessionAt(id, client string, createdAt time.Time) domain.Session {
	s, _, err := domain.NewSession(domain.Params{
		ID: id, ContributorRef: "tok-c1",
		ClientSessionID: client, MIME: "image/jpeg", DeclaredBytes: 512 << 10,
		ClaimedSHA256: strings.Repeat("a", 64), QuarantineKey: "q/" + strings.ReplaceAll(id, "-", ""),
		CreatedAt: createdAt,
	})
	if err != nil {
		panic(err)
	}
	return s
}

func backdateObject(t *testing.T, pool *pgxpool.Pool, objectID string, createdAt time.Time) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), "UPDATE evidence_objects SET created_at = $1 WHERE id = $2", createdAt, objectID); err != nil {
		t.Fatal(err)
	}
}

func TestExpireIdleSessions(t *testing.T) {
	s, _ := freshStore(t)
	ctx := context.Background()
	old := time.Now().Add(-25 * time.Hour)
	if _, _, err := s.ReserveSession(ctx, testSessionAt("e0000000-0000-4000-8000-000000000001", "upl-1", old)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ReserveSession(ctx, testSession("e0000000-0000-4000-8000-000000000002", "upl-2")); err != nil {
		t.Fatal(err)
	}
	expired, err := s.ExpireIdleSessions(ctx, time.Now(), 10)
	if err != nil || len(expired) != 1 || expired[0].ID != "e0000000-0000-4000-8000-000000000001" {
		t.Fatalf("expired = %+v, %v", expired, err)
	}
	if expired[0].QuarantineKey == "" {
		t.Error("expired row carries no quarantine key")
	}
	got, err := s.Session(ctx, "e0000000-0000-4000-8000-000000000001")
	if err != nil || got.Status != domain.StateExpired {
		t.Fatalf("session = %+v, %v", got, err)
	}
	again, err := s.ExpireIdleSessions(ctx, time.Now(), 10)
	if err != nil || len(again) != 0 {
		t.Fatalf("repeat = %+v, %v (want convergence)", again, err)
	}
}

func TestVerifyingListsSplitByAge(t *testing.T) {
	s, _ := freshStore(t)
	ctx := context.Background()
	var jobs [][]byte
	young := testSession("e0000000-0000-4000-8000-000000000001", "upl-1")
	if _, _, err := s.ReserveSession(ctx, young); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteSession(ctx, young.ID, enqueueStub(&jobs)); err != nil {
		t.Fatal(err)
	}
	stuck := testSessionAt("e0000000-0000-4000-8000-000000000002", "upl-2", time.Now().Add(-26*time.Hour))
	if _, _, err := s.ReserveSession(ctx, stuck); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteSession(ctx, stuck.ID, enqueueStub(&jobs)); err != nil {
		t.Fatal(err)
	}
	stuckRows, err := s.ListStuckVerifying(ctx, time.Now().Add(-24*time.Hour), 10)
	if err != nil || len(stuckRows) != 1 || stuckRows[0].ID != stuck.ID {
		t.Fatalf("stuck = %+v, %v", stuckRows, err)
	}
	youngRows, err := s.ListYoungVerifying(ctx, time.Now().Add(-24*time.Hour), 10)
	if err != nil || len(youngRows) != 1 || youngRows[0].ID != young.ID {
		t.Fatalf("young = %+v, %v", youngRows, err)
	}
	cutoff := time.Now().Add(-24 * time.Hour)
	if err := s.ExpireStuckSession(ctx, stuck.ID, cutoff); err != nil {
		t.Fatalf("expire stuck: %v", err)
	}
	if err := s.ExpireStuckSession(ctx, young.ID, cutoff); !errors.Is(err, domain.ErrBadTransition) {
		t.Errorf("young expiry = %v, want bad transition", err)
	}
	if err := s.ExpireStuckSession(ctx, stuck.ID, cutoff); !errors.Is(err, domain.ErrBadTransition) {
		t.Errorf("second expiry = %v, want bad transition", err)
	}
}

func TestQuarantineCandidatesAndMarks(t *testing.T) {
	s, _ := freshStore(t)
	ctx := context.Background()
	readyObj := readyFixture(t, s, "e0000000-0000-4000-8000-000000000001", "upl-1")
	_ = readyObj
	rej := testSession("e0000000-0000-4000-8000-000000000002", "upl-2")
	if _, _, err := s.ReserveSession(ctx, rej); err != nil {
		t.Fatal(err)
	}
	var jobs [][]byte
	if err := s.CompleteSession(ctx, rej.ID, enqueueStub(&jobs)); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordRejected(ctx, rej.ID, []string{"invalid-image"}); err != nil {
		t.Fatal(err)
	}
	verifying := testSession("e0000000-0000-4000-8000-000000000003", "upl-3")
	if _, _, err := s.ReserveSession(ctx, verifying); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteSession(ctx, verifying.ID, enqueueStub(&jobs)); err != nil {
		t.Fatal(err)
	}
	cands, err := s.QuarantineCandidates(ctx, time.Now().Add(-24*time.Hour), 10)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, c := range cands {
		ids[c.ID] = true
	}
	if !ids["e0000000-0000-4000-8000-000000000001"] || !ids["e0000000-0000-4000-8000-000000000002"] {
		t.Errorf("candidates = %v, want ready + rejected", ids)
	}
	if ids["e0000000-0000-4000-8000-000000000003"] {
		t.Errorf("young verifying listed for deletion: %v", ids)
	}
	for _, c := range cands {
		if err := s.MarkQuarantineDeleted(ctx, c.ID); err != nil {
			t.Fatalf("mark: %v", err)
		}
	}
	again, err := s.QuarantineCandidates(ctx, time.Now().Add(-24*time.Hour), 10)
	if err != nil || len(again) != 0 {
		t.Fatalf("repeat = %+v, %v (want convergence)", again, err)
	}
}

func TestStaleFinalsPurgeAndDHashSignals(t *testing.T) {
	s, pool := freshStore(t)
	ctx := context.Background()
	now := time.Now()
	oldObj := readyFixture(t, s, "e0000000-0000-4000-8000-000000000001", "upl-1")
	backdateObject(t, pool, oldObj.ID, now.Add(-15*24*time.Hour))
	freshObj := readyFixture(t, s, "e0000000-0000-4000-8000-000000000002", "upl-2")
	ancientObj := readyFixture(t, s, "e0000000-0000-4000-8000-000000000003", "upl-3")
	backdateObject(t, pool, ancientObj.ID, now.Add(-91*24*time.Hour))

	stale, err := s.StaleFinalCandidates(ctx, now.Add(-14*24*time.Hour), 10)
	if err != nil || len(stale) != 2 {
		t.Fatalf("stale = %+v, %v (want old + ancient)", stale, err)
	}
	if err := s.MarkFinalDeleted(ctx, oldObj.ID); err != nil {
		t.Fatalf("mark final: %v", err)
	}
	view, err := s.ForCommunity(ctx, oldObj.ID)
	if err != nil || !view.Found || view.Ready {
		t.Fatalf("deleted view = %+v, %v (want found but unavailable)", view, err)
	}
	matches, err := s.FindByDHash(ctx, 123, now.Add(-100*24*time.Hour))
	if err != nil || len(matches) != 3 {
		t.Fatalf("dhash matches = %d, %v (want all three)", len(matches), err)
	}
	n, err := s.PurgeObjectsBefore(ctx, now.Add(-90*24*time.Hour))
	if err != nil || n != 1 {
		t.Fatalf("purged = %d, %v (want the ancient row)", n, err)
	}
	matches, err = s.FindByDHash(ctx, 123, now.Add(-100*24*time.Hour))
	if err != nil || len(matches) != 2 {
		t.Fatalf("dhash after purge = %d, %v (bounded signals survive)", len(matches), err)
	}
	_ = freshObj
}

func TestSweepBatchValidation(t *testing.T) {
	s, _ := freshStore(t)
	ctx := context.Background()
	if _, err := s.ExpireIdleSessions(ctx, time.Now(), 0); err == nil {
		t.Error("zero batch accepted")
	}
}

func TestObjectSignalsResolveDHash(t *testing.T) {
	s, _ := freshStore(t)
	ctx := context.Background()
	obj := readyFixture(t, s, "e0000000-0000-4000-8000-000000000001", "upl-1")
	dhash, owner, err := s.ObjectSignals(ctx, obj.ID)
	if err != nil {
		t.Fatalf("signals: %v", err)
	}
	if dhash != 123 || owner != "tok-c1" {
		t.Errorf("signals = %d %q", dhash, owner)
	}
	missing, _ := newUUIDv4()
	if _, _, err := s.ObjectSignals(ctx, missing); !errors.Is(err, ErrNoObject) {
		t.Errorf("missing = %v, want no-object", err)
	}
}

func TestPhotoSessionIntentReplayAndOwnerBoundSharing(t *testing.T) {
	s, pool := freshStore(t)
	ctx := context.Background()
	id := "e0000000-0000-4000-8000-000000000041"
	capture := "c0000000-0000-4000-8000-000000000041"
	sess := testSession(id, "photo-41")
	sess.PhotoCaptureID = capture
	if _, _, err := s.ReserveSession(ctx, sess); err != nil {
		t.Fatal(err)
	}
	changed := sess
	changed.PhotoCaptureID = ""
	if _, _, err := s.ReserveSession(ctx, changed); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("stripped capture replay=%v", err)
	}
	var jobs [][]byte
	if err := s.CompleteSession(ctx, id, enqueueStub(&jobs)); err != nil {
		t.Fatal(err)
	}
	obj := testObject(id)
	if _, err := s.RecordVerified(ctx, id, obj); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []struct{ owner, capture string }{{"other", capture}, {"tok-c1", "c0000000-0000-4000-8000-000000000042"}} {
		if err := s.TryBindPhotoObject(ctx, obj.ID, bad.capture, bad.owner, time.Now()); err == nil {
			t.Fatal("foreign photo bound")
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.TryBindPhotoObject(ctx, obj.ID, capture, "tok-c1", time.Now()); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if err := s.TryBindObject(ctx, obj.ID, "d0000000-0000-4000-8000-000000000041", "tok-c1"); !errors.Is(err, ErrAlreadyBound) {
		t.Fatalf("legacy lane reused shared photo: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE evidence_sessions SET expires_at=now()-interval '1 second' WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
	if err := s.TryBindPhotoObject(ctx, obj.ID, capture, "tok-c1", time.Now()); err == nil {
		t.Fatal("expired ready photo bound")
	}
}
