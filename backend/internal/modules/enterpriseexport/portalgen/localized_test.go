package portalgen

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func TestLocalizedPortalRoutesAndIndexation(t *testing.T) {
	out := generateSEOPortal(t, true)
	server := http.FileServer(http.Dir(out))
	groups := [][]string{
		{"/precos/", "/en/fuel-prices/", "/es/precios/"},
		{"/precos/gasolina-comum/", "/en/fuel-prices/regular-gasoline/", "/es/precios/gasolina-comun/"},
		{"/precos/mt/", "/en/fuel-prices/mt/", "/es/precios/mt/"},
		{"/precos/gasolina-comum/mt/", "/en/fuel-prices/regular-gasoline/mt/", "/es/precios/gasolina-comun/mt/"},
		{"/precos/mt/cuiaba/", "/en/fuel-prices/mt/cuiaba/", "/es/precios/mt/cuiaba/"},
		{"/metodologia/", "/en/methodology/", "/es/metodologia/"},
		{"/dados/", "/en/data/", "/es/datos/"},
		{"/curiosidades/", "/en/insights/", "/es/curiosidades/"},
	}
	for _, group := range groups {
		for i, route := range group {
			response := httptest.NewRecorder()
			server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, route, nil))
			if response.Code != http.StatusOK {
				t.Errorf("%s returned %d", route, response.Code)
				continue
			}
			body := readPortal(t, out, strings.TrimPrefix(route, "/")+"index.html")
			for j, lang := range []string{"pt-BR", "en", "es", "x-default"} {
				target := group[j%len(group)]
				if !strings.Contains(body, `hreflang="`+lang+`" href="`+seoSite+target+`"`) {
					t.Errorf("%s lacks hreflang %s to %s", route, lang, target)
				}
			}
			if !strings.Contains(body, `rel="canonical" href="`+seoSite+route+`"`) ||
				!strings.Contains(body, `content="`+robotsIndexable+`"`) ||
				!strings.Contains(body, `"inLanguage":"`+[]string{"pt-BR", "en", "es"}[i]+`"`) {
				t.Errorf("invalid metadata at %s", route)
			}
			if i > 0 && strings.HasPrefix(route, []string{"", "/en/fuel-prices/", "/es/precios/"}[i]) {
				if !strings.Contains(body, []string{"", "Source:", "Fuente:"}[i]) || strings.Contains(strings.ToLower(body), []string{"", " today", " hoy"}[i]) {
					t.Errorf("missing source or misleading temporal claim on %s", route)
				}
			}
		}
	}
	for _, path := range []string{"en/fuel-prices/mt/varzea-grande", "es/precios/mt/sorriso",
		"en/fuel-prices/regular-gasoline/mt/cuiaba", "es/precios/gasolina-comun/mt/cuiaba",
		"en/stations", "es/puestos", "en/consultar", "es/fiscalizacion", "en/forum"} {
		if _, err := os.Stat(filepath.Join(out, path)); !os.IsNotExist(err) {
			t.Errorf("unexpected localized route %s: %v", path, err)
		}
	}
	for _, locale := range []string{"en", "es"} {
		sitemap := readPortal(t, out, "sitemaps/portal-precos-"+locale+"-1.xml")
		if !strings.Contains(sitemap, seoSite+"/"+locale+"/") || strings.Contains(sitemap, "/varzea-grande/") {
			t.Errorf("misclassified sitemap for %s", locale)
		}
	}
	if _, err := Verify(out, VerifyOptions{SiteURL: seoSite}); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyLinkGraph(out, LinkGraphOptions{SiteURL: seoSite}); err != nil {
		t.Fatal(err)
	}
}

func TestPortalI18nSharedRegistryAndRouteVectors(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..", "..", "contracts")
	shared, err := os.ReadFile(filepath.Join(root, "i18n", "locales.json"))
	if err != nil {
		t.Fatal(err)
	}
	embedded, err := fs.ReadFile(localizationAssets, "assets/locales.json")
	if err != nil || string(embedded) != string(shared) {
		t.Fatal("embedded locale registry is stale; run go generate in portalgen")
	}
	raw, err := os.ReadFile(filepath.Join(root, "testdata", "i18n", "routes.json"))
	if err != nil {
		t.Fatal(err)
	}
	var vectors []struct{ Locale, Kind, Slug, Area, City, Route string }
	if err := json.Unmarshal(raw, &vectors); err != nil {
		t.Fatal(err)
	}
	locales, err := loadPortalLocales(localizationAssets)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range vectors {
		if got := locales[v.Locale].route(v.Kind, v.Slug, v.Area, v.City); got != v.Route {
			t.Errorf("%s %s route: got %q, want %q", v.Locale, v.Kind, got, v.Route)
		}
	}
}

func TestPortalLocaleGateAndFormatting(t *testing.T) {
	assets := fstest.MapFS{}
	err := fs.WalkDir(localizationAssets, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		raw, err := fs.ReadFile(localizationAssets, path)
		if err == nil {
			assets[path] = &fstest.MapFile{Data: raw}
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	locales, err := loadPortalLocales(assets)
	if err != nil {
		t.Fatal(err)
	}
	if en, es := locales["en"], locales["es"]; en.money(6290) != "R$ 6.29" || es.money(6290) != "R$ 6,29" ||
		en.date("2026-11-14") != "Nov 14, 2026" || es.date("2026-11-14") != "14/11/2026" ||
		en.money(6299) != "R$ 6.299" || en.integer(1234) != "1,234" || es.integer(1234) != "1.234" {
		t.Fatal("incorrect locale-specific date, number or currency format")
	}
	const registryPath = "assets/locales.json"
	var registry struct {
		Version int
		Default string
		Locales []portalLocale
	}
	if err := json.Unmarshal(assets[registryPath].Data, &registry); err != nil {
		t.Fatal(err)
	}
	for i := range registry.Locales {
		if registry.Locales[i].Code == "en" {
			registry.Locales[i].Enabled = false
		}
	}
	assets[registryPath].Data, err = json.Marshal(registry)
	if err != nil {
		t.Fatal(err)
	}
	const catalogPath = "assets/messages/es.json"
	var catalog portalCatalog
	if err := json.Unmarshal(assets[catalogPath].Data, &catalog); err != nil {
		t.Fatal(err)
	}
	catalog.TranslationStatus = "draft"
	assets[catalogPath].Data, err = json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "portal")
	if _, err := Generate(Options{SnapshotDir: writeSEOFixture(t), OutputDir: out, SiteURL: seoSite,
		Indexable: true, localeAssets: assets}); err != nil {
		t.Fatal(err)
	}
	for _, lang := range []string{"en", "es"} {
		if _, err := os.Stat(filepath.Join(out, lang)); !os.IsNotExist(err) {
			t.Errorf("%s pages were emitted: %v", lang, err)
		}
	}
	body := readPortal(t, out, "precos/index.html")
	sitemap := readPortal(t, out, "sitemap.xml")
	if strings.Contains(body, `rel="alternate" hreflang`) || strings.Contains(sitemap, "-en-") || strings.Contains(sitemap, "-es-") {
		t.Fatal("gated locales leaked into hreflang or sitemaps")
	}
}

func TestPortalCatalogParityRejectsMissingKeysAndChangedPlaceholders(t *testing.T) {
	source := portalCatalog{Messages: map[string]string{"sample": "{fuel}, {fuel}"}}
	for _, translated := range []portalCatalog{
		{Messages: map[string]string{"another": "{fuel}, {fuel}"}},
		{Messages: map[string]string{"sample": "{fuel}"}},
		{Messages: map[string]string{"sample": "{fuel}, {area}"}},
	} {
		if err := validatePortalCatalog(source, translated); err == nil {
			t.Fatal("invalid catalog passed parity")
		}
	}
}
