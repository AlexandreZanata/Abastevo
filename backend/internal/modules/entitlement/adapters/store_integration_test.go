//go:build integration

package adapters

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	qent "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/entitlement"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/entitlement/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func freshDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("explicit disposable PostGIS DSN required")
	}
	u, err := url.Parse(dsn)
	if err != nil || u.Hostname() != "127.0.0.1" {
		t.Fatal("literal loopback test DSN required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal("disposable database unavailable")
	}
	name := fmt.Sprintf("entitlement_test_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		admin.Close(ctx)
		t.Fatal(err)
	}
	admin.Close(ctx)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		admin, err := pgx.Connect(ctx, dsn)
		if err != nil {
			t.Error("cleanup connection failed")
			return
		}
		defer admin.Close(ctx)
		if _, err = admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
			t.Error(err)
		}
	})
	u.Path = "/" + name
	if _, err = migrate.Apply(ctx, u.String(), migrations.Files); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func newID(t *testing.T) string {
	t.Helper()
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	raw := hex.EncodeToString(b[:])
	return raw[0:8] + "-" + raw[8:12] + "-" + raw[12:16] + "-" + raw[16:20] + "-" + raw[20:32]
}

func seedAccount(t *testing.T, pool *pgxpool.Pool, status string) string {
	t.Helper()
	id := newID(t)
	if _, err := pool.Exec(context.Background(), "INSERT INTO accounts(id,alias,status) VALUES($1,$2,$3)", id, "synthetic-"+id, status); err != nil {
		t.Fatal(err)
	}
	return id
}

func insertSession(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	id := newID(t)
	zero := fmt.Sprintf("%064x", 0)
	if _, err := pool.Exec(context.Background(), `INSERT INTO admin_sessions
		(id,token_lookup,token_salt,token_hash,csrf_salt,csrf_hash,key_fingerprint,created_at,last_seen_at,expires_at)
		VALUES($1,$2,'synthetic-salt-01',$3,'synthetic-salt-01',$4,'0123456789ab',clock_timestamp(),clock_timestamp(),clock_timestamp()+interval '1 hour')`,
		id, fmt.Sprintf("%064x", time.Now().UnixNano()), zero, zero); err != nil {
		t.Fatal(err)
	}
	return id
}

func newGrant(t *testing.T, accountID, sessionID string, tier domain.Tier, now time.Time) domain.NewGrant {
	return domain.NewGrant{
		ID: newID(t), AccountID: accountID, Tier: tier, Reason: "synthetic grant reason",
		CreatedSessionID: sessionID, Now: now,
	}
}

// TestGrant_ENT02_IneligibleTarget covers missing, suspended and deleted
// accounts.
func TestGrant_ENT02_IneligibleTarget(t *testing.T) {
	pool := freshDB(t)
	store := NewStore(pool)
	ctx := context.Background()
	for _, status := range []string{"suspended", "deleted"} {
		accountID := seedAccount(t, pool, status)
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.LockEligibleAccount(ctx, tx, accountID); !errors.Is(err, domain.ErrIneligible) {
			t.Fatalf("%s account err=%v", status, err)
		}
		tx.Rollback(ctx)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := store.LockEligibleAccount(ctx, tx, newID(t)); !errors.Is(err, domain.ErrIneligible) {
		t.Fatalf("missing account err=%v", err)
	}
}

// TestGrant_ENT03_ConcurrentDuplicateActive creates the same tier from two
// transactions; the partial unique index admits exactly one.
func TestGrant_ENT03_ConcurrentDuplicateActive(t *testing.T) {
	pool := freshDB(t)
	store := NewStore(pool)
	accountID := seedAccount(t, pool, "active")
	now := time.Now().UTC().Truncate(time.Microsecond)
	sessionID := insertSession(t, pool)
	grants := []domain.NewGrant{
		newGrant(t, accountID, sessionID, domain.TierPRO, now),
		newGrant(t, accountID, sessionID, domain.TierPRO, now),
	}
	start := make(chan struct{})
	type outcome struct {
		err error
	}
	results := make(chan outcome, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tx, err := pool.Begin(context.Background())
			if err != nil {
				results <- outcome{err}
				return
			}
			defer tx.Rollback(context.Background())
			<-start
			_, err = store.CreateTx(context.Background(), tx, grants[i])
			if err == nil {
				err = tx.Commit(context.Background())
			}
			results <- outcome{err}
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)
	success, exists := 0, 0
	for result := range results {
		switch {
		case result.err == nil:
			success++
		case errors.Is(result.err, domain.ErrExists):
			exists++
		default:
			t.Fatalf("unexpected err=%v", result.err)
		}
	}
	if success != 1 || exists != 1 {
		t.Fatalf("success=%d exists=%d", success, exists)
	}
}

// TestGrant_ENT03_ReplaceRollbackKeepsOld proves revoke+create atomicity: a
// failed replacement leaves the original grant active.
func TestGrant_ENT03_ReplaceRollbackKeepsOld(t *testing.T) {
	pool := freshDB(t)
	store := NewStore(pool)
	accountID := seedAccount(t, pool, "active")
	now := time.Now().UTC().Truncate(time.Microsecond)
	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	old, err := store.CreateTx(context.Background(), tx, newGrant(t, accountID, insertSession(t, pool), domain.TierPRO, now))
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}

	tx, err = pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.RevokeTx(context.Background(), tx, old.ID, now, "synthetic replacement reason"); err != nil {
		t.Fatal(err)
	}
	bad := newGrant(t, accountID, insertSession(t, pool), domain.TierPRO, now)
	tooSoon := now.Add(time.Minute)
	bad.EndAt = &tooSoon
	if _, err := store.CreateTx(context.Background(), tx, bad); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("invalid replacement err=%v", err)
	}
	tx.Rollback(context.Background())

	var state string
	if err := pool.QueryRow(context.Background(), "SELECT state FROM entitlement_grants WHERE id=$1", old.ID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != string(domain.StateActive) {
		t.Fatalf("old grant state=%s", state)
	}
}

// TestGrant_ENT05_NoCapabilityColumn proves the table stores no capability
// array.
func TestGrant_ENT05_NoCapabilityColumn(t *testing.T) {
	pool := freshDB(t)
	var exists bool
	if err := pool.QueryRow(context.Background(), `SELECT EXISTS (
		SELECT 1 FROM information_schema.columns
		WHERE table_name='entitlement_grants' AND column_name='capabilities')`).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("capabilities column exists")
	}
}

// TestGrant_ENT11_ErasureRevokes covers the account-deletion trigger.
func TestGrant_ENT11_ErasureRevokes(t *testing.T) {
	pool := freshDB(t)
	store := NewStore(pool)
	accountID := seedAccount(t, pool, "active")
	now := time.Now().UTC().Truncate(time.Microsecond)
	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	grant, err := store.CreateTx(context.Background(), tx, newGrant(t, accountID, insertSession(t, pool), domain.TierPRO, now))
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), "UPDATE accounts SET status='deleted' WHERE id=$1", accountID); err != nil {
		t.Fatal(err)
	}
	row, err := qent.New(pool).GetGrant(context.Background(), uid(grant.ID))
	if err != nil {
		t.Fatal(err)
	}
	if row.State != string(domain.StateRevoked) || !row.RevokeReason.Valid || row.RevokeReason.String != "account-erased" {
		t.Fatalf("grant=%+v", row)
	}
}
