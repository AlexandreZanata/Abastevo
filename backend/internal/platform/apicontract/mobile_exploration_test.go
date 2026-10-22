package apicontract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

// This fragment freezes future shapes and reviewed counterexamples only; no
// route, query evaluator, source availability or association is certified here.
func TestMobileExplorationPlannedContract(t *testing.T) {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile(filepath.Join(contractsDir(t), "openapi", "mobile-exploration-planned.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Validate(loader.Context); err != nil {
		t.Fatal(err)
	}
	if doc.Paths.Len() != 0 || doc.Extensions["x-implementation-status"] != "CONTRACT_FROZEN_RUNTIME_PENDING" {
		t.Fatal("contract task must not invent live endpoints")
	}
	raw, err := os.ReadFile(filepath.Join(contractsDir(t), "testdata", "mobile-expansion", "exploration-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		Version int      `json:"version"`
		Cases   []vector `json:"cases"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	if data.Version != 1 || len(data.Cases) < 30 {
		t.Fatal("missing reviewed compatibility examples")
	}
	seen := map[string]bool{}
	for _, v := range data.Cases {
		t.Run(v.ID, func(t *testing.T) {
			if v.ID == "" || seen[v.ID] {
				t.Fatal("missing/duplicate ID")
			}
			seen[v.ID] = true
			ref, ok := doc.Components.Schemas[v.Schema]
			if !ok {
				t.Fatalf("unknown schema %s", v.Schema)
			}
			err := ref.Value.VisitJSON(v.Value)
			if (err == nil) != v.Valid {
				t.Fatalf("valid=%v: schema error %v", v.Valid, err)
			}
		})
	}
	for _, id := range []string{"ambiguous-cheapest", "unknown-fuel-refused", "cng-litre-refused", "company-is-not-site", "company-site-claim-refused", "source-unavailable", "unavailable-zero-refused", "empty-not-safe", "unmatched-source-record", "offline-company-last-check", "month-is-not-event-day", "station-without-price", "private-report-refused", "unsupported-filter-explicit"} {
		if !seen[id] {
			t.Errorf("missing critical example %s", id)
		}
	}
}

// Independent set arithmetic for reviewed source examples. Runtime SQL/HTTP
// census, loss accounting and association acceptance belong to MEX-17 onward.
func TestMobileExplorationCountingExamples(t *testing.T) {
	var data struct {
		Version int `json:"version"`
		Cases   []struct {
			ID       string `json:"id"`
			Selected []struct {
				Source, Stream string
				Revision       string `json:"revision_id"`
			} `json:"selected"`
			Observations []struct {
				Source, Stream, Key string
				Revision            string `json:"revision_id"`
			} `json:"observations"`
			Valid    bool `json:"valid"`
			Expected *int `json:"expected_records"`
			Issued   *int `json:"expected_issued_fines"`
		} `json:"cases"`
	}
	raw, err := os.ReadFile(filepath.Join(contractsDir(t), "testdata", "mobile-expansion", "counting-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	if data.Version != 1 || len(data.Cases) != 3 {
		t.Fatal("missing reviewed counting examples")
	}
	seen := map[string]bool{}
	for _, v := range data.Cases {
		t.Run(v.ID, func(t *testing.T) {
			if v.ID == "" || seen[v.ID] {
				t.Fatal("missing/duplicate case")
			}
			seen[v.ID] = true
			selected := map[[2]string]string{}
			valid := true
			for _, s := range v.Selected {
				k := [2]string{s.Source, s.Stream}
				if _, exists := selected[k]; exists {
					valid = false
				}
				selected[k] = s.Revision
			}
			if valid != v.Valid {
				t.Fatal("ambiguous revision selection")
			}
			if !valid {
				if v.Expected != nil {
					t.Fatal("invalid selection cannot carry a count")
				}
				return
			}
			keys := map[[4]string]bool{}
			for _, row := range v.Observations {
				if selected[[2]string{row.Source, row.Stream}] == row.Revision {
					keys[[4]string{row.Source, row.Stream, row.Revision, row.Key}] = true
				}
			}
			if v.Expected == nil || *v.Expected != len(keys) || v.Issued != nil {
				t.Fatal("snapshot duplication, collapsed results or invented issued-fine count")
			}
		})
	}
}
