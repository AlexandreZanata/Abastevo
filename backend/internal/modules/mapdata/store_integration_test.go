//go:build integration

package mapdata

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("explicit disposable PostGIS DSN required")
	}
	admin, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("mapdata_%d", time.Now().UnixNano())
	if _, err := admin.Exec(context.Background(), "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)")
		_ = admin.Close(context.Background())
	})
	u, _ := url.Parse(dsn)
	u.Path = "/" + name
	return u.String()
}

func freshDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := testDSN(t)
	if _, err := migrate.Apply(context.Background(), dsn, dbmigrations.Files); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func fixture(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "contracts", "testdata", "map", "poi-assertions-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func envelopeFor(t *testing.T, assertions []byte) []byte {
	t.Helper()
	var corpus map[string]any
	if err := json.Unmarshal(assertions, &corpus); err != nil {
		t.Fatal(err)
	}
	count := len(corpus["assertions"].([]any))
	doc := map[string]any{
		"format":          "abastevo.map-prepared-v1",
		"version":         1,
		"product_class":   "PUBLIC_OVERLAY",
		"visibility":      "PUBLIC",
		"profile_id":      "map-overlay-test-v1",
		"profile_sha256":  fmt.Sprintf("%064x", 1),
		"rights_sha256":   fmt.Sprintf("%064x", 2),
		"source_revision": corpus["generation"],
		"inputs": []any{map[string]any{
			"file": "poi-assertions-v1.json", "bytes": len(assertions),
			"sha256": sha(assertions), "roots": count,
		}},
		"artifacts": map[string]any{"assertions": map[string]any{
			"bytes": len(assertions), "records": count, "sha256": sha(assertions),
		}},
		"counts": map[string]any{"input": count, "accepted": count, "duplicates": 0, "quarantined": 0},
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	doc["content_sha256"] = hex.EncodeToString(sum[:])
	raw, err = json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func sha(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestPublishQueryAndCursor(t *testing.T) {
	pool := freshDB(t)
	store := NewStore(pool)
	assertions := fixture(t)
	generation, err := store.Publish(context.Background(), envelopeFor(t, assertions), assertions)
	if err != nil {
		t.Fatal(err)
	}
	if generation.Status != "ACTIVE" {
		t.Fatalf("status %s", generation.Status)
	}
	active, err := store.ActiveGeneration(context.Background())
	if err != nil || active.ID != generation.ID {
		t.Fatalf("active %+v %v", active, err)
	}
	items, next, err := store.Query(context.Background(), generation.ID, Query{
		MinLat: -16, MaxLat: -15, MinLon: -48.1, MaxLon: -47.7,
		Source: "ANP", Limit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || next == "" {
		t.Fatalf("first page %d next=%q", len(items), next)
	}
	second, _, err := store.Query(context.Background(), generation.ID, Query{
		MinLat: -16, MaxLat: -15, MinLon: -48.1, MaxLon: -47.7,
		Source: "ANP", Limit: 10, AfterID: next,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 2 {
		t.Fatalf("second page %d, want 2", len(second))
	}
	var priced bool
	for _, item := range items {
		if item.AssertionID == "11111111-1111-4111-8111-111111111111" {
			priced = len(item.Prices) == 1 && item.Prices[0].AmountMilliBRL == 5890
		}
	}
	if !priced {
		t.Fatal("selected ANP price missing")
	}
}

func TestPublishIdempotentAndSupersede(t *testing.T) {
	pool := freshDB(t)
	store := NewStore(pool)
	assertions := fixture(t)
	envelope := envelopeFor(t, assertions)
	first, err := store.Publish(context.Background(), envelope, assertions)
	if err != nil {
		t.Fatal(err)
	}
	again, err := store.Publish(context.Background(), envelope, assertions)
	if err != nil || again.ID != first.ID {
		t.Fatalf("idempotent %+v %v", again, err)
	}
	var doc map[string]any
	_ = json.Unmarshal(assertions, &doc)
	doc["generation"] = "overlay-2026-10-25.1"
	assertions2, _ := json.Marshal(doc)
	second, err := store.Publish(context.Background(), envelopeFor(t, assertions2), assertions2)
	if err != nil {
		t.Fatal(err)
	}
	if second.ID == first.ID {
		t.Fatal("new generation reused old id")
	}
	var active int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM map_generations WHERE status='ACTIVE'`).Scan(&active); err != nil || active != 1 {
		t.Fatalf("active count %d err=%v", active, err)
	}
	var oldStatus string
	if err := pool.QueryRow(context.Background(), `SELECT status FROM map_generations WHERE id=$1`, first.ID).Scan(&oldStatus); err != nil || oldStatus != "SUPERSEDED" {
		t.Fatalf("old status %q err=%v", oldStatus, err)
	}
}

func TestConcurrentPublishKeepsOneActive(t *testing.T) {
	pool := freshDB(t)
	store := NewStore(pool)
	assertions := fixture(t)
	var doc map[string]any
	_ = json.Unmarshal(assertions, &doc)
	doc["generation"] = "overlay-concurrent-a"
	a, _ := json.Marshal(doc)
	doc["generation"] = "overlay-concurrent-b"
	b, _ := json.Marshal(doc)
	envelopes := [][]byte{envelopeFor(t, a), envelopeFor(t, b)}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i, raw := range [][]byte{a, b} {
		wg.Add(1)
		go func(raw, envelope []byte) {
			defer wg.Done()
			_, err := store.Publish(context.Background(), envelope, raw)
			errs <- err
		}(raw, envelopes[i])
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var active, total int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FILTER (WHERE status='ACTIVE'), count(*) FROM map_generations`).Scan(&active, &total); err != nil {
		t.Fatal(err)
	}
	if active != 1 || total != 2 {
		t.Fatalf("active=%d total=%d", active, total)
	}
}

func TestRevocationAndGenerationKillSwitch(t *testing.T) {
	pool := freshDB(t)
	store := NewStore(pool)
	assertions := fixture(t)
	generation, err := store.Publish(context.Background(), envelopeFor(t, assertions), assertions)
	if err != nil {
		t.Fatal(err)
	}
	const target = "11111111-1111-4111-8111-111111111111"
	if err := store.RevokeAssertion(context.Background(), target, "source withdrawn", "test"); err != nil {
		t.Fatal(err)
	}
	if err := store.RevokeAssertion(context.Background(), target, "source withdrawn", "test"); err != nil {
		t.Fatal(err)
	}
	items, _, err := store.Query(context.Background(), generation.ID, Query{
		MinLat: -16, MaxLat: -15, MinLon: -48.1, MaxLon: -47.7,
		Source: "ANP", Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.AssertionID == target {
			t.Fatal("revoked assertion served")
		}
	}
	var history int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM map_assertions WHERE assertion_id=$1`, target).Scan(&history); err != nil || history != 1 {
		t.Fatalf("history %d err=%v", history, err)
	}
	if err := store.RevokeGeneration(context.Background(), generation.ID, "kill switch", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ActiveGeneration(context.Background()); err != ErrNoActiveGeneration {
		t.Fatalf("active after revoke: %v", err)
	}
	if _, err := store.Publish(context.Background(), envelopeFor(t, assertions), assertions); err != ErrRevokedGeneration {
		t.Fatalf("republish revoked: %v", err)
	}
}
func TestCanceledPublishLeavesNoGeneration(t *testing.T) {
	pool := freshDB(t)
	store := NewStore(pool)
	assertions := fixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.Publish(ctx, envelopeFor(t, assertions), assertions); err == nil {
		t.Fatal("canceled publish succeeded")
	}
	var count int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM map_generations`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("generations after cancel %d err=%v", count, err)
	}
}

func TestPreviousSchemaUpgradeAddsMapTables(t *testing.T) {
	dsn := testDSN(t)
	previous := fstest.MapFS{}
	entries, err := fs.ReadDir(dbmigrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() >= "000122" {
			continue
		}
		raw, err := fs.ReadFile(dbmigrations.Files, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		previous[entry.Name()] = &fstest.MapFile{Data: raw}
	}
	if _, err := migrate.Apply(context.Background(), dsn, previous); err != nil {
		t.Fatal(err)
	}
	applied, err := migrate.Apply(context.Background(), dsn, dbmigrations.Files)
	if err != nil {
		t.Fatal(err)
	}
	expected := []string{"000122", "000123", "000124", "000125", "000126", "000127", "000128", "000129", "000130", "000131", "000132",
		"000133", "000134", "000135", "000136", "000137", "000138"}
	if !slices.Equal(applied, expected) {
		t.Fatalf("applied %v", applied)
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var exists bool
	if err := pool.QueryRow(context.Background(), `SELECT to_regclass('public.map_generations') IS NOT NULL`).Scan(&exists); err != nil || !exists {
		t.Fatalf("map table exists=%v err=%v", exists, err)
	}
}
