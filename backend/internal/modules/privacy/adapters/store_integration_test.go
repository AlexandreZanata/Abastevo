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
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/domain"
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
	name := fmt.Sprintf("privacy_test_%d", time.Now().UnixNano())
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

func mustRequest(t *testing.T, id, contributorID, ref, key string, at time.Time) domain.Request {
	t.Helper()
	r, err := domain.NewRequest(domain.RequestParams{
		ID: id, ContributorID: contributorID, ContributorRef: ref,
		ClientSubmissionID: key, Type: domain.TypeExport, RequestedAt: at,
	})
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	return r
}

func enqueueOK(jobs *int) func(context.Context, pgx.Tx, string, []byte, string) error {
	return func(_ context.Context, _ pgx.Tx, kind string, _ []byte, _ string) error {
		if kind != "privacy-export-build" {
			return fmt.Errorf("unexpected job kind %q", kind)
		}
		*jobs++
		return nil
	}
}

func TestRequestExportConvergesRetry(t *testing.T) {
	s, _ := freshStore(t)
	ctx := context.Background()
	base := time.Now().Truncate(time.Millisecond)
	var jobs int
	first := mustRequest(t, "e0000000-0000-4000-8000-000000000001", "c1", "tok-c1", "export-1", base)
	id, replayed, err := s.RequestExport(ctx, first, enqueueOK(&jobs))
	if err != nil || replayed || id != first.ID {
		t.Fatalf("first request = %q %v %v", id, replayed, err)
	}
	// Same owner/key with a freshly minted ID converges (B-BR-005).
	retry := mustRequest(t, "e0000000-0000-4000-8000-000000000002", "c1", "tok-c1", "export-1", base)
	id2, replayed2, err := s.RequestExport(ctx, retry, enqueueOK(&jobs))
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if !replayed2 || id2 != first.ID {
		t.Errorf("retry did not converge: id=%q replayed=%v want %q true", id2, replayed2, first.ID)
	}
	if jobs != 1 {
		t.Errorf("retry enqueued a duplicate build job")
	}
}

func TestRequestExportConflictsDivergentPayload(t *testing.T) {
	s, _ := freshStore(t)
	ctx := context.Background()
	base := time.Now().Truncate(time.Millisecond)
	var jobs int
	first := mustRequest(t, "e0000000-0000-4000-8000-000000000001", "c1", "tok-c1", "export-1", base)
	if _, _, err := s.RequestExport(ctx, first, enqueueOK(&jobs)); err != nil {
		t.Fatal(err)
	}
	// Same owner/key but a different type is a conflicting retry, not a
	// second intent.
	other, err := domain.NewRequest(domain.RequestParams{
		ID: "e0000000-0000-4000-8000-000000000002", ContributorID: "c1",
		ContributorRef: "tok-c1", ClientSubmissionID: "export-1",
		Type: domain.TypeDeletion, RequestedAt: base,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.RequestExport(ctx, other, enqueueOK(&jobs)); err == nil {
		t.Error("divergent payload accepted")
	}
}

func TestConcurrentRequestConverges(t *testing.T) {
	s, pool := freshStore(t)
	ctx := context.Background()
	base := time.Now().Truncate(time.Millisecond)
	const workers = 8
	ids := make([]string, workers)
	errs := make([]error, workers)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			r := mustRequest(t, fmt.Sprintf("e0000000-0000-4000-8000-0000000000%02d", 10+i), "c1", "tok-c1", "export-1", base)
			var jobs int
			id, _, err := s.RequestExport(ctx, r, enqueueOK(&jobs))
			ids[i], errs[i] = id, err
		}(i)
	}
	close(start)
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("worker %d: %v", i, err)
		}
	}
	for i := 1; i < workers; i++ {
		if ids[i] != ids[0] {
			t.Fatalf("concurrent requests diverged: %v", ids)
		}
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM privacy_requests").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("concurrent requests created %d rows, want 1", count)
	}
}

func TestCompleteExportGuards(t *testing.T) {
	s, _ := freshStore(t)
	ctx := context.Background()
	base := time.Now().Truncate(time.Millisecond)
	r := mustRequest(t, "e0000000-0000-4000-8000-000000000001", "c1", "tok-c1", "export-1", base)
	var jobs int
	if _, _, err := s.RequestExport(ctx, r, enqueueOK(&jobs)); err != nil {
		t.Fatal(err)
	}
	archive := []byte(`{"format":"privacy-export-v1"}`)
	sha := ArchiveSHA256(archive)
	if err := s.CompleteExport(ctx, r.ID, archive, sha, base.Add(time.Minute)); err != nil {
		t.Fatalf("complete: %v", err)
	}
	got, raw, err := s.GetForOwner(ctx, "c1", r.ID)
	if err != nil || got.Status != domain.StatusReady || string(raw) != string(archive) {
		t.Errorf("completed = %+v %v", got, err)
	}
	if got.ArchiveSHA256 != sha {
		t.Errorf("hash = %q want %q", got.ArchiveSHA256, sha)
	}
	if !got.ExpiresAt.Equal(base.Add(time.Minute).Add(domain.DownloadTTL)) {
		t.Errorf("window = %v", got.ExpiresAt)
	}
	// Double completion refuses through the guarded transition.
	if err := s.CompleteExport(ctx, r.ID, archive, sha, base.Add(2*time.Minute)); err == nil {
		t.Error("double complete accepted")
	}
	// Hash mismatch never stores.
	r2 := mustRequest(t, "e0000000-0000-4000-8000-000000000002", "c1", "tok-c1", "export-2", base)
	if _, _, err := s.RequestExport(ctx, r2, enqueueOK(&jobs)); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteExport(ctx, r2.ID, archive, "deadbeef", base); err == nil {
		t.Error("mismatched hash accepted")
	}
	// Oversize archives refuse before any write.
	if err := s.CompleteExport(ctx, r2.ID, make([]byte, domain.MaxArchiveBytes+1), "x", base); err == nil {
		t.Error("oversize archive accepted")
	}
}

func TestOwnerIsolation(t *testing.T) {
	s, _ := freshStore(t)
	ctx := context.Background()
	base := time.Now().Truncate(time.Millisecond)
	var jobs int
	mine := mustRequest(t, "e0000000-0000-4000-8000-000000000001", "c-mine", "tok-mine", "export-1", base)
	if _, _, err := s.RequestExport(ctx, mine, enqueueOK(&jobs)); err != nil {
		t.Fatal(err)
	}
	// Foreign owner reads share one not-found shape with missing rows.
	if _, _, err := s.GetForOwner(ctx, "c-other", mine.ID); err == nil {
		t.Error("foreign read accepted")
	}
	if _, _, err := s.GetForOwner(ctx, "c-mine", "e0000000-0000-4000-8000-000000000099"); err == nil {
		t.Error("missing read accepted")
	}
	if _, _, err := s.GetForOwner(ctx, "c-mine", mine.ID); err != nil {
		t.Errorf("owner read: %v", err)
	}
}

func TestRequestRollbackOnEnqueueFailure(t *testing.T) {
	s, pool := freshStore(t)
	ctx := context.Background()
	base := time.Now().Truncate(time.Millisecond)
	r := mustRequest(t, "e0000000-0000-4000-8000-000000000001", "c1", "tok-c1", "export-1", base)
	failing := func(context.Context, pgx.Tx, string, []byte, string) error {
		return context.DeadlineExceeded
	}
	if _, _, err := s.RequestExport(ctx, r, failing); err == nil {
		t.Fatal("enqueue failure accepted")
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM privacy_requests").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("orphaned request row after rollback")
	}
}

func TestSchemaCarriesNoUnexpectedPayload(t *testing.T) {
	_, pool := freshStore(t)
	ctx := context.Background()
	rows, err := pool.Query(ctx, `
		SELECT table_name, column_name, data_type FROM information_schema.columns
		WHERE table_name = 'privacy_requests'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	found := map[string]string{}
	for rows.Next() {
		var table, name, typ string
		if err := rows.Scan(&table, &name, &typ); err != nil {
			t.Fatal(err)
		}
		found[name] = typ
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for col, typ := range found {
		if typ == "json" || typ == "jsonb" {
			t.Errorf("unbounded semi-structured column: %q", col)
		}
	}
	for _, want := range []string{"id", "contributor_id", "contributor_ref", "client_submission_id", "type", "status", "archive", "archive_sha256", "requested_at", "expires_at", "completed_at", "policy_version"} {
		if _, ok := found[want]; !ok {
			t.Errorf("missing ledger column %q (got %v)", want, found)
		}
	}
}

func TestNoDestructiveSQLPaths(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	sqlPath := filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "db", "queries", "privacy", "privacy.sql")
	raw, err := os.ReadFile(sqlPath)
	if err != nil {
		t.Fatal(err)
	}
	upper := strings.ToUpper(string(raw))
	// The only mutating paths are the two guarded forward transitions
	// (REQUESTED to READY/FAILED); zero DELETE, no unguarded UPDATE.
	if got := strings.Count(upper, "UPDATE PRIVACY_REQUESTS"); got != 2 {
		t.Errorf("privacy.sql has %d UPDATEs, want exactly the two guarded transitions", got)
	}
	if strings.Count(upper, "STATUS = 'REQUESTED'") < 2 {
		t.Error("guarded transitions lost their state guard")
	}
	for _, verb := range []string{"\nDELETE ", "DELETE FROM PRIVACY_"} {
		if strings.Contains(upper, verb) {
			t.Errorf("destructive path in privacy.sql: %q", strings.TrimSpace(verb))
		}
	}
}
