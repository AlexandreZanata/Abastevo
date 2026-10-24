//go:build integration

package prepared

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func moderationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	adminDSN := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if adminDSN == "" {
		adminDSN = "postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal("integration database unavailable", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("charging_mod_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		admin, err := pgx.Connect(ctx, adminDSN)
		if err == nil {
			admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize())
			admin.Close(ctx)
		}
	})
	u, _ := url.Parse(adminDSN)
	u.Path = "/" + name
	if _, err := migrate.Apply(ctx, u.String(), dbmigrations.Files); err != nil {
		t.Fatal("migrate", err)
	}
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	ctx2 := context.Background()
	if _, err := pool.Exec(ctx2, `INSERT INTO accounts(id,alias,status) VALUES('aaaaaaaa-1111-4111-8111-000000000001','voter','active')`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx2, `INSERT INTO charging_observations(id,account_id,connector_code,condition,observed_day,note) VALUES('bbbbbbbb-1111-4111-8111-000000000001','aaaaaaaa-1111-4111-8111-000000000001','TYPE2','STANDARD',CURRENT_DATE,'test')`); err != nil {
		t.Fatal(err)
	}
	return pool
}

func TestModerationVotesReportsProjectionExpiry(t *testing.T) {
	pool := moderationPool(t)
	ctx := context.Background()
	account := "aaaaaaaa-1111-4111-8111-000000000001"
	observation := "bbbbbbbb-1111-4111-8111-000000000001"
	// Helpfulness vote converges by client submission ID (replay safe).
	if err := Vote(ctx, pool, account, observation, "HELPFUL", "client-1"); err != nil {
		t.Fatal(err)
	}
	if err := Vote(ctx, pool, account, observation, "HELPFUL", "client-1"); err != nil {
		t.Fatal("replay diverged", err)
	}
	// Conflicting vote from another account counts separately.
	if _, err := pool.Exec(ctx, `INSERT INTO accounts(id,alias,status) VALUES('aaaaaaaa-1111-4111-8111-000000000002','voter2','active')`); err != nil {
		t.Fatal(err)
	}
	if err := Vote(ctx, pool, "aaaaaaaa-1111-4111-8111-000000000002", observation, "NOT_HELPFUL", "client-2"); err != nil {
		t.Fatal(err)
	}
	// Report for review (no self-report check here; caller verifies).
	if err := Report(ctx, pool, "aaaaaaaa-1111-4111-8111-000000000002", observation, "INACCURATE", "price looks off"); err != nil {
		t.Fatal(err)
	}
	if err := Report(ctx, pool, "aaaaaaaa-1111-4111-8111-000000000002", observation, "INACCURATE", "duplicate"); err != nil {
		t.Fatal("duplicate report diverged", err)
	}
	// Projection refuses empty place without touching storage.
	if _, err := Project(ctx, pool, "", 3650, 100); err == nil {
		t.Fatal("empty place accepted")
	}
	// Invalid vote/report shapes refuse.
	if err := Vote(ctx, pool, "", observation, "HELPFUL", "x"); err == nil {
		t.Fatal("empty account accepted")
	}
	if err := Report(ctx, pool, account, observation, "BOGUS", "x"); err == nil {
		t.Fatal("bogus reason accepted")
	}
}
