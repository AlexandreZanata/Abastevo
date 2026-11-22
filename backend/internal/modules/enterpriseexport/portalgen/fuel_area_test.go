package portalgen

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport/analytics"
)

func TestFuelSlugsNeverCollideWithStateCodes(t *testing.T) {
	seen := map[string]string{}
	for code, words := range fuelVocabulary {
		if len(words.Slug) < 3 || isUF(strings.ToUpper(words.Slug)) || knownUF(strings.ToUpper(words.Slug)) {
			t.Errorf("fuel slug %q of %s can be read as a state code", words.Slug, code)
		}
		if other, dup := seen[words.Slug]; dup {
			t.Errorf("%s and %s share the slug %q", code, other, words.Slug)
		}
		seen[words.Slug] = code
		if !isFuelSlug(words.Slug) {
			t.Errorf("isFuelSlug(%q) = false", words.Slug)
		}
	}
	for _, slug := range []string{"mt", "sp", "", "gasolina", "cuiaba"} {
		if isFuelSlug(slug) {
			t.Errorf("isFuelSlug(%q) = true", slug)
		}
	}
	for route, want := range map[string]string{
		"/precos/mt/": "estados", "/precos/mt/cuiaba/": "cidades", "/precos/gasolina-comum/": "combustiveis",
		"/precos/gasolina-comum/mt/": "combustivel-estado", "/precos/gnv/mt/": "combustivel-estado",
		"/precos/gasolina-comum/mt/cuiaba/": "combustivel-cidade", "/precos/": "hubs",
	} {
		if got := routeFamily(route); got != want {
			t.Errorf("routeFamily(%q) = %q, want %q", route, got, want)
		}
	}
}

func TestFuelByStatePageShowsOneFuelInOneState(t *testing.T) {
	out := generatePricePortal(t, nil)
	pages := portalPages(t, out)
	page, ok := pages["precos/gasolina-comum/mt/index.html"]
	if !ok {
		t.Fatal("the gasoline page of Mato Grosso was not generated")
	}
	head := parseHead(page)
	if head.Title != "Preço da gasolina comum em Mato Grosso (MT) – ANP 17/10/2026" || head.Robots != robotsIndexable ||
		head.Canonical != seoSite+"/precos/gasolina-comum/mt/" {
		t.Errorf("head = %+v", head)
	}
	if utf8.RuneCountInString(head.Description) < descriptionMin || !strings.Contains(head.Description, "Gasolina comum em Mato Grosso na pesquisa ANP de 11/10 a 17/10/2026.") ||
		!strings.Contains(head.Description, "R$ 6,000 a R$ 6,500 em 10 postos.") || !strings.Contains(head.Description, "Média da amostra: R$ 6,230.") {
		t.Errorf("description = %q", head.Description)
	}
	for _, want := range []string{
		"<h1>Preço da gasolina comum em Mato Grosso (MT)</h1>",
		"em Mato Grosso, em R$ por litro. Não é preço ao vivo nem estatística oficial da ANP.",
		`<a href="/precos/gasolina-comum/">Gasolina comum</a> · <span aria-current="page">Mato Grosso</span>`,
		`href="/precos/gasolina-comum/sp/">Preço da gasolina comum em São Paulo</a>`,
		`href="/precos/etanol/mt/">Preço do etanol hidratado em Mato Grosso</a>`,
		`href="/precos/mt/">Preço dos combustíveis em Mato Grosso</a>`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %s", want)
		}
	}
	summary := sectionAfter(page, "<h2>Resumo da amostra</h2>")
	if got := tableRow(summary, "Gasolina comum"); !reflect.DeepEqual(got, []string{"Gasolina comum", "litro", "10 postos", "10", "R$ 6,000", "R$ 6,500", "R$ 6,230", "R$ 0,500", "—"}) {
		t.Errorf("summary row = %q", got)
	}
	if tableRow(summary, "Etanol hidratado") != nil {
		t.Error("the gasoline page summarizes ethanol")
	}
	comparison := sectionAfter(page, "<h2>Comparação com a pesquisa anterior</h2>")
	if got := tableRow(comparison, "Gasolina comum"); len(got) != 6 || got[1] != "10 → 10" || got[4] != "aumentou R$ 0,060" {
		t.Errorf("comparison row = %q", got)
	}

	// One row per survey week of the snapshot, newest first.
	history := sectionAfter(page, "<h2>Histórico por pesquisa</h2>")
	for period, want := range map[string][]string{
		"11/10/2026 a 17/10/2026": {"11/10/2026 a 17/10/2026", "10 postos", "R$ 6,000", "R$ 6,500", "R$ 6,230"},
		"04/10/2026 a 10/10/2026": {"04/10/2026 a 10/10/2026", "10 postos", "R$ 5,900", "R$ 6,400", "R$ 6,170"},
		"27/09/2026 a 03/10/2026": {"27/09/2026 a 03/10/2026", "5 postos", "R$ 5,850", "R$ 6,250", "R$ 6,050"},
	} {
		if got := tableRow(history, period); !reflect.DeepEqual(got, want) {
			t.Errorf("history %s = %q, want %q", period, got, want)
		}
	}
	if first, last := strings.Index(history, "11/10/2026 a 17/10/2026"), strings.Index(history, "27/09/2026 a 03/10/2026"); first < 0 || last < first {
		t.Error("history is not newest first")
	}

	cities := sectionAfter(page, "<h2>Cidades de Mato Grosso com preço na amostra</h2>")
	if got := tableRow(cities, "Cuiabá (MT)"); !reflect.DeepEqual(got, []string{"Cuiabá (MT)", "6 postos", "R$ 6,000", "R$ 6,500", "R$ 6,250"}) {
		t.Errorf("city row = %q", got)
	}
	if got := tableRow(cities, "Várzea Grande (MT)"); !reflect.DeepEqual(got, []string{"Várzea Grande (MT)", "4 postos", "amostra pequena"}) {
		t.Errorf("small city row = %q", got)
	}
	if tableRow(cities, "São Paulo (SP)") != nil {
		t.Error("the Mato Grosso page lists a city of another state")
	}
	if !strings.Contains(sectionAfter(page, "<h2>Menores preços encontrados na amostra</h2>"), "Posto Cuiabano 1") {
		t.Error("the state ranking of the fuel is missing")
	}
}

func TestFuelByStatePagesExistOnlyForFuelsOfTheState(t *testing.T) {
	out := generatePricePortal(t, nil)
	pages := portalPages(t, out)
	got := []string{}
	for route := range pages {
		if routeFamily("/"+strings.TrimSuffix(route, "index.html")) == "combustivel-estado" {
			got = append(got, route)
		}
	}
	want := map[string]bool{
		"precos/gasolina-comum/mt/index.html": true, "precos/gasolina-comum/sp/index.html": true,
		"precos/etanol/mt/index.html": true, "precos/etanol/sp/index.html": true,
		"precos/diesel-s10/mt/index.html": true, "precos/gnv/mt/index.html": true, "precos/glp/mt/index.html": true,
	}
	if len(got) != len(want) {
		t.Fatalf("fuel by state pages = %v", got)
	}
	for _, route := range got {
		if !want[route] {
			t.Errorf("unexpected page %s", route)
		}
		if n := utf8.RuneCountInString(parseHead(pages[route]).Title); n == 0 || n > titleMax {
			t.Errorf("%s title has %d runes", route, n)
		}
	}
	// São Paulo has no diesel in the sample: no page, and nothing links one.
	if _, err := os.Stat(filepath.Join(out, "precos/diesel-s10/sp/index.html")); !os.IsNotExist(err) {
		t.Error("a fuel by state page exists for a fuel the state does not have")
	}
	// A small sample still has its page, with no mean: GLP has one station.
	glp := pages["precos/glp/mt/index.html"]
	if got := tableRow(sectionAfter(glp, "<h2>Resumo da amostra</h2>"), "GLP"); len(got) != 9 || got[1] != "botijão de 13 kg" || got[6] != "—" {
		t.Errorf("GLP row = %q", got)
	}
	if strings.Contains(parseHead(glp).Description, "Média da amostra") {
		t.Error("a one-station sample advertises a mean")
	}
}

func TestFuelByStatePagesAreLinkedListedAndDeterministic(t *testing.T) {
	out := generatePricePortal(t, nil)
	pages := portalPages(t, out)
	if !strings.Contains(pages["precos/gasolina-comum/index.html"], `<a href="/precos/gasolina-comum/mt/">Mato Grosso (MT)</a>`) {
		t.Error("the fuel hub does not link the fuel by state page")
	}
	if !strings.Contains(pages["precos/mt/index.html"], `<a href="/precos/gasolina-comum/mt/">Gasolina comum</a>`) {
		t.Error("the state page does not link the fuel by state page from its summary")
	}
	if !strings.Contains(pages["precos/mt/cuiaba/index.html"], `href="/precos/gasolina-comum/mt/">Preço da gasolina comum em Mato Grosso</a>`) {
		t.Error("the city page does not link the fuel by state page")
	}
	sitemap := readPortal(t, out, "sitemaps/portal-precos-pt-BR-1.xml")
	for _, route := range []string{"/precos/gasolina-comum/mt/", "/precos/glp/mt/", "/precos/etanol/sp/"} {
		if !strings.Contains(sitemap, "<loc>"+seoSite+route+"</loc>") {
			t.Errorf("the precos sitemap lacks %s", route)
		}
	}
	graph, err := VerifyLinkGraph(out, LinkGraphOptions{SiteURL: seoSite})
	if err != nil {
		t.Fatalf("link graph: %v", err)
	}
	if graph.MinInbound["combustivel-estado"] < DefaultMinInboundLinks {
		t.Errorf("fuel by state inbound links = %d", graph.MinInbound["combustivel-estado"])
	}
	if _, err := Verify(out, VerifyOptions{SiteURL: seoSite}); err != nil {
		t.Fatalf("strict verifier: %v", err)
	}
	again := portalPages(t, generatePricePortal(t, nil))
	for route, body := range pages {
		if again[route] != body {
			t.Fatalf("%s differs between two builds of the same snapshot", route)
		}
	}
}

func TestFuelByStateHistoryMarksGapsAndUsesTheNewestRevisionOfAWeek(t *testing.T) {
	_, revisions, _ := pricePagesFixture()
	// A correction of week 2 with a higher revision id stands for that week.
	correction := revisions[1]
	correction.RevisionID = "00000000-0000-4000-b000-000000000999"
	weeks := surveyWeeks(append([]enterpriseexport.Revision{correction}, revisions...), pricePagesProjection(t))
	if len(weeks) != 3 || weeks[0].revision.RevisionID != revisions[2].RevisionID || weeks[1].revision.RevisionID != correction.RevisionID {
		t.Fatalf("survey weeks = %+v", weeks)
	}

	// Without the week 2 gasoline prices, the week between two observed weeks is a gap.
	pages := portalPages(t, generatePricePortal(t, func(_ *[]enterpriseexport.Station, prices *[]enterpriseexport.Price) {
		kept := (*prices)[:0]
		for _, price := range *prices {
			if price.FuelProduct == "GASOLINE_REGULAR" && price.RevisionID == revisions[1].RevisionID {
				continue
			}
			kept = append(kept, price)
		}
		*prices = kept
	}))
	gasoline := sectionAfter(pages["precos/gasolina-comum/mt/index.html"], "<h2>Histórico por pesquisa</h2>")
	if got := tableRow(gasoline, "04/10/2026 a 10/10/2026"); !reflect.DeepEqual(got, []string{"04/10/2026 a 10/10/2026", "Sem observação nesta pesquisa"}) {
		t.Errorf("gap row = %q", got)
	}
	if tableRow(gasoline, "27/09/2026 a 03/10/2026") == nil || tableRow(gasoline, "11/10/2026 a 17/10/2026") == nil {
		t.Error("the observed weeks around the gap are missing")
	}
	// Diesel S10 was first observed in week 2: the list does not start with a gap.
	diesel := sectionAfter(pages["precos/diesel-s10/mt/index.html"], "<h2>Histórico por pesquisa</h2>")
	if strings.Contains(diesel, "27/09/2026") || tableRow(diesel, "04/10/2026 a 10/10/2026") == nil {
		t.Errorf("diesel history = %s", plainText(diesel))
	}
}

func cityFuelMutation(n int, city, uf string, fuel string, amount int64) func(*[]enterpriseexport.Station, *[]enterpriseexport.Price) {
	return func(stations *[]enterpriseexport.Station, prices *[]enterpriseexport.Price) {
		_, revisions, _ := pricePagesFixture()
		*stations = append(*stations, enterpriseexport.Station{
			StationID: ppID(n), DisplayName: "POSTO EXTRA", State: uf, MunicipalityCode: city,
			OperationStatus: "active", LocationQuality: "unknown", Address: map[string]string{"uf": uf, "street": "RUA EXTRA", "number": "1"},
		})
		unit, _ := productUnit(fuel)
		*prices = append(*prices, enterpriseexport.Price{
			PriceID: "pp-extra", RevisionID: revisions[2].RevisionID, StationID: ppID(n), FuelProduct: fuel, Unit: unit,
			AmountMilliBRL: amount, Currency: "BRL", CollectedOn: addDays(revisions[2].SurveyStart, 2),
		})
	}
}

func TestFuelByCityPageNeedsFiveStationsOfTheFuel(t *testing.T) {
	const small = "precos/gasolina-comum/mt/varzea-grande/index.html"
	pages := portalPages(t, generatePricePortal(t, nil))
	got := []string{}
	for route := range pages {
		if routeFamily("/"+strings.TrimSuffix(route, "index.html")) == "combustivel-cidade" {
			got = append(got, route)
		}
	}
	want := map[string]bool{
		"precos/gasolina-comum/mt/cuiaba/index.html": true, "precos/etanol/mt/cuiaba/index.html": true,
		"precos/diesel-s10/mt/cuiaba/index.html":        true,
		"precos/gasolina-comum/sp/sao-paulo/index.html": true, "precos/etanol/sp/sao-paulo/index.html": true,
	}
	if len(got) != len(want) {
		t.Fatalf("fuel by city pages = %v", got)
	}
	for _, route := range got {
		if !want[route] {
			t.Errorf("unexpected page %s", route)
		}
	}
	// Four stations in Várzea Grande, two of GNV and one of GLP in Cuiabá: no page.
	for _, route := range []string{small, "precos/gnv/mt/cuiaba/index.html", "precos/glp/mt/cuiaba/index.html"} {
		if _, ok := pages[route]; ok {
			t.Errorf("%s exists below the minimum of %d stations", route, minMunicipality)
		}
	}
	// The fifth station of the fuel creates the page.
	withFifth := portalPages(t, generatePricePortal(t, cityFuelMutation(15, ppVarzeaGrande, "MT", "GASOLINE_REGULAR", 6450)))
	if _, ok := withFifth[small]; !ok {
		t.Errorf("%s is missing with %d stations", small, minMunicipality)
	}
	if _, ok := withFifth["precos/etanol/mt/varzea-grande/index.html"]; ok {
		t.Error("the fifth gasoline station created an ethanol page")
	}
}

func TestFuelByCityPlanIgnoresNonCanonicalUnitsAndUnknownCities(t *testing.T) {
	summary := func(level, code, fuel, unit string, stations int) analytics.Summary {
		return analytics.Summary{Geography: analytics.Geography{Level: level, Code: code}, Fuel: fuel, Unit: unit, StationCount: stations}
	}
	summaries := []analytics.Summary{
		summary(analytics.LevelNational, "BR", "CNG", "M3", 9), summary(analytics.LevelNational, "BR", "GASOLINE_REGULAR", "L", 9),
		summary(analytics.LevelMunicipality, ppCuiaba, "CNG", "L", 9),                // wrong unit
		summary(analytics.LevelMunicipality, ppCuiaba, "CNG", "M3", 4),               // below the minimum
		summary(analytics.LevelMunicipality, ppCuiaba, "GASOLINE_REGULAR", "L", 5),   // planned
		summary(analytics.LevelMunicipality, ppSaoPaulo, "GASOLINE_REGULAR", "L", 9), // no city page
		summary(analytics.LevelMunicipality, ppCuiaba, "ETHANOL", "L", 9),            // no hub
		summary(analytics.LevelState, "MT", "CNG", "L", 9),                           // wrong unit
		summary(analytics.LevelState, "MT", "CNG", "M3", 1),                          // planned
	}
	g := &generator{
		fuelHubs: planFuelHubs(summaries),
		cities:   map[string]*cityInfo{ppCuiaba: {Code: ppCuiaba, UF: "MT", Name: "Cuiabá", Route: "/precos/mt/cuiaba/", HasData: true}},
	}
	g.planFuelAreas(summaries)
	if !reflect.DeepEqual(g.fuelCityPages, map[string]bool{areaKey("GASOLINE_REGULAR", ppCuiaba): true}) {
		t.Errorf("planned city pages = %v", g.fuelCityPages)
	}
	if !reflect.DeepEqual(g.fuelStatePages, map[string]bool{areaKey("CNG", "MT"): true}) {
		t.Errorf("planned state pages = %v", g.fuelStatePages)
	}
	if got := g.fuelCityHref("GASOLINE_REGULAR", ppCuiaba); got != "/precos/gasolina-comum/mt/cuiaba/" {
		t.Errorf("fuel by city route = %q", got)
	}
	if got := g.fuelCityHref("CNG", ppCuiaba); got != "" {
		t.Errorf("unplanned fuel by city route = %q", got)
	}
}

func TestFuelByCityPageListsEveryStationWithItsDistrict(t *testing.T) {
	out := generatePricePortal(t, nil)
	pages := portalPages(t, out)
	page := pages["precos/gasolina-comum/mt/cuiaba/index.html"]
	head := parseHead(page)
	if head.Title != "Preço da gasolina comum em Cuiabá (MT) – ANP 17/10/2026" || head.Robots != robotsIndexable ||
		head.Canonical != seoSite+"/precos/gasolina-comum/mt/cuiaba/" {
		t.Errorf("head = %+v", head)
	}
	if !strings.Contains(head.Description, "Gasolina comum em Cuiabá (MT) na pesquisa ANP de 11/10 a 17/10/2026.") ||
		!strings.Contains(head.Description, "R$ 6,000 a R$ 6,500 em 6 postos.") || !strings.Contains(head.Description, "Média da amostra: R$ 6,250.") {
		t.Errorf("description = %q", head.Description)
	}
	for _, want := range []string{
		"<h1>Preço da gasolina comum em Cuiabá (MT)</h1>",
		"em Cuiabá (MT), em R$ por litro. Não é preço ao vivo nem estatística oficial da ANP.",
		`<a href="/precos/gasolina-comum/">Gasolina comum</a> · <a href="/precos/gasolina-comum/mt/">Mato Grosso</a> · <span aria-current="page">Cuiabá</span>`,
		"6 postos na amostra.",
		`href="/precos/etanol/mt/cuiaba/">Preço do etanol hidratado em Cuiabá (MT)</a>`,
		`href="/precos/mt/cuiaba/">Preços dos combustíveis e postos em Cuiabá (MT)</a>`,
		`href="/precos/gasolina-comum/sp/sao-paulo/">Preço da gasolina comum em São Paulo (SP)</a>`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %s", want)
		}
	}
	if strings.Contains(page, "Exibindo") {
		t.Error("a six-station table says it was cut")
	}
	stations := sectionAfter(page, "<h2>Postos com preço nesta pesquisa</h2>")
	// #, Posto, Preço, Bairro, Coleta, Empates: the price comes right after
	// the name, so a narrow screen shows it without scrolling the table.
	for first, want := range map[string][]string{
		"1": {"1", "Posto Cuiabano 1", "R$ 6,000", "Centro", "13/10/2026", "—"},
		"3": {"3", "Posto Cuiabano 3", "R$ 6,200", "—", "13/10/2026", "—"},
		// Registry districts are upper case; they get the title case of the station names.
		"6": {"6", "Posto Cuiabano 6", "R$ 6,500", "Cpa", "13/10/2026", "—"},
	} {
		if got := tableRow(stations, first); !reflect.DeepEqual(got, want) {
			t.Errorf("station row %s = %q, want %q", first, got, want)
		}
	}
	if !strings.Contains(stations, `<a href="/postos/`+ppID(1)+`/">Posto Cuiabano 1</a>`) {
		t.Error("the station name does not link the station page")
	}
	history := sectionAfter(page, "<h2>Histórico por pesquisa</h2>")
	if got := tableRow(history, "27/09/2026 a 03/10/2026"); !reflect.DeepEqual(got, []string{"27/09/2026 a 03/10/2026", "5 postos", "R$ 5,850", "R$ 6,250", "R$ 6,050"}) {
		t.Errorf("oldest history row = %q", got)
	}
	if strings.Count(history, "<tr><td>") != 3 {
		t.Errorf("history rows = %d, want the 3 survey weeks", strings.Count(history, "<tr><td>"))
	}
	context := plainText(sectionAfter(page, "<h2>Gasolina comum em Mato Grosso e no Brasil</h2>"))
	for _, want := range []string{
		"Preço da gasolina comum em Mato Grosso — R$ 6,000 a R$ 6,500 em 10 postos, média da amostra R$ 6,230",
		"Preço da gasolina comum no Brasil — R$ 5,800 a R$ 6,500 em 15 postos, média da amostra R$ 6,153",
	} {
		if !strings.Contains(context, want) {
			t.Errorf("context lacks %q in %q", want, context)
		}
	}
	comparison := sectionAfter(page, "<h2>Comparação com a pesquisa anterior</h2>")
	if got := tableRow(comparison, "Gasolina comum"); len(got) != 6 || got[4] != "aumentou R$ 0,100" {
		t.Errorf("comparison row = %q", got)
	}
}

func TestFuelByCityTableCountsTiesAndIsCutAtTheCap(t *testing.T) {
	// Two stations at the same price share the tie count.
	tied := portalPages(t, generatePricePortal(t, func(_ *[]enterpriseexport.Station, prices *[]enterpriseexport.Price) {
		for i := range *prices {
			if (*prices)[i].PriceID == "pp-02-3-GASOLINE_REGULAR" {
				(*prices)[i].AmountMilliBRL = 6000
			}
		}
	}))
	stations := sectionAfter(tied["precos/gasolina-comum/mt/cuiaba/index.html"], "<h2>Postos com preço nesta pesquisa</h2>")
	for _, first := range []string{"1", "2"} {
		if got := tableRow(stations, first); len(got) != 6 || got[2] != "R$ 6,000" || got[5] != "2 no mesmo preço" {
			t.Errorf("tied row %s = %q", first, got)
		}
	}

	items := []observation{}
	for i := 0; i < fuelCityStationCap+5; i++ {
		amount := int64(5000 + i)
		if i >= fuelCityStationCap-1 && i <= fuelCityStationCap+1 {
			amount = int64(5000 + fuelCityStationCap - 1) // three stations tied across the cutoff
		}
		items = append(items, observation{
			station: enterpriseexport.Station{StationID: ppID(1000 + i), DisplayName: fmt.Sprintf("POSTO %04d", i), Address: map[string]string{"district": "CENTRO"}},
			price:   enterpriseexport.Price{AmountMilliBRL: amount, CollectedOn: "2026-10-13"},
		})
	}
	rows, total := fuelStationRows(items)
	if total != fuelCityStationCap+5 || len(rows) != fuelCityStationCap+2 {
		t.Fatalf("rows = %d of %d, want the cap plus the two ties past it", len(rows), total)
	}
	if rows[0].Position != 1 || rows[0].Amount != 5000 || rows[0].District != "Centro" || rows[len(rows)-1].TieCount != 3 {
		t.Errorf("first row = %+v, last row = %+v", rows[0], rows[len(rows)-1])
	}
}

func TestFuelByCityPagesChangeTheLinksAndTitleOfTheCityPage(t *testing.T) {
	out := generatePricePortal(t, nil)
	pages := portalPages(t, out)
	city := pages["precos/mt/cuiaba/index.html"]
	if head := parseHead(city); head.Title != "Preços dos combustíveis e postos em Cuiabá (MT) – ANP 17/10/2026" {
		t.Errorf("city title = %q", head.Title)
	}
	for _, want := range []string{
		"<h1>Preços de combustíveis em Cuiabá (MT)</h1>",
		`<a href="/precos/gasolina-comum/mt/cuiaba/">Gasolina comum</a>`,
		`<a href="/precos/gnv/">GNV</a>`, // no fuel by city page: the cell keeps the national hub
	} {
		if !strings.Contains(city, want) {
			t.Errorf("city page lacks %s", want)
		}
	}
	hub := sectionAfter(pages["precos/gasolina-comum/index.html"], "<h2>Cidades com preço na amostra</h2>")
	if !strings.Contains(hub, `<a href="/precos/gasolina-comum/mt/cuiaba/">Cuiabá (MT)</a>`) || !strings.Contains(hub, `<a href="/precos/mt/varzea-grande/">Várzea Grande (MT)</a>`) {
		t.Error("the fuel hub city table does not link the fuel by city page, or lost the city page fallback")
	}
	state := sectionAfter(pages["precos/gasolina-comum/mt/index.html"], "<h2>Cidades de Mato Grosso com preço na amostra</h2>")
	if !strings.Contains(state, `<a href="/precos/gasolina-comum/mt/cuiaba/">Cuiabá (MT)</a>`) || !strings.Contains(state, `<a href="/precos/mt/varzea-grande/">Várzea Grande (MT)</a>`) {
		t.Error("the fuel by state city table does not link the fuel by city page, or lost the city page fallback")
	}
	sitemap := readPortal(t, out, "sitemaps/portal-precos-pt-BR-1.xml")
	if !strings.Contains(sitemap, "<loc>"+seoSite+"/precos/gasolina-comum/mt/cuiaba/</loc>") {
		t.Error("the precos sitemap lacks the fuel by city page")
	}
	graph, err := VerifyLinkGraph(out, LinkGraphOptions{SiteURL: seoSite})
	if err != nil {
		t.Fatalf("link graph: %v", err)
	}
	if graph.MinInbound["combustivel-cidade"] < DefaultMinInboundLinks {
		t.Errorf("fuel by city inbound links = %d", graph.MinInbound["combustivel-cidade"])
	}
	if _, err := Verify(out, VerifyOptions{SiteURL: seoSite}); err != nil {
		t.Fatalf("strict verifier: %v", err)
	}
	for route, body := range pages {
		if routeFamily("/"+strings.TrimSuffix(route, "index.html")) != "combustivel-cidade" {
			continue
		}
		if n := utf8.RuneCountInString(parseHead(body).Title); n == 0 || n > titleMax {
			t.Errorf("%s title has %d runes", route, n)
		}
	}
}
