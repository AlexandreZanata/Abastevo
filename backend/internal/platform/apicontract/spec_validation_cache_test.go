package apicontract

import (
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

const specCacheFixture = `{
  "openapi": "3.1.0",
  "info": {"title": "Synthetic cache fixture", "version": "1.0.0"},
  "paths": {},
  "components": {"schemas": {"ItemReference": {"$ref": "#/components/schemas/Item"}, "Item": {
    "type": "object", "required": ["id"],
    "properties": {"id": {"type": "string"}},
    "example": {"id": "known"}
  }}}
}`

func cacheFixtureLocation() *url.URL { return &url.URL{Path: "/synthetic/contracts/openapi.yaml"} }

func TestSpecValidationCacheFreshDocuments(t *testing.T) {
	var cache specValidationCache
	first, err := cache.load([]byte(specCacheFixture), cacheFixtureLocation())
	if err != nil {
		t.Fatal(err)
	}
	first.Info.Title = "Changed only in first caller"
	first.Components.Schemas["Item"].Value.Required[0] = "changed"
	first.Components.Schemas["Item"].Value.Example.(map[string]any)["id"] = 5
	first.Components.Schemas["Item"].Value.Properties["id"].Value.Type = &openapi3.Types{"integer"}

	second, err := cache.load([]byte(specCacheFixture), cacheFixtureLocation())
	if err != nil {
		t.Fatal(err)
	}
	item := second.Components.Schemas["Item"].Value
	if first == second || first.Components.Schemas["Item"].Value == item || second.Info.Title != "Synthetic cache fixture" || item.Required[0] != "id" || item.Example.(map[string]any)["id"] != "known" || !item.Properties["id"].Value.Type.Is("string") || second.Components.Schemas["ItemReference"].Value != item {
		t.Fatal("a caller's nested mutations reached a separately loaded document")
	}
	delete(second.Components.Schemas, "Item")
	if first.Components.Schemas["Item"] == nil {
		t.Fatal("a second caller's component deletion reached the first document")
	}
	if cache.validationRuns != 1 {
		t.Fatalf("unchanged canonical bytes validated %d times, want 1", cache.validationRuns)
	}
}

func TestSpecValidationCacheChangedBytes(t *testing.T) {
	var cache specValidationCache
	location := cacheFixtureLocation()
	if _, err := cache.load([]byte(specCacheFixture), location); err != nil {
		t.Fatal(err)
	}
	invalid := strings.Replace(specCacheFixture, `"id": "known"`, `"id": 5`, 1)
	for attempt := range 2 {
		document, err := cache.load([]byte(invalid), location)
		if document != nil || err == nil || !strings.Contains(err.Error(), "spec self-validation:") {
			t.Fatalf("invalid changed example attempt %d bypassed actual self-validation: %v", attempt, err)
		}
	}
	document, err := cache.load([]byte(specCacheFixture), location)
	if err != nil || document == nil {
		t.Fatalf("valid exact-byte snapshot did not recover after invalid input: %v", err)
	}
	if cache.validationRuns != 2 {
		t.Fatalf("valid and invalid byte snapshots validated %d times, want 2", cache.validationRuns)
	}
	if document, err := cache.load([]byte(`{"openapi":`), location); document != nil || err == nil || !strings.Contains(err.Error(), "load spec:") {
		t.Fatal("malformed bytes reused a previously valid snapshot")
	}
	// A different resolver base is a separate input even when bytes are equal.
	otherLocation := &url.URL{Path: "/synthetic/other/openapi.yaml"}
	if _, err := cache.load([]byte(specCacheFixture), otherLocation); err != nil {
		t.Fatal(err)
	}
	if cache.validationRuns != 3 {
		t.Fatal("changed source location reused another resolver's validation result")
	}
}

func TestSpecValidationCacheConcurrentLoads(t *testing.T) {
	var cache specValidationCache
	const callers = 8
	documents := make(chan *openapi3.T, callers)
	errors := make(chan error, callers)
	var workers sync.WaitGroup
	for range callers {
		workers.Go(func() {
			document, err := cache.load([]byte(specCacheFixture), cacheFixtureLocation())
			if err != nil {
				errors <- err
				return
			}
			documents <- document
		})
	}
	workers.Wait()
	close(documents)
	close(errors)
	for err := range errors {
		t.Error(err)
	}
	seen := make(map[*openapi3.T]bool)
	for document := range documents {
		if seen[document] {
			t.Fatal("concurrent callers shared a mutable document")
		}
		seen[document] = true
		document.Info.Title = "changed after independent load"
	}
	if len(seen) != callers || cache.validationRuns != 1 {
		t.Fatalf("concurrent inputs produced %d independent documents and %d validations, want %d and 1", len(seen), cache.validationRuns, callers)
	}
}
