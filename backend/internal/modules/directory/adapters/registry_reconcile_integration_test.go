//go:build integration

package adapters

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	directory "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/directory"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters/read"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters/registry"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

func registryTestDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("integration database unreachable: %v", err)
	}
	defer conn.Close(ctx)
	return dsn
}

// freshRegistryDB migrates an empty disposable database (proving the
// append-only registry migrations on a previous schema) and returns the
// pool plus staging store and canonicalizer on it.
func freshRegistryDB(t *testing.T) (*pgxpool.Pool, *registry.PGStore, RegistryCanonicalizer) {
	t.Helper()
	adminDSN := registryTestDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("recon_test_%d", time.Now().UnixNano())
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
	store := &registry.PGStore{Q: directory.New(pool)}
	return pool, store, RegistryCanonicalizer{Repo: &Repository{pool: pool}}
}

const reconCSV = `CNPJ;RAZAO_SOCIAL;COD_IBGE;UF;SITUACAO;ATO_AUTORIZACAO
04218406000104;[P25-TEST] ALFA;3550308;SP;ATIVA;PRC-1
12ABC34501DE35;[P25-TEST] GAMA;3550308;SP;DESCONHECIDA;
`

func TestReconcileIntegrationStableUUIDsAndZeroPriceReads(t *testing.T) {
	pool, store, canon := freshRegistryDB(t)
	ctx := context.Background()
	limits := registry.Limits{MaxBytes: 1 << 20, MaxRows: 100, BatchSize: 10}

	staged, err := registry.StageCSV(ctx, store, "recon-e2e", strings.NewReader(reconCSV), limits)
	if err != nil || staged.State != "complete" {
		t.Fatalf("stage = %+v, err = %v", staged, err)
	}
	report, err := registry.ReconcileRun(ctx, store, canon, registry.SourceCSV, "recon-e2e")
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if report.Reconciled != 2 || report.Skipped != 0 {
		t.Fatalf("report = %+v", report)
	}

	reader := read.NewReader(pool)
	// Zero-price station is searchable anonymously (no price rows exist).
	found, _, err := reader.Search(ctx, application.SearchFilter{Q: "ALFA", Limit: 10})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("search results = %d, want 1", len(found))
	}
	// ... and detail-readable with honest unknown location.
	detail, err := reader.Detail(ctx, found[0].ID)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if detail.DisplayName == "" {
		t.Fatal("detail without display name")
	}
	// Reconciling again converges on the same UUIDs (stable identity).
	again, err := registry.ReconcileRun(ctx, store, canon, registry.SourceCSV, "recon-e2e")
	if err != nil {
		t.Fatalf("re-reconcile: %v", err)
	}
	if again.Reconciled != 2 {
		t.Fatalf("report = %+v", again)
	}
	foundAgain, _, err := reader.Search(ctx, application.SearchFilter{Q: "ALFA", Limit: 10})
	if err != nil || len(foundAgain) != 1 || foundAgain[0].ID != found[0].ID {
		t.Fatalf("identity unstable: %+v, err = %v", foundAgain, err)
	}
	// Active identifier count is exactly the two asserted CNPJs.
	var activeIDs int64
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM directory_identifiers WHERE valid_to IS NULL").Scan(&activeIDs); err != nil {
		t.Fatalf("count identifiers: %v", err)
	}
	if activeIDs != 2 {
		t.Fatalf("active identifiers = %d, want 2", activeIDs)
	}
}

func TestReconcileIntegrationConcurrentFirstCreation(t *testing.T) {
	pool, store, canon := freshRegistryDB(t)
	ctx := context.Background()
	limits := registry.Limits{MaxBytes: 1 << 20, MaxRows: 100, BatchSize: 10}

	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			snap := fmt.Sprintf("recon-race-%d", i)
			if _, err := registry.StageCSV(ctx, store, snap, strings.NewReader(reconCSV), limits); err != nil {
				errs[i] = err
				return
			}
			_, errs[i] = registry.ReconcileRun(ctx, store, canon, registry.SourceCSV, snap)
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("race %d: %v", i, err)
		}
	}
	// Four snapshots × same two CNPJs converge on exactly two stations:
	// no duplicate identities, no address-only merge.
	var stations int64
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM directory_stations").Scan(&stations); err != nil {
		t.Fatalf("count stations: %v", err)
	}
	if stations != 2 {
		t.Fatalf("stations = %d, want 2", stations)
	}
	var activeIDs int64
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM directory_identifiers WHERE valid_to IS NULL").Scan(&activeIDs); err != nil {
		t.Fatalf("count identifiers: %v", err)
	}
	if activeIDs != 2 {
		t.Fatalf("active identifiers = %d, want 2", activeIDs)
	}
}
