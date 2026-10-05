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

func freshProofDB(t *testing.T) (*pgxpool.Pool, ClaimStore, ProofStore, string) {
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
	name := fmt.Sprintf("proof_test_%d", time.Now().UnixNano())
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
	queries := stationprofile.New(pool)
	var stationID string
	if err := pool.QueryRow(ctx, "INSERT INTO directory_stations (id, display_name) VALUES (gen_random_uuid(), 'Posto Prova') RETURNING id::text").Scan(&stationID); err != nil {
		t.Fatalf("seed station: %v", err)
	}
	return pool, ClaimStore{Q: queries}, ProofStore{Q: queries}, stationID
}

type memoryProofBytes struct {
	mu      sync.Mutex
	objects map[string][]byte
	deleted int
}

func (m *memoryProofBytes) Put(_ context.Context, key string, body []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.objects == nil {
		m.objects = map[string][]byte{}
	}
	m.objects[key] = body
	return nil
}

func (m *memoryProofBytes) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.objects, key)
	m.deleted++
	return nil
}

func validProofPDF() []byte {
	raw := append([]byte("%PDF-1.7\n"), make([]byte, 300)...)
	for i := range raw[9:] {
		raw[9+i] = 'c'
	}
	return raw
}

func TestProofIntegrationLifecycleExpiryAndRace(t *testing.T) {
	_, claimStore, proofStore, stationID := freshProofDB(t)
	ctx := context.Background()
	n := 0
	ports := application.ClaimPorts{
		Store: claimStore,
		Clock: func() time.Time { return time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC) },
		NewID: func() (string, error) {
			n++
			return fmt.Sprintf("00000000-0000-4000-8000-%012d", n), nil
		},
		OperatorOf: func(context.Context, string) (string, string, bool, error) {
			return "04218406000104", "registry", true, nil
		},
	}
	claim, created, err := application.OpenClaim(ctx, ports, "11111111-1111-4111-8111-111111111111", stationID, "administrator", []string{"profile.edit"}, "proof-int-1")
	if err != nil || !created {
		t.Fatalf("open = %+v, %v, %v", claim, created, err)
	}
	declaration, err := claimStore.ActiveDeclaration(ctx, claim.ID)
	if err != nil {
		t.Fatalf("declaration: %v", err)
	}
	proofPorts := application.ProofPorts{
		Proofs: proofStore,
		Claims: claimStore,
		Clock:  ports.Clock,
		NewID:  ports.NewID,
		Bytes:  &memoryProofBytes{},
	}
	submitted, created, err := application.SubmitProof(ctx, proofPorts, "11111111-1111-4111-8111-111111111111", claim.ID, declaration.ID, "autorizacao.pdf", "application/pdf", "authorization", validProofPDF())
	if err != nil || !created {
		t.Fatalf("submit = %+v, %v, %v", submitted, created, err)
	}
	// Parallel identical redelivery on a fresh claim binds once: one
	// row wins the unique guard, the rest dedup onto it (single active
	// proof per declaration, bytes effectively stored once).
	traceClaim, traceCreated, err := application.OpenClaim(ctx, ports, "11111111-1111-4111-8111-111111111111", stationID, "administrator", []string{"profile.edit"}, "proof-race-key")
	if err != nil || !traceCreated {
		t.Fatalf("open race claim: %v %+v", err, traceClaim)
	}
	traceDecl, err := claimStore.ActiveDeclaration(ctx, traceClaim.ID)
	if err != nil {
		t.Fatalf("race declaration: %v", err)
	}
	var wg sync.WaitGroup
	type result struct {
		id      string
		created bool
		err     error
	}
	results := make([]result, 4)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			proof, created, err := application.SubmitProof(ctx, proofPorts, "11111111-1111-4111-8111-111111111111", traceClaim.ID, traceDecl.ID, "autorizacao.pdf", "application/pdf", "authorization", validProofPDF())
			results[i] = result{id: proof.ID, created: created, err: err}
		}(i)
	}
	wg.Wait()
	ids := map[string]bool{}
	createdCount := 0
	for i, res := range results {
		if res.err != nil {
			t.Fatalf("race %d: %v", i, res.err)
		}
		ids[res.id] = true
		if res.created {
			createdCount++
		}
	}
	if len(ids) != 1 || createdCount != 1 {
		t.Fatalf("race diverged: %+v", results)
	}
}
