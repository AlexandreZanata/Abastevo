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
	fuel string
	area string
	city string
}

func localizedSourcePage(data routedPage) localizedSource {
	base := data.page()
	source := localizedSource{data: data}
	switch base.Route {
	case "/precos/":
		source.kind = "prices"
	case "/metodologia/":
		source.kind = "methodology"
	case "/dados/":
		source.kind = "data"
	case "/curiosidades/":
		source.kind = "insights"
	default:
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
	return locale.route(s.kind, s.fuel, s.area, s.city)
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
	base := data.page()
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
		facts := base.IndexFacts
		facts.Locale = &locale
		if route := source.route(locale); route != "" && pageIndexable(facts) {
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
	Title   string
	Headers []string
	Rows    [][]localizedCell
}

type localizedCell struct {
	Text string
	Href string
}

type localizedNote struct{ Title, Body string }

type localizedPage struct {
	pageBase
	Locale     portalLocale
	Links      []languageLink
	Nav        []moduleLink
	Home       string
	IsPrice    bool
	ShowPeriod bool
	NoPrices   bool
	Tables     []localizedTable
	Notes      []localizedNote
}

type localizedPortal struct {
	*generator
	portalLocale
	routes               map[string]string
	current              map[summaryKey]analytics.Summary
	coverage             analytics.Coverage
	observations         []observation
	municipalityStations map[string]int
}

func (g *generator) writeLocalizations(summaries []analytics.Summary, coverage analytics.Coverage, observations []observation, municipalityStations map[string]int) error {
	if len(g.localizations) == 0 {
		return nil
	}
	current := indexSummaries(summaries)
	for _, locale := range g.translationLocales() {
		p := localizedPortal{
			generator: g, portalLocale: locale, current: current, coverage: coverage,
			observations: observations, municipalityStations: municipalityStations,
			routes: make(map[string]string, len(g.localizations)),
		}
		for _, source := range g.localizations {
			p.routes[source.data.page().Route] = source.route(locale)
		}
		for _, source := range g.localizations {
			if err := p.write(source); err != nil {
				return fmt.Errorf("portalgen: %s: %w", source.route(locale), err)
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
	route := p.routes[original.Route]
	if route == "" {
		return fmt.Errorf("no localized route")
	}
	key, fields := p.pageText(source)
	title := p.text("title."+key, fields...)
	facts := original.IndexFacts
	facts.Locale = &p.portalLocale
	decision := pageIndexable(facts)
	base := pageBase{
		Title: title, HeadTitle: fitTitle(p.text("title.brand", "title", title), title),
		Description: p.priceDescription("description."+key, fields...),
		Indexable:   p.indexable && decision, Robots: robotsValue(p.indexable, decision),
		Route: route, Family: original.Family, Language: p.Code, LastMod: original.LastMod, CSSURL: p.cssURL,
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
	data.appendModule(p.text("section.related"), moduleLink{Label: p.text("search"), Href: "/consultar/"})
	var node ldNode
	switch page := source.data.(type) {
	case precosPage:
		data.addTable(p.prices(page.National, "", ""))
		data.NoPrices = len(page.National) == 0
		p.stateFuelDirectory(&data)
		p.addDirectory(&data, true)
	case fuelHubPage:
		data.addTable(p.prices(page.National, "", ""))
		data.addTable(p.geo("section.states", page.States))
		data.addTable(p.geo("section.cities", page.Cities))
		data.addTable(p.priorSurvey(page.National, analytics.LevelNational, "BR"))
		p.addDirectory(&data, false)
	case statePage:
		data.addTable(p.prices(page.Summaries, page.UF, ""))
		data.addTable(p.priorSurvey(page.Summaries, analytics.LevelState, page.UF))
		cities := append(append([]municipalityLink(nil), page.Municipalities...), page.NoDataCities...)
		for _, city := range cities {
			data.appendModule(p.text("section.cities"), p.localizedLink(city.Name, city.Href))
		}
		data.NoPrices = len(page.Summaries) == 0
		p.addDirectory(&data, false)
	case municipalityPage:
		data.addTable(p.prices(page.Summaries, page.UF, ""))
		data.addTable(p.priorSurvey(page.Summaries, analytics.LevelMunicipality, page.Code))
		data.appendModule(p.text("section.states"), p.localizedLink(ufName(page.UF), stateRoute(page.UF)))
		for _, link := range page.Stations {
			data.appendModule(p.text("section.stations"), moduleLink{Label: link.Label, Href: link.Href})
		}
		data.NoPrices = len(page.Summaries) == 0
	case fuelAreaPage:
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
		for _, section := range []string{"source", "sample", "revisions", "privacy"} {
			data.Notes = append(data.Notes, localizedNote{
				Title: p.text("methodology." + section + ".title"), Body: p.text("methodology." + section + ".body"),
			})
		}
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
	return moduleLink{Label: label, Href: p.routes[source]}
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
				data.appendModule(p.text("section.states"), p.localizedLink(ufName(other), fuelStateRoute(hub.Code, other)))
			}
		}
		data.appendModule(p.text("section.fuels"), p.localizedLink(p.fuel(hub.Code), fuelStateRoute(hub.Code, uf)))
	}
}

func (p localizedPortal) downloads(data *localizedPage, page dadosPage) ldNode {
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
	data.addTable(table)
	data.Notes = append(data.Notes, localizedNote{
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
	if p.latest == nil {
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
	data.Notes = append(data.Notes, localizedNote{Title: p.text("insights.coverage.title"), Body: p.text("insights.coverage.body",
		"period", data.Period, "stations", p.integer(p.coverage.StationCount),
		"included", p.integer(p.coverage.IncludedObservations), "total", p.integer(p.coverage.SourceObservations))})
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
		localizedNote{Title: p.text("insights.geography.title"), Body: p.text("insights.geography.body",
			"enough", p.integer(enough), "total", p.integer(len(p.municipalityStations)), "minimum", p.integer(minMunicipality))},
		localizedNote{Title: p.text("insights.dates.title"), Body: p.text("insights.dates.body",
			"period", data.Period, "collection", p.portalLocale.period(start, end))},
		localizedNote{Title: p.text("insights.excluded.title"), Body: p.text("insights.excluded.body", "total", p.integer(excluded))},
	)
	table := p.table("insights.fuels.title", "fuel", "unit", "stations")
	for _, fuel := range p.coverage.FuelUnits {
		table.rows(p.fuel(fuel.Fuel), p.unit(fuel.Unit), p.integer(fuel.Stations))
	}
	data.addTable(table)
	var gasoline []summaryRow
	if row, ok := p.current[summaryKey{analytics.LevelNational, "BR", "GASOLINE_REGULAR", "L"}]; ok {
		gasoline = []summaryRow{{Code: row.Fuel, Unit: row.Unit}}
	}
	data.addTable(p.priorSurvey(gasoline, analytics.LevelNational, "BR"))
}
