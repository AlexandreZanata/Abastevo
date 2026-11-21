package portalgen

import (
	"fmt"
	"sort"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport/analytics"
)

// The ethanol to gasoline ratio (B-BR-SEO-12): the sample mean of hydrous
// ethanol over the sample mean of regular gasoline, for one area and one
// survey, beside the general 70% reference. The page states a relation
// between two means; it never says which fuel a reader should buy.
const (
	parityRoute = "/precos/etanol-ou-gasolina/"
	parityLabel = "Etanol ou gasolina por estado e cidade"
	// parityReference is 70,0% in tenths of a percent.
	parityReference = 700
	parityEthanol   = "ETHANOL"
	parityGasoline  = "GASOLINE_REGULAR"
)

// fuelRatio returns the ratio in tenths of a percent, rounded half up. It is
// (sumE ÷ nE) ÷ (sumG ÷ nG) computed as one exact fraction, and it exists only
// when both samples reach the municipal minimum.
func fuelRatio(ethanol, gasoline analytics.Summary) (int64, bool) {
	if ethanol.StationCount < minMunicipality || gasoline.StationCount < minMunicipality ||
		ethanol.SumMilliBRL <= 0 || gasoline.SumMilliBRL <= 0 {
		return 0, false
	}
	numerator := ethanol.SumMilliBRL * int64(gasoline.StationCount) * 1000
	denominator := gasoline.SumMilliBRL * int64(ethanol.StationCount)
	return (2*numerator + denominator) / (2 * denominator), true
}

// percentTenths renders tenths of a percent as "68,0%".
func percentTenths(value int64) string {
	return fmt.Sprintf("%s,%d%%", groupThousands(value/10), value%10)
}

func parityPosition(ratio int64) string {
	switch {
	case ratio < parityReference:
		return "abaixo de 70%"
	case ratio > parityReference:
		return "acima de 70%"
	default:
		return "em 70%"
	}
}

// parityRow is one area of the ratio page.
type parityRow struct {
	Name             string
	Href             string
	EthanolMean      int64
	GasolineMean     int64
	Ratio            string
	EthanolStations  int
	GasolineStations int
	Position         string
}

type parityPage struct {
	pageBase
	Lead           string
	National       []parityRow
	States         []parityRow
	Cities         []parityRow
	CalculatorHref string
	GuideHref      string
}

func parityKey(level, code string) string { return level + "\x00" + code }

// planParity computes the ratio of every area before any page is written,
// so the state and city pages can show their line and link the ratio page.
func (g *generator) planParity(summaries []analytics.Summary) {
	g.parity = map[string]parityRow{}
	index := indexSummaries(summaries)
	for key, ethanol := range index {
		if key.fuel != parityEthanol || key.unit != "L" {
			continue
		}
		gasoline, ok := index[summaryKey{key.level, key.code, parityGasoline, "L"}]
		if !ok {
			continue
		}
		ratio, ok := fuelRatio(ethanol, gasoline)
		if !ok {
			continue
		}
		row := parityRow{
			Ratio: percentTenths(ratio), Position: parityPosition(ratio),
			EthanolStations: ethanol.StationCount, GasolineStations: gasoline.StationCount,
		}
		row.EthanolMean, _ = summaryMean(ethanol)
		row.GasolineMean, _ = summaryMean(gasoline)
		g.parity[parityKey(key.level, key.code)] = row
	}
}

// hasParityPage reports whether this build has the ratio page: it needs the
// national ratio.
func (g *generator) hasParityPage() bool {
	_, ok := g.parity[parityKey(analytics.LevelNational, "BR")]
	return ok
}

// parityLine is the ratio of one area for its own page, or "" when the area
// has no ratio or the build has no ratio page to link.
func (g *generator) parityLine(level, code string) string {
	row, ok := g.parity[parityKey(level, code)]
	if !ok || !g.hasParityPage() {
		return ""
	}
	return row.Ratio
}

// parityModule links the ratio page from the two fuel hubs it compares.
func (g *generator) parityModule(fuel string) linkModule {
	if !g.hasParityPage() || (fuel != parityEthanol && fuel != parityGasoline) {
		return linkModule{}
	}
	return linkModule{Title: "Etanol ou gasolina", Links: []moduleLink{{Label: parityLabel, Href: parityRoute}}}
}

func (g *generator) generateParity() error {
	if !g.hasParityPage() {
		return nil
	}
	national := g.parity[parityKey(analytics.LevelNational, "BR")]
	national.Name, national.Href = "Brasil", "/precos/"
	states, cities := []parityRow{}, []parityRow{}
	for _, uf := range sortedKeys(g.statePages) {
		if row, ok := g.parity[parityKey(analytics.LevelState, uf)]; ok {
			row.Name, row.Href = placeLabel(ufName(uf), uf), stateRoute(uf)
			states = append(states, row)
		}
	}
	ordered := []*cityInfo{}
	for code, city := range g.cities {
		if _, ok := g.parity[parityKey(analytics.LevelMunicipality, code)]; ok && g.cityPages[code] {
			ordered = append(ordered, city)
		}
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].UF != ordered[j].UF {
			return ordered[i].UF < ordered[j].UF
		}
		if ordered[i].Name != ordered[j].Name {
			return ordered[i].Name < ordered[j].Name
		}
		return ordered[i].Code < ordered[j].Code
	})
	for _, city := range ordered {
		row := g.parity[parityKey(analytics.LevelMunicipality, city.Code)]
		row.Name, row.Href = placeLabel(city.Name, city.UF), city.Route
		cities = append(cities, row)
	}

	title := "Etanol ou gasolina por estado e cidade"
	description := composeDescription(
		"Relação entre os preços do etanol e da gasolina comum na pesquisa ANP "+g.periodPhrase()+", por estado e cidade.",
		"No Brasil: "+national.Ratio+".", "Referência geral: 70%.")
	base := g.newBase(pageSpec{
		Route: parityRoute, Facts: indexFacts{Family: familyParity, HasCurrentData: true},
		Title: title, HeadTitle: fitTitle(title+" – ANP "+g.surveyDate, title), Description: description, Active: "precos",
		Crumbs: []crumb{{Label: "Preços", Href: "/precos/"}, {Label: "Etanol ou gasolina", Current: true}}, LastMod: g.dataDate,
	})
	base.Modules = []linkModule{
		g.fuelModule("Preço por combustível no Brasil", []string{parityEthanol, parityGasoline}),
		g.hubModule(parityRoute),
	}
	lead := "Relação entre a média da amostra do etanol hidratado e a da gasolina comum na pesquisa da ANP"
	if g.period != "" {
		lead += " de " + g.period
	}
	data := parityPage{
		pageBase: base, Lead: lead + ". Não é preço ao vivo nem estatística oficial da ANP.",
		National: []parityRow{national}, States: states, Cities: cities,
		CalculatorHref: g.forumLandingHref("/ferramentas/etanol-ou-gasolina/"), GuideHref: g.forumLandingHref("/guias/etanol-ou-gasolina/"),
	}
	items := make([]listItem, 0, len(states)+len(cities))
	for _, row := range append(append([]parityRow{}, states...), cities...) {
		items = append(items, listItem{Name: "Etanol ou gasolina em " + row.Name, Href: row.Href})
	}
	var err error
	if data.pageBase, err = g.collectionBase(data.pageBase, items); err != nil {
		return err
	}
	return g.writeRoute("parity", data)
}
