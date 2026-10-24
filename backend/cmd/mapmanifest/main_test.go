package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/mapcontract"
)

func file(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func baseConfig(t *testing.T, dir string) config {
	t.Helper()
	return config{
		output:        filepath.Join(dir, "manifest.json"),
		generation:    "native-test-1",
		engineSHA:     "4587b8ad4d12c9f59982709fdaf03cc925948827",
		generatorSHA:  "4587b8ad4d12c9f59982709fdaf03cc925948827",
		formatVersion: "261004",
		regionID:      "br-pr-curitiba",
		regionName:    "Curitiba",
		overview:      "world",
		rightsLicense: "ODbL-1.0",
		attribution:   "© OpenStreetMap contributors — ODbL 1.0",
		generatedAt:   "2026-10-24T12:00:00Z",
		bbox:          bboxArg{minLon: -49.5, minLat: -25.7, maxLon: -49.0, maxLat: -25.2},
		packages: []string{
			"br-pr-curitiba=REGION=261004/br-pr-curitiba.mwm=" + file(t, dir, "region.mwm", "region-bytes"),
			"world=OVERVIEW=261004/World.mwm=" + file(t, dir, "world.mwm", "world-bytes"),
		},
		dependencies: []string{"world"},
		sources: []string{
			"osm-curitiba=ODbL-1.0=" + file(t, dir, "Curitiba.osm.pbf", "osm-bytes"),
		},
	}
}

func TestBuildManifestHashesPackagesAndSources(t *testing.T) {
	dir := t.TempDir()
	raw, err := build(baseConfig(t, dir))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := mapcontract.ParseNativeManifest(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Packages) != 2 || manifest.Packages[0].Bytes != int64(len("region-bytes")) {
		t.Fatalf("packages %+v", manifest.Packages)
	}
	if len(manifest.Packages[0].SHA256) != 64 ||
		manifest.Packages[0].SHA256 == manifest.Packages[1].SHA256 {
		t.Fatalf("package digests %q %q", manifest.Packages[0].SHA256, manifest.Packages[1].SHA256)
	}
	if len(manifest.Sources) != 1 || manifest.Sources[0].License != "ODbL-1.0" {
		t.Fatalf("sources %+v", manifest.Sources)
	}
	if manifest.Dependencies[0] != "world" || manifest.Rights.Commercial != true {
		t.Fatalf("manifest %+v", manifest)
	}
}

func TestBuildRejectsMissingDependencyAndUnsafePath(t *testing.T) {
	dir := t.TempDir()
	config := baseConfig(t, dir)
	config.dependencies = []string{"missing"}
	if _, err := build(config); err == nil {
		t.Fatal("missing dependency accepted")
	}
	config = baseConfig(t, dir)
	config.packages[0] = "br-pr-curitiba=REGION=../escape.mwm=" + file(t, dir, "region.mwm", "region-bytes")
	if _, err := build(config); err == nil {
		t.Fatal("unsafe path accepted")
	}
	config = baseConfig(t, dir)
	config.sources[0] = "osm-curitiba=GPL-3.0=" + file(t, dir, "Curitiba.osm.pbf", "osm-bytes")
	if _, err := build(config); err == nil {
		t.Fatal("unadmitted licence accepted")
	}
}

func TestWriteAtomicPreservesPreviousOnRefusal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomic(path, []byte("new"), false); err == nil {
		t.Fatal("existing output overwritten without --replace")
	}
	if got, _ := os.ReadFile(path); string(got) != "old" {
		t.Fatalf("previous output changed: %q", got)
	}
	if err := writeAtomic(path, []byte("new"), true); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "new" {
		t.Fatalf("replacement failed: %q", got)
	}
}

func TestRunWritesValidatedManifest(t *testing.T) {
	dir := t.TempDir()
	config := baseConfig(t, dir)
	args := []string{
		"--out", config.output, "--generation", config.generation,
		"--engine-sha", config.engineSHA, "--generator-sha", config.generatorSHA,
		"--format-version", config.formatVersion, "--region-id", config.regionID,
		"--region-name", config.regionName, "--rights-license", config.rightsLicense,
		"--attribution", config.attribution, "--generated-at", config.generatedAt,
		"--bbox", "-49.5,-25.7,-49.0,-25.2",
		"--package", config.packages[0], "--package", config.packages[1],
		"--dependency", "world", "--source", config.sources[0],
	}
	if err := run(args); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(config.output)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mapcontract.ParseNativeManifest(raw); err != nil {
		t.Fatal(err)
	}
}
