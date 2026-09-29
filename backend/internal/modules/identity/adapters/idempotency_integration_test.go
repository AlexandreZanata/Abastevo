//go:build integration

package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

func testDSNIdem(t *testing.T) string {
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

func freshRunner(t *testing.T) (*Runner, *pgxpool.Pool, string) {
	t.Helper()
	adminDSN := testDSNIdem(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("idem_test_%d", time.Now().UnixNano())
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
	contributor := "d6c74c23-63db-4c24-a2e5-408cb23bad26"
	if _, err := pool.Exec(ctx, `INSERT INTO identity_contributors (id) VALUES ($1)`, contributor); err != nil {
		t.Fatalf("contributor: %v", err)
	}
	return NewRunner(pool), pool, contributor
}

func idemKey(contributor string) domain.IdempotencyKey {
	return domain.IdempotencyKey{
		ContributorID: contributor, Method: "POST",
		Route: "/v1/observations", Key: "cmd-1",
	}
}

// jsonEqual compares replayed bodies semantically: PostgreSQL jsonb
// normalizes whitespace, so byte equality with the input cannot hold.
func jsonEqual(t *testing.T, got []byte, want string) bool {
	t.Helper()
	var a, b any
	if err := json.Unmarshal(got, &a); err != nil {
		t.Fatalf("replay not JSON: %v", err)
	}
	if err := json.Unmarshal([]byte(want), &b); err != nil {
		t.Fatal(err)
	}
	return reflect.DeepEqual(a, b)
}

func TestReplaySameKeyBody(t *testing.T) {
	r, _, contributor := freshRunner(t)
	ctx := context.Background()
	var calls atomic.Int64
	fn := func(ctx context.Context, tx pgx.Tx) (int, []byte, error) {
		calls.Add(1)
		return 201, []byte(`{"id":"obs-1"}`), nil
	}
	first, err := r.Do(ctx, idemKey(contributor), []byte(`{"secret-marker-1":true}`), fn)
	if err != nil || first.Replayed || first.StatusCode != 201 {
		t.Fatalf("first = %+v, %v", first, err)
	}
	second, err := r.Do(ctx, idemKey(contributor), []byte(`{"secret-marker-1":true}`), fn)
	if err != nil || !second.Replayed {
		t.Fatalf("second = %+v, %v", second, err)
	}
	if calls.Load() != 1 {
		t.Errorf("executed %d times, want once", calls.Load())
	}
	if !jsonEqual(t, second.Response, `{"id":"obs-1"}`) || second.StatusCode != 201 {
		t.Errorf("replay = %+v", second)
	}
}

func TestChangedBodyConflicts(t *testing.T) {
	r, _, contributor := freshRunner(t)
	ctx := context.Background()
	fn := func(ctx context.Context, tx pgx.Tx) (int, []byte, error) {
		return 201, []byte(`{}`), nil
	}
	if _, err := r.Do(ctx, idemKey(contributor), []byte(`{"a":1}`), fn); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Do(ctx, idemKey(contributor), []byte(`{"a":2}`), fn); err != domain.ErrIdempotentConflict {
		t.Errorf("changed body = %v, want conflict", err)
	}
	// A new key for the same command executes again.
	k2 := idemKey(contributor)
	k2.Key = "cmd-2"
	if _, err := r.Do(ctx, k2, []byte(`{"a":2}`), fn); err != nil {
		t.Errorf("new key = %v", err)
	}
}

func TestFailedAttemptRetriesFresh(t *testing.T) {
	r, _, contributor := freshRunner(t)
	ctx := context.Background()
	var calls atomic.Int64
	flaky := func(ctx context.Context, tx pgx.Tx) (int, []byte, error) {
		if calls.Add(1) == 1 {
			return 0, nil, fmt.Errorf("transient")
		}
		return 201, []byte(`{}`), nil
	}
	if _, err := r.Do(ctx, idemKey(contributor), []byte(`{}`), flaky); err == nil {
		t.Fatal("transient failure accepted")
	}
	out, err := r.Do(ctx, idemKey(contributor), []byte(`{}`), flaky)
	if err != nil || out.Replayed || calls.Load() != 2 {
		t.Errorf("retry = %+v, %v, calls=%d", out, err, calls.Load())
	}
}

func TestConcurrentSameKeyExecutesOnce(t *testing.T) {
	r, _, contributor := freshRunner(t)
	ctx := context.Background()
	var calls atomic.Int64
	fn := func(ctx context.Context, tx pgx.Tx) (int, []byte, error) {
		calls.Add(1)
		time.Sleep(100 * time.Millisecond)
		return 201, []byte(`{"id":"obs-1"}`), nil
	}
	const workers = 8
	outs := make([]Outcome, workers)
	errs := make([]error, workers)
	var wg sync.WaitGroup
	for i := range outs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			outs[i], errs[i] = r.Do(ctx, idemKey(contributor), []byte(`{}`), fn)
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("worker %d: %v", i, err)
		}
		if !jsonEqual(t, outs[i].Response, `{"id":"obs-1"}`) || outs[i].StatusCode != 201 {
			t.Errorf("worker %d = %+v", i, outs[i])
		}
	}
	if calls.Load() != 1 {
		t.Errorf("executed %d times, want once", calls.Load())
	}
}

func TestExpiredReexecutesAndBodyNeverPersisted(t *testing.T) {
	r, pool, contributor := freshRunner(t)
	ctx := context.Background()
	marker := "super-secret-payload-marker"
	if _, err := pool.Exec(ctx, `INSERT INTO identity_idempotency
		(contributor_id, method, route, key, request_hash, response_code, response_json, expires_at)
		VALUES ($1, 'POST', '/v1/observations', 'old', 'hash', 201, '{}', now() - interval '1 day')`,
		contributor); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int64
	fn := func(ctx context.Context, tx pgx.Tx) (int, []byte, error) {
		calls.Add(1)
		return 201, []byte(`{}`), nil
	}
	old := idemKey(contributor)
	old.Key = "old"
	if _, err := r.Do(ctx, old, []byte(`{}`), fn); err != nil {
		t.Fatalf("expired re-execute: %v", err)
	}
	if calls.Load() != 1 {
		t.Errorf("expired record replayed instead of executing")
	}
	sensitive := []byte(`{"data":"` + marker + `"}`)
	if _, err := r.Do(ctx, idemKey(contributor), sensitive, func(ctx context.Context, tx pgx.Tx) (int, []byte, error) {
		return 201, []byte(`{"ok":true}`), nil
	}); err != nil {
		t.Fatal(err)
	}
	var dumped string
	rows, err := pool.Query(ctx, `SELECT contributor_id::text || method || route || key || request_hash || response_code::text || response_json::text FROM identity_idempotency`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		if err := rows.Scan(&dumped); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(dumped, marker) {
			t.Errorf("request body persisted in ledger: %q", dumped)
		}
	}
}
