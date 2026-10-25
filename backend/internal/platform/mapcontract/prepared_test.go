package mapcontract

import (
	"encoding/json"
	"testing"
)

func validPreparedEnvelope(t *testing.T, assertions []byte) []byte {
	t.Helper()
	corpus, err := ParseAssertions(assertions)
	if err != nil {
		t.Fatal(err)
	}
	input := map[string]any{
		"file":   "poi-assertions-v1.json",
		"bytes":  len(assertions),
		"sha256": sha256Hex(assertions),
		"roots":  len(corpus.Assertions),
	}
	artifacts := map[string]any{
		"assertions": map[string]any{
			"bytes":   len(assertions),
			"records": len(corpus.Assertions),
			"sha256":  sha256Hex(assertions),
		},
	}
	doc := map[string]any{
		"format":          PreparedFormat,
		"version":         Version,
		"product_class":   "PUBLIC_OVERLAY",
		"visibility":      "PUBLIC",
		"profile_id":      "map-overlay-test-v1",
		"profile_sha256":  repeat('a', 64),
		"rights_sha256":   repeat('b', 64),
		"source_revision": corpus.Generation,
		"inputs":          []any{input},
		"artifacts":       artifacts,
		"counts": map[string]any{
			"input":       len(corpus.Assertions),
			"accepted":    len(corpus.Assertions),
			"duplicates":  0,
			"quarantined": 0,
		},
	}
	raw, err := canonicalJSON(doc)
	if err != nil {
		t.Fatal(err)
	}
	withHash := map[string]any{}
	if err := json.Unmarshal(raw, &withHash); err != nil {
		t.Fatal(err)
	}
	withHash["content_sha256"] = sha256Hex(raw)
	out, err := canonicalJSON(withHash)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func repeat(c byte, n int) string {
	out := make([]byte, n)
	for i := range out {
		out[i] = c
	}
	return string(out)
}

func TestParsePreparedOverlay(t *testing.T) {
	assertions := fixture(t, "poi-assertions-v1.json")
	envelope := validPreparedEnvelope(t, assertions)
	parsed, corpus, err := ParsePreparedOverlay(envelope, assertions)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.SourceRevision != corpus.Generation || len(corpus.Assertions) != 5 {
		t.Fatalf("unexpected parsed overlay: %+v %d", parsed, len(corpus.Assertions))
	}
}

func TestParsePreparedOverlayRejectsMutations(t *testing.T) {
	assertions := fixture(t, "poi-assertions-v1.json")
	base := validPreparedEnvelope(t, assertions)
	cases := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"wrong-product-class", func(d map[string]any) { d["product_class"] = "REFERENCE" }},
		{"wrong-visibility", func(d map[string]any) { d["visibility"] = "PRIVATE" }},
		{"wrong-artifact-hash", func(d map[string]any) {
			d["artifacts"].(map[string]any)["assertions"].(map[string]any)["sha256"] = repeat('0', 64)
		}},
		{"wrong-records", func(d map[string]any) {
			d["artifacts"].(map[string]any)["assertions"].(map[string]any)["records"] = 4
		}},
		{"wrong-counts", func(d map[string]any) { d["counts"].(map[string]any)["accepted"] = 4 }},
		{"wrong-generation", func(d map[string]any) { d["source_revision"] = "other-generation" }},
		{"unsafe-input", func(d map[string]any) {
			d["inputs"].([]any)[0].(map[string]any)["file"] = "../escape.json"
		}},
		{"missing-artifact", func(d map[string]any) { delete(d["artifacts"].(map[string]any), "assertions") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var doc map[string]any
			if err := json.Unmarshal(base, &doc); err != nil {
				t.Fatal(err)
			}
			tc.mutate(doc)
			raw, err := canonicalJSON(doc)
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := ParsePreparedOverlay(raw, assertions); err == nil {
				t.Fatal("mutated overlay accepted")
			}
		})
	}
	if _, _, err := ParsePreparedOverlay(base, assertions[:100]); err == nil {
		t.Fatal("truncated assertions accepted")
	}
}
