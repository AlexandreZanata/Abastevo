package portalgen

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// readSitemaps concatenates the sitemap index and every child sitemap.
func readSitemaps(t *testing.T, out string) string {
	t.Helper()
	all := readPortal(t, out, "sitemap.xml")
	children, err := filepath.Glob(filepath.Join(out, "sitemaps", "*.xml"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(children)
	for _, child := range children {
		raw, err := os.ReadFile(child)
		if err != nil {
			t.Fatal(err)
		}
		all += string(raw)
	}
	return all
}

func TestBuildSitemapsSplitsFamiliesUnderTheURLLimit(t *testing.T) {
	entries := []sitemapEntry{
		{Route: "/precos/", Family: familyHub, LastMod: "2026-10-18"},
		{Route: "/fiscalizacao/", Family: familyFiscalHub, LastMod: "2026-10-20"},
		{Route: "/precos/mt/", Family: familyState, LastMod: "2026-10-18"},
	}
	for i := 0; i < SitemapChunkURLs+1; i++ {
		day := "2026-09-01"
		if i == SitemapChunkURLs {
			day = "2026-10-15"
		}
		entries = append(entries, sitemapEntry{Route: fmt.Sprintf("/postos/00000000-0000-4000-8000-%012d/", i), Family: familyStation, LastMod: day})
	}
	files, index, err := buildSitemaps("https://abastevo.example", entries, SitemapChunkURLs)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	counts := map[string]int{}
	for _, file := range files {
		paths = append(paths, file.Path)
		var set sitemapSet
		if err := xml.Unmarshal(file.Body, &set); err != nil {
			t.Fatal(err)
		}
		counts[file.Path] = len(set.URLs)
		if int64(len(file.Body)) > MaxSitemapBytes || strings.Contains(string(file.Body), "priority") || strings.Contains(string(file.Body), "changefreq") {
			t.Errorf("%s breaks the size or field contract", file.Path)
		}
	}
	want := []string{"sitemaps/portal-hubs-1.xml", "sitemaps/portal-precos-1.xml", "sitemaps/portal-postos-1.xml", "sitemaps/portal-postos-2.xml"}
	if strings.Join(paths, ",") != strings.Join(want, ",") {
		t.Fatalf("sitemap files = %v, want %v", paths, want)
	}
	if counts["sitemaps/portal-postos-1.xml"] != SitemapChunkURLs || counts["sitemaps/portal-postos-2.xml"] != 1 || counts["sitemaps/portal-hubs-1.xml"] != 2 {
		t.Errorf("sitemap counts = %v", counts)
	}
	var doc sitemapIndexDoc
	if err := xml.Unmarshal(index, &doc); err != nil || doc.XMLName.Local != "sitemapindex" || len(doc.Sitemaps) != 4 {
		t.Fatalf("index = %s (%v)", index, err)
	}
	if doc.Sitemaps[0].Loc != "https://abastevo.example/sitemaps/portal-hubs-1.xml" || doc.Sitemaps[0].LastMod != "2026-10-20" ||
		doc.Sitemaps[2].LastMod != "2026-09-01" || doc.Sitemaps[3].LastMod != "2026-10-15" {
		t.Errorf("index entries = %+v", doc.Sitemaps)
	}
	if _, _, err := buildSitemaps("https://abastevo.example", append(entries, entries[0]), SitemapChunkURLs); err == nil {
		t.Error("duplicate route accepted")
	}
	if _, _, err := buildSitemaps("https://abastevo.example", []sitemapEntry{{Route: "/fiscalizacao/registro/x/", Family: familyFiscalRecord}}, SitemapChunkURLs); err == nil {
		t.Error("non-indexable family accepted")
	}
	if _, _, err := buildSitemaps("https://abastevo.example", nil, SitemapChunkURLs); err == nil {
		t.Error("empty sitemap accepted")
	}
}

func TestGeneratedSitemapIndexAndDataDates(t *testing.T) {
	out := filepath.Join(t.TempDir(), "portal")
	if _, err := Generate(Options{SnapshotDir: writeFixture(t), OutputDir: out, Indexable: true, SiteURL: "https://portal.example"}); err != nil {
		t.Fatal(err)
	}
	index := readPortal(t, out, "sitemap.xml")
	for _, want := range []string{
		"<sitemapindex xmlns=\"http://www.sitemaps.org/schemas/sitemap/0.9\">",
		"<loc>https://portal.example/sitemaps/portal-hubs-1.xml</loc>",
		"<loc>https://portal.example/sitemaps/portal-precos-1.xml</loc>",
		"<loc>https://portal.example/sitemaps/portal-postos-1.xml</loc>",
	} {
		if !strings.Contains(index, want) {
			t.Errorf("sitemap index missing %q", want)
		}
	}
	precos := readPortal(t, out, "sitemaps/portal-precos-1.xml")
	postos := readPortal(t, out, "sitemaps/portal-postos-1.xml")
	hubs := readPortal(t, out, "sitemaps/portal-hubs-1.xml")
	for _, check := range []struct{ body, loc, lastmod string }{
		{precos, "https://portal.example/precos/sp/sao-paulo/", "2026-10-18"},
		{postos, "https://portal.example/postos/" + stA + "/", "2026-10-15"},
		{hubs, "https://portal.example/precos/", "2026-10-18"},
		{hubs, "https://portal.example/metodologia/", "2026-10-24"},
	} {
		if !strings.Contains(check.body, "<loc>"+check.loc+"</loc>\n    <lastmod>"+check.lastmod+"</lastmod>") {
			t.Errorf("%s lacks lastmod %s", check.loc, check.lastmod)
		}
	}
	if _, err := Verify(out, VerifyOptions{SiteURL: "https://portal.example"}); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyRejectsSitemapIndexTampering(t *testing.T) {
	generate := func(t *testing.T) string {
		t.Helper()
		out := filepath.Join(t.TempDir(), "portal")
		if _, err := Generate(Options{SnapshotDir: writeFixture(t), OutputDir: out, Indexable: true, SiteURL: "https://portal.example"}); err != nil {
			t.Fatal(err)
		}
		return out
	}
	rewrite := func(t *testing.T, out, rel, old, new string) {
		t.Helper()
		body := readPortal(t, out, rel)
		if !strings.Contains(body, old) {
			t.Fatalf("%s lacks %q", rel, old)
		}
		if err := os.WriteFile(filepath.Join(out, rel), []byte(strings.Replace(body, old, new, 1)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cases := map[string]func(t *testing.T, out string){
		"index points to a missing child": func(t *testing.T, out string) {
			rewrite(t, out, "sitemap.xml", "portal-postos-1.xml", "portal-postos-9.xml")
		},
		"unreferenced child": func(t *testing.T, out string) {
			if err := os.WriteFile(filepath.Join(out, "sitemaps", "portal-postos-2.xml"), []byte(readPortal(t, out, "sitemaps/portal-postos-1.xml")), 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"child lists a noindex page": func(t *testing.T, out string) {
			rewrite(t, out, "sitemaps/portal-postos-1.xml", "/postos/"+stA+"/", "/postos/"+stB+"/")
		},
		"page is not self-canonical": func(t *testing.T, out string) {
			rewrite(t, out, "metodologia/index.html", `<link rel="canonical" href="https://portal.example/metodologia/">`, `<link rel="canonical" href="https://portal.example/consultar/">`)
		},
		"indexable page outside the sitemap": func(t *testing.T, out string) {
			rewrite(t, out, "postos/"+stB+"/index.html", "noindex,follow", "index,follow")
		},
	}
	for name, tamper := range cases {
		t.Run(name, func(t *testing.T) {
			out := generate(t)
			tamper(t, out)
			refreshPortalManifest(t, out)
			if _, err := Verify(out, VerifyOptions{SiteURL: "https://portal.example"}); err == nil {
				t.Fatal("tampered sitemap accepted")
			}
		})
	}
}
