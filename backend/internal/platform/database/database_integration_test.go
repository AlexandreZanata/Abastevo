//go:build integration

package database

import (
	"context"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	platformqueries "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/platform"
)

func testDSN() string {
	if dsn := os.Getenv("ANPFUEL_TEST_DATABASE_URL"); dsn != "" {
		return dsn
	}
	return "postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable"
}

// blackhole accepts TCP connections and never speaks Postgres.
func blackhole(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				<-t.Context().Done()
				conn.Close()
			}()
		}
	}()
	return ln.Addr().String()
}

func TestPingTimesOutAgainstBlackhole(t *testing.T) {
	opts := DefaultOptions()
	opts.MaxConns = 1
	opts.MinConns = 0
	opts.ConnectTimeout = 300 * time.Millisecond
	pool, err := Open(context.Background(), "postgres://anpfuel:anpfuel@"+blackhole(t)+"/anpfuel?sslmode=disable", opts)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := time.Now()
	if err := pool.Ping(ctx); err == nil {
		t.Fatal("expected ping timeout, got nil")
	}
	if elapsed := time.Since(start); elapsed > 4*time.Second {
		t.Fatalf("ping took %v, connect timeout not enforced", elapsed)
	}
}

func TestPingCancelledContext(t *testing.T) {
	pool, err := Open(context.Background(), "postgres://anpfuel:anpfuel@"+blackhole(t)+"/anpfuel?sslmode=disable", DefaultOptions())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pool.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := pool.Ping(ctx); err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

func TestClosedPoolFailsOperations(t *testing.T) {
	pool, err := Open(context.Background(), testDSN(), DefaultOptions())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	pool.Close()
	if err := pool.Ping(context.Background()); err == nil {
		t.Fatal("expected error after Close, got nil")
	} else if !strings.Contains(strings.ToLower(err.Error()), "closed") {
		t.Fatalf("error %q should report the closed pool", err)
	}
}

func TestGeneratedQuerySmoke(t *testing.T) {
	pool, err := Open(context.Background(), testDSN(), DefaultOptions())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	version, err := platformqueries.New(pool.Underlying()).PostGISVersion(ctx)
	if err != nil {
		t.Fatalf("PostGISVersion: %v", err)
	}
	if version == "" {
		t.Fatal("PostGISVersion empty")
	}
	t.Logf("PostGIS %s", version)
}
