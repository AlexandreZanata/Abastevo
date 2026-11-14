package portalgen

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestStationDisplayTitle(t *testing.T) {
	for input, want := range map[string]string{
		"AUTO POSTO CUIABÁ LTDA":                    "Auto Posto Cuiabá Ltda",
		"POSTO DE COMBUSTIVEIS SÃO JOÃO DAS CRUZES": "Posto de Combustiveis São João das Cruzes",
		"I L N CARMINATI LTDA":                      "I L N Carminati Ltda",
		"COMERCIAL BR 163 E GNV ME":                 "Comercial BR 163 e GNV ME",
		"AUTO-POSTO D'OESTE EIRELI":                 "Auto-Posto d'Oeste Eireli",
		"POSTO RDC II":                              "Posto RDC II",
		"DE OLHO NA ESTRADA":                        "De Olho na Estrada",
		"  POSTO   GLP  S/A ":                       "Posto GLP S/A",
		"Posto Alfa":                                "Posto Alfa",
		"Posto ALFA mixed":                          "Posto ALFA mixed",
		"":                                          "",
	} {
		if got := stationDisplayTitle(input); got != want {
			t.Errorf("stationDisplayTitle(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestComposeDescriptionBounds(t *testing.T) {
	short := composeDescription("Preços em Cuiabá.")
	if n := utf8.RuneCountInString(short); n < 120 || n > 160 {
		t.Errorf("short description length %d: %q", n, short)
	}
	long := composeDescription(strings.Repeat("palavra ", 40))
	if n := utf8.RuneCountInString(long); n > 160 || !strings.HasSuffix(long, "…") {
		t.Errorf("long description length %d: %q", n, long)
	}
	skip := composeDescription("Primeira frase com fatos.", strings.Repeat("x", 150), "Terceira frase curta.")
	if strings.Contains(skip, "xxx") || !strings.Contains(skip, "Terceira frase curta.") {
		t.Errorf("oversized middle part not skipped: %q", skip)
	}
	if strings.Contains(strings.ToLower(composeDescription("a")), "hoje") {
		t.Error("description filler says hoje")
	}
}

func TestPriceTitleFormulas(t *testing.T) {
	if got := cityTitle("Cuiabá", "MT", []string{"ETHANOL", "DIESEL_S10", "GASOLINE_REGULAR", "DIESEL_S500"}, "17/10/2026"); got != "Preço da gasolina, etanol e diesel em Cuiabá (MT) – pesquisa ANP 17/10/2026" {
		t.Errorf("city title = %q", got)
	}
	if got := cityTitle("Sorriso", "MT", []string{"ETHANOL"}, "17/10/2026"); got != "Preço do etanol em Sorriso (MT) – pesquisa ANP 17/10/2026" {
		t.Errorf("single fuel city title = %q", got)
	}
	if got := cityTitle("Sorriso", "MT", []string{"CNG"}, "17/10/2026"); got != "Preço dos combustíveis em Sorriso (MT) – pesquisa ANP 17/10/2026" {
		t.Errorf("other fuel city title = %q", got)
	}
	if got := stateTitle("MT", "17/10/2026"); got != "Preço dos combustíveis em Mato Grosso por cidade – ANP 17/10/2026" {
		t.Errorf("state title = %q", got)
	}
	if got := fuelHubTitle("GASOLINE_REGULAR", "17/10/2026"); got != "Preço da gasolina comum no Brasil por estado – ANP 17/10/2026" {
		t.Errorf("fuel title = %q", got)
	}
	if got := fuelHubTitle("DIESEL_S10", "17/10/2026"); got != "Preço do diesel S10 no Brasil por estado – ANP 17/10/2026" {
		t.Errorf("diesel title = %q", got)
	}
	if got := stationTitle("Auto Posto Cuiabá Ltda", "Cuiabá", "MT"); got != "Auto Posto Cuiabá Ltda – preços em Cuiabá (MT)" {
		t.Errorf("station title = %q", got)
	}
}
