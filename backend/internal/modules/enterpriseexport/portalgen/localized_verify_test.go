package portalgen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyRejectsInvalidLanguageAlternates(t *testing.T) {
	const path = "en/fuel-prices/index.html"
	link := func(language, route string) string {
		return `<link rel="alternate" hreflang="` + language + `" href="` + seoSite + route + `">`
	}
	tests := []struct {
		name, old, replacement string
	}{
		{"missing self-reference", link("en", "/en/fuel-prices/"), ""},
		{"missing x-default", link("x-default", "/precos/"), ""},
		{"missing target", link("es", "/es/precios/"), link("es", "/es/precios/no-existe/")},
		{"wrong family", link("es", "/es/precios/"), link("es", "/es/metodologia/")},
		{"x-default targets the Spanish page", link("x-default", "/precos/"), link("x-default", "/es/precios/")},
		{"duplicate alternate", link("es", "/es/precios/"), link("es", "/es/precios/") + link("es", "/es/precios/")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := generateSEOPortal(t, true)
			rewriteLocalizedVerificationFixture(t, out, path, tc.old, tc.replacement)
			refreshPortalManifest(t, out)
			if _, err := Verify(out, VerifyOptions{SiteURL: seoSite}); err == nil || !strings.Contains(err.Error(), "hreflang") {
				t.Fatalf("portalcheck accepted the invalid hreflang or failed for another reason: %v", err)
			}
			if _, err := VerifyLinkGraph(out, LinkGraphOptions{SiteURL: seoSite}); err == nil || !strings.Contains(err.Error(), "hreflang") {
				t.Fatalf("link graph accepted the invalid hreflang or failed for another reason: %v", err)
			}
		})
	}
}

func TestVerifyRejectsWrongLanguageSitemap(t *testing.T) {
	out := generateSEOPortal(t, true)
	from := "sitemaps/portal-precos-en-1.xml"
	to := "sitemaps/portal-precos-fr-1.xml"
	if err := os.Rename(filepath.Join(out, from), filepath.Join(out, to)); err != nil {
		t.Fatal(err)
	}
	rewriteLocalizedVerificationFixture(t, out, "sitemap.xml", from, to)
	refreshPortalManifest(t, out)
	if _, err := Verify(out, VerifyOptions{SiteURL: seoSite}); err == nil || !strings.Contains(err.Error(), "sitemap for fr") {
		t.Fatalf("wrong locale accepted or failed for another reason: %v", err)
	}
}

func rewriteLocalizedVerificationFixture(t *testing.T, out, path, old, replacement string) {
	t.Helper()
	page := readPortal(t, out, path)
	if !strings.Contains(page, old) {
		t.Fatalf("%s does not contain %s", path, old)
	}
	if err := os.WriteFile(filepath.Join(out, path), []byte(strings.Replace(page, old, replacement, 1)), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLinkGraphRequiresInboundLinksInThePageLanguage(t *testing.T) {
	out := generateSEOPortal(t, true)
	const target = `href="/en/fuel-prices/regular-gasoline/"`
	for path, page := range portalPages(t, out) {
		if !strings.HasPrefix(path, "en/") {
			continue
		}
		main := string(mainContent([]byte(page)))
		withoutLink := strings.ReplaceAll(main, target, `href="/en/fuel-prices/"`)
		if withoutLink != main {
			rewriteLocalizedVerificationFixture(t, out, path, main, withoutLink)
		}
	}
	for _, path := range []string{"precos/index.html", "precos/mt/index.html", "metodologia/index.html"} {
		rewriteLocalizedVerificationFixture(t, out, path, "</main>",
			`<a `+target+`>Preços da gasolina comum em inglês</a></main>`)
	}
	if _, err := VerifyLinkGraph(out, LinkGraphOptions{SiteURL: seoSite}); err == nil ||
		!strings.Contains(err.Error(), "/en/fuel-prices/regular-gasoline/ has 0 inbound body links") {
		t.Fatalf("cross-language links satisfied the inbound link minimum: %v", err)
	}
}

func TestVerifyAcceptsLegacyPortugueseSitemapFilename(t *testing.T) {
	out := generateSEOPortal(t, true)
	from := "sitemaps/portal-postos-pt-BR-1.xml"
	to := "sitemaps/portal-postos-1.xml"
	if err := os.Rename(filepath.Join(out, from), filepath.Join(out, to)); err != nil {
		t.Fatal(err)
	}
	rewriteLocalizedVerificationFixture(t, out, "sitemap.xml", from, to)
	refreshPortalManifest(t, out)
	if _, err := Verify(out, VerifyOptions{SiteURL: seoSite}); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyLinkGraph(out, LinkGraphOptions{SiteURL: seoSite}); err != nil {
		t.Fatal(err)
	}
}

func TestVerificationLimitsTranslatedCityRoutesToTheClosedList(t *testing.T) {
	locales, err := loadPortalLocales(localizationAssets)
	if err != nil {
		t.Fatal(err)
	}
	portuguese, english := locales[defaultPortalLocale], locales["en"]
	for _, test := range []struct {
		locale portalLocale
		route  string
		want   string
	}{
		{locale: english, route: "/en/fuel-prices/mt/cuiaba/", want: "/precos/mt/cuiaba/"},
		{locale: english, route: "/en/fuel-prices/mt/cuiaba-5103403/", want: "/precos/mt/cuiaba-5103403/"},
		{locale: english, route: "/en/fuel-prices/mt/cuiaba-5108402/"},
		{locale: english, route: "/en/fuel-prices/sp/cuiaba/"},
		{locale: english, route: "/en/fuel-prices/mt/varzea-grande/"},
		{locale: portuguese, route: "/precos/mt/varzea-grande/", want: "/precos/mt/varzea-grande/"},
	} {
		if got := portalRouteIdentity(test.route, test.locale, portuguese); got != test.want {
			t.Errorf("%s identity = %q, want %q", test.route, got, test.want)
		}
	}
}
