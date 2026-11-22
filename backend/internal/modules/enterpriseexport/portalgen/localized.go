package portalgen

import (
	"fmt"
	"path"
	"strings"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport/analytics"
)

// Translation eligibility is a closed list of the capitals and the IBGE twin
// cities confirmed on the Portuguese portal at rollout. Sant'Ana do Livramento
// was named by the owner; it still needs a Portuguese page in the snapshot.
var localizedCityCodes = map[string]bool{
	// State and federal capitals.
	"1100205": true, "1200401": true, "1302603": true, "1400100": true,
	"1501402": true, "1600303": true, "1721000": true, "2111300": true,
	"2211001": true, "2304400": true, "2408102": true, "2507507": true,
	"2611606": true, "2704302": true, "2800308": true, "2927408": true,
	"3106200": true, "3205309": true, "3304557": true, "3550308": true,
	"4106902": true, "4205407": true, "4314902": true, "5002704": true,
	"5103403": true, "5208707": true, "5300108": true,
	// Border twin cities.
	"1100106": true, "1200054": true, "1200104": true, "1200252": true,
	"1200435": true, "1304062": true, "1400159": true, "1400456": true,
	"1600501": true, "4102604": true, "4108304": true, "4108809": true,
	"4124400": true, "4205001": true, "4300034": true, "4301875": true,
	"4305439": true, "4310603": true, "4311007": true, "4315057": true,
	"4315107": true, "4315305": true, "4317103": true, "4318002": true,
	"4322400": true, "5002100": true, "5003157": true, "5003207": true,
	"5005681": true, "5006358": true, "5006606": true, "5006903": true,
	"5102504": true,
}

type languageLink struct {
	Code  string
	Path  string
	URL   string
	OG    string
	Label string
}

type localizedSource struct {
	data routedPage
	kind string
	slug string
	fuel string
	area string
	city string
}

func localizedSourcePage(data routedPage) localizedSource {
	base := data.page()
	source := localizedSource{data: data}
	switch base.Route {
	case "/consultar/":
		source.kind = "search"
	case "/fiscalizacao/":
		source.kind = "enforcement"
	case "/forum/":
		source.kind = "forum"
	case "/precos/":
		source.kind = "prices"
	case "/metodologia/":
		source.kind = "methodology"
	case "/dados/":
		source.kind = "data"
	case "/curiosidades/":
		source.kind = "insights"
	default:
		switch base.Family {
		case familyStation:
			source.kind, source.slug = "stations", strings.TrimSuffix(strings.TrimPrefix(base.Route, "/postos/"), "/")
		case familyForumCategory, familyForumPost:
			source.kind, source.slug = "forum", strings.TrimSuffix(strings.TrimPrefix(base.Route, "/forum/"), "/")
		case familyFiscalPage:
			source.kind, source.slug = "enforcementPage", strings.TrimSuffix(strings.TrimPrefix(base.Route, "/fiscalizacao/pagina/"), "/")
		case familyFiscalRecord:
			source.kind, source.slug = "enforcementRecord", strings.TrimSuffix(strings.TrimPrefix(base.Route, "/fiscalizacao/registro/"), "/")
		}
		if !strings.HasPrefix(base.Route, "/precos/") {
			break
		}
		parts := strings.Split(strings.Trim(base.Route, "/"), "/")[1:]
		switch base.Family {
		case familyFuel:
			source.kind, source.fuel = "prices", parts[0]
		case familyState:
			source.kind, source.area = "prices", parts[0]
		case familyFuelState:
			source.kind, source.fuel, source.area = "prices", parts[0], parts[1]
		case familyCity:
			if city, ok := data.(municipalityPage); ok && localizedCityCodes[city.Code] {
				source.kind, source.area, source.city = "prices", parts[0], parts[1]
			}
		}
	}
	return source
}

func (s localizedSource) route(locale portalLocale) string {
	if s.kind != "prices" {
		return locale.route(s.kind, s.slug, "", "")
	}
	return locale.route(s.kind, s.fuel, s.area, s.city)
}

func (s localizedSource) indexFacts(locale portalLocale) indexFacts {
	facts := s.data.page().IndexFacts
	facts.Locale = &locale
	if post, ok := s.data.(forumPostPage); ok {
		_, reviewed := reviewedForumCopy(post.Post, locale.Code)
		facts.PublishedEditorial = facts.PublishedEditorial && reviewed
	}
	return facts
}

func (g *generator) translationLocales() []portalLocale {
	locales := make([]portalLocale, 0, 2)
	for _, code := range sortedKeys(g.locales) {
		if locale := g.locales[code]; code != defaultPortalLocale && locale.Enabled && locale.reviewed() {
			locales = append(locales, locale)
		}
	}
	return locales
}

func (g *generator) queueLocalizations(data routedPage) {
	if g.opts.OfficialOnly || len(g.locales) < 2 {
		return
	}
	source := localizedSourcePage(data)
	if source.kind == "" {
		return
	}
	locales := g.translationLocales()
	if len(locales) == 0 {
		return
	}
	g.localizations = append(g.localizations, source)
	g.registerLocalizationLinks(source, locales)
}

func (g *generator) registerLocalizationLinks(source localizedSource, locales []portalLocale) {
	base := source.data.page()
	original := g.locales[defaultPortalLocale]
	menu := []languageLink{g.languageLink(original, base.Route)}
	for _, locale := range locales {
		if route := source.route(locale); route != "" {
			menu = append(menu, g.languageLink(locale, route))
		}
	}
	for _, link := range menu {
		g.languageNavigation[link.Path] = menu
	}
	if !base.Indexable || len(menu) < 2 {
		return
	}
	links := []languageLink{menu[0]}
	for _, locale := range locales {
		if route := source.route(locale); route != "" && pageIndexable(source.indexFacts(locale)) {
			links = append(links, g.languageLink(locale, route))
		}
	}
	if len(links) == 1 {
		return
	}
	links = append(links, languageLink{Code: "x-default", Path: base.Route, URL: g.siteURL + base.Route})
	for _, link := range links {
		if link.Code == "x-default" {
			continue
		}
		siblings := append([]languageLink(nil), links...)
		for i := range siblings {
			if siblings[i].Code == link.Code {
				siblings[i].OG = ""
			}
		}
		g.languageGroups[link.Path] = siblings
	}
}

func (g *generator) languageMenu(route string) []languageLink {
	if menu := g.languageNavigation[route]; len(menu) > 1 {
		return menu
	}
	if g.opts.OfficialOnly || g.opts.LandingURL == "" {
		return nil
	}
	menu := []languageLink{}
	for _, locale := range g.translationLocales() {
		menu = append(menu, g.languageLink(locale, g.landingPrefix+locale.Prefix+"/"))
	}
	return menu
}

func (g *generator) languageLink(locale portalLocale, route string) languageLink {
	return languageLink{
		Code: locale.Hreflang, Path: route, URL: g.absoluteURL(route),
		OG: locale.OGLocale, Label: g.locales[defaultPortalLocale].text("locale." + locale.Code),
	}
}

type localizedTable struct {
	ID      string
	Detail  string
	Title   string
	Headers []string
	Rows    [][]localizedCell
}

type localizedCell struct {
	Text    string
	Href    string
	Details []string
}

type localizedNote struct{ ID, Title, Body, Detail, SourceHref string }

type localizedPage struct {
	pageBase
	Locale            portalLocale
	Links             []languageLink
	Nav               []moduleLink
	TopNav            []moduleLink
	Footer            []linkModule
	Home              string
	IsPrice           bool
	ShowPeriod        bool
	NoPrices          bool
	Insights          bool
	Tables            []localizedTable
	Notes             []localizedNote
	Lead              string
	Reading           string
	Rankings          []localizedTable
	NoRanking         string
	FAQ               []faqItem
	Explorer          *localizedExplorer
	Station           *localizedStationPage
	Forum             *localizedForumPage
	Search            *localizedSearchPage
	Enforcement       *localizedFiscalPage
	EnforcementRecord *localizedFiscalRecordPage
}

type localizedPortal struct {
	*generator
	portalLocale
	routes               map[string]string
	current              map[summaryKey]analytics.Summary
	coverage             analytics.Coverage
	observations         []observation
	municipalityStations map[string]int
	official             *officialDataset
	forumFeed            string
}

func (g *generator) localizationViews(summaries []analytics.Summary, coverage analytics.Coverage, observations []observation, municipalityStations map[string]int, official *officialDataset) []localizedPortal {
	current := indexSummaries(summaries)
	var views []localizedPortal
	for _, locale := range g.translationLocales() {
		p := localizedPortal{
			generator: g, portalLocale: locale, current: current, coverage: coverage,
			observations: observations, municipalityStations: municipalityStations, official: official,
			routes: make(map[string]string, len(g.localizations)),
		}
		for _, source := range g.localizations {
			p.routes[source.data.page().Route] = source.route(locale)
		}
		views = append(views, p)
	}
	return views
}

func (g *generator) writeLocalizations(views []localizedPortal) error {
	for _, p := range views {
		var err error
		if p.forumFeed, err = p.writeReviewedForumFeed(); err != nil {
			return err
		}
		for _, source := range g.localizations {
			if err := p.write(source); err != nil {
				return fmt.Errorf("portalgen: %s: %w", source.route(p.portalLocale), err)
			}
		}
	}
	return nil
}

var localizedSections = []struct{ kind, portuguese string }{
	{"prices", "/precos/"}, {"methodology", "/metodologia/"}, {"data", "/dados/"}, {"insights", "/curiosidades/"},
}

func (p localizedPortal) write(source localizedSource) error {
	original := source.data.page()
	route := source.route(p.portalLocale)
	if route == "" {
		return fmt.Errorf("no localized route")
	}
	key, fields := p.pageText(source)
	title := p.text("title."+key, fields...)
	facts := source.indexFacts(p.portalLocale)
	decision := pageIndexable(facts)
	base := pageBase{
		Title: title, HeadTitle: fitTitle(p.text("title.brand", "title", title), title),
		Description: p.priceDescription("description."+key, fields...),
		Indexable:   p.indexable && decision, Robots: robotsValue(p.indexable, decision),
		Route: route, Family: original.Family, Language: p.Code, LastMod: original.LastMod, CSSURL: p.cssURL,
		PublishedAt: p.publication(), InputHash: original.InputHash,
		HasLanding: original.HasLanding, Landing: original.Landing,
	}
	data := localizedPage{
		pageBase: base, Locale: p.portalLocale, Links: p.languageGroups[route],
		IsPrice: source.kind == "prices", ShowPeriod: source.kind != "methodology",
	}
	if p.opts.LandingURL != "" {
		data.Home = p.landingPrefix + p.Prefix + "/"
	} else {
		data.Home = p.route("prices", "", "", "")
	}
	if p.latest != nil {
		data.Period = p.portalLocale.period(p.latest.SurveyStart, p.latest.SurveyEnd)
		if data.IsPrice && (original.IndexFacts.HasCurrentData || original.Route == "/precos/") {
			data.HeadTitle = fitTitle(p.text("title.dated", "title", title, "date", p.date(p.latest.SurveyEnd)), title)
		}
	} else {
		data.Period = p.portalLocale.period("", "")
	}
	if p.siteURL != "" {
		data.Canonical, data.OGImage = p.siteURL+route, p.siteURL+ogImagePath
	}
	data.Breadcrumbs = []crumb{{Label: p.text("home"), Href: data.Home}}
	if data.IsPrice && original.Route != "/precos/" {
		data.Breadcrumbs = append(data.Breadcrumbs, crumb{Label: p.text("nav.prices"), Href: p.routes["/precos/"]})
	}
	data.Breadcrumbs = append(data.Breadcrumbs, crumb{Label: title, Current: true})
	for _, section := range localizedSections {
		href := p.routes[section.portuguese]
		link := moduleLink{Label: p.text("nav." + section.kind), Href: href}
		data.Nav = append(data.Nav, link)
		if href != route {
			data.appendModule(p.text("section.related"), link)
		}
	}
	data.appendModule(p.text("section.related"), moduleLink{Label: p.text("nav.search"), Href: p.routes["/consultar/"]})
	switch original.Route {
	case "/precos/", "/consultar/", "/dados/", "/metodologia/":
		if p.hasForum() {
			data.appendModule(p.text("section.related"), p.localizedLink(p.text("nav.forum"), "/forum/"))
		}
	}
	p.addChrome(&data)
	var node ldNode
	switch page := source.data.(type) {
	case consultarPage:
		p.addSearch(&data, page)
	case fiscalizacaoPage:
		p.addEnforcement(&data, page)
	case fiscalizacaoRecordPage:
		p.addEnforcementRecord(&data, page)
	case stationPage:
		node = p.addStation(&data, page)
	case forumHubPage:
		node = p.addForumHub(&data)
	case forumCategoryPage:
		node = p.addForumCategory(&data, page.Category)
	case forumPostPage:
		node = p.addForumPost(&data, page)
	case precosPage:
		data.Lead = p.text("price.sampleWarning")
		p.addRecentForumPosts(&data)
		if len(page.SearchFuels) > 0 {
			data.Explorer = p.explorer(page)
			data.ScriptURL = original.ScriptURL
		}
		data.addRankings(p, page.Rankings, "ranking.emptyNational")
		data.addTable(p.prices(page.National, "", ""))
		for _, hub := range page.FuelHubs {
			link := p.localizedLink(p.fuel(hub.Code), hub.Href)
			link.Note = p.text("sample.fuelRange", "min", p.money(hub.Min), "max", p.money(hub.Max), "unit", p.unit(hub.Unit))
			data.appendModule(p.text("section.fuels"), link)
		}
		data.NoPrices = len(page.National) == 0
		p.stateFuelDirectory(&data)
		for _, state := range page.States {
			link := p.localizedLink(placeLabel(state.Name, state.Code), state.Href)
			link.Note = p.text("sample.stateCount", "stations", p.integer(state.Stations), "fuels", p.integer(state.Fuels))
			data.appendModule(p.text("section.states"), link)
		}
		if page.ParityHref != "" {
			data.appendModule(p.text("section.related"), p.localizedLink(p.text("ratio.link"), page.ParityHref))
		}
		p.addDirectory(&data, true)
	case fuelHubPage:
		if len(page.National) > 0 {
			p.addFuelReading(&data, page.National[0], p.text("data.country"), len(page.States), page.Comparison)
		}
		data.addTable(p.prices(page.National, "", ""))
		data.addTable(p.geo("section.states", page.States))
		data.addTable(p.geo("section.cities", page.Cities))
		data.addTable(p.priorSurvey(page.National, analytics.LevelNational, "BR"))
		p.addDirectory(&data, false)
		if module := p.parityModule(page.Code); len(module.Links) > 0 {
			data.appendModule(p.text("section.related"), p.localizedLink(p.text("ratio.link"), module.Links[0].Href))
		}
	case statePage:
		data.Lead = p.text("price.stateLead", "place", placeLabel(page.Name, page.UF))
		data.addRankings(p, page.Rankings, "ranking.emptyState")
		p.addRatio(&data, page.Parity)
		data.addTable(p.prices(page.Summaries, page.UF, ""))
		data.addTable(p.priorSurvey(page.Summaries, analytics.LevelState, page.UF))
		p.addMunicipalities(&data, page.Municipalities, page.NoDataCities)
		data.NoPrices = len(page.Summaries) == 0
		p.addDirectory(&data, false)
	case municipalityPage:
		p.addCity(&data, page)
	case fuelAreaPage:
		if len(page.Summary) > 0 {
			p.addFuelReading(&data, page.Summary[0], ufName(strings.ToUpper(source.area)), 0, page.Comparison)
		}
		data.addRankings(p, page.Rankings, "")
		data.addTable(p.prices(page.Summary, "", ""))
		data.addTable(p.geo("section.cities", page.Cities))
		data.addTable(p.history(page.History))
		data.addTable(p.priorSurvey(page.Summary, analytics.LevelState, strings.ToUpper(source.area)))
		p.addFuelStateLinks(&data, source)
	case dadosPage:
		node = p.downloads(&data, page)
		p.addDirectory(&data, true)
	case curiosidadesPage:
		p.insights(&data)
		p.addDirectory(&data, true)
	default: // methodology
		p.addMethodology(&data)
		p.addDirectory(&data, true)
	}
	if node == nil && (data.IsPrice || source.kind == "insights") {
		items := []listItem{}
		for _, module := range data.Modules {
			if module.Title == p.text("section.related") {
				continue
			}
			for _, link := range module.Links {
				items = append(items, listItem{Name: link.Label, Href: link.Href})
			}
		}
		node = p.collectionNode(data.pageBase, items)
	}
	var err error
	if node != nil {
		data.pageBase, err = p.withStructuredData(data.pageBase, node)
	} else {
		data.pageBase, err = p.withStructuredData(data.pageBase)
	}
	if err != nil {
		return err
	}
	return p.writeRoute("localized", data)
}

func (p localizedPortal) pageText(source localizedSource) (string, []string) {
	if source.kind == "enforcementPage" {
		return source.kind, []string{"page", source.slug}
	}
	switch page := source.data.(type) {
	case stationPage:
		return "station", []string{"station", page.Station.Name, "place", p.stationPlace(page)}
	case forumHubPage:
		return "forum", nil
	case forumCategoryPage:
		return "forumCategory", []string{"category", p.forumCategory(page.Category)}
	case forumPostPage:
		return "forumPost", []string{"post", p.forumCopy(page.Post).Title, "category", p.forumCategory(page.Post.Category)}
	}
	if source.kind != "prices" {
		return source.kind, nil
	}
	switch page := source.data.(type) {
	case fuelHubPage:
		return "fuel", []string{"fuel", p.fuel(page.Code)}
	case statePage:
		return "state", []string{"place", page.Name}
	case municipalityPage:
		return "city", []string{"place", placeLabel(page.Name, page.UF)}
	case fuelAreaPage:
		return "fuelState", []string{"fuel", p.fuel(page.Summary[0].Code), "place", ufName(strings.ToUpper(source.area))}
	default:
		return "prices", nil
	}
}

func (page *localizedPage) appendModule(title string, link moduleLink) {
	if link.Href == "" || link.Href == page.Route {
		return
	}
	for i, module := range page.Modules {
		if module.Title != title {
			continue
		}
		for _, existing := range module.Links {
			if existing.Href == link.Href {
				return
			}
		}
		page.Modules[i].Links = append(module.Links, link)
		return
	}
	page.Modules = append(page.Modules, linkModule{Title: title, Links: []moduleLink{link}})
}

func (p localizedPortal) localizedLink(label, source string) moduleLink {
	if source == "" {
		return moduleLink{}
	}
	if href := p.routes[source]; href != "" {
		return moduleLink{Label: label, Href: href}
	}
	if tail, found := strings.CutPrefix(source, "/postos/"); found && strings.HasSuffix(tail, "/") {
		id := strings.TrimSuffix(tail, "/")
		if _, exists := p.stationIndex[id]; exists {
			return moduleLink{Label: label, Href: p.route("stations", id, "", "")}
		}
	}
	return moduleLink{Label: label, Href: source, Note: p.text("language.portuguese")}
}

func (p localizedPortal) addDirectory(data *localizedPage, cities bool) {
	for _, hub := range p.orderedFuelHubs() {
		data.appendModule(p.text("section.fuels"), p.localizedLink(p.fuel(hub.Code), hub.Href))
	}
	for _, uf := range sortedKeys(p.statePages) {
		data.appendModule(p.text("section.states"), p.localizedLink(ufName(uf), stateRoute(uf)))
	}
	if cities {
		for _, code := range sortedKeys(localizedCityCodes) {
			if city := p.cities[code]; city != nil && city.HasData {
				data.appendModule(p.text("section.cities"), p.localizedLink(placeLabel(city.Name, city.UF), city.Route))
			}
		}
	}
}

func (p localizedPortal) addFuelStateLinks(data *localizedPage, source localizedSource) {
	uf := strings.ToUpper(source.area)
	data.appendModule(p.text("section.states"), p.localizedLink(ufName(uf), stateRoute(uf)))
	for _, hub := range p.orderedFuelHubs() {
		if fuelSlug(hub.Code) == source.fuel {
			data.appendModule(p.text("section.fuels"), p.localizedLink(p.fuel(hub.Code), hub.Href))
			for _, other := range sortedKeys(p.statePages) {
				data.appendModule(p.text("section.states"), p.localizedLink(ufName(other), p.fuelStateHref(hub.Code, other)))
			}
		}
		data.appendModule(p.text("section.fuels"), p.localizedLink(p.fuel(hub.Code), p.fuelStateHref(hub.Code, uf)))
	}
}

func (p localizedPortal) downloads(data *localizedPage, page dadosPage) ldNode {
	data.Lead = p.text("data.lead")
	labels := map[string]string{
		"estacoes.csv": "data.stations", "precos.csv": "data.prices",
		"resumos.csv": "data.summaries", "manifest.json": "data.manifest",
	}
	table := p.table("section.files", "file", "rows", "bytes")
	distribution := []ldNode{}
	for _, file := range append(append([]downloadFile(nil), page.Files...), page.Manifest) {
		label := p.text(labels[path.Base(file.Href)])
		format, rows := "text/csv", p.integer(file.Rows)
		if file.Href == page.Manifest.Href {
			format, rows = "application/json", p.text("missing")
		}
		table.Rows = append(table.Rows, []localizedCell{
			{Text: label, Href: file.Href}, {Text: rows}, {Text: p.numbers.Sprintf("%d", file.Bytes)},
		})
		distribution = append(distribution, ldNode{"@type": "DataDownload", "name": label, "encodingFormat": format, "contentUrl": p.absoluteURL(file.Href)})
	}
	table.ID = "download-release"
	table.Detail = p.text("data.release", "release", page.Release, "period", data.Period)
	data.addTable(table)
	data.Notes = append(data.Notes, localizedNote{
		ID:    "notas",
		Title: p.text("section.notes"),
		Body:  strings.Join([]string{p.text("price.note"), p.text("methodology.privacy.body"), p.text("data.csv"), p.text("data.license")}, " "),
	})
	node := ldNode{
		"@type": "Dataset", "url": data.Canonical, "name": p.text("data.dataset"),
		"description": data.Description, "inLanguage": p.HTMLLang, "isAccessibleForFree": true,
		"creator":         ldNode{"@type": "Organization", "name": p.text("brand"), "url": p.absoluteURL(data.Home)},
		"license":         ldNode{"@type": "CreativeWork", "name": p.text("data.license"), "url": data.Canonical + "#notas"},
		"spatialCoverage": ldNode{"@type": "Place", "name": p.text("data.country")},
		"distribution":    distribution,
	}
	if p.latest != nil {
		node["temporalCoverage"] = p.latest.SurveyStart + "/" + p.latest.SurveyEnd
		if p.latest.SourceURL != "" {
			node["isBasedOn"] = p.latest.SourceURL
		}
	}
	return node
}

func (p localizedPortal) insights(data *localizedPage) {
	data.Lead = p.text("insights.lead")
	data.Insights = true
	method := p.routes["/metodologia/"]
	if p.latest == nil {
		data.Notes = append(data.Notes, localizedNote{Title: p.text("insights.coverage.title"),
			Body: p.text("period.unknown"), SourceHref: method})
		p.addOfficialInsights(data)
		return
	}

	start, end := "", ""
	for _, observation := range p.observations {
		date := observation.price.CollectedOn
		if start == "" || date < start {
			start = date
		}
		end = max(end, date)
	}
	data.Notes = append(data.Notes, localizedNote{
		Title: p.text("insights.coverage.title"), SourceHref: method, Detail: p.text("insights.coverage.detail"),
		Body: p.text("insights.coverage.body", "period", data.Period, "stations", p.integer(p.coverage.StationCount),
			"included", p.integer(p.coverage.IncludedObservations), "total", p.integer(p.coverage.SourceObservations),
			"unpriced", p.integer(p.coverage.UnpricedStations), "unlocated", p.integer(p.coverage.UnlocatedObservations)),
	})
	enough, excluded := 0, 0
	for _, count := range p.municipalityStations {
		if count >= minMunicipality {
			enough++
		}
	}
	for _, count := range p.coverage.Excluded {
		excluded += count
	}
	data.Notes = append(data.Notes,
		localizedNote{Title: p.text("insights.geography.title"), SourceHref: method, Detail: p.text("insights.geography.detail"),
			Body: p.text("insights.geography.body", "enough", p.integer(enough), "total", p.integer(len(p.municipalityStations)),
				"minimum", p.integer(minMunicipality), "below", p.integer(len(p.municipalityStations)-enough))},
		localizedNote{Title: p.text("insights.dates.title"), SourceHref: method, Detail: p.text("insights.dates.detail"),
			Body: p.text("insights.dates.body", "period", data.Period, "collection", p.portalLocale.period(start, end), "published", data.PublishedAt)},
		localizedNote{Title: p.text("insights.excluded.title"), SourceHref: method, Detail: p.text("insights.excluded.detail"),
			Body: p.text("insights.excluded.body", "total", p.integer(excluded))},
		p.gasolineInsight(method, data.Period),
	)
	reasons := p.table("insights.excluded.reasons", "reason", "observations")
	for _, code := range sortedKeys(p.coverage.Excluded) {
		label := p.catalog.Messages["insights.excluded."+code]
		if label == "" {
			label = code
		}
		reasons.rows(label, p.integer(p.coverage.Excluded[code]))
	}
	data.addTable(reasons)

	table := p.table("insights.fuels.title", "fuel", "unit", "stations")
	table.Detail = p.text("insights.fuels.detail")
	for _, fuel := range p.coverage.FuelUnits {
		table.rows(p.fuel(fuel.Fuel), p.unit(fuel.Unit), p.integer(fuel.Stations))
	}
	data.addTable(table)
	p.addOfficialInsights(data)
	data.Notes = append(data.Notes, localizedNote{Title: p.text("insights.print.title"), Body: p.text("insights.print.body"), SourceHref: p.routes["/dados/"]})
}

func (p localizedPortal) gasolineInsight(method, period string) localizedNote {
	note := localizedNote{Title: p.text("insights.gasoline.title"), SourceHref: method,
		Detail: p.text("insights.gasoline.detail"), Body: p.text("insights.gasoline.empty")}
	key := summaryKey{analytics.LevelNational, "BR", "GASOLINE_REGULAR", "L"}
	current, ok := p.current[key]
	if !ok {
		return note
	}
	note.Body = p.text("insights.gasoline.current", "period", period, "stations", p.stationCount(current.StationCount),
		"min", p.money(current.MinMilliBRL), "max", p.money(current.MaxMilliBRL), "spread", p.money(current.SpreadMilliBRL))
	if previous, ok := p.previousSummaries[key]; ok {
		change, err := analytics.CompareSummaries(previous, current)
		previousPeriod := p.portalLocale.period(previous.SurveyStart, previous.SurveyEnd)
		switch {
		case err == nil && change.SampleChanged:
			note.Body += " " + p.text("insights.gasoline.changed", "period", previousPeriod)
		case err == nil && change.Comparable:
			note.Body += " " + p.text("insights.gasoline.comparable", "period", previousPeriod, "min", p.movement(change.ChangeMinMilliBRL),
				"max", p.movement(change.ChangeMaxMilliBRL), "spread", p.movement(change.ChangeSpreadMilliBRL))
		}
	}
	return note
}
