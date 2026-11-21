package portalgen

import (
	"sort"
	"strings"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport/analytics"
)

// Fuel by area pages (B-BR-SEO-11): one fuel in one state,
// /precos/{fuel}/{uf}/. They answer the one-fuel queries that the state page
// answers only as one row among many.

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

// planFuelAreas decides, before any page is written, which fuel by area
// pages this build has, so the other pages can link them.
func (g *generator) planFuelAreas(summaries []analytics.Summary) {
	g.fuelStatePages = map[string]bool{}
	for _, summary := range summaries {
		hub, ok := g.fuelHubs[summary.Fuel]
		if !ok || hub.Unit != summary.Unit {
			continue
		}
		if summary.Geography.Level == analytics.LevelState && isUF(summary.Geography.Code) {
			g.fuelStatePages[areaKey(summary.Fuel, summary.Geography.Code)] = true
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
