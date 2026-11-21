package portalgen

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestUnitShortNamesEveryCanonicalUnit(t *testing.T) {
	for unit, want := range map[string]string{"L": "litro", "M3": "m³", "KG_13": "botijão de 13 kg", "OTHER": "OTHER"} {
		if got := unitShort(unit); got != want {
			t.Errorf("unitShort(%q) = %q, want %q", unit, got, want)
		}
	}
}

func TestFitTitlePicksTheFirstCandidateInsideTheBudget(t *testing.T) {
	long := strings.Repeat("a", titleMax+1)
	exact := strings.Repeat("ã", titleMax)
	if got := fitTitle(long, exact, "short"); got != exact {
		t.Errorf("fitTitle skipped a candidate of exactly %d runes: %q", titleMax, got)
	}
	if got := fitTitle(long, "", "short"); got != "short" {
		t.Errorf("fitTitle = %q, want the first fitting candidate", got)
	}
	cut := fitTitle("Posto de combustíveis com um nome comercial muito comprido – preços em Cuiabá (MT)")
	if utf8.RuneCountInString(cut) > titleMax || strings.HasSuffix(cut, "–") || strings.HasSuffix(cut, " ") {
		t.Errorf("fitTitle cut = %q", cut)
	}
}

func TestLongPlaceNamesKeepTitlesInsideTheBudget(t *testing.T) {
	fuels := []string{"GASOLINE_REGULAR", "ETHANOL", "DIESEL_S10"}
	for name, got := range map[string]string{
		"city":         cityTitle("São José do Rio Preto", "SP", fuels, "17/10/2026"),
		"state":        stateTitle("MS", "17/10/2026"),
		"fuel hub":     fuelHubTitle("GASOLINE_ADDITIVED", "17/10/2026"),
		"station":      stationTitle("Auto Posto e Serviços Rodoviários Bandeirantes do Oeste Paulista Ltda", "São José do Rio Preto", "SP"),
		"no-data city": noDataTitle(placeLabel("São José do Rio Preto", "SP")),
	} {
		if n := utf8.RuneCountInString(got); n > titleMax || got == "" {
			t.Errorf("%s title has %d runes: %q", name, n, got)
		}
	}
	// The date is the first part to go; the place never is.
	if got := stateTitle("MS", "17/10/2026"); got != "Preço dos combustíveis em Mato Grosso do Sul – ANP 17/10/2026" {
		t.Errorf("state fallback = %q", got)
	}
	if got := cityTitle("São José do Rio Preto", "SP", fuels, "17/10/2026"); got != "Preço da gasolina, etanol e diesel em São José do Rio Preto (SP)" {
		t.Errorf("city fallback = %q", got)
	}
}

func TestPriceAndStationPagesFitTheTitleBudgetAndShowNoUnitCode(t *testing.T) {
	seen := map[string]string{}
	for route, body := range portalPages(t, generateSEOPortal(t, true)) {
		for _, code := range []string{"KG_13", "· M3", "<td>M3</td>", "<td>L</td>", "· L<"} {
			if strings.Contains(body, code) {
				t.Errorf("%s shows the raw unit code %q", route, code)
			}
		}
		if !strings.HasPrefix(route, "precos/") && !strings.HasPrefix(route, "postos/") {
			continue
		}
		title := parseHead(body).Title
		if n := utf8.RuneCountInString(title); n == 0 || n > titleMax {
			t.Errorf("%s title has %d runes: %q", route, n, title)
		}
		if other, dup := seen[title]; dup && strings.HasPrefix(route, "precos/") {
			t.Errorf("%s and %s share the title %q", route, other, title)
		}
		seen[title] = route
	}
}

func TestFuelSynonymAppearsOnlyOnItsOwnHub(t *testing.T) {
	pages := portalPages(t, generateSEOPortal(t, true))
	for route, synonym := range map[string]string{
		"precos/etanol/index.html": "álcool", "precos/gnv/index.html": "gás natural veicular",
	} {
		body, ok := pages[route]
		if !ok {
			t.Fatalf("%s was not generated", route)
		}
		if !strings.Contains(body, "Também chamado de "+synonym+".") {
			t.Errorf("%s lead does not name the synonym %q", route, synonym)
		}
		if !strings.Contains(parseHead(body).Description, "("+synonym+") na pesquisa ANP") {
			t.Errorf("%s description does not name the synonym: %q", route, parseHead(body).Description)
		}
	}
	for route, body := range pages {
		if strings.Contains(body, "Também chamado de") && route != "precos/etanol/index.html" && route != "precos/gnv/index.html" {
			t.Errorf("%s names a fuel synonym", route)
		}
	}
	if fuelVocabulary["LPG_P13"].Synonym != "gás de cozinha" {
		t.Errorf("GLP synonym = %q", fuelVocabulary["LPG_P13"].Synonym)
	}
}
