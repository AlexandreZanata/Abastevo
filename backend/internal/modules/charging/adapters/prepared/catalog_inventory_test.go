package prepared

import (
	"crypto/sha256"
	"fmt"
	"testing"
	"testing/fstest"
)

func TestVerifyNationalSourceInventoryBindsDecisions(t *testing.T) {
	valid := []byte(`{"key":"osm:node:1","state":"ACCEPTED","reason":null}
{"key":"ocm:poi:7","state":"QUARANTINED","reason":"planned_site"}
`)
	conflated, normalized, pin := sourceInventoryFixture(valid, 2)
	verified, err := VerifyNationalSourceInventory(conflated, normalized, pin)
	if err != nil || len(verified.states) != 2 || verified.states["ocm:poi:7"] != "QUARANTINED" {
		t.Fatalf("inventory=%+v err=%v", verified, err)
	}
	if _, err := VerifyNationalSourceInventory(conflated, normalized, fmt.Sprintf("%064x", 1)); err == nil {
		t.Fatal("wrong conflation pin accepted")
	}
	normalized["manifest.json"].Data = []byte("{}")
	if _, err := VerifyNationalSourceInventory(conflated, normalized, pin); err == nil {
		t.Fatal("tampered normalization manifest accepted")
	}
	conflated, normalized, pin = sourceInventoryFixture(valid, 2)
	normalized["decisions.ndjson"].Data = []byte(`{"key":"osm:node:1","state":"ACCEPTED"}`)
	if _, err := VerifyNationalSourceInventory(conflated, normalized, pin); err == nil {
		t.Fatal("tampered decisions accepted")
	}
	conflated, normalized, pin = sourceInventoryFixture(valid, 1)
	if _, err := VerifyNationalSourceInventory(conflated, normalized, pin); err == nil {
		t.Fatal("inconsistent decision count accepted")
	}
	duplicate := []byte(`{"key":"osm:node:1","state":"ACCEPTED"}
{"key":"osm:node:1","state":"QUARANTINED"}
`)
	conflated, normalized, pin = sourceInventoryFixture(duplicate, 2)
	if _, err := VerifyNationalSourceInventory(conflated, normalized, pin); err == nil {
		t.Fatal("duplicate decision accepted")
	}
	conflated, normalized, pin = sourceInventoryFixture(valid[:len(valid)-1], 2)
	if _, err := VerifyNationalSourceInventory(conflated, normalized, pin); err == nil {
		t.Fatal("unterminated decision stream accepted")
	}
}

func sourceInventoryFixture(decisions []byte, records int) (fstest.MapFS, fstest.MapFS, string) {
	normalization := fmt.Appendf(nil, `{"format":"abastevo.ev-source-sites-v1","source_pins":{"ocm_rows":1},
	"files":{"decisions.ndjson":{"sha256":"%x","bytes":%d,"records":%d}},
	"summary":{"accepted_source_sites":1,"source_equipment":0,"osm_candidates":1,"quarantined":1,"ignored":0}}`,
		sha256.Sum256(decisions), len(decisions), records)
	conflation := fmt.Appendf(nil, `{"format":"abastevo.ev-conflated-v1","input_manifest_sha256":"%x",
	"files":{"catalog.ndjson":{"sha256":"%x","bytes":7,"records":1}}}`,
		sha256.Sum256(normalization), sha256.Sum256([]byte("catalog")))
	return fstest.MapFS{"manifest.json": {Data: conflation}},
		fstest.MapFS{"manifest.json": {Data: normalization}, "decisions.ndjson": {Data: decisions}},
		fmt.Sprintf("%x", sha256.Sum256(conflation))
}
