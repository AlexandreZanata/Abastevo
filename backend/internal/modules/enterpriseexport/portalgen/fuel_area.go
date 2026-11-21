package portalgen

import (
	"sort"
	"strings"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport/analytics"
)

// Fuel by area pages (B-BR-SEO-11): one fuel in one state,
// /precos/{fuel}/{uf}/, or in one city, /precos/{fuel}/{uf}/{cidade}/. They
// answer the one-fuel queries that the state and city pages answer only as
// one row among many.

// fuelCityStationCap bounds the station table of a fuel by city page; ties
// at the cutoff stay together.
const fuelCityStationCap = 200

// surveyWeek is one survey week of the snapshot with its summaries.
type surveyWeek struct {
	revision  enterpriseexport.Revision
	summaries map[summaryKey]analytics.Summary
}

// surveyWeeks orders the snapshot's survey weeks newest first; when several
// revisions share a week, the newest revision stands for it.
func surveyWeeks(revisions []enterpriseexport.Revision, projection analytics.Projection) []surveyWeek {
	sorted := append([]enterpriseexport.Revision(nil), revisions...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].SurveyStart != sorted[j].SurveyStart {
			return sorted[i].SurveyStart > sorted[j].SurveyStart
		}
		return sorted[i].RevisionID > sorted[j].RevisionID
	})
	byRevision := map[string]map[summaryKey]analytics.Summary{}
	for _, summary := range projection.Summaries {
		if byRevision[summary.RevisionID] == nil {
			byRevision[summary.RevisionID] = map[summaryKey]analytics.Summary{}
		}
		byRevision[summary.RevisionID][keyOf(summary)] = summary
	}
	weeks := []surveyWeek{}
	for i, revision := range sorted {
		if i > 0 && sorted[i-1].SurveyStart == revision.SurveyStart {
			continue
		}
		weeks = append(weeks, surveyWeek{revision: revision, summaries: byRevision[revision.RevisionID]})
	}
	return weeks
}

// fuelHistoryRow is one survey week of a fuel in an area; Missing marks a
// week without an observation, which is a gap and never a zero price.
type fuelHistoryRow struct {
	Period   string
	Stations int
	Min, Max int64
	Mean     int64
	HasMean  bool
	Missing  bool
}

// fuelHistory lists every survey week of the snapshot for one summary key,
// newest first, from the area's first observation onward.
func (g *generator) fuelHistory(key summaryKey) []fuelHistoryRow {
	rows := []fuelHistoryRow{}
	for _, week := range g.weeks {
		row := fuelHistoryRow{Period: periodLabel(week.revision.SurveyStart, week.revision.SurveyEnd), Missing: true}
		if summary, ok := week.summaries[key]; ok {
			row.Missing, row.Stations, row.Min, row.Max = false, summary.StationCount, summary.MinMilliBRL, summary.MaxMilliBRL
			row.Mean, row.HasMean = summaryMean(summary)
		}
		rows = append(rows, row)
	}
	for len(rows) > 0 && rows[len(rows)-1].Missing {
		rows = rows[:len(rows)-1]
	}
	return rows
}

type fuelAreaPage struct {
	pageBase
	H1         string
	Lead       string
	Summary    []summaryRow
	Comparison *comparisonView
	History    []fuelHistoryRow
	// CitiesTitle and Cities list the cities of a state that have the fuel.
	CitiesTitle string
	Cities      []fuelGeoRow
	Rankings    []rankingView
	// Stations is the price table of a city page; StationsTotal counts the
	// whole sample when the table is cut.
	Stations      []fuelStationRow
	StationsTotal int
	// ContextTitle and Context place a city next to its state and Brazil.
	ContextTitle string
	Context      []fuelContextLine
	// WeekSummary and FAQ are the dated answers of B-BR-SEO-09.
	WeekSummary string
	FAQ         []faqItem
}

// fuelStationRow is one station of a fuel by city page.
type fuelStationRow struct {
	Position  int
	Name      string
	LegalName string
	StationID string
	District  string
	Amount    int64
	Collected string
	TieCount  int
}

// fuelContextLine is the same fuel in a wider area.
type fuelContextLine struct {
	Label    string
	Href     string
	Stations int
	Min, Max int64
	Mean     int64
	HasMean  bool
}

func areaKey(fuel, area string) string { return fuel + "\x00" + area }

func fuelStateRoute(code, uf string) string {
	return fuelRoute(code) + strings.ToLower(uf) + "/"
}

// fuelStateHref is the fuel by state route when this build has that page.
func (g *generator) fuelStateHref(code, uf string) string {
	if g.fuelStatePages[areaKey(code, uf)] {
		return fuelStateRoute(code, uf)
	}
	return ""
}

// fuelCityRoute nests the city slug of /precos/{uf}/{cidade}/ under the fuel.
func fuelCityRoute(code string, city *cityInfo) string {
	return strings.TrimSuffix(fuelRoute(code), "/") + strings.TrimPrefix(city.Route, "/precos")
}

// fuelCityHref is the fuel by city route when this build has that page.
func (g *generator) fuelCityHref(code, municipality string) string {
	if city, ok := g.cities[municipality]; ok && g.fuelCityPages[areaKey(code, municipality)] {
		return fuelCityRoute(code, city)
	}
	return ""
}

// planFuelAreas decides, before any page is written, which fuel by area
// pages this build has, so the other pages can link them. A city needs the
// municipal minimum of stations for the fuel; a state needs the fuel.
func (g *generator) planFuelAreas(summaries []analytics.Summary) {
	g.fuelStatePages, g.fuelCityPages = map[string]bool{}, map[string]bool{}
	for _, summary := range summaries {
		hub, ok := g.fuelHubs[summary.Fuel]
		if !ok || hub.Unit != summary.Unit {
			continue
		}
		code := summary.Geography.Code
		switch summary.Geography.Level {
		case analytics.LevelState:
			if isUF(code) {
				g.fuelStatePages[areaKey(summary.Fuel, code)] = true
			}
		case analytics.LevelMunicipality:
			if city, ok := g.cities[code]; ok && city.HasData && city.Route != "" && summary.StationCount >= minMunicipality {
				g.fuelCityPages[areaKey(summary.Fuel, code)] = true
			}
		}
	}
}

// fuelAreaPhrase is "da gasolina comum" for titles and link labels.
func fuelAreaPhrase(code string) string {
	words := fuelVocabulary[code]
	return words.Article + " " + words.Name
}

// linkFuelRows points each fuel cell at its fuel by area page.
func linkFuelRows(rows []summaryRow, href func(code string) string) {
	for i := range rows {
		if target := href(rows[i].Code); target != "" {
			rows[i].FuelHref = target
		}
	}
}

// fuelStateModule links the fuel by state pages of the given products.
func (g *generator) fuelStateModule(uf string, codes []string) linkModule {
	links := []moduleLink{}
	for _, hub := range g.orderedFuelHubs() {
		for _, code := range codes {
			if code != hub.Code {
				continue
			}
			if href := g.fuelStateHref(code, uf); href != "" {
				links = append(links, moduleLink{Label: "Preço " + fuelAreaPhrase(code) + " em " + ufName(uf), Href: href})
			}
			break
		}
	}
	return linkModule{Title: "Preço por combustível em " + ufName(uf), Links: links}
}

func (g *generator) generateFuelStates(summaries []analytics.Summary, rankings []analytics.Ranking, stations map[string]enterpriseexport.Station) error {
	index := indexSummaries(summaries)
	hubs := g.orderedFuelHubs()
	for _, hub := range hubs {
		states := []string{}
		for uf := range g.statePages {
			if g.fuelStatePages[areaKey(hub.Code, uf)] {
				states = append(states, uf)
			}
		}
		sort.Strings(states)
		for _, uf := range states {
			key := summaryKey{analytics.LevelState, uf, hub.Code, hub.Unit}
			summary := index[key]
			rows := []summaryRow{}
			for _, row := range g.summaryRows(summaries, analytics.LevelState, uf, false) {
				if row.Code == hub.Code && row.Unit == hub.Unit {
					rows = append(rows, row)
				}
			}
			if len(rows) != 1 {
				continue
			}
			cities := []fuelGeoRow{}
			for _, city := range g.dataCities(uf) {
				item, ok := index[summaryKey{analytics.LevelMunicipality, city.Code, hub.Code, hub.Unit}]
				if !ok || !g.cityPages[city.Code] {
					continue
				}
				row := fuelGeoRow{
					Name: city.Name, UF: uf, Href: city.Route, Stations: item.StationCount,
					Min: item.MinMilliBRL, Max: item.MaxMilliBRL, Spread: item.SpreadMilliBRL,
					Insufficient: item.StationCount < minMunicipality,
				}
				if href := g.fuelCityHref(hub.Code, city.Code); href != "" {
					row.Href = href
				}
				row.Mean, row.HasMean = summaryMean(item)
				cities = append(cities, row)
			}
			own := []analytics.Ranking{}
			for _, ranking := range rankings {
				if ranking.Fuel == hub.Code && ranking.Unit == hub.Unit {
					own = append(own, ranking)
				}
			}
			views, err := rankingViews(own, analytics.LevelState, uf, stations, true)
			if err != nil {
				return err
			}
			phrase, state := fuelAreaPhrase(hub.Code), ufName(uf)
			h1 := "Preço " + phrase + " em " + placeLabel(state, uf)
			parts := []string{capitalize(fuelName(hub.Code)) + " em " + state + " na pesquisa ANP " + g.periodPhrase() + ".", rangeSentence(rows[0])}
			if rows[0].HasMean {
				parts = append(parts, "Média da amostra: "+brl(rows[0].Mean)+".")
			}
			parts = append(parts, "Tabela por cidade e histórico por pesquisa.")
			base := g.newBase(pageSpec{
				Route: fuelStateRoute(hub.Code, uf), Facts: indexFacts{Family: familyFuelState, HasCurrentData: summary.StationCount > 0},
				Title: h1, HeadTitle: fitTitle(h1+" – ANP "+g.surveyDate, h1), Description: composeDescription(parts...), Active: "precos",
				Crumbs:  []crumb{{Label: "Preços", Href: "/precos/"}, {Label: hub.Label, Href: hub.Href}, {Label: state, Current: true}},
				LastMod: g.dataDate,
			})
			otherStates := []moduleLink{}
			for _, other := range states {
				if other != uf {
					otherStates = append(otherStates, moduleLink{Label: "Preço " + phrase + " em " + ufName(other), Href: fuelStateRoute(hub.Code, other)})
				}
			}
			otherFuels := []string{}
			for _, other := range hubs {
				if other.Code != hub.Code {
					otherFuels = append(otherFuels, other.Code)
				}
			}
			siblings := g.fuelStateModule(uf, otherFuels)
			siblings.Title = "Outros combustíveis em " + state
			siblings.Links = append(siblings.Links, moduleLink{Label: "Preço dos combustíveis em " + state, Href: stateRoute(uf)})
			base.Modules = []linkModule{
				{Title: hub.Label + " em outros estados", Links: otherStates},
				siblings,
				g.fuelModule("Preço por combustível no Brasil", []string{hub.Code}),
				g.priceGuides(" em "+state, []string{hub.Code}),
			}
			lead := "Preços encontrados na pesquisa da ANP"
			if g.period != "" {
				lead += " de " + g.period
			}
			lead += " em " + state + ", em R$ por " + unitLabel(hub.Unit) + ". Não é preço ao vivo nem estatística oficial da ANP."
			data := fuelAreaPage{
				pageBase: base, H1: h1, Lead: lead, Summary: rows,
				Comparison: g.comparison(summaries, analytics.LevelState, uf, hub.Code), History: g.fuelHistory(key),
				CitiesTitle: "Cidades de " + state + " com preço na amostra", Cities: cities, Rankings: views,
			}
			facts := fuelFacts{Code: hub.Code, Place: state, Row: rows[0], Comparison: data.Comparison}
			data.WeekSummary, data.FAQ = g.weekSummary(facts), g.fuelFAQ(facts)
			items := make([]listItem, 0, len(cities))
			for _, row := range cities {
				items = append(items, listItem{Name: "Preço " + phrase + " em " + placeLabel(row.Name, uf), Href: row.Href})
			}
			if data.pageBase, err = g.collectionBase(data.pageBase, items); err != nil {
				return err
			}
			if err := g.writeRoute("fuelArea", data); err != nil {
				return err
			}
		}
	}
	return nil
}

// fuelStationRows orders one fuel's observations of a city by price, then
// name, then id, and cuts the table at fuelCityStationCap plus ties.
func fuelStationRows(items []observation) ([]fuelStationRow, int) {
	districts := make(map[string]string, len(items))
	for _, item := range items {
		districts[item.station.StationID] = stationDisplayTitle(addressComponent(item.station.Address["district"]))
	}
	ranked := observationRows(items, 0, true)
	total := len(ranked)
	if total > fuelCityStationCap {
		end, cutoff := fuelCityStationCap, ranked[fuelCityStationCap-1].Amount
		for end < total && ranked[end].Amount == cutoff {
			end++
		}
		ranked = ranked[:end]
	}
	rows := make([]fuelStationRow, 0, len(ranked))
	for _, row := range ranked {
		rows = append(rows, fuelStationRow{
			Position: row.Position, Name: row.Name, LegalName: row.LegalName, StationID: row.StationID,
			District: districts[row.StationID], Amount: row.Amount, Collected: row.Collected, TieCount: row.TieCount,
		})
	}
	return rows, total
}

func (g *generator) generateFuelCities(summaries []analytics.Summary, observations []observation) error {
	index := indexSummaries(summaries)
	type fuelCity struct{ fuel, city string }
	grouped := map[fuelCity][]observation{}
	for _, item := range observations {
		key := fuelCity{item.price.FuelProduct, item.station.MunicipalityCode}
		if !g.fuelCityPages[areaKey(key.fuel, key.city)] {
			continue
		}
		if hub := g.fuelHubs[key.fuel]; hub.Unit != item.price.Unit || g.cities[key.city].UF != item.station.State {
			continue
		}
		grouped[key] = append(grouped[key], item)
	}
	hubs := g.orderedFuelHubs()
	for _, hub := range hubs {
		for _, code := range sortedKeys(g.cities) {
			if !g.fuelCityPages[areaKey(hub.Code, code)] {
				continue
			}
			city := g.cities[code]
			uf, state, place := city.UF, ufName(city.UF), placeLabel(city.Name, city.UF)
			key := summaryKey{analytics.LevelMunicipality, code, hub.Code, hub.Unit}
			rows := []summaryRow{}
			for _, row := range g.summaryRows(summaries, analytics.LevelMunicipality, code, true) {
				if row.Code == hub.Code && row.Unit == hub.Unit {
					rows = append(rows, row)
				}
			}
			if len(rows) != 1 {
				continue
			}
			stationRows, total := fuelStationRows(grouped[fuelCity{hub.Code, code}])
			phrase := fuelAreaPhrase(hub.Code)
			h1 := "Preço " + phrase + " em " + place
			parts := []string{capitalize(fuelName(hub.Code)) + " em " + place + " na pesquisa ANP " + g.periodPhrase() + ".", rangeSentence(rows[0])}
			if rows[0].HasMean {
				parts = append(parts, "Média da amostra: "+brl(rows[0].Mean)+".")
			}
			parts = append(parts, "Preço por posto, bairro, data da coleta e histórico.")
			stateCrumb := crumb{Label: state, Href: g.fuelStateHref(hub.Code, uf)}
			if stateCrumb.Href == "" {
				stateCrumb.Href = stateRoute(uf)
			}
			base := g.newBase(pageSpec{
				Route: fuelCityRoute(hub.Code, city), Facts: indexFacts{Family: familyFuelCity, HasCurrentData: total > 0},
				Title: h1, HeadTitle: fitTitle(h1+" – ANP "+g.surveyDate, h1), Description: composeDescription(parts...), Active: "precos",
				Crumbs:  []crumb{{Label: "Preços", Href: "/precos/"}, {Label: hub.Label, Href: hub.Href}, stateCrumb, {Label: city.Name, Current: true}},
				LastMod: g.dataDate,
			})

			context := []fuelContextLine{}
			for _, wider := range []struct {
				key   summaryKey
				label string
				href  string
			}{
				{summaryKey{analytics.LevelState, uf, hub.Code, hub.Unit}, "Preço " + phrase + " em " + state, stateCrumb.Href},
				{summaryKey{analytics.LevelNational, "BR", hub.Code, hub.Unit}, "Preço " + phrase + " no Brasil", hub.Href},
			} {
				summary, ok := index[wider.key]
				if !ok {
					continue
				}
				line := fuelContextLine{Label: wider.label, Href: wider.href, Stations: summary.StationCount, Min: summary.MinMilliBRL, Max: summary.MaxMilliBRL}
				line.Mean, line.HasMean = summaryMean(summary)
				context = append(context, line)
			}

			nearby := []moduleLink{}
			for _, other := range g.nearestDataCities(city, nearbyCities) {
				if href := g.fuelCityHref(hub.Code, other.Code); href != "" {
					nearby = append(nearby, moduleLink{Label: "Preço " + phrase + " em " + placeLabel(other.Name, other.UF), Href: href})
				}
			}
			siblings := []moduleLink{}
			for _, other := range hubs {
				if href := g.fuelCityHref(other.Code, code); href != "" && other.Code != hub.Code {
					siblings = append(siblings, moduleLink{Label: "Preço " + fuelAreaPhrase(other.Code) + " em " + place, Href: href})
				}
			}
			siblings = append(siblings, moduleLink{Label: "Preços dos combustíveis e postos em " + place, Href: city.Route})
			base.Modules = []linkModule{
				{Title: hub.Label + " em cidades próximas", Links: nearby},
				{Title: "Outros combustíveis em " + city.Name, Links: siblings},
				g.priceGuides(" em "+city.Name, []string{hub.Code}),
			}
			lead := "Preços encontrados na pesquisa da ANP"
			if g.period != "" {
				lead += " de " + g.period
			}
			lead += " em " + place + ", em R$ por " + unitLabel(hub.Unit) + ". Não é preço ao vivo nem estatística oficial da ANP."
			data := fuelAreaPage{
				pageBase: base, H1: h1, Lead: lead, Summary: rows,
				Comparison: g.comparison(summaries, analytics.LevelMunicipality, code, hub.Code), History: g.fuelHistory(key),
				Stations: stationRows, StationsTotal: total,
				ContextTitle: hub.Label + " em " + state + " e no Brasil", Context: context,
			}
			facts := fuelFacts{Code: hub.Code, Place: place, Row: rows[0], Comparison: data.Comparison}
			data.WeekSummary, data.FAQ = g.weekSummary(facts), g.fuelFAQ(facts)
			items := make([]listItem, 0, len(stationRows))
			for _, row := range stationRows {
				items = append(items, listItem{Name: row.Name, Href: "/postos/" + row.StationID + "/"})
			}
			var err error
			if data.pageBase, err = g.collectionBase(data.pageBase, items); err != nil {
				return err
			}
			if err := g.writeRoute("fuelArea", data); err != nil {
				return err
			}
		}
	}
	return nil
}
