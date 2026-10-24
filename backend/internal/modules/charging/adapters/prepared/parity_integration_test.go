//go:build integration

package prepared

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRustChargingParity(t *testing.T) {
	dir := os.Getenv("ANPFUEL_TEST_CHARGING_DIR")
	if dir == "" {
		t.Skip("prepared charging batch unavailable")
	}
	manifestRaw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := VerifyManifest(manifestRaw, "charging-prepared-v1", "ocm-br-20261021-v1")
	if err != nil {
		t.Fatal("actual Rust manifest mismatch", err)
	}
	entitiesRaw, err := os.ReadFile(filepath.Join(dir, "entities.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	decisionsRaw, err := os.ReadFile(filepath.Join(dir, "decisions.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	batch, err := LoadBatch(context.Background(), manifest, entitiesRaw, decisionsRaw, "ocm")
	if err != nil {
		t.Fatal("actual Rust contract mismatch", err)
	}
	if len(batch.Entities) != 3387 {
		t.Fatal("full-source parity lost", len(batch.Entities))
	}
	places := 0
	for _, entity := range batch.Entities {
		if entity.Kind == "PLACE" {
			places++
		}
	}
	if places != 1298 {
		t.Fatal("place parity lost", places)
	}
}

func TestRustVehicleParity(t *testing.T) {
	dir := os.Getenv("ANPFUEL_TEST_VEHICLE_DIR")
	if dir == "" {
		t.Skip("prepared vehicle batch unavailable")
	}
	manifestRaw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := VerifyManifest(manifestRaw, "vehicle-energy-reference-v1", "pbe-2026-pdf-v1")
	if err != nil {
		t.Fatal("actual Rust manifest mismatch", err)
	}
	entitiesRaw, err := os.ReadFile(filepath.Join(dir, "entities.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	decisionsRaw, err := os.ReadFile(filepath.Join(dir, "decisions.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	batch, err := LoadBatch(context.Background(), manifest, entitiesRaw, decisionsRaw, "pbe")
	if err != nil {
		t.Fatal("actual Rust contract mismatch", err)
	}
	if len(batch.Entities) != 181 {
		t.Fatal("full-source parity lost", len(batch.Entities))
	}
}
