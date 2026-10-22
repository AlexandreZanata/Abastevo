package domain

import (
	"testing"
)

func resolvedCharging(t *testing.T, question string) ChargingPlaceQuery {
	t.Helper()
	got := Interpret(question, testNow)
	if got.State != StateResolved {
		t.Fatalf("question %q unresolved: %+v", question, got)
	}
	query, ok := got.Query.(ChargingPlaceQuery)
	if !ok {
		t.Fatalf("question %q resolved to non-charging query: %T", question, got.Query)
	}
	return query
}

func TestEVCompatiblePlacesParaphrasesConverge(t *testing.T) {
	for _, question := range []string{
		"Onde posso carregar com conector CCS2?",
		"Onde carregar com CCS 2?",
		"Me mostre eletropostos com CCS2",
	} {
		query := resolvedCharging(t, question)
		if query.Connector != "CCS2" || query.Profile != "ocm-br-20261021-v1" {
			t.Fatal("connector/profile drifted", question, query)
		}
		if query.Geog.Level != "COUNTRY" || query.Geog.Country != "BR" || query.Policy != PolicyVersion {
			t.Fatal("scope drifted", question, query)
		}
		if query.Page.Limit < 1 || query.Page.Limit > 100 {
			t.Fatal("unbounded page", question, query.Page)
		}
	}
}

func TestEVMissingConnectorAsksInsteadOfGuessing(t *testing.T) {
	got := Interpret("Onde posso carregar meu carro?", testNow)
	if got.State != StateNeedsChoice || got.Reason != ReasonConnectorRequired {
		t.Fatalf("missing connector not asked: %+v", got)
	}
	if len(got.Alternatives) != 4 {
		t.Fatalf("expected four connector alternatives: %+v", got.Alternatives)
	}
}

func TestEVAmbiguousCheapestFastestAsks(t *testing.T) {
	for _, question := range []string{
		"Qual a recarga mais barata?",
		"Onde carregar mais rápido?",
	} {
		got := Interpret(question, testNow)
		if got.State != StateNeedsChoice || got.Reason != ReasonMetricRequired {
			t.Fatalf("ambiguous metric not asked %q: %+v", question, got)
		}
		if len(got.Alternatives) != 2 {
			t.Fatalf("expected price/speed alternatives %q: %+v", question, got.Alternatives)
		}
	}
}

func TestEVFreeWithParkingAsks(t *testing.T) {
	got := Interpret("Recarga grátis com estacionamento?", testNow)
	if got.State != StateNeedsChoice {
		t.Fatalf("free-with-parking not asked: %+v", got)
	}
}

func TestEVMissingScenarioComponentAsks(t *testing.T) {
	got := Interpret("Quanto custa carregar?", testNow)
	if got.State != StateNeedsChoice || got.Reason != ReasonComponentRequired {
		t.Fatalf("missing component not asked: %+v", got)
	}
	if len(got.Alternatives) != 3 {
		t.Fatalf("expected three dimension alternatives: %+v", got.Alternatives)
	}
}

func TestEVScenarioResolvesDeclaredDimensions(t *testing.T) {
	got := Interpret("Quanto custa carregar 50 kWh com 2 horas de estacionamento?", testNow)
	if got.State != StateResolved {
		t.Fatalf("scenario unresolved: %+v", got)
	}
	scenario, ok := got.Query.(ScenarioCostQuery)
	if !ok {
		t.Fatalf("non-scenario query: %T", got.Query)
	}
	if !scenario.Dimensions["ENERGY_WH"] || !scenario.Dimensions["PARKING_SECOND"] {
		t.Fatalf("dimensions lost: %+v", scenario)
	}
	if len(scenario.Missing) == 0 {
		t.Fatal("missing dimensions not disclosed")
	}
}

func TestEVEnergyPriceKeepsConditionAndGeography(t *testing.T) {
	got := Interpret("Qual o preço do kWh para recarga no RJ?", testNow)
	if got.State != StateResolved {
		t.Fatalf("energy price unresolved: %+v", got)
	}
	query, ok := got.Query.(EnergyPriceQuery)
	if !ok {
		t.Fatalf("non-energy query: %T", got.Query)
	}
	if query.Geog.Level != "STATE" || query.Geog.UF != "RJ" {
		t.Fatalf("geography drifted: %+v", query.Geog)
	}
}

func TestEVAdversarialAndLiquidStayOut(t *testing.T) {
	for question, want := range map[string]string{
		"Qual o maior preço observado da gasolina comum no Brasil?": "not-ev",
		"Quais ações de fiscalização da ANP no DF?":                 "not-ev",
		"'; DROP TABLE charging; --":                                ReasonUnsupportedIntent,
		"Quanto custa o kWh da tomada de casa?":                      "energy",
	} {
		got := Interpret(question, testNow)
		switch want {
		case "not-ev":
			if _, ok := got.Query.(ChargingPlaceQuery); ok {
				t.Fatalf("EV stole liquid question %q", question)
			}
			if _, ok := got.Query.(EnergyPriceQuery); ok {
				t.Fatalf("EV stole liquid question %q", question)
			}
			if _, ok := got.Query.(ScenarioCostQuery); ok {
				t.Fatalf("EV stole liquid question %q", question)
			}
		case "energy":
			if _, ok := got.Query.(EnergyPriceQuery); got.State == StateResolved && !ok {
				t.Fatalf("grid wording not read as energy price %q: %+v", question, got)
			}
		default:
			if got.State != StateUnsupported || got.Reason != want {
				t.Fatalf("adversarial not refused %q: %+v", question, got)
			}
		}
	}
}

func TestEVQueriesCarryServerFilters(t *testing.T) {
	place := resolvedCharging(t, "Onde carregar com CHAdeMO em São Paulo?")
	if place.Profile == "" || place.Page.Limit < 1 || place.Page.Limit > 100 {
		t.Fatalf("server filter missing: %+v", place)
	}
	if place.Geog.Level != "STATE" || place.Geog.UF != "SP" {
		t.Fatalf("geography drifted: %+v", place.Geog)
	}
}
