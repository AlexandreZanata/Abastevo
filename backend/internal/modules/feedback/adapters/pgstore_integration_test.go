//go:build integration

package adapters

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/feedback/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

type integClock struct {
	mu  sync.Mutex
	now int64
}

func (c *integClock) NowUnix() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func testUUID(n int) string {
	return fmt.Sprintf("aaaaaaaa-1111-4111-8111-%012d", n)
}

func freshService(t *testing.T) (*application.Service, *pgxpool.Pool) {
	t.Helper()
	adminDSN := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if adminDSN == "" {
		adminDSN = "postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("integration database unreachable at %s: %v (start it: docker compose -f infra/compose.dev.yml up -d db)", adminDSN, err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("feedback_test_%d", time.Now().UnixNano())
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
	applied, err := migrate.Apply(ctx, dsn, dbmigrations.Files)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	found := false
	for _, v := range applied {
		if v == "000023" {
			found = true
		}
	}
	if !found {
		t.Fatalf("feedback migration missing from %v", applied)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)

	if _, err := pool.Exec(ctx,
		`INSERT INTO directory_stations (id, display_name) VALUES ($1, 'Posto Teste')`, testUUID(9001)); err != nil {
		t.Fatalf("seed station: %v", err)
	}
	clock := &integClock{now: 1_700_000_000}
	var mu sync.Mutex
	gi := 0
	svc := &application.Service{
		Clock: clock,
		Store: NewPGStore(pool),
		CheckAccount: func(ctx context.Context, accountID string) error {
			var status string
			err := pool.QueryRow(ctx, `SELECT status FROM accounts WHERE id = $1`, accountID).Scan(&status)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return errors.New("feedback: unknown account")
				}
				return err
			}
			// The accounts table owns lifecycle vocabulary; here only
			// "active" writes (test-local mirror of the T04 rule).
			if status != "active" {
				return errors.New("feedback: account not active")
			}
			return nil
		},
		StationExists: func(ctx context.Context, stationID string) (bool, error) {
			var ok bool
			err := pool.QueryRow(ctx, `SELECT true FROM directory_stations WHERE id = $1`, stationID).Scan(&ok)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return false, nil
				}
				return false, err
			}
			return ok, nil
		},
		IDGen: func() (string, error) {
			mu.Lock()
			defer mu.Unlock()
			gi++
			return testUUID(gi), nil
		},
	}
	return svc, pool
}

func seedAccount(t *testing.T, pool *pgxpool.Pool, n int, status string) string {
	t.Helper()
	id := testUUID(8000 + n)
	alias := fmt.Sprintf("alias-%d", n)
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO accounts (id, alias, status) VALUES ($1, $2, $3)`, id, alias, status); err != nil {
		t.Fatalf("seed account: %v", err)
	}
	return id
}

func TestPGConcurrentRateConverges(t *testing.T) {
	svc, pool := freshService(t)
	ctx := context.Background()
	acc := seedAccount(t, pool, 1, "active")
	station := testUUID(9001)

	const racers = 16
	var wg sync.WaitGroup
	results := make([]application.RateResult, racers)
	errs := make([]error, racers)
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = svc.Rate(ctx, acc, station, "GASOLINE_REGULAR", 5)
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("concurrent rate: %v", err)
		}
	}
	first := results[0].Rating.ID
	created := 0
	for _, r := range results {
		if r.Rating.ID != first {
			t.Fatalf("concurrent rates must converge on one row, got %v vs %v", r.Rating.ID, first)
		}
		if r.Created {
			created++
		}
		if r.Stats.Count != 1 || r.Stats.Sum != 5 {
			t.Fatalf("counts must not inflate: %+v", r.Stats)
		}
	}
	if created != 1 {
		t.Errorf("exactly one racer must create, got %d", created)
	}
}

func TestPGConcurrentMultiAccountRatingsExact(t *testing.T) {
	svc, pool := freshService(t)
	ctx := context.Background()
	station := testUUID(9001)

	const raters = 8
	accounts := make([]string, raters)
	for i := range accounts {
		accounts[i] = seedAccount(t, pool, 30+i, "active")
	}
	var wg sync.WaitGroup
	errs := make([]error, raters)
	for i := 0; i < raters; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = svc.Rate(ctx, accounts[i], station, "GASOLINE_REGULAR", 1+(i%5))
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("concurrent rate: %v", err)
		}
	}
	stats, found, err := svc.Store.Stats(ctx, station, "GASOLINE_REGULAR")
	if err != nil || !found {
		t.Fatalf("stats must persist: %+v %v %v", stats, found, err)
	}
	// Stars cycle 1..5,1,2,3 over 8 raters: sum = 1+2+3+4+5+1+2+3.
	if stats.Count != raters || stats.Sum != 21 {
		t.Errorf("concurrent counts must not inflate or drop: %+v", stats)
	}
	rebuilt, err := svc.RebuildStats(ctx, station, "GASOLINE_REGULAR")
	if err != nil {
		t.Fatal(err)
	}
	if rebuilt != stats {
		t.Errorf("rebuild must match hot stats: %+v vs %+v", rebuilt, stats)
	}
}

func TestPGEditDeleteAndRebuild(t *testing.T) {
	svc, pool := freshService(t)
	ctx := context.Background()
	acc1 := seedAccount(t, pool, 1, "active")
	acc2 := seedAccount(t, pool, 2, "active")
	station := testUUID(9001)

	if _, err := svc.Rate(ctx, acc1, station, "GASOLINE_REGULAR", 5); err != nil {
		t.Fatal(err)
	}
	edited, err := svc.Rate(ctx, acc1, station, "GASOLINE_REGULAR", 3)
	if err != nil {
		t.Fatal(err)
	}
	if edited.Created || edited.Rating.Revision != 2 {
		t.Errorf("edit must bump revision, got %+v", edited.Rating)
	}
	if _, err := svc.Rate(ctx, acc2, station, "GASOLINE_REGULAR", 4); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteRating(ctx, acc1, station, "GASOLINE_REGULAR"); err != nil {
		t.Fatal(err)
	}
	stats, found, err := svc.Store.Stats(ctx, station, "GASOLINE_REGULAR")
	if err != nil || !found {
		t.Fatalf("stats must persist: %+v %v %v", stats, found, err)
	}
	if stats.Count != 1 || stats.Sum != 4 {
		t.Errorf("tombstoned rating must leave stats, got %+v", stats)
	}
	rebuilt, err := svc.RebuildStats(ctx, station, "GASOLINE_REGULAR")
	if err != nil {
		t.Fatal(err)
	}
	if rebuilt.Count != 1 || rebuilt.Sum != 4 {
		t.Errorf("rebuild must recover 1/4, got %+v", rebuilt)
	}
	if mean, ok := rebuilt.MeanMilli(); !ok || mean != 4000 {
		t.Errorf("mean must be 4000 milli, got %d %v", mean, ok)
	}
}

func TestPGSuspendedAndWrongTarget(t *testing.T) {
	svc, pool := freshService(t)
	ctx := context.Background()
	susp := seedAccount(t, pool, 3, "suspended")
	station := testUUID(9001)

	if _, err := svc.Rate(ctx, susp, station, "GASOLINE_REGULAR", 5); err == nil {
		t.Error("suspended session must refuse")
	}
	if _, err := svc.Rate(ctx, testUUID(111), station, "GASOLINE_REGULAR", 5); err == nil {
		t.Error("unknown account must refuse")
	}
	if _, err := svc.Rate(ctx, seedAccount(t, pool, 4, "active"), testUUID(9999), "GASOLINE_REGULAR", 5); err == nil {
		t.Error("unknown station must refuse")
	}
}
