package mapdist

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/mapcontract"
)

func testManifest(t *testing.T, packageBytes []byte) []byte {
	t.Helper()
	sum := sha256.Sum256(packageBytes)
	manifest := mapcontract.NativeManifest{
		Format:     mapcontract.ManifestFormat,
		Version:    1,
		Generation: "map-test-1",
		Engine: mapcontract.Engine{
			Name: "comaps", UpstreamSHA: "4587b8ad4d12c9f59982709fdaf03cc925948827",
			GeneratorSHA: "4587b8ad4d12c9f59982709fdaf03cc925948827", FormatVersion: "261004",
		},
		Region: mapcontract.Region{
			ID: "brazil-parana-east", Name: "Parana East", Overview: "world",
			BBox: mapcontract.BBox{MinLonDeg: -50, MinLatDeg: -26, MaxLonDeg: -48, MaxLatDeg: -24},
		},
		Packages: []mapcontract.Package{{
			ID: "brazil-parana-east", Role: "REGION", Path: "brazil-parana-east.mwm",
			Bytes: int64(len(packageBytes)), SHA256: hex.EncodeToString(sum[:]),
		}},
		Dependencies: []string{},
		Sources: []mapcontract.Source{{
			ID: "osm-curitiba", License: "ODbL-1.0", Attribution: "© OpenStreetMap contributors",
			SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		}},
		Rights: mapcontract.ManifestRights{
			License: "ODbL-1.0", Attribution: "© OpenStreetMap contributors",
			Commercial: true, Offline: true, Redistribution: true,
		},
		GeneratedAt: "2026-10-24T00:00:00Z",
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mapcontract.ParseNativeManifest(raw); err != nil {
		t.Fatal(err)
	}
	return raw
}

func testDistribution(t *testing.T) (*Distribution, ed25519.PublicKey, ed25519.PrivateKey, string) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	packageBytes := []byte("fake-mwm-package-bytes")
	manifest := testManifest(t, packageBytes)
	envelope, err := Sign(manifest, "test-key", privateKey)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "distribution.json"), envelope, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "brazil-parana-east.mwm"), packageBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	dist, err := Load(dir, map[string]ed25519.PublicKey{"test-key": publicKey})
	if err != nil {
		t.Fatal(err)
	}
	return dist, publicKey, privateKey, dir
}

func TestDistributionHandlerManifestAndRange(t *testing.T) {
	dist, _, _, _ := testDistribution(t)
	router := chi.NewRouter()
	Handler{Dist: dist}.RegisterRoutes(router)
	server := httptest.NewServer(router)
	defer server.Close()

	resp, err := http.Get(server.URL + "/v1/map/regions/brazil-parana-east/manifest")
	if err != nil {
		t.Fatal(err)
	}
	body := make([]byte, 4096)
	n, _ := resp.Body.Read(body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || n == 0 {
		t.Fatalf("manifest status %d bytes %d", resp.StatusCode, n)
	}
	etag := resp.Header.Get("ETag")
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/map/regions/brazil-parana-east/manifest", nil)
	req.Header.Set("If-None-Match", etag)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotModified {
		t.Fatalf("conditional manifest status %d", resp.StatusCode)
	}

	req, _ = http.NewRequest(http.MethodGet, server.URL+"/v1/map/packages/brazil-parana-east", nil)
	req.Header.Set("Range", "bytes=0-3")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body = make([]byte, 8)
	n, _ = resp.Body.Read(body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent || string(body[:n]) != "fake" {
		t.Fatalf("range status %d body %q", resp.StatusCode, body[:n])
	}
	changed := []byte("FAKE-mwm-package-bytes")
	if err := os.WriteFile(dist.Files["brazil-parana-east"], changed, 0o644); err != nil {
		t.Fatal(err)
	}
	resp, err = http.Get(server.URL + "/v1/map/packages/brazil-parana-east")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("mutated package status %d, want 409", resp.StatusCode)
	}
}

func TestDistributionRejectsTamperingAndUnsafePackages(t *testing.T) {
	dist, publicKey, _, dir := testDistribution(t)
	envelope, err := os.ReadFile(filepath.Join(dir, "distribution.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(envelope, &doc); err != nil {
		t.Fatal(err)
	}
	doc["signature"] = base64.RawURLEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))
	tampered, _ := json.Marshal(doc)
	if _, _, err := Verify(tampered, map[string]ed25519.PublicKey{"test-key": publicKey}); err == nil {
		t.Fatal("tampered signature accepted")
	}
	if _, err := Load(dir, map[string]ed25519.PublicKey{"other": publicKey}); err == nil {
		t.Fatal("untrusted key accepted")
	}
	if err := os.WriteFile(filepath.Join(dir, "brazil-parana-east.mwm"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir, map[string]ed25519.PublicKey{"test-key": publicKey}); err == nil {
		t.Fatal("changed package accepted")
	}
	if err := os.Remove(filepath.Join(dir, "brazil-parana-east.mwm")); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir, map[string]ed25519.PublicKey{"test-key": publicKey}); err == nil {
		t.Fatal("missing package accepted")
	}
	if _, err := ParsePublicKeys("test-key:" + base64.RawURLEncoding.EncodeToString(publicKey)); err != nil {
		t.Fatal(err)
	}
	if _, err := ParsePublicKeys("bad"); err == nil {
		t.Fatal("bad key list accepted")
	}
	_ = dist
}
