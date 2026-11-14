package portalgen

import (
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
)

func TestHeadContractOnEveryGeneratedPage(t *testing.T) {
	out := generateSEOPortal(t, true)
	pages := portalPages(t, out)
	titles, descriptions := map[string]string{}, map[string]string{}
	indexable := 0
	for path, body := range pages {
		head := parseHead(body)
		if path == "404.html" {
			if head.Robots != "noindex,follow" || head.Canonical != "" {
				t.Errorf("404 head = %+v", head)
			}
			continue
		}
		route := "/" + strings.TrimSuffix(path, "index.html")
		if head.Canonical != seoSite+route {
			t.Errorf("%s canonical = %q, want self %q", path, head.Canonical, seoSite+route)
		}
		// Forum post pages are articles (B-BR-SEO-08); hubs stay websites.
		ogType := "website"
		if strings.HasPrefix(path, "forum/") && strings.Count(path, "/") == 3 {
			ogType = "article"
		}
		for _, want := range []string{
			`<meta property="og:type" content="` + ogType + `">`, `<meta property="og:site_name" content="abastevo">`,
			`<meta property="og:locale" content="pt_BR">`, `<meta property="og:url" content="` + head.Canonical + `">`,
			`<meta property="og:image" content="` + seoSite + `/assets/brand/og-image-v1.png">`,
			`<meta name="twitter:card" content="summary_large_image">`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("%s lacks %s", path, want)
			}
		}
		if strings.Contains(strings.ToLower(head.Title+" "+head.Description), "hoje") {
			t.Errorf("%s head says hoje", path)
		}
		if head.Robots != "index,follow" {
			if head.Robots != "noindex,follow" {
				t.Errorf("%s robots = %q", path, head.Robots)
			}
			continue
		}
		indexable++
		if n := utf8.RuneCountInString(htmlUnescape(head.Description)); n < 120 || n > 160 {
			t.Errorf("%s description has %d characters: %q", path, n, head.Description)
		}
		if other, dup := titles[head.Title]; dup {
			t.Errorf("duplicate title %q on %s and %s", head.Title, path, other)
		}
		titles[head.Title] = path
		if other, dup := descriptions[head.Description]; dup {
			t.Errorf("duplicate description on %s and %s", path, other)
		}
		descriptions[head.Description] = path
	}
	if indexable < 20 {
		t.Fatalf("only %d indexable pages in the SEO fixture", indexable)
	}
	city := parseHead(pages["precos/mt/cuiaba/index.html"])
	if city.Title != "Preço da gasolina, etanol e diesel em Cuiabá (MT) – pesquisa ANP 17/10/2026" {
		t.Errorf("city title = %q", city.Title)
	}
	if !strings.Contains(city.Description, "R$ 6,099 a R$ 6,399") || !strings.Contains(city.Description, "11/10 a 17/10/2026") {
		t.Errorf("city description lacks real figures: %q", city.Description)
	}
	if state := parseHead(pages["precos/mt/index.html"]); state.Title != "Preço dos combustíveis em Mato Grosso por cidade – ANP 17/10/2026" {
		t.Errorf("state title = %q", state.Title)
	}
	station := pages["postos/"+seoID(1)+"/index.html"]
	if head := parseHead(station); head.Title != "Auto Posto Cuiabá Ltda – preços em Cuiabá (MT)" || head.Robots != "index,follow" ||
		!strings.Contains(head.Description, "pesquisa ANP de 11/10 a 17/10/2026") || !strings.Contains(head.Description, "Gasolina comum R$ 6,199") {
		t.Errorf("station head = %+v", head)
	}
	if !strings.Contains(station, "<h1>Auto Posto Cuiabá Ltda</h1>") || !strings.Contains(station, "Nome no cadastro da ANP: AUTO POSTO CUIABÁ LTDA") {
		t.Error("station H1 is not title case or the registry name is hidden")
	}
	shellA := parseHead(pages["postos/"+seoID(2)+"/index.html"]).Title
	shellB := parseHead(pages["postos/"+seoID(3)+"/index.html"]).Title
	if shellA == shellB || !strings.Contains(shellA, "Rua A") || !strings.Contains(shellB, "Rua B") {
		t.Errorf("duplicate station names not disambiguated: %q / %q", shellA, shellB)
	}
	for id, why := range map[string]string{seoID(6): "stale price", seoID(7): "no street", seoID(18): "no price"} {
		if head := parseHead(pages["postos/"+id+"/index.html"]); head.Robots != "noindex,follow" {
			t.Errorf("station with %s is %q", why, head.Robots)
		}
	}
	if head := parseHead(pages["postos/"+seoID(12)+"/index.html"]); head.Robots != "index,follow" {
		t.Errorf("station inside the 8-week window is %q", head.Robots)
	}
	if head := parseHead(pages["fiscalizacao/index.html"]); head.Robots != "index,follow" {
		t.Errorf("fiscalização hub = %+v", head)
	}
}

func TestPreviewBuildIsHiddenEverywhere(t *testing.T) {
	out := generateSEOPortal(t, false)
	for path, body := range portalPages(t, out) {
		if head := parseHead(body); head.Robots != "noindex,nofollow" {
			t.Errorf("%s robots = %q in a non-indexable build", path, head.Robots)
		}
	}
	if _, err := Verify(out, VerifyOptions{}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(readPortal(t, out, "robots.txt"), "Allow: /") {
		t.Error("preview robots allows crawling")
	}
}

func TestHeadEscapesHostileStationNames(t *testing.T) {
	snapshot := writeFixtureWithNames(t, `POSTO "X" <B>&</B>`)
	out := filepath.Join(t.TempDir(), "portal")
	if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: out, Indexable: true, SiteURL: seoSite}); err != nil {
		t.Fatal(err)
	}
	body := readPortal(t, out, "postos/"+stA+"/index.html")
	if strings.Contains(body, "<B>&</B>") || strings.Contains(body, "<b>&</b>") || !strings.Contains(parseHead(body).Title, "Posto &#34;X&#34; &lt;") {
		t.Errorf("hostile name not escaped in head: %s", parseHead(body).Title)
	}
	if _, err := Verify(out, VerifyOptions{SiteURL: seoSite}); err != nil {
		t.Fatal(err)
	}
}

func writeFixtureWithNames(t *testing.T, name string) string {
	t.Helper()
	return writeFixtureWith(t, func(stations *[]enterpriseexport.Station, _ *[]enterpriseexport.Price) {
		(*stations)[0].DisplayName = name
	})
}

func htmlUnescape(value string) string {
	return strings.NewReplacer("&#34;", `"`, "&#39;", "'", "&amp;", "&", "&lt;", "<", "&gt;", ">").Replace(value)
}
