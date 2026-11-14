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
		HasForum: g.hasForum(),
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
	// Marshalling maps of strings and numbers cannot fail; a page without a
	// canonical simply carries no structured data.
	base, _ = g.withStructuredData(base)
	return base
}

// collectionBase adds the CollectionPage/ItemList node to a page base.
func (g *generator) collectionBase(base pageBase, items []listItem) (pageBase, error) {
	return g.withStructuredData(base, g.collectionNode(base, items))
}

func stateItems(links []stateLink) []listItem {
	items := make([]listItem, 0, len(links))
	for _, link := range links {
		items = append(items, listItem{Name: "Preços em " + link.Name, Href: link.Href})
	}
	return items
}

func rankingItems(views []rankingView, rows []rankingRow) []listItem {
	seen := map[string]bool{}
	items := []listItem{}
	add := func(row rankingRow) {
		if !seen[row.StationID] {
			seen[row.StationID] = true
			items = append(items, listItem{Name: row.Name, Href: "/postos/" + row.StationID + "/"})
		}
	}
	for _, view := range views {
		for _, row := range view.Rows {
			add(row)
		}
	}
	for _, row := range rows {
		add(row)
	}
	return items
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
		States: stateLinks(summaries, states), National: g.summaryRows(summaries, analytics.LevelNational, "BR", false),
		SearchStates: searchStates, SearchRelease: release,
	}
	data.Modules = []linkModule{g.hubModule("/consultar/")}
	var err error
	if data.pageBase, err = g.collectionBase(data.pageBase, stateItems(data.States)); err != nil {
		return err
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
	national := g.summaryRows(summaries, analytics.LevelNational, "BR", false)
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
		National: national, Rankings: views, States: stateLinks(summaries, states), FuelHubs: g.orderedFuelHubs(),
		SearchRelease: g.searchRelease, SearchFuels: g.searchFuels,
	}
	codes := make([]string, 0, len(national))
	for _, row := range national {
		codes = append(codes, row.Code)
	}
	data.Modules = []linkModule{g.priceGuides(" no Brasil", codes), g.forumPrecosModule(), g.hubModule("/precos/")}
	items := []listItem{}
	for _, hub := range data.FuelHubs {
		items = append(items, listItem{Name: "Preço de " + hub.Label + " por estado", Href: hub.Href})
	}
	if data.pageBase, err = g.collectionBase(data.pageBase, append(items, stateItems(data.States)...)); err != nil {
		return err
	}
	return g.writeRoute("precos", data)
}

func (g *generator) generateStates(summaries []analytics.Summary, rankings []analytics.Ranking, stations map[string]enterpriseexport.Station, municipalityStations map[string]int) error {
	states := map[string]bool{}
	for _, summary := range summaries {
		if summary.Geography.Level == analytics.LevelState {
			if !isUF(summary.Geography.Code) {
				return fmt.Errorf("portalgen: invalid state code")
			}
			states[summary.Geography.Code] = true
		}
	}
	registry := map[string]bool{}
	for _, city := range g.cities {
		registry[city.UF] = true
	}
	all := map[string]bool{}
	for uf := range states {
		all[uf] = true
	}
	for uf := range registry {
		if knownUF(uf) {
			all[uf] = true
		}
	}
	for uf := range all {
		g.statePages[uf] = true
	}
	for _, uf := range sortedKeys(all) {
		stateSummaries := g.summaryRows(summaries, analytics.LevelState, uf, false)
		views, err := rankingViews(rankings, analytics.LevelState, uf, stations, true)
		if err != nil {
			return err
		}
		links := []municipalityLink{}
		for _, city := range g.dataCities(uf) {
			count := municipalityStations[city.Code]
			links = append(links, municipalityLink{Code: city.Code, Name: city.Name, UF: uf, Href: city.Route, Stations: count, Insufficient: count < minMunicipality})
		}
		noData := []municipalityLink{}
		for _, code := range sortedKeys(g.cities) {
			if city := g.cities[code]; !city.HasData && city.UF == uf {
				noData = append(noData, municipalityLink{Code: code, Name: city.Name, UF: uf, Href: city.Route, Stations: len(city.Stations)})
			}
		}
		sort.Slice(noData, func(i, j int) bool {
			if noData[i].Name != noData[j].Name {
				return noData[i].Name < noData[j].Name
			}
			return noData[i].Code < noData[j].Code
		})
		if len(noData) > maxStateCityListing {
			noData = noData[:maxStateCityListing]
		}
		hasData := len(stateSummaries) > 0
		head, description := stateTitle(uf, g.surveyDate), ""
		if hasData {
			parts := []string{"Pesquisa ANP " + g.periodPhrase() + " em " + ufName(uf) + ", por cidade."}
			for _, row := range byFuelRank(stateSummaries) {
				parts = append(parts, rangeSentence(row))
			}
			description = composeDescription(parts...)
		} else {
			head = "Postos de combustíveis em " + ufName(uf) + " – sem pesquisa ANP recente | abastevo"
			description = composeDescription(ufName(uf)+" não tem preço na pesquisa ANP "+g.periodPhrase()+".", "Veja as cidades com postos cadastrados.")
		}
		base := g.newBase(pageSpec{
			Route: stateRoute(uf), Facts: indexFacts{Family: familyState, HasCurrentData: hasData},
			Title: "Preços em " + ufName(uf), HeadTitle: head, Description: description,
			Active: "precos", Crumbs: []crumb{{Label: "Preços", Href: "/precos/"}, {Label: ufName(uf), Current: true}}, LastMod: g.dataDate,
		})
		codes := make([]string, 0, len(stateSummaries))
		for _, row := range stateSummaries {
			codes = append(codes, row.Code)
		}
		otherStates := []moduleLink{}
		for _, other := range sortedKeys(all) {
			if other != uf {
				otherStates = append(otherStates, moduleLink{Label: "Preço dos combustíveis em " + ufName(other), Href: stateRoute(other)})
			}
		}
		base.Modules = []linkModule{
			g.fuelModule("Preço por combustível no Brasil", codes),
			{Title: "Outros estados", Links: otherStates},
			g.forumRegionModule(uf, ""),
			g.priceGuides(" em "+ufName(uf), codes),
		}
		data := statePage{pageBase: base, UF: uf, Name: ufName(uf), Summaries: stateSummaries, Rankings: views, Municipalities: links, NoDataCities: noData}
		items := make([]listItem, 0, len(links))
		for _, link := range links {
			items = append(items, listItem{Name: "Preços em " + placeLabel(link.Name, uf), Href: link.Href})
		}
		if data.pageBase, err = g.collectionBase(data.pageBase, items); err != nil {
			return err
		}
		if err := g.writeRoute("state", data); err != nil {
			return err
		}
	}
	return nil
}

func (g *generator) generateMunicipalities(summaries []analytics.Summary, observations []observation, municipalityStations map[string]int) error {
	for _, summary := range summaries {
		if summary.Geography.Level == analytics.LevelMunicipality && !isIBGE(summary.Geography.Code) {
			return fmt.Errorf("portalgen: invalid municipality code")
		}
	}
	byCity := map[string][]observation{}
	for _, item := range observations {
		byCity[item.station.MunicipalityCode] = append(byCity[item.station.MunicipalityCode], item)
	}
	for _, code := range sortedKeys(g.cities) {
		city := g.cities[code]
		code, uf, name := city.Code, city.UF, city.Name
		insufficient := municipalityStations[code] < minMunicipality
		rows := []summaryRow{}
		if city.HasData {
			rows = g.summaryRows(summaries, analytics.LevelMunicipality, code, true)
		}
		fuels := make([]string, 0, len(rows))
		for _, row := range rows {
			fuels = append(fuels, row.Code)
		}
		head := cityTitle(name, uf, fuels, g.surveyDate)
		parts := []string{"Pesquisa ANP " + g.periodPhrase() + " em " + placeLabel(name, uf) + "."}
		switch {
		case !city.HasData:
			head = "Postos de combustíveis em " + placeLabel(name, uf) + " – sem pesquisa ANP recente | abastevo"
			parts = []string{placeLabel(name, uf) + " não tem preço na pesquisa ANP " + g.periodPhrase() + ".",
				"Veja os postos cadastrados, cidades próximas com preço e outras cidades de " + ufName(uf) + "."}
		case insufficient:
			parts = append(parts, fmt.Sprintf("%d %s com preço, sem ranking por amostra pequena.", municipalityStations[code], plural(municipalityStations[code], "posto", "postos")),
				"Veja o preço de cada posto, a data da coleta e a fonte.")
		default:
			for _, row := range byFuelRank(rows) {
				parts = append(parts, rangeSentence(row))
			}
		}
		lastMod := g.dataDate
		if !city.HasData {
			lastMod = g.buildDate
		}
		base := g.newBase(pageSpec{
			Route: city.Route, Facts: indexFacts{Family: familyCity, HasCurrentData: city.HasData && len(rows) > 0},
			Title: "Preços em " + name, HeadTitle: head, Description: composeDescription(parts...),
			Active: "precos", Crumbs: []crumb{{Label: "Preços", Href: "/precos/"}, {Label: ufName(uf), Href: stateRoute(uf)}, {Label: name, Current: true}},
			LastMod: lastMod,
		})
		limit := cityStationListCap
		if !city.HasData {
			limit = noDataStationsCap
		}
		stationList, total := stationRows(city, g.observed, limit)
		base.Modules = append(g.cityModules(city),
			g.fuelModule("Preço por combustível no Brasil", fuels),
			g.forumRegionModule(uf, code),
			g.priceGuides(" em "+name, fuels))
		data := municipalityPage{
			pageBase: base, NoData: !city.HasData, Stations: stationList, StationsTotal: total,
			Code: code, Name: name, UF: uf,
			Summaries: rows, Insufficient: insufficient, Population: municipalityStations[code],
		}
		if city.HasData {
			items := make([]observation, 0, len(byCity[code]))
			for _, item := range byCity[code] {
				if item.station.State == uf {
					items = append(items, item)
				}
			}
			if insufficient {
				data.Rows = observationRows(items, 0, false)
			} else {
				data.Rankings = observationRankings(items)
			}
		}
		listed := make([]listItem, 0, len(stationList))
		for _, row := range stationList {
			listed = append(listed, listItem{Name: row.Label, Href: row.Href})
		}
		var err error
		if data.pageBase, err = g.collectionBase(data.pageBase, listed); err != nil {
			return err
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

func (g *generator) generateStations(snapshot enterpriseexport.Snapshot, stations map[string]enterpriseexport.Station, revisions map[string]enterpriseexport.Revision, official *officialDataset) error {
	pricesByStation, observations := g.pricesByStation, g.observed
	source := sourceView{}
	if latest := latestRevision(snapshot.Revisions); latest != nil {
		source = sourceView{
			URL: latest.SourceURL, Checksum: latest.SourceChecksum,
			ParserVersion: latest.ParserVersion, PublishedAt: publicationLabel(latest.PublishedAt),
			Period: periodLabel(latest.SurveyStart, latest.SurveyEnd),
		}
	}
	for i := range g.entries {
		entry := &g.entries[i]
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
		var modules []linkModule
		fuels := make([]string, 0, len(historyKeys))
		for _, key := range historyKeys {
			fuels = append(fuels, key.fuel)
		}
		sort.Strings(fuels)
		if g.statePages[station.State] {
			view.StateHref = stateRoute(station.State)
			view.StateName = ufName(station.State)
			crumbs = []crumb{{Label: "Preços", Href: "/precos/"}, {Label: ufName(station.State), Href: view.StateHref}}
			if city, ok := g.cities[station.MunicipalityCode]; ok && city.UF == station.State {
				view.Municipality = placeLabel(city.Name, station.State)
				view.MunicipalityHref = city.Route
				crumbs = append(crumbs, crumb{Label: city.Name, Href: city.Route})
				modules = append(modules, linkModule{Title: "Outros postos em " + city.Name, Links: siblingLinks(city, entry)})
			}
		}
		modules = append(modules, g.fuelModule("Combustíveis deste posto no Brasil", fuels))
		if len(fuels) > 0 {
			modules = append(modules, g.landingModule(stationGuides(fuels)...))
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
		data.Modules = modules
		var err error
		if data.pageBase, err = g.withStructuredData(data.pageBase, g.gasStationNode(data.pageBase, *entry)); err != nil {
			return err
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
	data.Modules = []linkModule{g.hubModule("/curiosidades/")}
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
	data.Modules = []linkModule{g.hubModule("/dados/")}
	var err error
	if data.pageBase, err = g.withStructuredData(data.pageBase, g.datasetNode(data.pageBase, files, manifest, g.latest)); err != nil {
		return err
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
	metodologia.Modules = []linkModule{g.hubModule("/metodologia/")}
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
