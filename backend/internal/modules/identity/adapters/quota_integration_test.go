//go:build integration

package adapters

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
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

func testDSNQuota(t *testing.T) string {
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

func freshLimiter(t *testing.T, policy domain.QuotaPolicy) (*Limiter, *pgxpool.Pool) {
	t.Helper()
	adminDSN := testDSNQuota(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("quota_test_%d", time.Now().UnixNano())
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
	return NewLimiter(pool, policy), pool
}

func tinyPolicy() domain.QuotaPolicy {
	return domain.QuotaPolicy{
		Version: 1,
		Ops: map[string]domain.OperationQuota{
			domain.OperationWrite:     {Limit: 3, Window: time.Hour},
			domain.OperationRegister:  {Limit: 2, Window: time.Hour},
			domain.OperationChallenge: {Limit: 2, Window: time.Hour},
		},
	}
}

func TestConcurrentQuotaBoundary(t *testing.T) {
	lim, _ := freshLimiter(t, tinyPolicy())
	ctx := context.Background()
	const workers = 12
	var allowed atomic.Int64
	var denied atomic.Int64
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := lim.Check(ctx, "fp:subject", domain.OperationWrite)
			if err == nil {
				allowed.Add(1)
				return
			}
			var qerr *QuotaError
			if errors.As(err, &qerr) {
				denied.Add(1)
				return
			}
			t.Errorf("unexpected: %v", err)
		}()
	}
	wg.Wait()
	if allowed.Load() != 3 || denied.Load() != 9 {
		t.Errorf("allowed=%d denied=%d, want exactly 3/9", allowed.Load(), denied.Load())
	}
}

func TestNATBindingAndIsolation(t *testing.T) {
	lim, _ := freshLimiter(t, tinyPolicy())
	ctx := context.Background()
	key := []byte("0123456789abcdef0123456789abcdef")
	ipA, err := domain.HashIPSubject("k1", key, "192.0.2.10")
	if err != nil {
		t.Fatal(err)
	}
	ipB, err := domain.HashIPSubject("k1", key, "192.0.2.11")
	if err != nil {
		t.Fatal(err)
	}
	// One IP digest binds every contributor behind that NAT address: three
	// calls exhaust the shared budget however many users share it.
	for i := 0; i < 3; i++ {
		if _, err := lim.Check(ctx, ipA, domain.OperationWrite); err != nil {
			t.Fatalf("shared budget call %d: %v", i, err)
		}
	}
	if _, err := lim.Check(ctx, ipA, domain.OperationWrite); err == nil {
		t.Error("NAT budget not bound")
	}
	// Another address keeps its own budget.
	if _, err := lim.Check(ctx, ipB, domain.OperationWrite); err != nil {
		t.Errorf("isolated subject denied: %v", err)
	}
	// Operations are isolated too.
	if _, err := lim.Check(ctx, ipA, domain.OperationRegister); err != nil {
		t.Errorf("operation isolation broken: %v", err)
	}
}

func TestRetryAfterAndCleanup(t *testing.T) {
	lim, pool := freshLimiter(t, tinyPolicy())
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if _, err := lim.Check(ctx, "fp:reg", domain.OperationRegister); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	_, err := lim.Check(ctx, "fp:reg", domain.OperationRegister)
	var qerr *QuotaError
	if !errors.As(err, &qerr) {
		t.Fatalf("denial = %v, want QuotaError", err)
	}
	if qerr.RetryAfter <= 0 || qerr.RetryAfter > time.Hour {
		t.Errorf("retry-after = %v", qerr.RetryAfter)
	}
	// Expired windows clean up and the budget returns.
	if _, err := pool.Exec(ctx, `UPDATE identity_rate_windows SET expires_at = now() - interval '1 hour'`); err != nil {
		t.Fatal(err)
	}
	n, err := lim.Cleanup(ctx)
	if err != nil || n != 1 {
		t.Fatalf("cleanup = %d, %v", n, err)
	}
	if _, err := lim.Check(ctx, "fp:reg", domain.OperationRegister); err != nil {
		t.Errorf("post-cleanup denied: %v", err)
	}
}

func TestUnknownOperationRefused(t *testing.T) {
	lim, _ := freshLimiter(t, tinyPolicy())
	if _, err := lim.Check(context.Background(), "fp:x", "nope"); err == nil {
		t.Error("unknown operation allowed")
	}
}
