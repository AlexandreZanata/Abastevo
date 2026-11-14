package portalgen

import (
	"fmt"
	"sort"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport/analytics"
)

// fuelHubLink is one fuel hub as listed on other pages.
type fuelHubLink struct {
	Code  string
	Label string
	Href  string
	Unit  string
	Min   int64
	Max   int64
}

// fuelGeoRow is one state or city row of a fuel hub; a small city sample
// shows no minimum or maximum.
type fuelGeoRow struct {
	Name         string
	UF           string
	Href         string
	Stations     int
	Min          int64
	Max          int64
	Spread       int64
	Insufficient bool
}

type fuelHubPage struct {
	pageBase
	Code      string
	Label     string
	H1        string
	UnitLabel string
	National  []summaryRow
	States    []fuelGeoRow
	Cities    []fuelGeoRow
	Others    []fuelHubLink
}

// unitLabel names a canonical unit for "R$ por ..." captions.
func unitLabel(unit string) string {
	switch unit {
	case "L":
		return "litro"
	case "M3":
		return "metro cúbico (m³)"
	case "KG_13":
		return "botijão de 13 kg"
	default:
		return unit
	}
}

// planFuelHubs selects the fuels with a hub: products of the current survey's
// national summary with a known slug, in their canonical unit only, so no
// page compares different units.
func planFuelHubs(summaries []analytics.Summary) map[string]fuelHubLink {
	hubs := map[string]fuelHubLink{}
	for _, summary := range summaries {
		if summary.Geography.Level != analytics.LevelNational || fuelSlug(summary.Fuel) == "" {
			continue
		}
		if unit, err := productUnit(summary.Fuel); err != nil || unit != summary.Unit {
			continue
		}
		hubs[summary.Fuel] = fuelHubLink{
			Code: summary.Fuel, Label: fuelLabel(summary.Fuel), Href: fuelRoute(summary.Fuel), Unit: summary.Unit,
			Min: summary.MinMilliBRL, Max: summary.MaxMilliBRL,
		}
	}
	return hubs
}

// orderedFuelHubs lists hubs in the drivers' fuel order.
func (g *generator) orderedFuelHubs() []fuelHubLink {
	out := make([]fuelHubLink, 0, len(g.fuelHubs))
	for _, hub := range g.fuelHubs {
		out = append(out, hub)
	}
	sort.Slice(out, func(i, j int) bool {
		if fuelRank(out[i].Code) != fuelRank(out[j].Code) {
			return fuelRank(out[i].Code) < fuelRank(out[j].Code)
		}
		return out[i].Code < out[j].Code
	})
	return out
}

// summaryRows adds the fuel hub link to each row with a hub.
func (g *generator) summaryRows(summaries []analytics.Summary, level, code string, markInsufficient bool) []summaryRow {
	rows := summaryRows(summaries, level, code, markInsufficient)
	for i := range rows {
		if hub, ok := g.fuelHubs[rows[i].Code]; ok && hub.Unit == rows[i].Unit {
			rows[i].FuelHref = hub.Href
		}
	}
	return rows
}

func (g *generator) generateFuelHubs(summaries []analytics.Summary, municipalityName map[string]string) error {
	hubs := g.orderedFuelHubs()
	for _, hub := range hubs {
		national := []summaryRow{}
		for _, row := range g.summaryRows(summaries, analytics.LevelNational, "BR", false) {
			if row.Code == hub.Code && row.Unit == hub.Unit {
				national = append(national, row)
			}
		}
		states, cities := []fuelGeoRow{}, []fuelGeoRow{}
		for _, summary := range summaries {
			if summary.Fuel != hub.Code || summary.Unit != hub.Unit {
				continue
			}
			row := fuelGeoRow{Stations: summary.StationCount, Min: summary.MinMilliBRL, Max: summary.MaxMilliBRL, Spread: summary.SpreadMilliBRL}
			switch summary.Geography.Level {
			case analytics.LevelState:
				if !g.statePages[summary.Geography.Code] {
					continue
				}
				row.UF, row.Name, row.Href = summary.Geography.Code, ufName(summary.Geography.Code), stateRoute(summary.Geography.Code)
				states = append(states, row)
			case analytics.LevelMunicipality:
				code := summary.Geography.Code
				if !g.cityPages[code] {
					continue
				}
				row.Name, row.UF, row.Href = municipalityName[code], g.cityState[code], g.cityRoutes[code]
				if row.Name == "" {
					row.Name = "Município " + code
				}
				row.Insufficient = summary.StationCount < minMunicipality
				cities = append(cities, row)
			}
		}
		sort.Slice(states, func(i, j int) bool { return states[i].Name < states[j].Name })
		sort.Slice(cities, func(i, j int) bool {
			if cities[i].UF != cities[j].UF {
				return cities[i].UF < cities[j].UF
			}
			if cities[i].Name != cities[j].Name {
				return cities[i].Name < cities[j].Name
			}
			return cities[i].Href < cities[j].Href
		})
		others := make([]fuelHubLink, 0, len(hubs)-1)
		for _, other := range hubs {
			if other.Code != hub.Code {
				others = append(others, other)
			}
		}
		words := fuelVocabulary[hub.Code]
		parts := []string{capitalize(words.Name) + " na pesquisa ANP " + g.periodPhrase() + "."}
		if len(national) == 1 {
			row := national[0]
			parts = append(parts, fmt.Sprintf("No Brasil: %s a %s em %s postos de %d %s.", brl(row.Min), brl(row.Max),
				groupThousands(int64(row.Stations)), len(states), plural(len(states), "estado", "estados")))
		}
		parts = append(parts, "Tabela por estado e cidades com preço na amostra.")
		data := fuelHubPage{
			pageBase: g.newBase(pageSpec{
				Route: hub.Href, Facts: indexFacts{Family: familyFuel, HasCurrentData: len(national) == 1},
				Title: "Preço " + words.Article + " " + words.Name + " no Brasil", HeadTitle: fuelHubTitle(hub.Code, g.surveyDate),
				Description: composeDescription(parts...), Active: "precos",
				Crumbs: []crumb{{Label: "Preços", Href: "/precos/"}, {Label: hub.Label, Current: true}}, LastMod: g.dataDate,
			}),
			Code: hub.Code, Label: hub.Label, H1: "Preço " + words.Article + " " + words.Name + " no Brasil",
			UnitLabel: unitLabel(hub.Unit), National: national, States: states, Cities: cities, Others: others,
		}
		if err := g.writeRoute("fuel", data); err != nil {
			return err
		}
	}
	return nil
}
