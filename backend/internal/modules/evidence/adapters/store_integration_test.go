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

func TestRecordVerifiedRollsBackDuplicateKey(t *testing.T) {
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
	clash := testObject(sess.ID)
	clash.FinalKey = first.FinalKey
	if _, err := s.RecordVerified(ctx, sess.ID, clash); err == nil {
		t.Fatal("duplicate final key accepted")
	}
	got, err := s.Session(ctx, sess.ID)
	if err != nil || got.Status != domain.StateVerifying {
		t.Errorf("status after rollback = %+v, %v (want VERIFYING)", got, err)
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
	// stay insert-only apart from the single-column bind claim. Any
	// DELETE, TRUNCATE or second objects UPDATE fails this test.
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "db", "queries", "evidence", "evidence.sql"))
	if err != nil {
		t.Fatalf("read owned queries: %v", err)
	}
	upper := strings.ToUpper(string(raw))
	for _, verb := range []string{"DELETE FROM evidence_", "TRUNCATE"} {
		if strings.Contains(upper, verb) {
			t.Errorf("destructive statement present: %s", verb)
		}
	}
	if n := strings.Count(upper, "UPDATE EVIDENCE_OBJECTS"); n != 1 {
		t.Errorf("objects updates = %d, want exactly the bind claim", n)
	}
}
