// Package apicontract validates the golden API vectors in
// contracts/testdata/api against contracts/openapi/v1.yaml.
//
// Vectors carry their own expectation (valid bool); the suite fails when a
// valid example is rejected or an invalid one is accepted. It also guards
// the A05 vocabulary bridge: the wire enum uses GASOLINE_ADDITIVED, and the
// legacy Android GASOLINE_PREMIUM name must not appear on the wire.
package apicontract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

type vector struct {
	ID     string `json:"id"`
	Schema string `json:"schema"`
	Valid  bool   `json:"valid"`
	Value  any    `json:"value"`
}

func contractsDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "contracts")
}

func loadSpec(t *testing.T) *openapi3.T {
	t.Helper()
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile(filepath.Join(contractsDir(t), "openapi", "v1.yaml"))
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	if err := doc.Validate(loader.Context); err != nil {
		t.Fatalf("spec self-validation: %v", err)
	}
	return doc
}

func TestGoldenVectors(t *testing.T) {
	doc := loadSpec(t)
	matches, err := filepath.Glob(filepath.Join(contractsDir(t), "testdata", "api", "*.json"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("no vectors found: %v", err)
	}
	for _, path := range matches {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", filepath.Base(path), err)
		}
		var v vector
		if err := json.Unmarshal(raw, &v); err != nil {
			t.Fatalf("parse %s: %v", filepath.Base(path), err)
		}
		if v.ID == "" || v.Schema == "" {
			t.Fatalf("%s: vector needs id and schema", filepath.Base(path))
		}
		t.Run(v.ID, func(t *testing.T) {
			ref, ok := doc.Components.Schemas[v.Schema]
			if !ok {
				t.Fatalf("schema %q not in spec", v.Schema)
			}
			err := ref.Value.VisitJSON(v.Value)
			if v.Valid && err != nil {
				t.Errorf("valid vector rejected: %v", err)
			}
			if !v.Valid && err == nil {
				t.Error("invalid vector accepted")
			}
		})
	}
}

func TestFuelProductVocabularyBridge(t *testing.T) {
	doc := loadSpec(t)
	ref, ok := doc.Components.Schemas["FuelProduct"]
	if !ok {
		t.Fatal("FuelProduct schema missing")
	}
	var names []string
	for _, item := range ref.Value.Enum {
		s, ok := item.(string)
		if !ok {
			t.Fatalf("non-string enum entry: %v", item)
		}
		names = append(names, s)
	}
	if !slices.Contains(names, "GASOLINE_ADDITIVED") {
		t.Errorf("wire enum must use GASOLINE_ADDITIVED, got %v", names)
	}
	if slices.Contains(names, "GASOLINE_PREMIUM") {
		t.Errorf("legacy GASOLINE_PREMIUM must not appear on the wire (A05): %v", names)
	}
	if len(names) != 8 {
		t.Errorf("want exactly 8 fuel products, got %v", names)
	}
}

func TestSpecRefsAreLocal(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(contractsDir(t), "openapi", "v1.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "$ref:") && strings.Contains(trimmed, "http") {
			t.Errorf("spec $refs must stay in-repo: %q", trimmed)
		}
	}
}
