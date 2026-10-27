package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func row(quality, lat, lon string) string {
	raw, _ := json.Marshal(locationRow{
		SchemaVersion: rowSchema, CNPJ: "11222333000181", Tier: "T", Quality: quality,
		LatitudeDeg: lat, LongitudeDeg: lon, Provider: "anp-revendedores-api", SourceReference: "anp-api|srid=4674",
	})
	return string(raw) + "\n"
}

func writeBatch(t *testing.T, body string, d1, d2 bool, tamper func(map[string]any)) (string, string) {
	t.Helper()
	dir := t.TempDir()
	locations := filepath.Join(dir, "locations.ndjson")
	if err := os.WriteFile(locations, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(body))
	m := map[string]any{
		"format": manifestFormat, "schema_version": rowSchema, "generation": "test",
		"owner_decisions": map[string]bool{"D1_anp_reported": d1, "D2_address_point": d2},
		"files": map[string]any{"locations.ndjson": map[string]any{
			"bytes": len(body), "sha256": hex.EncodeToString(sum[:])}},
	}
	if tamper != nil {
		tamper(m)
	}
	raw, _ := json.Marshal(m)
	manifestPath := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(manifestPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return locations, manifestPath
}

func load(t *testing.T, locations, manifestPath string) error {
	raw, m, err := readVerified(locations, manifestPath)
	if err != nil {
		return err
	}
	_, err = parseRows(raw, m)
	return err
}

func TestValidBatchParses(t *testing.T) {
	locations, manifestPath := writeBatch(t, row("anp-reported", "-23.55", "-46.63"), true, false, nil)
	raw, m, err := readVerified(locations, manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := parseRows(raw, m)
	if err != nil || len(rows) != 1 || rows[0].Latitude != -23.55 || rows[0].CNPJ != "11222333000181" {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
}

func TestOwnerDecisionsGateTiers(t *testing.T) {
	cases := []struct {
		quality string
		d1, d2  bool
		ok      bool
	}{
		{"anp-reported", false, true, false},
		{"address-point", true, false, false},
		{"address-point", true, true, true},
		{"reviewed", false, false, true},
		{"osm-mapped", false, false, true},
	}
	for _, c := range cases {
		locations, manifestPath := writeBatch(t, row(c.quality, "-23.55", "-46.63"), c.d1, c.d2, nil)
		if err := load(t, locations, manifestPath); (err == nil) != c.ok {
			t.Errorf("%s d1=%v d2=%v: err=%v", c.quality, c.d1, c.d2, err)
		}
	}
}

func TestRejectsNonPinsAndPointsOutsideBrazil(t *testing.T) {
	for name, body := range map[string]string{
		"centroid": row("city-centroid", "-23.55", "-46.63"),
		"unknown":  row("unknown", "-23.55", "-46.63"),
		"paris":    row("reviewed", "48.85", "2.35"),
		"nan":      row("reviewed", "NaN", "-46.63"),
		"empty":    "",
	} {
		locations, manifestPath := writeBatch(t, body, true, true, nil)
		if err := load(t, locations, manifestPath); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestManifestBindsTheLocationsFile(t *testing.T) {
	locations, manifestPath := writeBatch(t, row("reviewed", "-23.55", "-46.63"), true, true, func(m map[string]any) {
		m["files"].(map[string]any)["locations.ndjson"].(map[string]any)["sha256"] = strings.Repeat("0", 64)
	})
	if err := load(t, locations, manifestPath); err == nil || !strings.Contains(err.Error(), "hash") {
		t.Fatalf("tampered hash: %v", err)
	}
	locations, manifestPath = writeBatch(t, row("reviewed", "-23.55", "-46.63"), true, true, func(m map[string]any) {
		m["format"] = "other"
	})
	if err := load(t, locations, manifestPath); err == nil {
		t.Fatal("foreign manifest accepted")
	}
}
