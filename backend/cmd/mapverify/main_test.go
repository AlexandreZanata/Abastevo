package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/mapcontract"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func hash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func manifestJSON(t *testing.T, regionPath string) []byte {
	t.Helper()
	manifest := mapcontract.NativeManifest{
		Format:     mapcontract.ManifestFormat,
		Version:    mapcontract.Version,
		Generation: "native-test-1",
		Engine: mapcontract.Engine{
			Name: "comaps", UpstreamSHA: "4587b8ad4d12c9f59982709fdaf03cc925948827",
			GeneratorSHA: "4587b8ad4d12c9f59982709fdaf03cc925948827", FormatVersion: "261004",
		},
		Region: mapcontract.Region{
			ID: "br-pr-curitiba", Name: "Curitiba", Overview: "world",
			BBox: mapcontract.BBox{MinLonDeg: -49.5, MinLatDeg: -25.7, MaxLonDeg: -49.0, MaxLatDeg: -25.2},
		},
		Packages: []mapcontract.Package{
			{ID: "br-pr-curitiba", Role: "REGION", Path: regionPath, Bytes: 8, SHA256: hash("region!!")},
			{ID: "world", Role: "OVERVIEW", Path: "261004/World.mwm", Bytes: 7, SHA256: hash("world!!")},
		},
		Dependencies: []string{"world"},
		Sources: []mapcontract.Source{
			{ID: "osm-curitiba", License: "ODbL-1.0", Attribution: "© OpenStreetMap contributors", SHA256: hash("source")},
		},
		Rights: mapcontract.ManifestRights{
			License: "ODbL-1.0", Attribution: "© OpenStreetMap contributors",
			Commercial: true, Offline: true, Redistribution: true,
		},
		GeneratedAt: "2026-10-24T12:00:00Z",
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func fixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	write(t, filepath.Join(root, "261004", "br-pr-curitiba.mwm"), "region!!")
	write(t, filepath.Join(root, "261004", "World.mwm"), "world!!")
	manifestPath := filepath.Join(root, "manifest.json")
	if err := os.WriteFile(manifestPath, manifestJSON(t, "261004/br-pr-curitiba.mwm"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, manifestPath
}

func TestVerifyAcceptsCompletePackageSet(t *testing.T) {
	root, manifest := fixture(t)
	out, err := verify(manifest, root)
	if err != nil {
		t.Fatal(err)
	}
	if out.Packages != 2 || out.Bytes != 15 {
		t.Fatalf("result %+v", out)
	}
}

func TestVerifyRejectsCorruptMissingAndTraversalPackages(t *testing.T) {
	root, manifest := fixture(t)
	if err := os.WriteFile(filepath.Join(root, "261004", "World.mwm"), []byte("wrong!!"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := verify(manifest, root); err == nil {
		t.Fatal("corrupt package accepted")
	}
	root, manifest = fixture(t)
	if err := os.Remove(filepath.Join(root, "261004", "World.mwm")); err != nil {
		t.Fatal(err)
	}
	if _, err := verify(manifest, root); err == nil {
		t.Fatal("missing package accepted")
	}
	if _, err := safeJoin(root, "../escape.mwm"); err == nil {
		t.Fatal("traversal accepted")
	}
	if _, err := safeJoin(root, "/absolute.mwm"); err == nil {
		t.Fatal("absolute path accepted")
	}
}
