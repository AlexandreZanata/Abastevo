package apicontract

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Lexical JSON/hash identity belongs to the strict codecs; these checks prove
// the structural DTOs and result catalog agree with the shared wire oracle.
func TestEnterpriseSyncComponents(t *testing.T) {
	doc := loadSpec(t)
	// Exactly the delivered sync routes: the OFFL-002 batch plus the OFFL-008
	// company Meter review and signed own-result endpoints.
	delivered := map[string]bool{
		"/v1/orgs/{org_id}/sync/operations":                                      false,
		"/v1/orgs/{org_id}/sync/review-results":                                  false,
		"/v1/orgs/{org_id}/sync/reviews/meter-readings/list":                     false,
		"/v1/orgs/{org_id}/sync/reviews/meter-readings/{review_item_id}/read":    false,
		"/v1/orgs/{org_id}/sync/reviews/meter-readings/{review_item_id}/resolve": false,
	}
	for path := range doc.Paths.Map() {
		if strings.Contains(path, "/sync") && strings.Contains(path, "/orgs/") {
			if _, ok := delivered[path]; !ok {
				t.Fatal("sync component exposed an undelivered enterprise route", path)
			}
			delivered[path] = true
		}
	}
	for path, found := range delivered {
		if !found {
			t.Fatal("delivered enterprise sync route missing from the contract", path)
		}
	}
	file, err := os.Open(filepath.Join(contractsDir(t), "testdata", "enterprise", "sync", "operations-v1.psv"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 2*1024*1024)
	if !scanner.Scan() || scanner.Text() != "id|operation|args|expected" {
		t.Fatal("bad protocol header")
	}
	var operation, ack map[string]any
	counts := map[string]int{}
	for scanner.Scan() {
		parts := strings.Split(scanner.Text(), "|")
		if len(parts) != 4 {
			t.Fatal("bad protocol row")
		}
		args, expected := syncVectorFields(t, parts[2]), syncVectorFields(t, parts[3])
		if expected["error"] != "" {
			continue
		}
		name, rawHex := "", ""
		switch parts[1] {
		case "SEAL", "VERIFY":
			name, rawHex = "EnterpriseSyncOperation", expected["canonical_hex"]
		case "ACK":
			name, rawHex = "EnterpriseSyncAcknowledgement", args["raw_hex"]
		default:
			continue
		}
		raw, err := hex.DecodeString(rawHex)
		if err != nil || len(raw) == 0 {
			t.Fatal("bad protocol wire sample")
		}
		var value map[string]any
		if err := json.Unmarshal(raw, &value); err != nil {
			t.Fatal(err)
		}
		if doc.Components.Schemas[name] == nil {
			t.Fatal("missing sync component", name)
		}
		if err := doc.Components.Schemas[name].Value.VisitJSON(value); err != nil {
			t.Fatal(parts[0], name, err)
		}
		counts[name]++
		if parts[0] == "seal-base" {
			operation = value
		}
		if expected["result"] == "ACCEPTED" && ack == nil {
			ack = value
		}
	}
	if scanner.Err() != nil || operation == nil || ack == nil || len(counts) != 2 {
		t.Fatal("shared samples not checked", counts, scanner.Err())
	}
	batch := map[string]any{"schema_version": 1, "family_id": "7b0e2a54-0000-4000-8000-000000000013", "access_token": "synthetic-only", "operations": []any{operation}}
	batch = cloneSyncObject(t, batch)
	for name, value := range map[string]map[string]any{"EnterpriseSyncOperation": operation, "EnterpriseSyncBatch": batch, "EnterpriseSyncAcknowledgement": ack} {
		schema := doc.Components.Schemas[name].Value
		if err := schema.VisitJSON(value); err != nil {
			t.Fatal(name, err)
		}
		for _, field := range schema.Required {
			mutated := cloneSyncObject(t, value)
			delete(mutated, field)
			if schema.VisitJSON(mutated) == nil {
				t.Fatal("missing sync field accepted", name, field)
			}
		}
		for _, field := range []string{"final", "contributor_id", "session_id", "device_key"} {
			mutated := cloneSyncObject(t, value)
			mutated[field] = true
			if schema.VisitJSON(mutated) == nil {
				t.Fatal("unknown authority/finality accepted", name, field)
			}
		}
	}
	for _, changes := range []map[string]any{
		{"code": ""}, {"entity_revision": nil}, {"received_at": nil}, {"received_at": "0001-01-01T00:00:00Z"}, {"review_item_id": "7b0e2a54-0000-4000-8000-000000000014"},
		{"result": "REJECTED", "code": "MALFORMED"},
		{"result": "RETRY", "code": "UNAVAILABLE"},
		{"result": "ACCEPTED_FOR_REVIEW", "code": "ASSET_BUSY"},
		{"warnings": []any{"CLOCK_SKEW", "CLOCK_SKEW"}}, {"warnings": []any{"UNKNOWN"}},
	} {
		mutated := cloneSyncObject(t, ack)
		for k, v := range changes {
			mutated[k] = v
		}
		if doc.Components.Schemas["EnterpriseSyncAcknowledgement"].Value.VisitJSON(mutated) == nil {
			t.Fatal("inconsistent result accepted", changes)
		}
	}
	t.Logf("shared structural samples checked: %v", counts)
}

func syncVectorFields(t *testing.T, raw string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, part := range strings.Split(raw, ";") {
		key, value, ok := strings.Cut(part, "=")
		if !ok || key == "" {
			t.Fatal("invalid protocol field")
		}
		if _, duplicate := out[key]; duplicate {
			t.Fatal("duplicate protocol field")
		}
		out[key] = value
	}
	return out
}

func cloneSyncObject(t *testing.T, value map[string]any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	return result
}
