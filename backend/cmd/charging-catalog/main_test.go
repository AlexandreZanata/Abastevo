package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyIsFileOnlyAndApprovalIsExact(t *testing.T) {
	dir := t.TempDir()
	rightsDir := filepath.Join(dir, "rights")
	if err := os.Mkdir(rightsDir, 0700); err != nil {
		t.Fatal(err)
	}
	write := func(path string, raw []byte) string {
		t.Helper()
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		return fmt.Sprintf("%x", sha256.Sum256(raw))
	}
	encode := func(value any) []byte {
		t.Helper()
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	key := "ev-site-v1:" + strings.Repeat("a", 64)
	point := map[string]string{"latitude_decimal": "-23.5", "longitude_decimal": "-46.6"}
	catalog := encode(map[string]any{
		"key": key, "kind": "CHARGING_SITE", "confidence": "single-source", "point": point,
		"field_sources": map[string]any{"point": map[string]string{"source_key": "osm:node:1", "field": "point"}},
		"sources": []any{map[string]any{"key": "osm:node:1", "kind": "CHARGING_SITE", "access": "PUBLIC", "point": point,
			"source": map[string]string{"source": "OSM", "external_id": "node:1", "revision": "synthetic",
				"record_sha256": strings.Repeat("b", 64), "license": "ODbL-1.0", "attribution": "Synthetic"}}},
	})
	catalog = append(catalog, '\n')
	dataHash := write(filepath.Join(rightsDir, "derived-database.ndjson"), catalog)
	data := map[string]any{"path": "derived-database.ndjson", "sha256": dataHash, "bytes": len(catalog), "records": 1}
	notice := encode(map[string]any{"format": "abastevo.ev-derived-database-offer-v1", "license": "ODbL-1.0", "data": data})
	noticeHash := write(filepath.Join(rightsDir, "derived-database-offer.json"), notice)
	manifest, boundary := strings.Repeat("c", 64), strings.Repeat("d", 64)
	rights := encode(map[string]any{"format": "abastevo.ev-generation-rights-v1", "catalog_sha256": dataHash,
		"catalog_manifest_sha256": manifest, "catalog_sites": 1, "requires_derived_database_offer": true,
		"offer": map[string]any{"data": data, "notice": map[string]any{"path": "derived-database-offer.json",
			"sha256": noticeHash, "bytes": len(notice)}}})
	rightsHash := write(filepath.Join(rightsDir, "rights.json"), rights)
	geo := filepath.Join(dir, "geography.json")
	geoHash := write(geo, encode(map[string]any{"format": "abastevo.ev-catalog-geography-v1",
		"catalog_sha256": dataHash, "boundary_sha256": boundary,
		"sites": []map[string]string{{"key": key, "uf": "SP", "reason": "ibge_polygon"}}}))
	flags := []string{"--bundle", rightsDir, "--rights-sha256", rightsHash, "--manifest-sha256", manifest,
		"--geography", geo, "--geography-sha256", geoHash, "--boundary-sha256", boundary}
	noDB := func(key string) string {
		t.Fatalf("verification attempted to read environment key %s", key)
		return ""
	}
	var output bytes.Buffer
	if err := run(flags, noDB, &output); err != nil || !strings.Contains(output.String(), `"SP":1`) {
		t.Fatalf("verify output=%q err=%v", &output, err)
	}
	if err := run(append(flags, "--activate", "--approve-batch-sha256", strings.Repeat("0", 64)), noDB, &output); err == nil {
		t.Fatal("wrong approval digest accepted")
	}
	var report struct {
		BatchSHA256 string `json:"batch_sha256"`
	}
	if err := json.Unmarshal(output.Bytes(), &report); err != nil || report.BatchSHA256 == "" {
		t.Fatalf("verify receipt invalid: %v", err)
	}
	production := func(key string) string {
		if key != "ANPFUEL_ENV" {
			t.Fatalf("refused target tried to read %s", key)
		}
		return "production"
	}
	if err := run(append(flags, "--activate", "--approve-batch-sha256", report.BatchSHA256), production, &output); err == nil {
		t.Fatal("production activation accepted")
	}
}
