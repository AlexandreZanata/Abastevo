package domain

import (
	"strings"
	"testing"
	"time"
)

var testNow = time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)

func resolvedPrice(t *testing.T, question string) PriceQuery {
	t.Helper()
	got := Interpret(question, testNow)
	if got.State != StateResolved {
		t.Fatalf("question %q unresolved: %+v", question, got)
	}
	query, ok := got.Query.(PriceQuery)
	if !ok {
		t.Fatalf("question %q resolved to non-price query: %T", question, got.Query)
	}
	return query
}

func TestInterpretResolvesParaphrasesToOneTypedQuery(t *testing.T) {
	for _, question := range []string{
		"Qual o maior preço observado da gasolina comum no Brasil?",
		"Onde está o maior preço observado da gasolina comum no país?",
		"Me mostra o maior preco observado de gasolina comum",
	} {
		query := resolvedPrice(t, question)
		if query.Product.Fuel != "GASOLINE_REGULAR" || query.Product.Unit != "L" {
			t.Fatal("fuel drifted", question, query.Product)
		}
		if query.Metric.Kind != "OBSERVATION_PRICE" || query.Metric.GroupBy != "STATION" || query.Metric.Sort != "DESC" {
			t.Fatal("metric drifted", question, query.Metric)
		}
		if query.Geog.Level != "COUNTRY" || query.Geog.Country != "BR" {
			t.Fatal("geography drifted", question, query.Geog)
		}
		if query.Source.Kind != "ANP_WEEKLY" || query.Source.SurveyWeekStart != "2026-10-18" {
			t.Fatal("week drifted", question, query.Source)
		}
		if query.Cond.Kind != "STANDARD" || query.Policy != PolicyVersion || query.Page.Limit != 20 {
			t.Fatal("defaults drifted", question, query)
		}
	}
}

func TestInterpretReadsAccentsAndUFSpellings(t *testing.T) {
	for question, uf := range map[string]string{
		"Maior preço observado do etanol em São Paulo":          "SP",
		"Maior preco observado do etanol em Sao Paulo":          "SP",
		"Maior preço observado do etanol em SP":                 "SP",
		"Menor preço observado do diesel s10 no Rio de Janeiro": "RJ",
		"Menor preço observado do diesel s10 no RJ":             "RJ",
		"Menor preço observado do GNV no Distrito Federal":      "DF",
		"Menor preço observado do GNV em Brasília":              "DF",
		"Menor preço observado do GNV em Brasilia":              "DF",
	} {
		query := resolvedPrice(t, question)
		if query.Geog.Level != "STATE" || query.Geog.UF != uf {
			t.Fatal("geography misread", question, query.Geog)
		}
	}
}

func TestInterpretAsksExplicitMetricForMostExpensive(t *testing.T) {
	got := Interpret("Qual a gasolina mais cara do Brasil?", testNow)
	if got.State != StateNeedsChoice || got.Reason != ReasonMetricRequired || len(got.Alternatives) != 2 {
		t.Fatalf("ambiguous metric not offered: %+v", got)
	}
	kinds := map[string]bool{}
	for _, alternative := range got.Alternatives {
		query, ok := alternative.(PriceQuery)
		if !ok {
			t.Fatal("alternative outside typed vocabulary", alternative)
		}
		kinds[query.Metric.Kind+"/"+query.Metric.GroupBy] = true
	}
	if !kinds["OBSERVATION_PRICE/STATION"] || !kinds["AREA_MEAN/STATE"] {
		t.Fatal("metric alternatives incomplete", kinds)
	}
}

func TestInterpretRefusesUnknownFuelAndCondition(t *testing.T) {
	got := Interpret("Qual o preço da eletricidade no Brasil?", testNow)
	if got.State != StateUnsupported || got.Reason != ReasonUnknownFuel {
		t.Fatalf("unknown fuel fabricated: %+v", got)
	}
	got = Interpret("Qual o preço da gasolina no pix?", testNow)
	if got.State != StateUnsupported || got.Reason != ReasonUnsupportedFilter {
		t.Fatalf("unknown condition dropped: %+v", got)
	}
}

func TestInterpretTreatsAdversarialInputLiterally(t *testing.T) {
	for _, question := range []string{
		"'; DROP TABLE quality_revisions; --",
		"gasolina <script>alert(1)</script>",
		"http://evil.example.com/gasolina",
		strings.Repeat("gasolina cara ", 100),
		"",
	} {
		got := Interpret(question, testNow)
		if got.State == StateResolved {
			if query, ok := got.Query.(PriceQuery); ok && query.Product.Fuel == "" {
				t.Fatal("empty fuel resolved", question)
			}
		}
		if got.State != StateUnsupported && got.State != StateNeedsChoice {
			t.Fatal("adversarial input mishandled", question, got.State)
		}
	}
	long := strings.Repeat("a", MaxQuestionRunes+1)
	if got := Interpret(long, testNow); got.State != StateUnsupported {
		t.Fatal("oversize input accepted")
	}
}

func TestInterpretResolvesOfficialSourcesSeparately(t *testing.T) {
	for question, want := range map[string][2]string{
		"Quais ações de fiscalização da ANP no DF?":         {"enforcement", "ENFORCEMENT_ACTION"},
		"Quais multas aplicadas em São Paulo?":              {"sanctions", "FINE_RECORD"},
		"Quais análises de qualidade do combustível no RJ?": {"pmqc", "ASSAY"},
	} {
		got := Interpret(question, testNow)
		if got.State != StateResolved {
			t.Fatalf("question %q unresolved: %+v", question, got)
		}
		query, ok := got.Query.(OfficialQuery)
		if !ok || len(query.Sources) != 1 || query.Sources[0] != want[0] || len(query.RecordKinds) != 1 || query.RecordKinds[0] != want[1] {
			t.Fatalf("question %q misrouted: %+v", question, got.Query)
		}
		if query.Date.Basis != "EVENT_DAY" || query.Status.Kind != "ANY" || query.Policy != PolicyVersion {
			t.Fatal("official basis lost", query)
		}
	}
	got := Interpret("Quais registros da ANP existem?", testNow)
	if got.State != StateNeedsChoice || got.Reason != ReasonSourceRequired || len(got.Alternatives) != 3 {
		t.Fatalf("unclear source not offered: %+v", got)
	}
}

func TestInterpretAsksDieselVariantExplicitly(t *testing.T) {
	got := Interpret("Qual o menor preço observado do diesel no Brasil?", testNow)
	if got.State != StateNeedsChoice || got.Reason != ReasonFuelRequired || len(got.Alternatives) != 2 {
		t.Fatalf("diesel variant not offered: %+v", got)
	}
	fuels := map[string]bool{}
	for _, alternative := range got.Alternatives {
		query := alternative.(PriceQuery)
		fuels[query.Product.Fuel] = true
	}
	if !fuels["DIESEL_S10"] || !fuels["DIESEL_S500"] {
		t.Fatal("diesel alternatives incomplete", fuels)
	}
}

func TestInterpretKeepsUnitsAndConditionsExact(t *testing.T) {
	query := resolvedPrice(t, "Qual o menor preço observado do GNV no Brasil?")
	if query.Product.Fuel != "CNG" || query.Product.Unit != "M3" {
		t.Fatal("CNG unit drifted", query.Product)
	}
	if query.Metric.Sort != "ASC" || query.Metric.Kind != "OBSERVATION_PRICE" {
		t.Fatal("cheapest direction lost", query.Metric)
	}
	query = resolvedPrice(t, "Qual o menor preço observado do GLP?")
	if query.Product.Fuel != "LPG_P13" || query.Product.Unit != "KG_13" {
		t.Fatal("LPG unit drifted", query.Product)
	}
	// Bare "mais barato" without a metric kind asks explicitly, like "mais caro".
	got := Interpret("Qual o preço do GNV mais barato no Brasil?", testNow)
	if got.State != StateNeedsChoice || got.Reason != ReasonMetricRequired {
		t.Fatalf("bare cheapest not offered: %+v", got)
	}
}
