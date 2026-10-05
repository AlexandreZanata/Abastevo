package registry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// fixtureManifest mirrors contracts/testdata/registry/manifest.json.
type fixtureManifest struct {
	Files []struct {
		Path string `json:"path"`
	} `json:"files"`
}

func registryDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller unavailable")
	}
	// backend/internal/modules/directory/adapters/registry -> repo root.
	root := filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "..", "..")
	dir := filepath.Join(root, "contracts", "testdata", "registry")
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("registry fixtures missing: %v", err)
	}
	return dir
}

// FixtureManifest returns the manifest-listed fixture paths for
// contract tests. It fails the test when the manifest is unreadable so
// a missing manifest can never silently pass.
func FixtureManifest(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(registryDir(t), "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest fixtureManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	paths := make([]string, 0, len(manifest.Files))
	for _, entry := range manifest.Files {
		if entry.Path == "" {
			t.Fatal("manifest entry without path")
		}
		paths = append(paths, entry.Path)
	}
	return paths
}

// ReadFixture returns one manifest-listed fixture body. Unknown paths
// fail the test; fixtures outside the manifest are never silently read.
func ReadFixture(t *testing.T, path string) string {
	t.Helper()
	allowed := false
	for _, entry := range FixtureManifest(t) {
		if entry == path {
			allowed = true
			break
		}
	}
	if !allowed {
		t.Fatalf("fixture %q not in manifest", path)
	}
	if filepath.Base(path) != path || path == "manifest.json" {
		t.Fatalf("fixture %q is not a listed data file", path)
	}
	raw, err := os.ReadFile(filepath.Join(registryDir(t), filepath.Clean(path)))
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return string(raw)
}
