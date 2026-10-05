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
	stationprofile "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/stationprofile"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/stationprofile/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

func freshProfileDB(t *testing.T) (*pgxpool.Pool, Store) {
	t.Helper()
	adminDSN := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if adminDSN == "" {
		adminDSN = "postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("profile_test_%d", time.Now().UnixNano())
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
	parsed, err := url.Parse(adminDSN)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	parsed.Path = "/" + name
	dsn := parsed.String()
	if _, err := migrate.Apply(ctx, dsn, dbmigrations.Files); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool, Store{Q: stationprofile.New(pool)}
}

func seedStation(t *testing.T, pool *pgxpool.Pool, display string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(),
		"INSERT INTO directory_stations (id, display_name) VALUES (gen_random_uuid(), $1) RETURNING id::text", display).Scan(&id); err != nil {
		t.Fatalf("seed station: %v", err)
	}
	return id
}

func TestProfileIntegrationUnclaimedAndOperatorChain(t *testing.T) {
	pool, store := freshProfileDB(t)
	ctx := context.Background()
	stationID := seedStation(t, pool, "Posto Perfil")

	if err := application.EnsureUnclaimed(ctx, store, stationID); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if err := application.EnsureUnclaimed(ctx, store, stationID); err != nil {
		t.Fatalf("replay: %v", err)
	}
	n := 0
	newID := func() string {
		n++
		return fmt.Sprintf("00000000-0000-4000-8000-%012d", n)
	}
	if err := application.LinkOperator(ctx, store, newID, stationID, "04218406000104", "registry", "PRC-1"); err != nil {
		t.Fatalf("link: %v", err)
	}
	if err := application.LinkOperator(ctx, store, newID, stationID, "00428184000195", "dou", "ANP-9"); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	current, found, err := store.CurrentOperator(ctx, stationID)
	if err != nil || !found || current.CNPJ != "00428184000195" {
		t.Fatalf("current = %+v, %v, %v", current, found, err)
	}
	var open int64
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM station_operator_revisions WHERE station_id = $1 AND valid_to IS NULL", stationID).Scan(&open); err != nil {
		t.Fatalf("count open: %v", err)
	}
	if open != 1 {
		t.Fatalf("open revisions = %d, want exactly 1", open)
	}
	profile, err := application.ReadProfile(ctx, store, func(context.Context, string) (string, string, *float64, *float64, error) {
		return "Posto Perfil", "unknown", nil, nil, nil
	}, stationID)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if profile.HasBadge || profile.OperatorCNPJ != "00428184000195" || profile.Latitude != nil {
		t.Fatalf("profile = %+v", profile)
	}
}

func TestProfileIntegrationConcurrentLinksConverge(t *testing.T) {
	pool, store := freshProfileDB(t)
	ctx := context.Background()
	stationID := seedStation(t, pool, "Posto Raca")

	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			newID := func() string { return fmt.Sprintf("10000000-0000-4000-8000-%012d", i) }
			errs[i] = application.LinkOperator(ctx, store, newID, stationID, "04218406000104", "registry", "PRC-1")
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("race %d: %v", i, err)
		}
	}
	// Exactly one open revision despite the race: no forked history.
	var open int64
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM station_operator_revisions WHERE station_id = $1 AND valid_to IS NULL", stationID).Scan(&open); err != nil {
		t.Fatalf("count open: %v", err)
	}
	if open != 1 {
		t.Fatalf("open revisions = %d, want exactly 1", open)
	}
}
