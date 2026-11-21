package portalgen

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport/analytics"
)

func TestFuelRatioIsExactRoundsHalfUpAndNeedsBothSamples(t *testing.T) {
	sample := func(sum int64, stations int) analytics.Summary {
		return analytics.Summary{SumMilliBRL: sum, StationCount: stations}
	}
	cases := []struct {
		name              string
		ethanol, gasoline analytics.Summary
		want              int64
		ok                bool
	}{
		{"exact 68.0%", sample(25500, 6), sample(37500, 6), 680, true},
		{"different sample sizes", sample(42300, 10), sample(92300, 15), 687, true}, // 4230 ÷ 6153.33 = 68.74%
		{"rounds down below the half", sample(13009, 5), sample(20000, 5), 650, true},
		{"rounds up at the half", sample(13010, 5), sample(20000, 5), 651, true},
		{"exactly 70%", sample(21000, 5), sample(30000, 5), 700, true},
		{"above 100%", sample(33000, 5), sample(30000, 5), 1100, true},
		{"four ethanol stations", sample(16800, 4), sample(37500, 6), 0, false},
		{"four gasoline stations", sample(25500, 6), sample(24400, 4), 0, false},
		{"empty gasoline", sample(25500, 6), sample(0, 6), 0, false},
		{"empty ethanol", sample(0, 6), sample(37500, 6), 0, false},
	}
	for _, c := range cases {
		got, ok := fuelRatio(c.ethanol, c.gasoline)
		if got != c.want || ok != c.ok {
			t.Errorf("%s: fuelRatio = %d, %t; want %d, %t", c.name, got, ok, c.want, c.ok)
		}
	}
	for value, want := range map[int64]string{680: "68,0%", 679: "67,9%", 700: "70,0%", 1100: "110,0%", 5: "0,5%"} {
		if got := percentTenths(value); got != want {
			t.Errorf("percentTenths(%d) = %q, want %q", value, got, want)
		}
	}
	for value, want := range map[int64]string{699: "abaixo de 70%", 700: "em 70%", 701: "acima de 70%"} {
		if got := parityPosition(value); got != want {
			t.Errorf("parityPosition(%d) = %q, want %q", value, got, want)
		}
	}
}

func TestRatioRouteIsReservedAndHasItsOwnFamily(t *testing.T) {
	const slug = "etanol-ou-gasolina"
	if isFuelSlug(slug) || knownUF(strings.ToUpper(slug)) {
		t.Fatalf("%q is also a fuel slug or a state code", slug)
	}
	if got := routeFamily(parityRoute); got != "paridade" {
		t.Errorf("routeFamily(%q) = %q", parityRoute, got)
	}
	if got, err := sitemapFamily(familyParity); err != nil || got != "precos" {
		t.Errorf("sitemapFamily(parity) = %q, %v", got, err)
	}
	if !pageIndexable(indexFacts{Family: familyParity, HasCurrentData: true}) || pageIndexable(indexFacts{Family: familyParity}) {
		t.Error("the ratio page indexation does not follow its data")
	}
}

func TestRatioPageListsOnlyAreasWithBothSamples(t *testing.T) {
	out := generatePricePortal(t, nil)
	pages := portalPages(t, out)
	page, ok := pages["precos/etanol-ou-gasolina/index.html"]
	if !ok {
		t.Fatal("the ratio page was not generated")
	}
	head := parseHead(page)
	if head.Title != "Etanol ou gasolina por estado e cidade – ANP 17/10/2026" || head.Robots != robotsIndexable || head.Canonical != seoSite+parityRoute {
		t.Errorf("head = %+v", head)
	}
	if !strings.Contains(head.Description, "na pesquisa ANP de 11/10 a 17/10/2026, por estado e cidade.") ||
		!strings.Contains(head.Description, "No Brasil: 70,2%.") || !strings.Contains(head.Description, "Referência geral: 70%.") {
		t.Errorf("description = %q", head.Description)
	}
	for _, want := range []string{
		"<h1>Etanol ou gasolina: relação de preços por estado e cidade</h1>",
		"na pesquisa da ANP de 11/10/2026 a 17/10/2026. Não é preço ao vivo nem estatística oficial da ANP.",
		"O ponto de equilíbrio do seu carro pode ser diferente.",
		`<a href="/ferramentas/etanol-ou-gasolina/">na calculadora de etanol ou gasolina</a>`,
		`<a href="/guias/etanol-ou-gasolina/">Entenda a regra dos 70%</a>`,
		"Só há relação quando o etanol e a gasolina comum têm pelo menos 5 postos cada na amostra da área.",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %s", want)
		}
	}
	// Área, Etanol, Gasolina comum, Relação, Postos (etanol / gasolina), Referência de 70%.
	if got := tableRow(sectionAfter(page, "<h2>Brasil</h2>"), "Brasil"); !reflect.DeepEqual(got, []string{"Brasil", "R$ 4,320", "R$ 6,153", "70,2%", "15 / 15", "acima de 70%"}) {
		t.Errorf("national row = %q", got)
	}
	states := sectionAfter(page, "<h2>Por estado</h2>")
	for name, want := range map[string][]string{
		"Mato Grosso (MT)": {"Mato Grosso (MT)", "R$ 4,230", "R$ 6,230", "67,9%", "10 / 10", "abaixo de 70%"},
		"São Paulo (SP)":   {"São Paulo (SP)", "R$ 4,500", "R$ 6,000", "75,0%", "5 / 5", "acima de 70%"},
	} {
		if got := tableRow(states, name); !reflect.DeepEqual(got, want) {
			t.Errorf("state row %s = %q, want %q", name, got, want)
		}
	}
	if !strings.Contains(states, `<a href="/precos/mt/">Mato Grosso (MT)</a>`) {
		t.Error("the state row does not link the state page")
	}
	cities := sectionAfter(page, "<h2>Por cidade</h2>")
	for name, want := range map[string][]string{
		"Cuiabá (MT)":    {"Cuiabá (MT)", "R$ 4,250", "R$ 6,250", "68,0%", "6 / 6", "abaixo de 70%"},
		"São Paulo (SP)": {"São Paulo (SP)", "R$ 4,500", "R$ 6,000", "75,0%", "5 / 5", "acima de 70%"},
	} {
		if got := tableRow(cities, name); !reflect.DeepEqual(got, want) {
			t.Errorf("city row %s = %q, want %q", name, got, want)
		}
	}
	// Four stations of each fuel in Várzea Grande: no ratio.
	if tableRow(cities, "Várzea Grande (MT)") != nil || strings.Count(cities, "<tr><td>") != 2 {
		t.Errorf("city rows = %s", plainText(cities))
	}
	if first, second := strings.Index(cities, "Cuiabá (MT)"), strings.Index(cities, "São Paulo (SP)"); first < 0 || second < first {
		t.Error("cities are not ordered by state, then name")
	}

	main := page[strings.Index(page, `<main id="conteudo"`):strings.Index(page, "</main>")]
	for _, word := range []string{"esta semana", "hoje", "compensa", "mais barat"} {
		if strings.Contains(strings.ToLower(main), word) {
			t.Errorf("the ratio page says %q", word)
		}
	}
}

func TestRatioLineAndLinksAppearOnlyWhereTheAreaHasARatio(t *testing.T) {
	out := generatePricePortal(t, nil)
	pages := portalPages(t, out)
	const link = `<a href="/precos/etanol-ou-gasolina/">Ver por estado e cidade</a>`
	for route, ratio := range map[string]string{
		"precos/mt/index.html": "67,9%", "precos/sp/index.html": "75,0%",
		"precos/mt/cuiaba/index.html": "68,0%", "precos/sp/sao-paulo/index.html": "75,0%",
	} {
		if want := "<p>Etanol ÷ gasolina comum nesta amostra: " + ratio + " (referência geral: 70%). " + link + "</p>"; !strings.Contains(pages[route], want) {
			t.Errorf("%s lacks its ratio line %s", route, ratio)
		}
	}
	if strings.Contains(pages["precos/mt/varzea-grande/index.html"], "Etanol ÷ gasolina") {
		t.Error("a city without a ratio shows a ratio line")
	}
	const hubLink = `href="/precos/etanol-ou-gasolina/">Etanol ou gasolina por estado e cidade</a>`
	for _, route := range []string{"precos/index.html", "precos/etanol/index.html", "precos/gasolina-comum/index.html"} {
		if !strings.Contains(pages[route], hubLink) {
			t.Errorf("%s does not link the ratio page", route)
		}
	}
	if strings.Contains(pages["precos/diesel-s10/index.html"], "etanol-ou-gasolina/\">Etanol ou gasolina por estado") {
		t.Error("the diesel hub links the ratio page")
	}
	if !strings.Contains(readPortal(t, out, "sitemaps/portal-precos-1.xml"), "<loc>"+seoSite+parityRoute+"</loc>") {
		t.Error("the precos sitemap lacks the ratio page")
	}
	graph, err := VerifyLinkGraph(out, LinkGraphOptions{SiteURL: seoSite})
	if err != nil {
		t.Fatalf("link graph: %v", err)
	}
	if graph.MinInbound["paridade"] < DefaultMinInboundLinks {
		t.Errorf("ratio page inbound links = %d", graph.MinInbound["paridade"])
	}
	if _, err := Verify(out, VerifyOptions{SiteURL: seoSite}); err != nil {
		t.Fatalf("strict verifier: %v", err)
	}
	if again := portalPages(t, generatePricePortal(t, nil)); again["precos/etanol-ou-gasolina/index.html"] != pages["precos/etanol-ou-gasolina/index.html"] {
		t.Error("two builds of the same snapshot produced different ratio pages")
	}
}

func TestRatioPageNeedsTheNationalRatio(t *testing.T) {
	// Without ethanol there is no ratio anywhere: no page, no line, no link.
	out := generatePricePortal(t, func(_ *[]enterpriseexport.Station, prices *[]enterpriseexport.Price) {
		kept := (*prices)[:0]
		for _, price := range *prices {
			if price.FuelProduct != "ETHANOL" {
				kept = append(kept, price)
			}
		}
		*prices = kept
	})
	if _, err := os.Stat(filepath.Join(out, "precos/etanol-ou-gasolina/index.html")); !os.IsNotExist(err) {
		t.Fatalf("the ratio page exists without ethanol (stat error %v)", err)
	}
	for route, body := range portalPages(t, out) {
		if strings.Contains(body, "etanol-ou-gasolina/\">") && strings.Contains(body, `href="/precos/etanol-ou-gasolina/"`) {
			t.Errorf("%s links a ratio page that does not exist", route)
		}
		if strings.Contains(body, "Etanol ÷ gasolina") {
			t.Errorf("%s shows a ratio line", route)
		}
	}
	if _, err := Verify(out, VerifyOptions{SiteURL: seoSite}); err != nil {
		t.Fatalf("strict verifier: %v", err)
	}
	if _, err := VerifyLinkGraph(out, LinkGraphOptions{SiteURL: seoSite}); err != nil {
		t.Fatalf("link graph: %v", err)
	}
}

func TestRatioPageWithoutALandingHasNoLandingLinks(t *testing.T) {
	out := filepath.Join(t.TempDir(), "portal")
	if _, err := Generate(Options{SnapshotDir: writePricePagesSnapshot(t, nil), OutputDir: out, Indexable: true, SiteURL: seoSite}); err != nil {
		t.Fatal(err)
	}
	page := readPortal(t, out, "precos/etanol-ou-gasolina/index.html")
	if strings.Contains(page, "calculadora de etanol ou gasolina") || strings.Contains(page, "/guias/") || strings.Contains(page, "/ferramentas/") {
		t.Error("a build without a landing links landing pages from the ratio page")
	}
	if !strings.Contains(page, "O ponto de equilíbrio do seu carro pode ser diferente.</p>") {
		t.Error("the reading note lost its last sentence")
	}
}
