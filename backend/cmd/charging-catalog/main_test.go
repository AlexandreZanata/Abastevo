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
	for _, sub := range []string{"normalized/generation", "conflated/generation"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0700); err != nil {
			t.Fatal(err)
		}
	}
	decisions := []byte("{\"key\":\"osm:node:1\",\"state\":\"ACCEPTED\"}\n")
	decisionHash := write(filepath.Join(dir, "normalized/generation/decisions.ndjson"), decisions)
	normal := encode(map[string]any{
		"format": "abastevo.ev-source-sites-v1", "source_pins": map[string]int{"ocm_rows": 0},
		"summary": map[string]int{"accepted_source_sites": 1, "osm_candidates": 1},
		"files":   map[string]any{"decisions.ndjson": map[string]any{"sha256": decisionHash, "bytes": len(decisions), "records": 1}},
	})
	normalHash := write(filepath.Join(dir, "normalized/generation/manifest.json"), normal)
	manifest := write(filepath.Join(dir, "conflated/generation/manifest.json"), encode(map[string]any{
		"format": "abastevo.ev-conflated-v1", "input_manifest_sha256": normalHash,
		"files": map[string]any{"catalog.ndjson": data},
	}))
	boundary := strings.Repeat("d", 64)
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
	config := filepath.Join(dir, "review.json")
	spec := map[string]string{"directory": dir, "rights_sha256": rightsHash, "manifest_sha256": manifest,
		"geography_sha256": geoHash, "boundary_sha256": boundary}
	write(config, encode(map[string]any{"candidate": spec, "baseline": spec}))
	reports := filepath.Join(dir, "reports")
	if err := os.Mkdir(reports, 0700); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"review", "--config", config, "--output-dir", reports}, noDB, &output); err != nil {
		t.Fatalf("file-only review: %v", err)
	}
	entries, err := os.ReadDir(reports)
	if err != nil || len(entries) != 1 {
		t.Fatalf("review reports=%v err=%v", entries, err)
	}
	info, err := entries[0].Info()
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("review report permissions: %v %v", info, err)
	}
	review, err := os.ReadFile(filepath.Join(reports, entries[0].Name()))
	if err != nil || !bytes.Contains(review, []byte(`"inventory_records":1`)) ||
		!bytes.Contains(review, []byte(report.BatchSHA256)) || bytes.Contains(review, []byte(`"added"`)) {
		t.Fatalf("review report=%q err=%v", review, err)
	}
	write(config, encode(map[string]any{"candidate": spec, "activate": true}))
	if err := run([]string{"review", "--config", config}, noDB, &output); err == nil {
		t.Fatal("unexpected review config accepted")
	}

	write(config, encode(map[string]any{"candidate": spec}))
	if err := run(append(flags, "--activate", "--approve-batch-sha256", report.BatchSHA256), noDB, &output); err == nil {
		t.Fatal("missing transition review accepted")
	}
	productionChecked := false
	production := func(key string) string {
		if key != "ANPFUEL_ENV" {
			t.Fatalf("refused target tried to read %s", key)
		}
		productionChecked = true
		return "production"
	}
	if err := run(append(flags, "--activate", "--approve-batch-sha256", report.BatchSHA256, "--review-config", config), production, &output); err == nil {
		t.Fatal("production activation accepted")
	}
	if !productionChecked {
		t.Fatal("valid review did not reach the target guard")
	}
}

func TestControlRequiresExactApprovalAndRejectsProduction(t *testing.T) {
	args := []string{"control", "--action", "rollback", "--expected-sha256", strings.Repeat("a", 64),
		"--actor", "synthetic-owner", "--reason", "test recovery"}
	noDB := func(key string) string {
		t.Fatalf("incomplete control attempted to read environment key %s", key)
		return ""
	}
	if err := run(args, noDB, &bytes.Buffer{}); err == nil {
		t.Fatal("missing restore digest accepted")
	}
	production := func(key string) string {
		if key != "ANPFUEL_ENV" {
			t.Fatalf("production control tried to read %s", key)
		}
		return "production"
	}
	if err := run(append(args, "--restore-sha256", "none"), production, &bytes.Buffer{}); err == nil ||
		!strings.Contains(err.Error(), "explicit local or staging environment") {
		t.Fatalf("production target was not rejected by the environment guard: %v", err)
	}
}

func TestStagingCatalogWritesRequireTheOfficialCanonicalHost(t *testing.T) {
	staging := func(host string) func(string) string {
		return func(key string) string {
			switch key {
			case "ANPFUEL_ENV":
				return "staging"
			case "ANPFUEL_CANONICAL_HOST":
				return host
			case "ANPFUEL_DATABASE_URL":
				return "postgres://fixture:fixture@abastevo-temp-db:5432/fixture"
			}
			return ""
		}
	}
	for _, host := range []string{"teste.abastevo.com.br", "api.abastevo.com.br", ""} {
		if _, err := ownerDatabaseConfig(staging(host)); err == nil ||
			!strings.Contains(err.Error(), "staging canonical host") {
			t.Fatalf("staging host %q was not refused by the host guard: %v", host, err)
		}
	}
	if cfg, err := ownerDatabaseConfig(staging("abastevo.com.br")); err != nil || cfg.MaxConns != 2 {
		t.Fatalf("official canonical host refused: %v", err)
	}
}
