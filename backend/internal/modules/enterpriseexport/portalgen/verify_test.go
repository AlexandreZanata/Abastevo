package portalgen

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
)

func refreshPortalManifest(t *testing.T, dir string) {
	t.Helper()
	var manifest Manifest
	if err := json.Unmarshal([]byte(readPortal(t, dir, "portal-manifest.json")), &manifest); err != nil {
		t.Fatal(err)
	}
	var files []FileDigest
	pages := 0
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "portal-manifest.json" {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(body)
		files = append(files, FileDigest{Path: rel, SHA256: "sha256:" + hex.EncodeToString(sum[:]), Bytes: int64(len(body))})
		if strings.HasSuffix(rel, ".html") {
			pages++
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	manifest.Files = files
	manifest.Pages = pages
	content, err := manifestContentDigest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifest.ContentSHA256 = content
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "portal-manifest.json"), append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyNoindexArtifactAndSitemap(t *testing.T) {
	snapshot := writeFixture(t)
	noindex := filepath.Join(t.TempDir(), "noindex")
	generated, err := Generate(Options{SnapshotDir: snapshot, OutputDir: noindex})
	if err != nil {
		t.Fatal(err)
	}
	verified, err := Verify(noindex, VerifyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if verified.Pages != generated.Pages || verified.Files != generated.Files || verified.Bytes != generated.Bytes || verified.ContentSHA256 != generated.ContentSHA256 {
		t.Fatalf("verify result = %+v, generate = %+v", verified, generated)
	}
	if verified.SitemapURLs != 0 || verified.LargestHTMLGzipBytes <= 0 || verified.LargestScriptGzipBytes != 0 || verified.LargestShardGzipBytes <= 0 || verified.InitialViewGzipBytes <= 0 {
		t.Fatalf("unexpected noindex metrics = %+v", verified)
	}
	if _, err := os.Stat(filepath.Join(noindex, "sitemap.xml")); !os.IsNotExist(err) {
		t.Fatal("noindex artifact has a sitemap")
	}

	indexable := filepath.Join(t.TempDir(), "indexable")
	if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: indexable, Indexable: true, SiteURL: "https://portal.example"}); err != nil {
		t.Fatal(err)
	}
	verified, err = Verify(indexable, VerifyOptions{SiteURL: "https://portal.example"})
	if err != nil {
		t.Fatal(err)
	}
	if verified.SitemapURLs != 16 {
		t.Fatalf("sitemap URLs = %d, want 16", verified.SitemapURLs)
	}
	sitemap := readPortal(t, indexable, "sitemap.xml")
	for _, want := range []string{
		"<loc>https://portal.example/consultar/</loc>",
		"<loc>https://portal.example/precos/uf/SP/municipio/3550308/</loc>",
		"<loc>https://portal.example/postos/" + stA + "/</loc>",
		"<loc>https://portal.example/metodologia/</loc>",
	} {
		if !strings.Contains(sitemap, want) {
			t.Errorf("sitemap missing %q", want)
		}
	}
	if strings.Contains(sitemap, "404.html") || strings.Contains(sitemap, "fiscalizacao") {
		t.Error("sitemap lists an unavailable page")
	}
	robots := readPortal(t, indexable, "robots.txt")
	if !strings.Contains(robots, "Allow: /") || !strings.Contains(robots, "Sitemap: https://portal.example/sitemap.xml") {
		t.Errorf("robots = %q", robots)
	}
	if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: filepath.Join(t.TempDir(), "invalid"), Indexable: true}); err == nil {
		t.Fatal("indexable output without site URL accepted")
	}
}

func TestVerifyReproducibleBuildsAndTampering(t *testing.T) {
	snapshot := writeFixture(t)
	first := filepath.Join(t.TempDir(), "first")
	second := filepath.Join(t.TempDir(), "second")
	options := Options{SnapshotDir: snapshot, Indexable: true, SiteURL: "https://portal.example"}
	options.OutputDir = first
	if _, err := Generate(options); err != nil {
		t.Fatal(err)
	}
	options.OutputDir = second
	if _, err := Generate(options); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(first, VerifyOptions{SiteURL: "https://portal.example", CompareDir: second}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(second, "consultar/index.html"), []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(first, VerifyOptions{SiteURL: "https://portal.example", CompareDir: second}); err == nil {
		t.Fatal("tampered comparison accepted")
	}
}

func TestVerifyRejectsTamperingBudgetsAndPrivateFields(t *testing.T) {
	snapshot := writeFixture(t)
	base := filepath.Join(t.TempDir(), "base")
	if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: base}); err != nil {
		t.Fatal(err)
	}
	t.Run("file hash", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "hash")
		if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: out}); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(out, "consultar/index.html")
		body, _ := os.ReadFile(path)
		if err := os.WriteFile(path, append(body, 'x'), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Verify(out, VerifyOptions{}); err == nil {
			t.Fatal("tampered file accepted")
		}
	})
	t.Run("extra file", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "extra")
		if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: out}); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(out, "extra.txt"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Verify(out, VerifyOptions{}); err == nil {
			t.Fatal("unlisted file accepted")
		}
	})
	t.Run("symlink", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "symlink")
		if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: out}); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("consultar/index.html", filepath.Join(out, "link.html")); err != nil {
			t.Fatal(err)
		}
		if _, err := Verify(out, VerifyOptions{}); err == nil {
			t.Fatal("symlink accepted")
		}
	})
	t.Run("html budget", func(t *testing.T) {
		if _, err := Verify(base, VerifyOptions{MaxHTMLGzipBytes: 1}); err == nil {
			t.Fatal("HTML budget not enforced")
		}
	})
	t.Run("file budget", func(t *testing.T) {
		if _, err := Verify(base, VerifyOptions{MaxFiles: 1}); err == nil {
			t.Fatal("file budget not enforced")
		}
	})
	t.Run("byte budget", func(t *testing.T) {
		if _, err := Verify(base, VerifyOptions{MaxBytes: 1}); err == nil {
			t.Fatal("byte budget not enforced")
		}
	})
	t.Run("private JSON key", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "privatejson")
		if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: out}); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(out, "dados/busca/fixture/SP.json")
		var shard map[string]any
		if err := json.Unmarshal([]byte(readPortal(t, out, "dados/busca/fixture/SP.json")), &shard); err != nil {
			t.Fatal(err)
		}
		shard["cpf"] = "private"
		writeJSON(t, path, shard)
		refreshPortalManifest(t, out)
		if _, err := Verify(out, VerifyOptions{}); err == nil || !strings.Contains(err.Error(), "private JSON key") {
			t.Fatalf("private key not refused: %v", err)
		}
	})
	t.Run("broken link", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "broken")
		if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: out}); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(out, "consultar/index.html")
		body, _ := os.ReadFile(path)
		body = bytes.Replace(body, []byte("</main>"), []byte(`<a href="/missing/">missing</a></main>`), 1)
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatal(err)
		}
		refreshPortalManifest(t, out)
		if _, err := Verify(out, VerifyOptions{}); err == nil || !strings.Contains(err.Error(), "broken link") {
			t.Fatalf("broken link not refused: %v", err)
		}
	})
	t.Run("csv allowlist", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "csv")
		if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: out}); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(out, "dados/fixture/estacoes.csv")
		body, _ := os.ReadFile(path)
		body = bytes.Replace(body, []byte("station_id,"), []byte("cpf,station_id,"), 1)
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatal(err)
		}
		refreshPortalManifest(t, out)
		if _, err := Verify(out, VerifyOptions{}); err == nil {
			t.Fatal("private CSV header accepted")
		}
	})
}

func TestVerifyRejectsNonContentAddressedAssetsAndMixedRelease(t *testing.T) {
	snapshot := writeFixture(t)
	t.Run("asset", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "asset")
		if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: out}); err != nil {
			t.Fatal(err)
		}
		matches, err := filepath.Glob(filepath.Join(out, "assets", "portal.*.css"))
		if err != nil || len(matches) != 1 {
			t.Fatalf("css = %v %v", matches, err)
		}
		oldURL := "/assets/" + filepath.Base(matches[0])
		if err := os.Rename(matches[0], filepath.Join(out, "assets", "portal.css")); err != nil {
			t.Fatal(err)
		}
		err = filepath.Walk(out, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".html") {
				return err
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			return os.WriteFile(path, bytes.ReplaceAll(body, []byte(oldURL), []byte("/assets/portal.css")), 0o600)
		})
		if err != nil {
			t.Fatal(err)
		}
		refreshPortalManifest(t, out)
		if _, err := Verify(out, VerifyOptions{}); err == nil || !strings.Contains(err.Error(), "content-addressed") {
			t.Fatalf("non-content-addressed asset accepted: %v", err)
		}
	})
	t.Run("release", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "release")
		if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: out}); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(out, "consultar/index.html")
		body, _ := os.ReadFile(path)
		body = bytes.Replace(body, []byte(`data-release="fixture"`), []byte(`data-release="other"`), 1)
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatal(err)
		}
		refreshPortalManifest(t, out)
		if _, err := Verify(out, VerifyOptions{}); err == nil || !strings.Contains(err.Error(), "search release") {
			t.Fatalf("mixed release accepted: %v", err)
		}
	})
}

func TestHostileSourceTextIsEscapedAndVerified(t *testing.T) {
	snapshot := writeFixtureWith(t, func(stations *[]enterpriseexport.Station, prices *[]enterpriseexport.Price) {
		(*stations)[0].DisplayName = `<script>alert("x")</script>`
		(*stations)[0].OperationStatus = `"><img src=x onerror=alert(1)>`
	})
	writeOfficialFixture(t, snapshot)
	payloadPath := filepath.Join(snapshot, "official-records.jsonl")
	raw, err := os.ReadFile(payloadPath)
	if err != nil {
		t.Fatal(err)
	}
	var records []officialRecord
	for _, line := range strings.Split(strings.TrimRight(string(raw), "\n"), "\n") {
		var record officialRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	records[0].Status = `<script>alert("status")</script>`
	records[0].ProceedingRef = `"><img src=x onerror=alert(2)>`
	var payload bytes.Buffer
	encoder := json.NewEncoder(&payload)
	encoder.SetEscapeHTML(false)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(payloadPath, payload.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	var official officialManifest
	if err := json.Unmarshal([]byte(readPortal(t, snapshot, "official-records-manifest.json")), &official); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload.Bytes())
	official.PayloadSHA256 = hex.EncodeToString(sum[:])
	writeJSON(t, filepath.Join(snapshot, "official-records-manifest.json"), official)

	out := filepath.Join(t.TempDir(), "hostile")
	if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: out}); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(out, VerifyOptions{}); err != nil {
		t.Fatal(err)
	}
	station := readPortal(t, out, "postos/"+stA+"/index.html")
	record := readPortal(t, out, "fiscalizacao/registro/"+officialRecordID("f1")+"/index.html")
	index := readPortal(t, out, "fiscalizacao/index.html")
	for _, page := range []string{station, record, index} {
		lower := strings.ToLower(page)
		if strings.Contains(lower, `<script>alert`) || strings.Contains(lower, "<img src=x onerror") || strings.Contains(lower, "javascript:alert") {
			t.Fatal("hostile source text was not escaped")
		}
	}
	if !strings.Contains(station, "&lt;script&gt;") || !strings.Contains(record, "&lt;script&gt;") {
		t.Fatal("escaped hostile text missing")
	}
}

func TestVerifyOfficialSitemapCensus(t *testing.T) {
	snapshot := writeFixture(t)
	writeOfficialFixture(t, snapshot)
	out := filepath.Join(t.TempDir(), "official-indexable")
	if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: out, Indexable: true, SiteURL: "https://portal.example"}); err != nil {
		t.Fatal(err)
	}
	verified, err := Verify(out, VerifyOptions{SiteURL: "https://portal.example"})
	if err != nil {
		t.Fatal(err)
	}
	if verified.SitemapURLs != 20 {
		t.Fatalf("official sitemap URLs = %d, want 20", verified.SitemapURLs)
	}
	sitemap := readPortal(t, out, "sitemap.xml")
	for _, want := range []string{
		"<loc>https://portal.example/fiscalizacao/</loc>",
		"<loc>https://portal.example/fiscalizacao/registro/" + officialRecordID("f1") + "/</loc>",
		"<loc>https://portal.example/fiscalizacao/registro/" + officialRecordID("f2") + "/</loc>",
		"<loc>https://portal.example/fiscalizacao/registro/" + officialRecordID("f3") + "/</loc>",
	} {
		if !strings.Contains(sitemap, want) {
			t.Errorf("official sitemap missing %q", want)
		}
	}
	if strings.Contains(sitemap, "/fiscalizacao/pagina/") {
		t.Error("single-page official index emitted pagination")
	}
}

func TestVerifyOfficialOnlyFileAllowlist(t *testing.T) {
	newOfficialOnly := func(t *testing.T) string {
		t.Helper()
		snapshot := writeFixture(t)
		writeOfficialFixture(t, snapshot)
		out := filepath.Join(t.TempDir(), "official-only")
		if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: out, OfficialOnly: true}); err != nil {
			t.Fatal(err)
		}
		if _, err := Verify(out, VerifyOptions{OfficialOnly: true}); err != nil {
			t.Fatal(err)
		}
		return out
	}
	addManifestedFile := func(t *testing.T, out, path string, data []byte) {
		t.Helper()
		full := filepath.Join(out, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, data, 0o600); err != nil {
			t.Fatal(err)
		}
		manifest, err := readPortalManifest(out)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		manifest.Files = append(manifest.Files, FileDigest{Path: path, SHA256: fmt.Sprintf("sha256:%x", sum), Bytes: int64(len(data))})
		sort.Slice(manifest.Files, func(i, j int) bool { return manifest.Files[i].Path < manifest.Files[j].Path })
		manifest.ContentSHA256, err = manifestContentDigest(manifest)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(out, "portal-manifest.json"), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name string
		path string
		data []byte
	}{
		{"root price CSV", "price-export.csv", []byte("amount_milli_brl\n5990\n")},
		{"nested data CSV", "dados/precos.csv", []byte("amount_milli_brl\n5990\n")},
		{"unexpected script root", "static/extra.js", []byte("console.log(1)\n")},
		{"export JSONL copy", "official-records-copy.jsonl", []byte("{}\n")},
		{"export manifest copy", "official-records-manifest-copy.json", []byte("{}\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := newOfficialOnly(t)
			addManifestedFile(t, out, tc.path, tc.data)
			if _, err := Verify(out, VerifyOptions{OfficialOnly: true}); err == nil {
				t.Fatalf("official-only verifier accepted manifested %s", tc.path)
			}
		})
	}
	t.Run("mixed mode full artifact", func(t *testing.T) {
		snapshot := writeFixture(t)
		writeOfficialFixture(t, snapshot)
		out := filepath.Join(t.TempDir(), "full")
		if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: out}); err != nil {
			t.Fatal(err)
		}
		if _, err := Verify(out, VerifyOptions{OfficialOnly: true}); err == nil {
			t.Fatal("official-only verifier accepted a full portal artifact")
		}
	})
	t.Run("mixed mode official-only artifact", func(t *testing.T) {
		out := newOfficialOnly(t)
		if _, err := Verify(out, VerifyOptions{}); err == nil {
			t.Fatal("full verifier accepted an official-only artifact")
		}
	})
}
