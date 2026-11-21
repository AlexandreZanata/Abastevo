package portalgen

import (
	"reflect"
	"strings"
	"testing"
)

func TestSampleMeanRoundsHalfUpAndNeedsFiveStations(t *testing.T) {
	cases := []struct {
		sum      int64
		stations int
		want     int64
		shown    bool
	}{
		{37500, 6, 6250, true},
		{30002, 5, 6000, true},  // 6000.4
		{30003, 5, 6001, true},  // 6000.6
		{25, 10, 3, true},       // exactly 2.5 rounds up
		{35, 10, 4, true},       // exactly 3.5 rounds up
		{24, 10, 2, true},       // 2.4
		{24400, 4, 0, false},    // below the municipal minimum
		{0, 5, 0, false},        // no observation
		{92300, 15, 6153, true}, // 6153.33
		{130000, 1, 0, false},   // a single station is not a mean
		{30000, 5, 6000, true},  // exact
		{-5, 5, 0, false},       // never a negative sum
		{6249999, 1000, 6250, true},
	}
	for _, c := range cases {
		got, shown := sampleMean(c.sum, c.stations)
		if got != c.want || shown != c.shown {
			t.Errorf("sampleMean(%d, %d) = %d, %t; want %d, %t", c.sum, c.stations, got, shown, c.want, c.shown)
		}
	}
}

func TestSummaryTablesShowTheMeanOnlyFromFiveStations(t *testing.T) {
	pages := portalPages(t, generatePricePortal(t, nil))
	city := sectionAfter(pages["precos/mt/cuiaba/index.html"], "<h2>Resumo por combustível</h2>")
	// Combustível, Unidade, Amostra, Observações, Mínimo, Máximo, Média, Dispersão, Amostra.
	for fuel, want := range map[string][]string{
		"Gasolina comum":   {"Gasolina comum", "litro", "6 postos", "6", "R$ 6,000", "R$ 6,500", "R$ 6,250", "R$ 0,500", "—"},
		"Etanol hidratado": {"Etanol hidratado", "litro", "6 postos", "6", "R$ 4,000", "R$ 4,500", "R$ 4,250", "R$ 0,500", "—"},
		"Diesel S10":       {"Diesel S10", "litro", "5 postos", "5", "R$ 6,800", "R$ 7,200", "R$ 7,000", "R$ 0,400", "—"},
		"GNV":              {"GNV", "m³", "2 postos", "2", "R$ 4,590", "R$ 4,690", "—", "R$ 0,100", "amostra pequena"},
		"GLP":              {"GLP", "botijão de 13 kg", "1 posto", "1", "R$ 130,000", "R$ 130,000", "—", "R$ 0,000", "amostra pequena"},
	} {
		if got := tableRow(city, fuel); !reflect.DeepEqual(got, want) {
			t.Errorf("Cuiabá %s row = %q, want %q", fuel, got, want)
		}
	}
	state := sectionAfter(pages["precos/mt/index.html"], "<h2>Resumo por combustível</h2>")
	if got := tableRow(state, "Gasolina comum"); len(got) != 9 || got[2] != "10 postos" || got[6] != "R$ 6,230" {
		t.Errorf("Mato Grosso gasoline row = %q", got)
	}
	for route, body := range pages {
		if strings.Contains(body, "KG_13") {
			t.Errorf("%s shows the GLP unit code", route)
		}
	}
}

func TestFuelHubTablesCarryTheMean(t *testing.T) {
	hub := portalPages(t, generatePricePortal(t, nil))["precos/gasolina-comum/index.html"]
	states := sectionAfter(hub, "<h2>Por estado</h2>")
	if got := tableRow(states, "Mato Grosso (MT)"); !reflect.DeepEqual(got, []string{"Mato Grosso (MT)", "10 postos", "R$ 6,000", "R$ 6,500", "R$ 6,230", "R$ 0,500"}) {
		t.Errorf("fuel hub state row = %q", got)
	}
	cities := sectionAfter(hub, "<h2>Cidades com preço na amostra</h2>")
	if got := tableRow(cities, "Cuiabá (MT)"); !reflect.DeepEqual(got, []string{"Cuiabá (MT)", "6 postos", "R$ 6,000", "R$ 6,500", "R$ 6,250"}) {
		t.Errorf("fuel hub city row = %q", got)
	}
	// Four stations: no minimum, maximum or mean.
	if got := tableRow(cities, "Várzea Grande (MT)"); !reflect.DeepEqual(got, []string{"Várzea Grande (MT)", "4 postos", "amostra pequena"}) {
		t.Errorf("fuel hub small city row = %q", got)
	}
	national := sectionAfter(hub, "<h2>Brasil</h2>")
	if got := tableRow(national, "Gasolina comum"); len(got) != 9 || got[2] != "15 postos" || got[6] != "R$ 6,153" {
		t.Errorf("fuel hub national row = %q", got)
	}
}

func TestComparisonShowsMovementOnlyForTheSameStationSample(t *testing.T) {
	pages := portalPages(t, generatePricePortal(t, nil))
	const heading = "<h2>Comparação com a pesquisa anterior</h2>"
	city := sectionAfter(pages["precos/mt/cuiaba/index.html"], heading)
	if !strings.Contains(city, "Pesquisa anterior: 04/10/2026 a 10/10/2026.") {
		t.Fatalf("Cuiabá comparison does not name the previous survey: %s", plainText(city))
	}
	// Combustível, Postos, Mínimo, Máximo, Média da amostra, Situação.
	for fuel, want := range map[string][]string{
		"Gasolina comum":   {"Gasolina comum", "6 → 6", "aumentou R$ 0,100", "aumentou R$ 0,100", "aumentou R$ 0,100", "mesma amostra de postos"},
		"Etanol hidratado": {"Etanol hidratado", "6 → 6", "ficou igual", "ficou igual", "ficou igual", "mesma amostra de postos"},
		"Diesel S10":       {"Diesel S10", "4 → 5", "antes R$ 6,750", "antes R$ 7,050", "—", "amostra diferente; sem variação calculada"},
	} {
		if got := tableRow(city, fuel); !reflect.DeepEqual(got, want) {
			t.Errorf("Cuiabá comparison %s row = %q, want %q", fuel, got, want)
		}
	}
	// GNV and GLP had no observation in the previous survey: no row.
	for _, fuel := range []string{"GNV", "GLP"} {
		if tableRow(city, fuel) != nil {
			t.Errorf("Cuiabá comparison has a %s row without a previous survey", fuel)
		}
	}
	if !strings.Contains(city, "Só calculamos variação quando os postos da amostra são os mesmos nas duas pesquisas.") {
		t.Error("Cuiabá comparison lost its rule sentence")
	}

	// The state gained the four Várzea Grande ethanol stations: sample changed.
	state := sectionAfter(pages["precos/mt/index.html"], heading)
	if got := tableRow(state, "Etanol hidratado"); !reflect.DeepEqual(got, []string{"Etanol hidratado", "6 → 10", "antes R$ 4,000", "antes R$ 4,500", "antes R$ 4,250", "amostra diferente; sem variação calculada"}) {
		t.Errorf("Mato Grosso ethanol comparison = %q", got)
	}
	if got := tableRow(state, "Gasolina comum"); !reflect.DeepEqual(got, []string{"Gasolina comum", "10 → 10", "aumentou R$ 0,100", "aumentou R$ 0,100", "aumentou R$ 0,060", "mesma amostra de postos"}) {
		t.Errorf("Mato Grosso gasoline comparison = %q", got)
	}

	// The fuel hub compares only its own fuel, at the national level.
	hub := sectionAfter(pages["precos/gasolina-comum/index.html"], heading)
	if got := tableRow(hub, "Gasolina comum"); len(got) != 6 || got[1] != "10 → 15" || got[5] != "amostra diferente; sem variação calculada" {
		t.Errorf("fuel hub comparison = %q", got)
	}
	if tableRow(hub, "Etanol hidratado") != nil {
		t.Error("the gasoline hub compares ethanol")
	}

	// No previous survey for São Paulo, and no summary for a city below the minimum.
	for _, route := range []string{"precos/sp/sao-paulo/index.html", "precos/sp/index.html", "precos/mt/varzea-grande/index.html"} {
		if strings.Contains(pages[route], heading) {
			t.Errorf("%s has a comparison block", route)
		}
	}
}

func TestMethodologyExplainsTheMeanAndThePortalStaysValid(t *testing.T) {
	out := generatePricePortal(t, nil)
	methodology := readPortal(t, out, "metodologia/index.html")
	if !strings.Contains(methodology, `id="media-da-amostra"`) || !strings.Contains(methodology, "Só aparece quando a amostra tem pelo menos 5 postos.") {
		t.Error("the methodology page does not explain the sample mean")
	}
	if _, err := Verify(out, VerifyOptions{SiteURL: seoSite}); err != nil {
		t.Fatalf("strict verifier: %v", err)
	}
	if _, err := VerifyLinkGraph(out, LinkGraphOptions{SiteURL: seoSite}); err != nil {
		t.Fatalf("link graph: %v", err)
	}
}
