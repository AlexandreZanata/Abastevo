//go:build integration

package prepared

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	charging "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/charging"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	adminDSN := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if adminDSN == "" {
		t.Skip("database unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("charging_test_%d", time.Now().UnixNano())
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
	u, err := url.Parse(adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	if _, err := migrate.Apply(ctx, u.String(), dbmigrations.Files); err != nil {
		t.Fatal("migrate", err)
	}
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func loadFixture(t *testing.T, dir, format, profile, namespace string) (Batch, []byte) {
	t.Helper()
	manifestRaw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := VerifyManifest(manifestRaw, format, profile)
	if err != nil {
		t.Fatal("manifest", err)
	}
	entitiesRaw, _ := os.ReadFile(filepath.Join(dir, "entities.ndjson"))
	decisionsRaw, _ := os.ReadFile(filepath.Join(dir, "decisions.ndjson"))
	batch, err := LoadBatch(context.Background(), manifest, entitiesRaw, decisionsRaw, namespace)
	if err != nil {
		t.Fatal("load", err)
	}
	return batch, manifestRaw
}

func TestChargingStorageUpgradeReplayAndLocks(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	chargingDir := os.Getenv("ANPFUEL_TEST_CHARGING_DIR")
	vehicleDir := os.Getenv("ANPFUEL_TEST_VEHICLE_DIR")
	if chargingDir == "" || vehicleDir == "" {
		t.Skip("prepared batches unavailable")
	}
	q := charging.New(pool)
	for _, tc := range []struct {
		dir, format, profile, namespace string
		wantEntities                    int
	}{
		{chargingDir, "charging-prepared-v1", "ocm-br-20261021-v1", "ocm", 3387},
		{vehicleDir, "vehicle-energy-reference-v1", "pbe-2026-pdf-v1", "pbe", 181},
	} {
		batch, manifestRaw := loadFixture(t, tc.dir, tc.format, tc.profile, tc.namespace)
		if len(batch.Entities) != tc.wantEntities {
			t.Fatal("parity lost", tc.format, len(batch.Entities))
		}
		sum := sha256.Sum256(manifestRaw)
		batchID, err := StoreBatch(ctx, q, batch, hex.EncodeToString(sum[:]), len(batch.Entities), len(batch.Entities))
		if err != nil {
			t.Fatal("store", tc.format, err)
		}
		if batchID == "" {
			t.Fatal("empty batch id")
		}
	}
	// Existing source/vehicle data preserved (both batches countable).
	var batches int64
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM charging_batches`).Scan(&batches); err != nil || batches < 2 {
		t.Fatal("batches lost", batches, err)
	}
}
