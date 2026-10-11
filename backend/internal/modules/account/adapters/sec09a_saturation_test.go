//go:build integration

package adapters

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	privacyadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

// SEC09-A (current-head review): a stuck auth admission writer holding
// its per-account advisory lock must not deadlock critical privacy/auth
// reads on the shared pool. Reads complete, other-account admission
// proceeds (lock granularity), same-account admission waits (never
// deadlocks) and succeeds after release.
func TestPoolSaturationKeepsPrivacyAuthResponsive(t *testing.T) {
	adminDSN := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if adminDSN == "" {
		adminDSN = "postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("integration database unreachable: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("sec09a_sat_%d", time.Now().UnixNano())
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
		t.Fatal(err)
	}
	u.Path = "/" + name
	if _, err := migrate.Apply(ctx, u.String(), dbmigrations.Files); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	cfg, err := pgxpool.ParseConfig(u.String())
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 2
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	accounts := NewPGStore(pool)
	privacy := privacyadapters.NewStore(pool)
	stuckAcct := "bbbbbbbb-2222-4222-8222-000000000001"
	freeAcct := "bbbbbbbb-2222-4222-8222-000000000002"
	for _, id := range []string{stuckAcct, freeAcct} {
		if _, err := pool.Exec(ctx, `INSERT INTO accounts (id, alias, status) VALUES ($1, $2, 'active')`, id, "sat-"+id[len(id)-4:]); err != nil {
			t.Fatalf("seed account: %v", err)
		}
	}

	// Stuck writer: holds the per-account advisory lock in an open tx.
	holder, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Release()
	tx, err := holder.Conn().Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('families:' || $1))`, stuckAcct); err != nil {
		t.Fatal(err)
	}

	// Critical reads proceed on the shared pool while the lock is held.
	var wg sync.WaitGroup
	errs := make([]error, 4)
	wg.Add(4)
	go func() { defer wg.Done(); _, _, errs[0] = accounts.Family(ctx, "bbbbbbbb-2222-4222-8222-000000000099") }()
	go func() { defer wg.Done(); _, errs[1] = accounts.RefreshHistory(ctx, stuckAcct) }()
	go func() { defer wg.Done(); _, errs[2] = privacy.Get(ctx, "bbbbbbbb-2222-4222-8222-000000000099") }()
	go func() {
		defer wg.Done()
		errs[3] = accounts.TryCreateFamily(ctx, domain.SessionFamily{
			ID: "bbbbbbbb-2222-4222-8222-000000000010", AccountID: freeAcct,
			RefreshSalt: "s", RefreshHash: "h", AccessSalt: "s", AccessHash: "h",
			AccessExpires: 1_800_000_000, IssuedAt: 1_700_000_000,
		}, 1_700_000_000)
	}()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("reads/admission deadlocked behind the stuck writer")
	}
	for i, err := range errs {
		if i == 2 {
			// Privacy owner-read path executed (missing row refuses
			// fail-closed); hanging would have tripped the watchdog.
			if err != nil && err.Error() != "privacy: request not found" {
				t.Fatalf("op %d failed behind stuck writer: %v", i, err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("op %d failed behind stuck writer: %v", i, err)
		}
	}

	// Same-account admission waits (bounded), never deadlocks.
	waitCtx, waitCancel := context.WithTimeout(ctx, 5*time.Second)
	defer waitCancel()
	sameErr := make(chan error, 1)
	go func() {
		err := accounts.TryCreateFamily(waitCtx, domain.SessionFamily{
			ID: "bbbbbbbb-2222-4222-8222-000000000011", AccountID: stuckAcct,
			RefreshSalt: "s", RefreshHash: "h", AccessSalt: "s", AccessHash: "h",
			AccessExpires: 1_800_000_000, IssuedAt: 1_700_000_000,
		}, 1_700_000_000)
		sameErr <- err
	}()
	select {
	case err := <-sameErr:
		if err == nil {
			t.Fatal("same-account admission must wait behind the held lock, not slip through")
		}
		// Bounded ctx error (not deadlock): the lock serializes.
	case <-time.After(30 * time.Second):
		t.Fatal("same-account admission hung without bound")
	}

	// Release: the queued admission succeeds.
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	holder.Release()
	if err := func() error {
		err := accounts.TryCreateFamily(ctx, domain.SessionFamily{
			ID: "bbbbbbbb-2222-4222-8222-000000000011", AccountID: stuckAcct,
			RefreshSalt: "s", RefreshHash: "h", AccessSalt: "s", AccessHash: "h",
			AccessExpires: 1_800_000_000, IssuedAt: 1_700_000_000,
		}, 1_700_000_000)
		return err
	}(); err != nil {
		t.Fatalf("admission after release: %v", err)
	}
}
