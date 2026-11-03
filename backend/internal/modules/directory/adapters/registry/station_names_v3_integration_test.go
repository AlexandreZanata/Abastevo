//go:build integration

package registry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestStationNamesActualRustV3AdmissionAndPreparedRollback(t *testing.T) {
	preparer := os.Getenv("ANPFUEL_TEST_PREPARER_PATH")
	if preparer == "" {
		t.Fatal("ANPFUEL_TEST_PREPARER_PATH required for actual Rust v3")
	}
	dir := t.TempDir()
	write := func(name string, raw []byte) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("registry.csv", []byte("CODIGOISIMP;AUTORIZACAO;DATAPUBLICACAO;RAZAOSOCIAL;CNPJ;ENDERECO;COMPLEMENTO;BAIRRO;CEP;UF;MUNICIPIO;BANDEIRA;DATAVINCULACAO\nSNA-TEST;SYNTHETIC;01/01/2024; [P25-TEST] Legal LTDA ;91000000000066;TEST STREET 100;UNIT A;TEST DISTRICT;01000000;SP;TEST CITY;BRANCA;01/01/2023\n"))
	write("aliases.json", []byte(`{"entries":[{"uf":"SP","ibge":"3550308","aliases":["TEST CITY"]}]}`))
	names := []byte("{\"schema_version\":\"rfb-station-names-v1\",\"cnpj\":\"91000000000066\",\"trade_name\":\"[P25-TEST] Trade\",\"active\":true,\"observed_at\":\"2026-10-01\"}\n")
	write("rfb.jsonl", names)
	sum := sha256.Sum256(names)
	descriptor := map[string]any{"format_version": "registry-name-inputs-v1", "inputs": []any{map[string]any{"source": "rfb", "profile": "rfb-station-names-v1", "file": "rfb.jsonl", "reference": "https://example.invalid/rfb", "edition": "synthetic", "sha256": hex.EncodeToString(sum[:]), "bytes": len(names), "rows": 1}}}
	raw, _ := json.Marshal(descriptor)
	write("names.json", raw)
	cmd := exec.Command(preparer, filepath.Join(dir, "registry.csv"), filepath.Join(dir, "aliases.json"), filepath.Join(dir, "out"), "https://example.invalid/registry", "synthetic", "55555555-1111-4111-8111-111111111111", "2026-11-03T00:00:00Z", "--name-inputs", filepath.Join(dir, "names.json"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("synthetic prep: %s: %v", out, err)
	}
	read := func(name string) []byte {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(dir, "out", name))
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	manifest := read("manifest.json")
	streams := BatchStreams{Assertions: read(PrepAssertionsFile), Candidates: read(PrepCandidatesFile), Quarantine: read(PrepQuarantineFile)}
	if _, err := ValidateBatch(manifest, streams); err != nil {
		t.Fatal(err)
	}
	var row BatchAssertion
	if err := decodeStrict([]byte(strings.TrimSpace(string(streams.Assertions))), &row, "synthetic"); err != nil {
		t.Fatal(err)
	}
	a, err := mapAssertionRow(row)
	if err != nil || a.LegalName != " [P25-TEST] Legal LTDA " || a.TradeName != "[P25-TEST] Trade" || a.DisplayName != a.TradeName || a.Address["street"] != "TEST STREET" || a.Address["municipio"] != "TEST CITY" {
		t.Fatalf("v3 mapping %+v: %v", a, err)
	}
	pool, store := preparedPool(t)
	var decoded BatchManifest
	if err := json.Unmarshal(manifest, &decoded); err != nil {
		t.Fatal(err)
	}
	decoded.NameInputs[0].SHA256 = vectorChecksum("tamper")
	bad, _ := json.Marshal(decoded)
	var before, after int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM registry_source_runs").Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadBatch(context.Background(), store, bad, streams); err == nil {
		t.Fatal("in-memory loader admitted unbound names")
	}
	if _, err := LoadPreparedBatch(context.Background(), pool, bad, preparedOpener(streams)); err == nil {
		t.Fatal("prepared loader admitted unbound names")
	}
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM registry_source_runs").Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("invalid names left a partial run")
	}
	for range 2 {
		reports, err := LoadPreparedBatch(context.Background(), pool, manifest, preparedOpener(streams))
		if err != nil {
			t.Fatal(err)
		}
		if reports["registry-13col"].Accepted != 1 {
			t.Fatal(reports)
		}
		assertions, err := store.ListAssertions(context.Background(), reports["registry-13col"].RunID)
		if err != nil || len(assertions) != 1 || assertions[0].DisplayName != "[P25-TEST] Trade" || assertions[0].LegalName != a.LegalName || assertions[0].TradeNameProvenance == nil || assertions[0].TradeNameProvenance.SHA256 != hex.EncodeToString(sum[:]) || assertions[0].AddressSource != "anp" || assertions[0].SourceDisplayName != "[P25-TEST] Legal LTDA" {
			t.Fatalf("v3 staging %+v: %v", assertions, err)
		}
	}
}
