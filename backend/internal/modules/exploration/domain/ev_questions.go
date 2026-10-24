package domain

import (
	"time"
)

// EE-25 electric question intents. Typed, deterministic and reviewable:
// compatible charging places, reported energy prices and complete
// scenario costs. Visible source/conditions stay on every query;
// unknown fees are left to the tariff engine, never defaulted here.
// Nothing here dials, stores or prices: execution resolves coverage
// and cancels late responses on its own bounded path.
const (
	KindChargingPlaces      = "CHARGING_COMPATIBLE_PLACES"
	KindEnergyPrice         = "REPORTED_ENERGY_PRICE"
	KindScenarioCost        = "COMPLETE_SCENARIO_COST"
	ReasonConnectorRequired = "CONNECTOR_REQUIRED"
	ReasonComponentRequired = "COMPONENT_REQUIRED"
)

// ChargingPlaceQuery is one typed compatible-place question. Connector,
// current and access filter server-side; empty means unfiltered, never
// a hidden default. Profile is pinned to the admitted open corpus.
type ChargingPlaceQuery struct {
	Version   int       `json:"version"`
	Kind      string    `json:"kind"`
	Profile   string    `json:"profile"`
	Connector string    `json:"connector,omitempty"`
	Current   string    `json:"current,omitempty"`
	Access    string    `json:"access,omitempty"`
	Geog      Geography `json:"geography"`
	Page      Page      `json:"page"`
	Policy    string    `json:"policy_version"`
}

// EnergyPriceQuery is one typed reported-energy-price question with an
// exact condition. Qualifier IDs are never invented: member/app wording
// without a qualifier refuses upstream instead of guessing.
type EnergyPriceQuery struct {
	Version int       `json:"version"`
	Kind    string    `json:"kind"`
	Profile string    `json:"profile"`
	Cond    Condition `json:"condition"`
	Geog    Geography `json:"geography"`
	Sort    string    `json:"sort"`
	Page    Page      `json:"page"`
	Policy  string    `json:"policy_version"`
}

// ScenarioCostQuery is one typed complete-scenario question. Dimensions
// lists the declared billing dimensions; Missing lists the undeclared
// ones the executor reports as unknown fees. No quantities travel here:
// amounts are scenario inputs at execution, never query defaults.
type ScenarioCostQuery struct {
	Version    int             `json:"version"`
	Kind       string          `json:"kind"`
	Profile    string          `json:"profile"`
	Dimensions map[string]bool `json:"dimensions"`
	Missing    []string        `json:"missing"`
	Cond       Condition       `json:"condition"`
	Geog       Geography       `json:"geography"`
	Page       Page            `json:"page"`
	Policy     string          `json:"policy_version"`
}

// Scenario dimensions mirror the tariff engine vocabulary.
var scenarioDimensions = []struct {
	dimension string
	fragments []string
}{
	{"ENERGY_WH", []string{"kwh", "kw h", "quilowatt", "energia", "carga"}},
	{"CHARGING_SECOND", []string{"tempo de recarga", "tempo carregando", "duracao da recarga", "minutos de recarga", "horas de recarga"}},
	{"PARKING_SECOND", []string{"estacionamento", "estacionar", "tempo estacionado"}},
	{"SESSION", []string{"sessao", "sessoes", "por recarga", "por carga"}},
}

// evScore counts allowlisted charging signals. EV nouns weigh most;
// generic energy words weigh least. English fragments are deliberately
// absent so injected ASCII keywords never route here.
func evScore(normalized string) int {
	score := 0
	for _, fragment := range []string{"carregar", "carregando", "recarga", "recarregar", "eletroposto", "eletropostos", "carregador", "carregadores", "ponto de recarga", "pontos de recarga", "kwh", "conector", "conectores", "chademo", "ccs", "tipo 2", "nacs", "tomada tipo 2"} {
		if contains(normalized, fragment) {
			score += 3
			break
		}
	}
	if contains(normalized, "bateria", "autonomia", "eletrico", "eletrica", "eletromobilidade") {
		score++
	}
	return score
}

var evConnectors = []struct {
	code      string
	fragments []string
}{
	{"CCS2", []string{"ccs2", "ccs 2", "combo 2", "combo2"}},
	{"TYPE2", []string{"tipo 2", "tipo2", "type 2", "mennekes"}},
	{"CHADEMO", []string{"chademo", "cha de mo"}},
	{"NACS", []string{"nacs", "tesla", "padrao tesla"}},
}

// extractConnector maps normalized text to an exact connector code,
// preferring the longest match. An unnamed connector is missing, never
// guessed.
func extractConnector(normalized string) (string, bool) {
	best, bestLen := "", 0
	for _, c := range evConnectors {
		for _, fragment := range c.fragments {
			if contains(normalized, fragment) && len(fragment) > bestLen {
				best, bestLen = c.code, len(fragment)
			}
		}
	}
	return best, best == ""
}

func evPlaceQuery(connector, current, access string, geography Geography) ChargingPlaceQuery {
	return ChargingPlaceQuery{
		Version: 1, Kind: KindChargingPlaces, Profile: "ocm-br-20261021-v1",
		Connector: connector, Current: current, Access: access,
		Geog: geography, Page: Page{Limit: 20}, Policy: PolicyVersion,
	}
}

func evPriceQuery(condition Condition, geography Geography, sort string) EnergyPriceQuery {
	if sort != "ASC" {
		sort = "DESC"
	}
	return EnergyPriceQuery{
		Version: 1, Kind: KindEnergyPrice, Profile: "ocm-br-20261021-v1",
		Cond: condition, Geog: geography, Sort: sort, Page: Page{Limit: 20}, Policy: PolicyVersion,
	}
}

func evScenarioQuery(dimensions map[string]bool, condition Condition, geography Geography) ScenarioCostQuery {
	known := map[string]bool{"ENERGY_WH": true, "CHARGING_SECOND": true, "PARKING_SECOND": true, "SESSION": true}
	missing := []string{}
	for dimension := range known {
		if !dimensions[dimension] {
			missing = append(missing, dimension)
		}
	}
	return ScenarioCostQuery{
		Version: 1, Kind: KindScenarioCost, Profile: "ocm-br-20261021-v1",
		Dimensions: dimensions, Missing: missing,
		Cond: condition, Geog: geography, Page: Page{Limit: 20}, Policy: PolicyVersion,
	}
}

func connectorAlternatives(geography Geography) []any {
	out := make([]any, 0, 4)
	for _, code := range []string{"CCS2", "TYPE2", "CHADEMO", "NACS"} {
		out = append(out, evPlaceQuery(code, "", "", geography))
	}
	return out
}

// interpretEV resolves charging questions. Place, price and scenario
// intents stay distinct; ambiguous cheapest/fastest, free-with-parking
// and missing components ask with labelled concrete alternatives;
// member/app wording without a qualifier refuses instead of inventing
// one. Geography reuses the shared extractor (never invented IBGE).
func interpretEV(normalized string, now time.Time) Interpretation {
	_ = now
	geography := extractGeography(normalized)
	connector, missingConnector := extractConnector(normalized)
	hasPlace := contains(normalized, "onde", "eletroposto", "eletropostos", "ponto de recarga", "pontos de recarga", "perto", "proximo", "proxima", "estacao de recarga", "endereco")
	hasPrice := contains(normalized, "preco", "precos", "custa", "custam", "tarifa", "tarifas", "valor", "valores", "caro", "cara")
	hasScenario := contains(normalized, "total", "completa", "completo", "custo total", "quanto custa carregar", "incluindo", "incluindo estacionamento", "com estacionamento")
	hasFree := contains(normalized, "gratis", "gratuito", "gratuita", "de graca", "sem pagar", "sem custo")
	hasCheapest := contains(normalized, "mais barato", "mais barata", "cheapest")
	hasFastest := contains(normalized, "mais rapid", "mais veloz", "fastest")
	if unsupportedCondition(normalized) {
		return Interpretation{State: StateUnsupported, Reason: ReasonUnsupportedFilter}
	}
	condition := extractCondition(normalized)
	if hasCheapest || hasFastest {
		return Interpretation{State: StateNeedsChoice, Reason: ReasonMetricRequired, Alternatives: []any{
			evPriceQuery(condition, geography, sortDirection(normalized)),
			evPlaceQuery("", "DC", "", geography),
		}}
	}
	if hasFree && (hasScenario || hasPlace) {
		return Interpretation{State: StateNeedsChoice, Reason: ReasonComponentRequired, Alternatives: []any{
			evScenarioQuery(map[string]bool{"ENERGY_WH": true}, condition, geography),
			evScenarioQuery(map[string]bool{"PARKING_SECOND": true}, condition, geography),
		}}
	}
	// A scenario question absorbs price wording ("quanto custa carregar"):
	// it is one intent, never a price/scenario pair.
	count := 0
	for _, present := range []bool{hasPlace, hasPrice && !hasScenario, hasScenario} {
		if present {
			count++
		}
	}
	if count > 1 {
		return Interpretation{State: StateUnsupported, Reason: ReasonUnsupportedIntent}
	}
	switch {
	case hasScenario:
		dimensions := map[string]bool{}
		for _, dimension := range scenarioDimensions {
			if contains(normalized, dimension.fragments...) {
				dimensions[dimension.dimension] = true
			}
		}
		if len(dimensions) == 0 {
			return Interpretation{State: StateNeedsChoice, Reason: ReasonComponentRequired, Alternatives: []any{
				evScenarioQuery(map[string]bool{"ENERGY_WH": true}, condition, geography),
				evScenarioQuery(map[string]bool{"CHARGING_SECOND": true}, condition, geography),
				evScenarioQuery(map[string]bool{"PARKING_SECOND": true}, condition, geography),
			}}
		}
		return Interpretation{State: StateResolved, Query: evScenarioQuery(dimensions, condition, geography)}
	case hasPrice:
		return Interpretation{State: StateResolved, Query: evPriceQuery(condition, geography, sortDirection(normalized))}
	default:
		if missingConnector {
			return Interpretation{State: StateNeedsChoice, Reason: ReasonConnectorRequired, Alternatives: connectorAlternatives(geography)}
		}
		current := ""
		if contains(normalized, "rapida", "rapido", "fast", "dc", "corrente continua") {
			current = "DC"
		} else if contains(normalized, "lenta", "lento", "ac", "corrente alternada") {
			current = "AC"
		}
		return Interpretation{State: StateResolved, Query: evPlaceQuery(connector, current, "", geography)}
	}
}
