package portalgen

import (
	"fmt"
	"sort"
	"strings"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport/analytics"
)

// pageSpec describes one page head: its route, indexation facts and text.
type pageSpec struct {
	Route       string
	Facts       indexFacts
	Title       string
	HeadTitle   string
	Description string
	Active      string
	Crumbs      []crumb
	LastMod     string
}

// newBase applies the single indexation decision, the global switch and the
// head contract (canonical, Open Graph, breadcrumb root) to one page.
func (g *generator) newBase(spec pageSpec) pageBase {
	decision := pageIndexable(spec.Facts)
	head := spec.HeadTitle
	if head == "" {
		head = spec.Title + " | abastevo"
	}
	lastMod := spec.LastMod
	if lastMod == "" {
		lastMod = g.buildDate
	}
	base := pageBase{
		Title: spec.Title, HeadTitle: head, Description: spec.Description, ActiveNav: spec.Active,
		Indexable: g.indexable && decision, Robots: robotsValue(g.indexable, decision),
		OfficialOnly: g.opts.OfficialOnly, HasLanding: g.opts.LandingURL != "", Landing: g.landingPrefix,
		Period: g.period, PublishedAt: g.published, InputHash: g.inputHash,
		CSSURL: g.cssURL, ScriptURL: g.scriptURL,
		Route: spec.Route, Family: spec.Facts.Family, LastMod: lastMod,
	}
	crumbs := spec.Crumbs
	if base.HasLanding {
		crumbs = append([]crumb{{Label: "Início", Href: g.landingPrefix + "/"}}, crumbs...)
	}
	base.Breadcrumbs = crumbs
	if g.siteURL != "" && spec.Route != "" {
		base.Canonical = g.siteURL + spec.Route
		base.OGImage = g.siteURL + ogImagePath
	}
	return base
}

func (g *generator) periodPhrase() string {
	if g.periodShort == "" {
		return "mais recente"
	}
	return "de " + g.periodShort
}

func stateLinks(summaries []analytics.Summary, states map[string]int) []stateLink {
	links := make([]stateLink, 0, len(states))
	for code, stations := range states {
		fuels := map[string]bool{}
		for _, summary := range summaries {
			if summary.Geography.Level == analytics.LevelState && summary.Geography.Code == code {
				fuels[summary.Fuel] = true
			}
		}
		links = append(links, stateLink{Code: code, Name: ufName(code), Href: stateRoute(code), Stations: stations, Fuels: len(fuels)})
	}
	sort.Slice(links, func(i, j int) bool { return links[i].Code < links[j].Code })
	return links
}

func (g *generator) generateConsultar(summaries []analytics.Summary, states map[string]int, searchStates []searchState, release string) error {
	total := 0
	for _, count := range states {
		total += count
	}
	description := composeDescription(
		"Busque postos de combustíveis por nome, CNPJ, cidade ou estado.",
		"Preços da pesquisa ANP "+g.periodPhrase()+" com data da coleta e fonte.",
		fmt.Sprintf("%s postos com preço na amostra.", groupThousands(int64(total))),
	)
	data := consultarPage{
		pageBase: g.newBase(pageSpec{
			Route: "/consultar/", Facts: indexFacts{Family: familyHub},
			Title: "Consultar preços de combustíveis", HeadTitle: "Consultar preços de combustíveis por posto, cidade ou CNPJ | abastevo",
			Description: description, Active: "consultar", Crumbs: []crumb{{Label: "Consultar", Current: true}}, LastMod: g.dataDate,
		}),
		States: stateLinks(summaries, states), National: summaryRows(summaries, analytics.LevelNational, "BR", false),
		SearchStates: searchStates, SearchRelease: release,
	}
	return g.writeRoute("consultar", data)
}

func (g *generator) generatePrecos(summaries []analytics.Summary, rankings []analytics.Ranking, stations map[string]enterpriseexport.Station, states map[string]int) error {
	views, err := rankingViews(rankings, analytics.LevelNational, "", stations, true)
	if err != nil {
		return err
	}
	head := "Preços de combustíveis no Brasil por estado e cidade | abastevo"
	if g.surveyDate != "" {
		head = "Preços de combustíveis no Brasil por estado e cidade – ANP " + g.surveyDate
	}
	national := summaryRows(summaries, analytics.LevelNational, "BR", false)
	parts := []string{"Preços de combustíveis na pesquisa ANP " + g.periodPhrase() + ": resumo nacional, por combustível, estado e cidade."}
	for _, row := range byFuelRank(national) {
		parts = append(parts, rangeSentence(row))
	}
	data := precosPage{
		pageBase: g.newBase(pageSpec{
			Route: "/precos/", Facts: indexFacts{Family: familyHub},
			Title: "Preços por região", HeadTitle: head, Description: composeDescription(parts...),
			Active: "precos", Crumbs: []crumb{{Label: "Preços", Current: true}}, LastMod: g.dataDate,
		}),
		National: national, Rankings: views, States: stateLinks(summaries, states),
		SearchRelease: g.searchRelease, SearchFuels: g.searchFuels,
	}
	return g.writeRoute("precos", data)
}

func (g *generator) generateStates(summaries []analytics.Summary, rankings []analytics.Ranking, stations map[string]enterpriseexport.Station, municipalityStations map[string]int, municipalityState, municipalityName map[string]string) error {
	states := map[string]bool{}
	for _, summary := range summaries {
		if summary.Geography.Level == analytics.LevelState {
			states[summary.Geography.Code] = true
		}
	}
	for _, uf := range sortedKeys(states) {
		if !isUF(uf) {
			return fmt.Errorf("portalgen: invalid state code")
		}
		stateSummaries := summaryRows(summaries, analytics.LevelState, uf, false)
		views, err := rankingViews(rankings, analytics.LevelState, uf, stations, true)
		if err != nil {
			return err
		}
		municipalities := map[string]bool{}
		for _, summary := range summaries {
			if summary.Geography.Level == analytics.LevelMunicipality {
				code := summary.Geography.Code
				if municipalityState[code] == uf {
					municipalities[code] = true
				}
			}
		}
		links := make([]municipalityLink, 0, len(municipalities))
		for code := range municipalities {
			stationsCount := municipalityStations[code]
			name := municipalityName[code]
			if name == "" {
				name = "Município " + code
			}
			links = append(links, municipalityLink{
				Code: code, Name: name, UF: uf,
				Href:     g.cityRoutes[code],
				Stations: stationsCount, Insufficient: stationsCount < minMunicipality,
			})
		}
		sort.Slice(links, func(i, j int) bool {
			if links[i].Name != links[j].Name {
				return links[i].Name < links[j].Name
			}
			return links[i].Code < links[j].Code
		})
		parts := []string{"Pesquisa ANP " + g.periodPhrase() + " em " + ufName(uf) + ", por cidade."}
		for _, row := range byFuelRank(stateSummaries) {
			parts = append(parts, rangeSentence(row))
		}
		data := statePage{
			pageBase: g.newBase(pageSpec{
				Route: stateRoute(uf), Facts: indexFacts{Family: familyState, HasCurrentData: len(stateSummaries) > 0},
				Title: "Preços em " + ufName(uf), HeadTitle: stateTitle(uf, g.surveyDate), Description: composeDescription(parts...),
				Active: "precos", Crumbs: []crumb{{Label: "Preços", Href: "/precos/"}, {Label: ufName(uf), Current: true}}, LastMod: g.dataDate,
			}),
			UF: uf, Name: ufName(uf), Summaries: stateSummaries, Rankings: views, Municipalities: links,
		}
		if err := g.writeRoute("state", data); err != nil {
			return err
		}
		g.statePages[uf] = true
	}
	return nil
}

func (g *generator) generateMunicipalities(summaries []analytics.Summary, observations []observation, municipalityStations map[string]int, municipalityState, municipalityName map[string]string) error {
	municipalities := map[string]bool{}
	for _, summary := range summaries {
		if summary.Geography.Level == analytics.LevelMunicipality {
			municipalities[summary.Geography.Code] = true
		}
	}
	for _, code := range sortedKeys(municipalities) {
		if !isIBGE(code) {
			return fmt.Errorf("portalgen: invalid municipality code")
		}
		uf := municipalityState[code]
		if uf == "" {
			continue
		}
		if !isUF(uf) {
			return fmt.Errorf("portalgen: invalid municipality state")
		}
		name := municipalityName[code]
		if name == "" {
			name = "Município " + code
		}
		insufficient := municipalityStations[code] < minMunicipality
		rows := summaryRows(summaries, analytics.LevelMunicipality, code, true)
		fuels := make([]string, 0, len(rows))
		for _, row := range rows {
			fuels = append(fuels, row.Code)
		}
		parts := []string{"Pesquisa ANP " + g.periodPhrase() + " em " + placeLabel(name, uf) + "."}
		if insufficient {
			parts = append(parts, fmt.Sprintf("%d %s com preço, sem ranking por amostra pequena.", municipalityStations[code], plural(municipalityStations[code], "posto", "postos")),
				"Veja o preço de cada posto, a data da coleta e a fonte.")
		} else {
			for _, row := range byFuelRank(rows) {
				parts = append(parts, rangeSentence(row))
			}
		}
		data := municipalityPage{
			pageBase: g.newBase(pageSpec{
				Route: g.cityRoutes[code], Facts: indexFacts{Family: familyCity, HasCurrentData: len(rows) > 0},
				Title: "Preços em " + name, HeadTitle: cityTitle(name, uf, fuels, g.surveyDate), Description: composeDescription(parts...),
				Active: "precos", Crumbs: []crumb{{Label: "Preços", Href: "/precos/"}, {Label: ufName(uf), Href: stateRoute(uf)}, {Label: name, Current: true}},
				LastMod: g.dataDate,
			}),
			Code: code, Name: name, UF: uf,
			Summaries: rows, Insufficient: insufficient, Population: municipalityStations[code],
		}
		items := make([]observation, 0)
		for _, item := range observations {
			if item.station.MunicipalityCode == code && item.station.State == uf {
				items = append(items, item)
			}
		}
		if insufficient {
			data.Rows = observationRows(items, 0, false)
		} else {
			data.Rankings = observationRankings(items)
		}
		if err := g.writeRoute("municipality", data); err != nil {
			return err
		}
		g.cityPages[code] = true
	}
	return nil
}

// stationEntry is the precomputed identity, gate and head of one station.
type stationEntry struct {
	station     enterpriseexport.Station
	name        string
	city        string
	facts       stationFacts
	indexable   bool
	headTitle   string
	description string
}

func (e stationEntry) place() string {
	switch {
	case e.city != "" && knownUF(e.station.State):
		return placeLabel(e.city, e.station.State)
	case knownUF(e.station.State):
		return ufName(e.station.State)
	default:
		return "localização não informada"
	}
}

func (g *generator) stationDescription(entry stationEntry, items []observation, extra string) string {
	lead := entry.name + " em " + entry.place() + ": cadastro ANP, endereço e status informado; sem preço ANP nas pesquisas publicadas."
	prices := ""
	switch {
	case len(items) > 0:
		lead = entry.name + " em " + entry.place() + ": preços da pesquisa ANP " + g.periodPhrase() + "."
		ordered := append([]observation(nil), items...)
		sort.Slice(ordered, func(i, j int) bool {
			if fuelRank(ordered[i].price.FuelProduct) != fuelRank(ordered[j].price.FuelProduct) {
				return fuelRank(ordered[i].price.FuelProduct) < fuelRank(ordered[j].price.FuelProduct)
			}
			return ordered[i].price.FuelProduct < ordered[j].price.FuelProduct
		})
		values := make([]string, 0, 3)
		for _, item := range ordered {
			if len(values) == 3 {
				break
			}
			values = append(values, fuelName(item.price.FuelProduct)+" "+brl(item.price.AmountMilliBRL))
		}
		prices = capitalize(strings.Join(values, "; ")) + "."
	case entry.facts.LastObserved != "":
		lead = entry.name + " em " + entry.place() + ": histórico de preços da pesquisa ANP, última coleta em " + dateBR(entry.facts.LastObserved) + "."
	}
	address := ""
	if street := stationStreet(entry.station); street != "" {
		address = "Endereço: " + stationDisplayTitle(street) + "."
	}
	return composeDescription(lead, extra, prices, address)
}

// prepareStations computes names, gate facts and unique heads. Titles and
// descriptions of indexable stations are disambiguated by street, CNPJ and
// identifier, in that order, so every indexable head is unique.
func (g *generator) prepareStations(ordered []enterpriseexport.Station, prices map[string][]enterpriseexport.Price, revisions map[string]enterpriseexport.Revision, recent map[string]bool, observations map[string][]observation) []stationEntry {
	entries := make([]stationEntry, len(ordered))
	for i, station := range ordered {
		facts := stationIndexFacts(station, prices[station.StationID], revisions, recent)
		entry := stationEntry{
			station: station, name: stationDisplayTitle(station.DisplayName), city: stationMunicipalityName(station), facts: facts,
			indexable: pageIndexable(indexFacts{Family: familyStation, RecentOfficialPrice: facts.RecentOfficialPrice, CompleteAddress: facts.CompleteAddress}),
		}
		if !knownUF(station.State) {
			entry.city = ""
		}
		entry.headTitle = stationTitle(entry.name, entry.city, station.State)
		entry.description = g.stationDescription(entry, observations[station.StationID], "")
		entries[i] = entry
	}
	titleSuffixes := []func(stationEntry) string{
		func(e stationEntry) string { return stationDisplayTitle(stationStreet(e.station)) },
		func(e stationEntry) string {
			if e.station.CNPJ == "" {
				return ""
			}
			return "CNPJ " + cnpjBR(e.station.CNPJ)
		},
		func(e stationEntry) string { return e.station.StationID[:8] },
	}
	for _, suffix := range titleSuffixes {
		for _, i := range duplicateIndexes(entries, func(e stationEntry) string { return e.headTitle }) {
			if value := suffix(entries[i]); value != "" {
				entries[i].headTitle += " – " + value
			}
		}
	}
	extras := []func(stationEntry) string{
		func(e stationEntry) string {
			if e.station.CNPJ == "" {
				return "Identificador " + e.station.StationID[:8] + "."
			}
			return "CNPJ " + cnpjBR(e.station.CNPJ) + "."
		},
		func(e stationEntry) string {
			return "CNPJ " + cnpjBR(e.station.CNPJ) + ", identificador " + e.station.StationID[:8] + "."
		},
	}
	for _, extra := range extras {
		for _, i := range duplicateIndexes(entries, func(e stationEntry) string { return e.description }) {
			entries[i].description = g.stationDescription(entries[i], observations[entries[i].station.StationID], extra(entries[i]))
		}
	}
	return entries
}

// duplicateIndexes lists indexable entries whose key is shared with another
// indexable entry.
func duplicateIndexes(entries []stationEntry, key func(stationEntry) string) []int {
	groups := map[string][]int{}
	for i, entry := range entries {
		if entry.indexable {
			groups[key(entry)] = append(groups[key(entry)], i)
		}
	}
	out := []int{}
	for _, value := range sortedKeys(groups) {
		if len(groups[value]) > 1 {
			out = append(out, groups[value]...)
		}
	}
	sort.Ints(out)
	return out
}

func (g *generator) generateStations(snapshot enterpriseexport.Snapshot, stations map[string]enterpriseexport.Station, revisions map[string]enterpriseexport.Revision, observations map[string][]observation, municipalityStations map[string]int, official *officialDataset) error {
	ordered := make([]enterpriseexport.Station, 0, len(stations))
	for _, station := range stations {
		ordered = append(ordered, station)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].DisplayName != ordered[j].DisplayName {
			return ordered[i].DisplayName < ordered[j].DisplayName
		}
		return ordered[i].StationID < ordered[j].StationID
	})
	pricesByStation := map[string][]enterpriseexport.Price{}
	for _, price := range snapshot.Prices {
		pricesByStation[price.StationID] = append(pricesByStation[price.StationID], price)
	}
	recent := recentSurveyRevisions(revisions, stationRecentSurveyWeeks)
	entries := g.prepareStations(ordered, pricesByStation, revisions, recent, observations)
	source := sourceView{}
	if latest := latestRevision(snapshot.Revisions); latest != nil {
		source = sourceView{
			URL: latest.SourceURL, Checksum: latest.SourceChecksum,
			ParserVersion: latest.ParserVersion, PublishedAt: publicationLabel(latest.PublishedAt),
			Period: periodLabel(latest.SurveyStart, latest.SurveyEnd),
		}
	}
	for _, entry := range entries {
		station := entry.station
		items := observations[station.StationID]
		obsView := make([]observationView, 0, len(items))
		historyKeys := map[string]struct{ fuel, unit string }{}
		for _, item := range items {
			obsView = append(obsView, observationView{
				Fuel: fuelLabel(item.price.FuelProduct), Unit: item.price.Unit,
				Condition: "À vista", Amount: item.price.AmountMilliBRL,
				Collected: dateBR(item.price.CollectedOn), Period: g.period,
			})
			historyKeys[item.price.FuelProduct+"\x00"+item.price.Unit] = struct{ fuel, unit string }{item.price.FuelProduct, item.price.Unit}
		}
		own := pricesByStation[station.StationID]
		for _, price := range own {
			if _, ok := revisions[price.RevisionID]; !ok {
				continue
			}
			if unit, err := productUnit(price.FuelProduct); err != nil || price.Unit != unit {
				continue
			}
			historyKeys[price.FuelProduct+"\x00"+price.Unit] = struct{ fuel, unit string }{price.FuelProduct, price.Unit}
		}
		historyViews := make([]historyView, 0, len(historyKeys))
		for _, key := range historyKeys {
			history, err := analytics.StationHistory(own, snapshot.Revisions, station.StationID, key.fuel, key.unit)
			if err != nil {
				return err
			}
			points := make([]historyPointView, 0, len(history.Points))
			hasObservation := false
			for _, point := range history.Points {
				points = append(points, historyPointView{
					Period:    periodLabel(point.SurveyStart, point.SurveyEnd),
					Collected: dateBR(point.CollectedOn), Amount: point.AmountMilliBRL, Missing: point.Missing,
				})
				if !point.Missing {
					hasObservation = true
				}
			}
			if hasObservation {
				historyViews = append(historyViews, historyView{Fuel: fuelLabel(key.fuel), Unit: key.unit, Points: points})
			}
		}
		sort.Slice(historyViews, func(i, j int) bool {
			if historyViews[i].Fuel != historyViews[j].Fuel {
				return historyViews[i].Fuel < historyViews[j].Fuel
			}
			return historyViews[i].Unit < historyViews[j].Unit
		})
		place := "Localização não informada"
		if station.MunicipalityCode != "" {
			name := entry.city
			if name == "" {
				name = "Município " + station.MunicipalityCode
			}
			place = name + "/" + station.State
		} else if station.State != "" {
			place = ufName(station.State)
		}
		view := stationView{
			ID: station.StationID, Name: entry.name, Registry: stationRegistryName(station), LegalName: stationLegalName(station), CNPJ: cnpjBR(station.CNPJ),
			Address: addressLine(station.Address), Status: statusLabel(station.OperationStatus),
			Location: locationLabel(station.LocationQuality), Place: place,
		}
		crumbs := []crumb{{Label: "Consultar", Href: "/consultar/"}}
		if g.statePages[station.State] {
			view.StateHref = stateRoute(station.State)
			view.StateName = ufName(station.State)
			crumbs = []crumb{{Label: "Preços", Href: "/precos/"}, {Label: ufName(station.State), Href: view.StateHref}}
			if g.cityPages[station.MunicipalityCode] && municipalityStations[station.MunicipalityCode] > 0 {
				view.Municipality = placeLabel(entry.city, station.State)
				view.MunicipalityHref = g.cityRoutes[station.MunicipalityCode]
				crumbs = append(crumbs, crumb{Label: entry.city, Href: view.MunicipalityHref})
			}
		}
		crumbs = append(crumbs, crumb{Label: entry.name, Current: true})
		lastMod := entry.facts.LastObserved
		officialRecords, officialCount := officialStationViews(official, station.StationID, stations)
		data := stationPage{
			pageBase: g.newBase(pageSpec{
				Route: "/postos/" + station.StationID + "/",
				Facts: indexFacts{Family: familyStation, RecentOfficialPrice: entry.facts.RecentOfficialPrice, CompleteAddress: entry.facts.CompleteAddress},
				Title: entry.name, HeadTitle: entry.headTitle, Description: entry.description,
				Active: "consultar", Crumbs: crumbs, LastMod: lastMod,
			}),
			Station: view, Observations: obsView, Histories: historyViews, Source: source,
			OfficialRecords: officialRecords, OfficialCount: officialCount, OfficialMore: officialCount > len(officialRecords),
		}
		if err := g.writeRoute("station", data); err != nil {
			return err
		}
	}
	return nil
}

func (g *generator) generateCuriosidades(cards []insightCard) error {
	data := curiosidadesPage{
		pageBase: g.newBase(pageSpec{
			Route: "/curiosidades/", Facts: indexFacts{Family: familyHub},
			Title: "Perguntas aprovadas sobre os dados", HeadTitle: "Perguntas aprovadas sobre os preços de combustíveis | abastevo",
			Description: composeDescription("Respostas aprovadas sobre a pesquisa ANP "+g.periodPhrase()+": amostra, variação e cobertura dos preços.", "Cada resposta mostra a fórmula e a fonte."),
			Active:      "curiosidades", Crumbs: []crumb{{Label: "Curiosidades", Current: true}}, LastMod: g.dataDate,
		}),
		Cards: cards, DownloadsHref: "/dados/",
	}
	return g.writeRoute("curiosidades", data)
}

func (g *generator) generateDownloads(files []downloadFile, manifest downloadFile, release string) error {
	data := dadosPage{
		pageBase: g.newBase(pageSpec{
			Route: "/dados/", Facts: indexFacts{Family: familyHub},
			Title: "Baixar dados públicos", HeadTitle: "Baixar dados públicos de preços de combustíveis (CSV) | abastevo",
			Description: composeDescription("CSV com postos, preços e resumos da pesquisa ANP "+g.periodPhrase()+".", "Inclui manifesto com hashes, contagens e notas de uso dos dados."),
			Crumbs:      []crumb{{Label: "Dados para download", Current: true}}, LastMod: g.dataDate,
		}),
		Release: release, Period: g.period, Files: files, Manifest: manifest, Notes: downloadNotes,
	}
	return g.writeRoute("dados", data)
}

func (g *generator) generateStatic() error {
	metodologia := g.newBase(pageSpec{
		Route: "/metodologia/", Facts: indexFacts{Family: familyHub},
		Title: "Metodologia e fontes", HeadTitle: "Metodologia e fontes dos preços de combustíveis | abastevo",
		Description: composeDescription("Como o abastevo trata fontes, datas, amostra, rankings, correções e privacidade nos preços da pesquisa ANP e na fiscalização."),
		Active:      "metodologia", Crumbs: []crumb{{Label: "Metodologia", Current: true}},
	})
	if err := g.writeRoute("metodologia", metodologia); err != nil {
		return err
	}
	notFound := g.newBase(pageSpec{
		Facts: indexFacts{Family: familyNotFound},
		Title: "Página não encontrada", Description: "A página ou o posto solicitado não existe neste portal estático.",
		Crumbs: []crumb{{Label: "Página não encontrada", Current: true}},
	})
	return g.writePage("404.html", "notfound", notFound)
}
