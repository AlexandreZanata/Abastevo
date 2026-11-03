package registry

import (
	"encoding/json"
	"testing"
)

func namedV3Streams(t *testing.T) (BatchStreams, []byte) {
	t.Helper()
	streams := prepStreams(t)
	lines, err := splitJSONL(streams.Assertions, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	streams.Assertions = nil
	for _, line := range lines {
		var row map[string]any
		_ = json.Unmarshal(line, &row)
		row["schema_version"] = "station-assertion-v3"
		row["published_at"] = row["effective_at"]
		row["brand_linked_at"] = ""
		row["legal_name"] = " [P25-TEST] Legal Name LTDA "
		row["business_name_raw"] = "[P25-TEST] Legal Name LTDA"
		row["business_name_normalized"] = "[P25-TEST] Legal Name LTDA"
		row["trade_name"] = "[P25-TEST] Sourced Trade"
		row["trade_name_provenance"] = map[string]any{"source": "rfb", "profile": "rfb-station-names-v1", "reference": "https://example.invalid/rfb", "edition": "synthetic-2026-11", "sha256": vectorChecksum("names-input"), "observed_at": "2026-10-01"}
		streams.Assertions = append(streams.Assertions, marshalLines(t, row)...)
	}
	var manifest map[string]any
	_ = json.Unmarshal(prepManifest(t, streams), &manifest)
	manifest["parser_version"] = "station-prep-v0.3.1+names-v1"
	manifest["name_inputs"] = []any{map[string]any{"source": "rfb", "profile": "rfb-station-names-v1", "reference": "https://example.invalid/rfb", "edition": "synthetic-2026-11", "sha256": vectorChecksum("names-input"), "bytes": 128, "rows": 1}}
	raw, _ := json.Marshal(manifest)
	return streams, raw
}

func TestStationNamesV3PreservesRawNamesAndBindsPins(t *testing.T) {
	streams, manifest := namedV3Streams(t)
	if _, err := ValidateBatch(manifest, streams); err != nil {
		t.Fatal(err)
	}
	lines, _ := splitJSONL(streams.Assertions, "fixture")
	var row BatchAssertion
	if err := decodeStrict(lines[0], &row, "fixture"); err != nil {
		t.Fatal(err)
	}
	a, err := mapAssertionRow(row)
	if err != nil || a.DisplayName != "[P25-TEST] Sourced Trade" || a.LegalName != " [P25-TEST] Legal Name LTDA " || a.TradeName != "[P25-TEST] Sourced Trade" {
		t.Fatal(a, err)
	}
	var root map[string]any
	_ = json.Unmarshal(manifest, &root)
	root["name_inputs"].([]any)[0].(map[string]any)["sha256"] = vectorChecksum("different-input")
	bad, _ := json.Marshal(root)
	if _, err := ValidateBatch(bad, streams); err == nil {
		t.Fatal("unbound name provenance admitted")
	}
}

func TestStationNamesV3RefusesMalformedNamesAndLegacySmuggling(t *testing.T) {
	for _, fault := range []string{"missing_legal", "legal_mismatch", "missing_trade", "missing_provenance", "empty_with_provenance", "bad_clock", "old_schema", "wrong_parser", "duplicate_pin", "unknown_profile", "oversize", "row_cap"} {
		t.Run(fault, func(t *testing.T) {
			streams, raw := namedV3Streams(t)
			var manifest BatchManifest
			if err := json.Unmarshal(raw, &manifest); err != nil {
				t.Fatal(err)
			}
			lines, _ := splitJSONL(streams.Assertions, "fixture")
			var row BatchAssertion
			if err := decodeStrict(lines[0], &row, "fixture"); err != nil {
				t.Fatal(err)
			}
			switch fault {
			case "legal_mismatch":
				wrong := "[P25-TEST] Wrong Legal"
				row.LegalName = &wrong
			case "missing_legal":
				row.LegalName = nil
			case "missing_trade":
				row.TradeName = nil
			case "missing_provenance":
				row.TradeNameProvenance = nil
			case "empty_with_provenance":
				empty := ""
				row.TradeName = &empty
			case "bad_clock":
				row.TradeNameProvenance.ObservedAt = "2026-02-30"
			case "old_schema":
				row.SchemaVersion = SchemaAssertionV2
			case "wrong_parser":
				manifest.ParserVersion = "station-prep-v0.3.1"
			case "duplicate_pin":
				manifest.NameInputs = append(manifest.NameInputs, manifest.NameInputs[0])
			case "unknown_profile":
				manifest.NameInputs[0].Profile = "unknown"
			case "oversize":
				manifest.NameInputs[0].Bytes = 64<<20 + 1
			case "row_cap":
				manifest.NameInputs[0].Rows = 500001
			}
			streams.Assertions = marshalLines(t, row)
			for _, line := range lines[1:] {
				streams.Assertions = append(streams.Assertions, line...)
				streams.Assertions = append(streams.Assertions, '\n')
			}
			for i, out := range manifest.Outputs {
				if out.Path == PrepAssertionsFile {
					manifest.Outputs[i] = streamOutput(out.Path, streams.Assertions)
				}
			}
			raw, _ = json.Marshal(manifest)
			if _, err := ValidateBatch(raw, streams); err == nil {
				t.Fatal("invalid sourced name accepted")
			}
		})
	}
}
