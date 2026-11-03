//go:build integration

package registry

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	directory "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/directory"
)

func realAddressBatch(t *testing.T) ([]byte, BatchStreams) {
	t.Helper()
	binary := os.Getenv("ANPFUEL_TEST_PREPARER_PATH")
	if binary == "" {
		t.Fatal("ANPFUEL_TEST_PREPARER_PATH required for actual Rust v4")
	}
	dir := t.TempDir()
	write := func(name string, data []byte) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("registry.csv", []byte("CODIGOISIMP;AUTORIZACAO;DATAPUBLICACAO;RAZAOSOCIAL;CNPJ;ENDERECO;COMPLEMENTO;BAIRRO;CEP;UF;MUNICIPIO;BANDEIRA;DATAVINCULACAO\nSNA-TEST;SYNTHETIC;01/01/2024;[P25-TEST] Legal LTDA;91000000000066;;;;;SP;TEST CITY;BRANCA;01/01/2023\n"))
	write("aliases.json", []byte(`{"entries":[{"uf":"SP","ibge":"3550308","aliases":["TEST CITY"]}]}`))
	evidence := []byte(`{"schema_version":"rfb-station-addresses-v1","cnpj":"91000000000066","active":true,"address":{"street":"[P25-TEST] RFB Street","number":"12A"},"observed_at":"2026-10-01"}` + "\n")
	write("rfb.jsonl", evidence)
	pin := streamOutput("rfb.jsonl", evidence)
	rule := addressSources["rfb"]
	descriptor, _ := json.Marshal(map[string]any{"format_version": "registry-address-inputs-v1", "inputs": []any{map[string]any{"source": "rfb", "profile": rule.Profile, "file": "rfb.jsonl", "reference": "https://example.invalid/rfb-address", "edition": "[P25-TEST] synthetic", "sha256": pin.SHA256, "bytes": pin.Bytes, "rows": pin.Rows, "license": rule.License, "attribution": rule.Attribution}}})
	write("addresses.json", descriptor)
	out := filepath.Join(dir, "out")
	cmd := exec.Command(binary, filepath.Join(dir, "registry.csv"), filepath.Join(dir, "aliases.json"), out, "https://example.invalid/registry", "synthetic", "55555555-1111-4111-8111-111111111111", "2026-11-03T00:00:00Z", "--address-inputs", filepath.Join(dir, "addresses.json"))
	if log, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("actual Rust v4: %s: %v", log, err)
	}
	read := func(name string) []byte {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	return read("manifest.json"), BatchStreams{Assertions: read(PrepAssertionsFile), Candidates: read(PrepCandidatesFile), Quarantine: read(PrepQuarantineFile)}
}
func TestStationAddressesActualRustV4PostGISReplayAndRollback(t *testing.T) {
	raw, streams := realAddressBatch(t)
	m, err := ValidateBatch(raw, streams)
	if err != nil {
		t.Fatal(err)
	}
	pool, store := preparedPool(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			reports, err := LoadPreparedBatch(ctx, pool, raw, preparedOpener(streams))
			if err != nil || reports["registry-13col"].Accepted != 1 {
				t.Errorf("concurrent v4 replay: %v %v", reports, err)
			}
		})
	}
	wg.Wait()
	if t.Failed() {
		return
	}
	run, err := store.GetRun(ctx, SourcePrep, "station-prep:"+m.RunID+":registry-13col")
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.ListAssertions(ctx, run.RunID)
	if err != nil || len(got) != 1 {
		t.Fatal(got, err)
	}
	a := got[0]
	p := a.AddressProvenance
	if a.AddressSource != "rfb" || a.Address["street"] != "[P25-TEST] RFB Street" || a.Address["number"] != "12A" || a.Address["municipio"] != "TEST CITY" || a.HasCoords || p == nil || p.InputSHA256 != m.AddressInputs[0].SHA256 || p.RightsPolicySHA256 != addressPolicySHA256 {
		t.Fatal("staged v4 proof/rights/identity lost")
	}
	individual, _, err := store.GetAssertion(ctx, a.ID)
	if err != nil || individual.AddressProvenance == nil || individual.AddressProvenance.InputSHA256 != p.InputSHA256 {
		t.Fatal("individual assertion lost provenance", err)
	}
	station := newUUID()
	if _, err := pool.Exec(ctx, "INSERT INTO directory_stations(id,display_name) VALUES($1::uuid,'[P25-TEST] Station')", station); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO directory_identifiers(id,station_id,kind,normalized_value) VALUES($1::uuid,$2::uuid,'CNPJ',$3)", newUUID(), station, a.SourceKey); err != nil {
		t.Fatal(err)
	}
	if err := store.SetAssertionStation(ctx, a.ID, station); err != nil {
		t.Fatal(err)
	}
	sid, _ := mustUUID(station)
	aid, _ := mustUUID(a.ID)
	facts, err := store.Q.SourceStationFacts(ctx, directory.SourceStationFactsParams{StationID: sid, AssertionID: aid})
	if err != nil || facts.AddressSource.String != "rfb" || !facts.SourceClock.Valid {
		t.Fatal("bound v4 not eligible for facts projection", err)
	}
	for _, sql := range []string{"UPDATE registry_assertions SET address_provenance='[]'::jsonb WHERE id=$1", "UPDATE registry_assertions SET address_source=NULL WHERE id=$1"} {
		if _, err := pool.Exec(ctx, sql, aid); err == nil {
			t.Fatal("invalid provenance constraint admitted")
		}
	}
	// The first new row stages successfully inside the attempt, then a bad proof
	// aborts the second row. Neither the new run nor the first assertion survives.
	var first, second map[string]any
	_ = json.Unmarshal(streams.Assertions, &first)
	_ = json.Unmarshal(streams.Assertions, &second)
	first["checksum"] = vectorChecksum("first-new-v4")
	second["checksum"] = vectorChecksum("bad-proof-v4")
	second["address_facts"].(map[string]any)["provenance"].(map[string]any)["proof"].(map[string]any)["active"] = false
	bad := streams
	bad.Assertions = marshalLines(t, first, second)
	m.RunID = newUUID()
	m.Inputs[0].Rows = 2
	m.Counts["registry-13col"] = BatchCounts{Input: 2, Accepted: 2}
	for i, output := range m.Outputs {
		if output.Path == PrepAssertionsFile {
			m.Outputs[i] = streamOutput(PrepAssertionsFile, bad.Assertions)
		}
	}
	badManifest, _ := json.Marshal(m)
	if _, err := LoadBatch(ctx, store, badManifest, bad); err == nil || !strings.Contains(err.Error(), "invalid v4 address facts") {
		t.Fatal("in-memory bad proof rejection", err)
	}
	if _, err := LoadPreparedBatch(ctx, pool, badManifest, preparedOpener(bad)); err == nil || !strings.Contains(err.Error(), "invalid v4 address facts") {
		t.Fatal("streamed bad proof rejection", err)
	}
	var runs, assertions int
	if err := pool.QueryRow(ctx, "SELECT (SELECT count(*) FROM registry_source_runs),(SELECT count(*) FROM registry_assertions)").Scan(&runs, &assertions); err != nil || runs != 1 || assertions != 1 {
		t.Fatal("partial v4 attempt persisted", runs, assertions, err)
	}
}
