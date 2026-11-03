package registry

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func addressV4Streams(t *testing.T) (BatchStreams, []byte) {
	t.Helper()
	streams, raw := namedV3Streams(t)
	lines, _ := splitJSONL(streams.Assertions, "synthetic")
	streams.Assertions = nil
	for _, line := range lines {
		var row map[string]any
		_ = json.Unmarshal(line, &row)
		row["schema_version"] = "station-assertion-v4"
		row["address_raw"] = ""
		row["address_normalized"] = map[string]string{}
		row["address_facts"] = map[string]any{"source": "rfb", "components": map[string]string{"street": "[P25-TEST] RFB Street", "number": "12A"}, "provenance": map[string]any{"profile": "rfb-station-addresses-v1", "reference": "https://example.invalid/rfb-address", "edition": "[P25-TEST] synthetic", "input_sha256": vectorChecksum("addresses-input"), "observed_at": "2026-10-01", "license": "Creative Commons Attribution", "attribution": "Receita Federal do Brasil — Cadastro Nacional da Pessoa Jurídica (CNPJ)", "rights_reference": "https://dados.gov.br/dados/conjuntos-dados/cadastro-nacional-da-pessoa-juridica---cnpj", "rights_policy_sha256": addressPolicySHA256, "proof": map[string]any{"kind": "rfb", "cnpj": row["source_key"], "active": true}}}
		streams.Assertions = append(streams.Assertions, marshalLines(t, row)...)
	}
	var old, m map[string]any
	_ = json.Unmarshal(raw, &old)
	_ = json.Unmarshal(prepManifest(t, streams), &m)
	m["name_inputs"] = old["name_inputs"]
	m["parser_version"] = addressParserVersion
	m["address_policy_sha256"] = addressPolicySHA256
	m["address_inputs"] = []any{map[string]any{"source": "rfb", "profile": "rfb-station-addresses-v1", "reference": "https://example.invalid/rfb-address", "edition": "[P25-TEST] synthetic", "sha256": vectorChecksum("addresses-input"), "bytes": 256, "rows": 1, "license": "Creative Commons Attribution", "attribution": "Receita Federal do Brasil — Cadastro Nacional da Pessoa Jurídica (CNPJ)"}}
	m["address_skips"] = []any{}
	transition := map[string]any{"before": map[string]int{"municipality": 1}, "after": map[string]int{"rfb": 1}}
	m["address_coverage"] = map[string]any{"station_count": 1, "national": transition, "by_uf": map[string]any{"SP": transition}, "review_reasons": map[string]int{}}
	raw, _ = json.Marshal(m)
	return streams, raw
}
func TestStationAddressPolicyHashMatchesFrozenContract(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(registryDir(t), "..", "..", "station-prep", "address-source-policy-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != addressPolicySHA256 {
		t.Fatal("source rights policy hash drift")
	}
}
func TestStationAddressesV4BindsSourceFactsAndKeepsOldProfiles(t *testing.T) {
	streams, raw := addressV4Streams(t)
	if _, err := ValidateBatch(raw, streams); err != nil {
		t.Fatal(err)
	}
	lines, _ := splitJSONL(streams.Assertions, "synthetic")
	var row BatchAssertion
	if err := decodeStrict(lines[0], &row, "synthetic"); err != nil {
		t.Fatal(err)
	}
	a, err := mapAssertionRow(row)
	if err != nil {
		t.Fatal(err)
	}
	if a.AddressSource != "rfb" || a.Address["street"] != "[P25-TEST] RFB Street" || a.Address["number"] != "12A" || a.AddressProvenance == nil || a.AddressProvenance.InputSHA256 != vectorChecksum("addresses-input") {
		t.Fatal("v4 address facts/provenance lost")
	}
	legacy := prepStreams(t)
	if _, err := ValidateBatch(prepManifest(t, legacy), legacy); err != nil {
		t.Fatal(err)
	}
	v3, m := namedV3Streams(t)
	if _, err := ValidateBatch(m, v3); err != nil {
		t.Fatal(err)
	}
}
func TestStationAddressesV4RejectsUnboundRightsProofAndLegacySmuggling(t *testing.T) {
	for _, fault := range []string{"input_hash", "policy", "license", "attribution", "profile", "duplicate_pin", "proof_identity", "proof_inactive", "missing_provenance", "bad_date", "coordinates", "unknown_component", "old_schema", "old_parser", "cnefe_not_admitted", "coverage_count"} {
		t.Run(fault, func(t *testing.T) {
			streams, raw := addressV4Streams(t)
			var m map[string]any
			_ = json.Unmarshal(raw, &m)
			lines, _ := splitJSONL(streams.Assertions, "synthetic")
			var r map[string]any
			_ = json.Unmarshal(lines[0], &r)
			f := r["address_facts"].(map[string]any)
			p := f["provenance"].(map[string]any)
			pin := m["address_inputs"].([]any)[0].(map[string]any)
			switch fault {
			case "input_hash":
				pin["sha256"] = vectorChecksum("tamper")
			case "policy":
				m["address_policy_sha256"] = vectorChecksum("tamper")
			case "license":
				pin["license"] = "invented"
			case "attribution":
				pin["attribution"] = ""
			case "profile":
				pin["profile"] = "invented"
			case "duplicate_pin":
				m["address_inputs"] = []any{pin, pin}
			case "proof_identity":
				p["proof"].(map[string]any)["cnpj"] = "91000000000000"
			case "proof_inactive":
				p["proof"].(map[string]any)["active"] = false
			case "missing_provenance":
				f["provenance"] = nil
			case "bad_date":
				p["observed_at"] = "2026-02-30"
			case "coordinates":
				f["components"] = map[string]string{"raw": "-20.12345, -50.12345"}
			case "unknown_component":
				f["components"].(map[string]any)["email"] = "synthetic@example.invalid"
			case "old_schema":
				r["schema_version"] = "station-assertion-v3"
			case "old_parser":
				m["parser_version"] = namedParserVersion
			case "cnefe_not_admitted":
				pin["source"] = "cnefe"
				pin["profile"] = "cnefe-station-addresses-v1"
			case "coverage_count":
				m["address_coverage"].(map[string]any)["station_count"] = 90000
			}
			streams.Assertions = marshalLines(t, r)
			for _, line := range lines[1:] {
				streams.Assertions = append(streams.Assertions, line...)
				streams.Assertions = append(streams.Assertions, '\n')
			}
			var updated map[string]any
			_ = json.Unmarshal(prepManifest(t, streams), &updated)
			m["outputs"] = updated["outputs"]
			raw, _ = json.Marshal(m)
			if _, err := ValidateBatch(raw, streams); err == nil {
				t.Fatal("unbound v4 evidence admitted")
			}
		})
	}
}
