// Package apicontract validates the golden API vectors in
// contracts/testdata/api against contracts/openapi/v1.yaml.
//
// Vectors carry their own expectation (valid bool); the suite fails when a
// valid example is rejected or an invalid one is accepted. It also guards
// the A05 vocabulary bridge: the wire enum uses GASOLINE_ADDITIVED, and the
// legacy Android GASOLINE_PREMIUM name must not appear on the wire.
package apicontract

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

type specValidationKey struct {
	source string
	digest [sha256.Size]byte
}

type specValidationResult struct {
	documentJSON []byte
	message      string
}

// Cache the validated document as immutable bytes. Every caller gets its own
// parsed document and references, including tests that deliberately mutate it.
type specValidationCache struct {
	mu             sync.Mutex
	results        map[specValidationKey]specValidationResult
	validationRuns int
}

func (cache *specValidationCache) load(raw []byte, location *url.URL) (*openapi3.T, error) {
	snapshot := bytes.Clone(raw)
	source := *location
	key := specValidationKey{source: source.String(), digest: sha256.Sum256(snapshot)}
	cache.mu.Lock()
	result, cached := cache.results[key]
	if cached {
		cache.mu.Unlock()
		if result.message != "" {
			return nil, fmt.Errorf("spec self-validation: %s", result.message)
		}
		document, err := openapi3.NewLoader().LoadFromDataWithPath(result.documentJSON, &source)
		if err != nil {
			return nil, fmt.Errorf("load cached spec: %w", err)
		}
		return document, nil
	}
	defer cache.mu.Unlock()

	loader := openapi3.NewLoader()
	document, err := loader.LoadFromDataWithPath(snapshot, &source)
	if err != nil {
		return nil, fmt.Errorf("load spec: %w", err)
	}
	cache.validationRuns++
	if cache.results == nil {
		cache.results = make(map[specValidationKey]specValidationResult)
	}
	if err := document.Validate(loader.Context); err != nil {
		cache.results[key] = specValidationResult{message: err.Error()}
		return nil, fmt.Errorf("spec self-validation: %s", err)
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("snapshot validated spec: %w", err)
	}
	cache.results[key] = specValidationResult{documentJSON: encoded}
	return document, nil
}

var canonicalSpecValidation specValidationCache

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
	path := filepath.Join(contractsDir(t), "openapi", "v1.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	doc, err := canonicalSpecValidation.load(raw, &url.URL{Path: filepath.ToSlash(path)})
	if err != nil {
		t.Fatal(err)
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
