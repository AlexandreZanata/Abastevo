package portalgen

import (
	"strings"
	"testing"
)

const ppPeriod = "11/10/2026 a 17/10/2026"

func TestWeekSummaryUsesOnlySentencesWhoseConditionHolds(t *testing.T) {
	g := &generator{period: ppPeriod}
	row := summaryRow{Code: "GASOLINE_REGULAR", Stations: 6, Min: 6000, Max: 6500, Mean: 6250, HasMean: true}
	change := func(comparable, hasDelta bool, delta int64) *comparisonView {
		return &comparisonView{Rows: []comparisonRow{
			{Code: "ETHANOL", Comparable: true, HasMeanDelta: true, MeanDelta: 999}, // another fuel never leaks in
			{Code: "GASOLINE_REGULAR", Comparable: comparable, HasMeanDelta: hasDelta, MeanDelta: delta},
		}}
	}
	base := "Na pesquisa da ANP de " + ppPeriod + ", 6 postos de Cuiabá (MT) tinham preço de gasolina comum. " +
		"O menor preço da amostra foi R$ 6,000 e o maior, R$ 6,500. A média da amostra foi R$ 6,250."
	cases := []struct {
		name  string
		facts fuelFacts
		want  string
	}{
		{"no previous survey", fuelFacts{Code: "GASOLINE_REGULAR", Place: "Cuiabá (MT)", Row: row}, base},
		{"mean rose", fuelFacts{Code: "GASOLINE_REGULAR", Place: "Cuiabá (MT)", Row: row, Comparison: change(true, true, 100)},
			base + " Com os mesmos postos da pesquisa anterior, a média subiu R$ 0,100."},
		{"mean fell", fuelFacts{Code: "GASOLINE_REGULAR", Place: "Cuiabá (MT)", Row: row, Comparison: change(true, true, -50)},
			base + " Com os mesmos postos da pesquisa anterior, a média caiu R$ 0,050."},
		{"mean equal", fuelFacts{Code: "GASOLINE_REGULAR", Place: "Cuiabá (MT)", Row: row, Comparison: change(true, true, 0)},
			base + " Com os mesmos postos da pesquisa anterior, a média não mudou."},
		{"sample changed", fuelFacts{Code: "GASOLINE_REGULAR", Place: "Cuiabá (MT)", Row: row, Comparison: change(false, false, 0)},
			base + " A amostra de postos mudou em relação à pesquisa anterior; por isso não calculamos variação."},
		{"comparable without a mean", fuelFacts{Code: "GASOLINE_REGULAR", Place: "Cuiabá (MT)", Row: row, Comparison: change(true, false, 0)}, base},
		{"national", fuelFacts{Code: "GASOLINE_REGULAR", States: 2, Row: summaryRow{Stations: 1500, Min: 5800, Max: 6500, Mean: 6153, HasMean: true}},
			"Na pesquisa da ANP de " + ppPeriod + ", 1.500 postos de 2 estados tinham preço de gasolina comum. " +
				"O menor preço da amostra foi R$ 5,800 e o maior, R$ 6,500. A média da amostra foi R$ 6,153."},
		{"one state", fuelFacts{Code: "CNG", States: 1, Row: summaryRow{Stations: 2, Min: 4590, Max: 4690}},
			"Na pesquisa da ANP de " + ppPeriod + ", 2 postos de 1 estado tinham preço de GNV. O menor preço da amostra foi R$ 4,590 e o maior, R$ 4,690."},
		{"flat sample", fuelFacts{Code: "ETHANOL", Place: "Mato Grosso", Row: summaryRow{Stations: 3, Min: 4000, Max: 4000}},
			"Na pesquisa da ANP de " + ppPeriod + ", 3 postos de Mato Grosso tinham preço de etanol hidratado. Todos os postos da amostra tinham o mesmo preço: R$ 4,000."},
		{"one station", fuelFacts{Code: "LPG_P13", Place: "Mato Grosso", Row: summaryRow{Stations: 1, Min: 130000, Max: 130000}},
			"Na pesquisa da ANP de " + ppPeriod + ", 1 posto de Mato Grosso tinha preço de GLP. O preço encontrado foi R$ 130,000."},
	}
	for _, c := range cases {
		if got := g.weekSummary(c.facts); got != c.want {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, c.want)
		}
	}
	if got := (&generator{}).weekSummary(cases[0].facts); got != "" {
		t.Errorf("a build without a survey has a summary: %q", got)
	}
	if got := g.fuelFAQ(fuelFacts{Code: "GASOLINE_REGULAR", Place: "Cuiabá (MT)"}); got != nil {
		t.Errorf("an empty sample has questions: %+v", got)
	}
}

func TestFuelPagesAnswerTodayOnlyWithTheSurveyPeriod(t *testing.T) {
	pages := portalPages(t, generatePricePortal(t, nil))
	city := pages["precos/gasolina-comum/mt/cuiaba/index.html"]
	for _, want := range []string{
		`<p class="lead">Na pesquisa da ANP de ` + ppPeriod + `, 6 postos de Cuiabá (MT) tinham preço de gasolina comum. O menor preço da amostra foi R$ 6,000 e o maior, R$ 6,500. A média da amostra foi R$ 6,250. Com os mesmos postos da pesquisa anterior, a média subiu R$ 0,100.</p>`,
		"<h2>Perguntas frequentes</h2>",
		"<h3>Qual o preço da gasolina comum hoje em Cuiabá (MT)?</h3>",
		"<p>Este site não mostra preço ao vivo. A referência mais recente é a pesquisa da ANP de " + ppPeriod + ": de R$ 6,000 a R$ 6,500 em 6 postos de Cuiabá (MT). A média da amostra foi R$ 6,250. O preço de hoje em cada posto pode ser diferente.</p>",
		"<h3>Qual foi o menor preço da gasolina comum em Cuiabá (MT)?</h3>",
		"<p>Na amostra da ANP de " + ppPeriod + ", o menor preço foi R$ 6,000. A amostra não cobre todos os postos, e o preço pode ter mudado desde a coleta.</p>",
		"<h3>De quando são estes preços?</h3>",
		"<h3>Como a média é calculada?</h3>",
		`(6 postos). É um cálculo do abastevo a partir dos dados da ANP, e não uma estatística oficial. <a href="/metodologia/#media-da-amostra">Ver a metodologia</a>.</p>`,
	} {
		if !strings.Contains(city, want) {
			t.Errorf("fuel by city page lacks %s", want)
		}
	}
	state := pages["precos/etanol/mt/index.html"]
	for _, want := range []string{
		"10 postos de Mato Grosso tinham preço de etanol hidratado.",
		"A amostra de postos mudou em relação à pesquisa anterior; por isso não calculamos variação.",
		"<h3>Qual o preço do etanol hidratado hoje em Mato Grosso?</h3>",
		"<h3>Qual foi o menor preço do etanol hidratado em Mato Grosso?</h3>",
	} {
		if !strings.Contains(state, want) {
			t.Errorf("fuel by state page lacks %s", want)
		}
	}
	// A one-station sample has no mean, so no mean sentence and no mean question.
	glp := pages["precos/glp/mt/index.html"]
	if !strings.Contains(glp, "1 posto de Mato Grosso tinha preço de GLP. O preço encontrado foi R$ 130,000.") ||
		strings.Contains(glp, "A média da amostra foi") || strings.Contains(glp, "Como a média é calculada?") {
		t.Error("the one-station page reads as if it had a mean")
	}
	hub := pages["precos/gasolina-comum/index.html"]
	for _, want := range []string{
		"15 postos de 2 estados tinham preço de gasolina comum.",
		"<h3>Qual o preço da gasolina comum hoje no Brasil?</h3>",
		"em 15 postos do Brasil. A média da amostra foi R$ 6,153.",
	} {
		if !strings.Contains(hub, want) {
			t.Errorf("fuel hub lacks %s", want)
		}
	}
	if strings.Contains(hub, "Qual foi o menor preço") {
		t.Error("the national hub asks the lowest-price question without a station table")
	}
	cityPage := pages["precos/mt/cuiaba/index.html"]
	for _, want := range []string{
		"<h3>Qual o preço dos combustíveis hoje em Cuiabá (MT)?</h3>",
		"A referência mais recente é a pesquisa da ANP de " + ppPeriod + ". Gasolina comum: R$ 6,000 a R$ 6,500 em 6 postos. Etanol hidratado: R$ 4,000 a R$ 4,500 em 6 postos.",
		"O preço de hoje em cada posto pode ser diferente.</p>",
	} {
		if !strings.Contains(cityPage, want) {
			t.Errorf("city page lacks %s", want)
		}
	}
	// A city below the municipal minimum shows no summary, so it answers nothing.
	if strings.Contains(pages["precos/mt/varzea-grande/index.html"], "Perguntas frequentes") {
		t.Error("a city below the minimum has a question block")
	}
}

func TestPricePagesNeverPromiseALivePriceOrACheapestStation(t *testing.T) {
	for name, out := range map[string]string{"price": generatePricePortal(t, nil), "seo": generateSEOPortal(t, true)} {
		questions := 0
		for route, body := range portalPages(t, out) {
			if strings.Contains(body, "FAQPage") {
				t.Errorf("%s %s declares FAQPage structured data", name, route)
			}
			if !strings.HasPrefix(route, "precos/") {
				continue
			}
			head := parseHead(body)
			if strings.Contains(strings.ToLower(head.Title), "hoje") || strings.Contains(strings.ToLower(head.Description), "hoje") {
				t.Errorf("%s %s says \"hoje\" in its title or description", name, route)
			}
			lower := strings.ToLower(body)
			if strings.Contains(lower, "hoje") {
				questions++
				if !strings.Contains(body, noLivePrice) {
					t.Errorf("%s %s says \"hoje\" without saying it has no live price", name, route)
				}
			}
			for _, claim := range []string{"mais barato", "mais barata", "preço atual", "tempo real"} {
				if strings.Contains(lower, claim) {
					t.Errorf("%s %s contains the claim %q", name, route, claim)
				}
			}
		}
		if questions == 0 {
			t.Errorf("%s: no price page answers a \"hoje\" question", name)
		}
		if _, err := Verify(out, VerifyOptions{SiteURL: seoSite}); err != nil {
			t.Errorf("%s strict verifier: %v", name, err)
		}
		if _, err := VerifyLinkGraph(out, LinkGraphOptions{SiteURL: seoSite}); err != nil {
			t.Errorf("%s link graph: %v", name, err)
		}
	}
}
