//go:build integration

package dou

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	directory "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/directory"
	parent "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters/read"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters/registry"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

func uuidv4(t *testing.T) string {
	t.Helper()
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatalf("rand: %v", err)
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	hexed := hex.EncodeToString(b[:])
	return hexed[0:8] + "-" + hexed[8:12] + "-" + hexed[12:16] + "-" +
		hexed[16:20] + "-" + hexed[20:32]
}

func douTestDSN(t *testing.T) string {
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

func freshDouStore(t *testing.T) (*registry.PGStore, *pgxpool.Pool, parent.RegistryCanonicalizer) {
	t.Helper()
	adminDSN := douTestDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("dou_test_%d", time.Now().UnixNano())
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
	canon := parent.RegistryCanonicalizer{Repo: parent.NewRepository(pool)}
	return store, pool, canon
}

func stageEditionActs(t *testing.T, store *registry.PGStore, snapshot, date string, acts []Act) {
	t.Helper()
	ctx := context.Background()
	edition := Edition{Date: date, Number: "190", Checksum: "ed-" + snapshot}
	assertions, skipped := StageActs(edition, acts)
	if skipped != 0 {
		t.Fatalf("skipped = %d", skipped)
	}
	runID, created, err := store.CreateRun(ctx, uuidv4(t), SourceActs, snapshot, "ed-"+snapshot)
	if err != nil || !created {
		t.Fatalf("create run: %v %v", runID, err)
	}
	for _, a := range assertions {
		if _, err := store.StageAssertion(ctx, a.WithRun(runID)); err != nil {
			t.Fatalf("stage: %v", err)
		}
	}
	if err := store.FinishRun(ctx, runID, "complete", int64(len(assertions)), 0, 0, ""); err != nil {
		t.Fatalf("finish: %v", err)
	}
}

func TestDouActsIntegrateChronologyAndRevocation(t *testing.T) {
	store, pool, canon := freshDouStore(t)
	ctx := context.Background()

	grant := Act{ID: "ANP-2026-0001", Text: "autoriza [P26-TEST] POSTO EPSILON LTDA, CNPJ 55881177000136"}
	stageEditionActs(t, store, "ed-1", "2026-10-01", []Act{grant})
	first, err := registry.ReconcileRun(ctx, store, canon, SourceActs, "ed-1")
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if first.Reconciled != 1 {
		t.Fatalf("report = %+v", first)
	}
	reader := read.NewReader(pool)
	epson, err := reader.Detail(ctx, stationIDForCNPJ(t, pool, "55881177000136"))
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if epson.DisplayName == "" {
		t.Fatal("detail without display name")
	}

	// Republication of the revocation in a later edition revokes the
	// same station; the grant date never became an opening date (no
	// such output exists on assertions or stations).
	revoke := Act{ID: "ANP-2026-0002", Text: "revoga a autorização de [P26-TEST] POSTO EPSILON LTDA, CNPJ 55881177000136."}
	stageEditionActs(t, store, "ed-2", "2026-10-05", []Act{revoke})
	second, err := registry.ReconcileRun(ctx, store, canon, SourceActs, "ed-2")
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if second.Reconciled != 1 {
		t.Fatalf("report = %+v", second)
	}
	after, err := reader.Detail(ctx, stationIDForCNPJ(t, pool, "55881177000136"))
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if after.ID != epson.ID {
		t.Fatal("revocation created a second identity")
	}
	var status string
	if err := pool.QueryRow(ctx, "SELECT status FROM directory_stations WHERE id = $1", epson.ID).Scan(&status); err != nil {
		t.Fatalf("status: %v", err)
	}
	if status != "revoked" {
		t.Fatalf("status = %q, want revoked", status)
	}
}

func stationIDForCNPJ(t *testing.T, pool *pgxpool.Pool, cnpj string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(),
		"SELECT s.id::text FROM directory_identifiers i JOIN directory_stations s ON s.id = i.station_id WHERE i.kind = 'CNPJ' AND i.normalized_value = $1 AND i.valid_to IS NULL",
		cnpj).Scan(&id); err != nil {
		t.Fatalf("resolve %s: %v", cnpj, err)
	}
	return id
}
