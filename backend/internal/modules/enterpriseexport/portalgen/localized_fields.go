package portalgen

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport/analytics"
)

type localizedExplorer struct {
	Release       string
	Copy          string
	StationPrefix string
	Fuels         []localizedExplorerFuel
}

type localizedExplorerFuel struct{ Code, Label, Unit, Count string }

func (p localizedPortal) explorer(page precosPage) *localizedExplorer {
	copy := map[string]string{}
	for _, key := range []string{
		"loading", "empty", "unavailable", "results", "country", "area", "ascending", "descending",
		"limit", "state", "city", "remove", "station", "stations",
	} {
		copy[key] = p.catalog.Messages["explorer."+key]
	}
	encoded, _ := json.Marshal(copy) // a map of strings always encodes
	explorer := &localizedExplorer{Release: page.SearchRelease, Copy: string(encoded), StationPrefix: p.searchStationPrefix()}
	for _, fuel := range page.SearchFuels {
		explorer.Fuels = append(explorer.Fuels, localizedExplorerFuel{
			Code: fuel.Code, Label: p.fuel(fuel.Code), Unit: p.unit(fuel.Unit), Count: p.stationCount(fuel.Count),
		})
	}
	return explorer
}

func (p localizedPortal) publication() string {
	date, err := time.Parse("02/01/2006 15:04", p.published)
	if err != nil {
		return p.text("unknown")
	}
	return date.Format(p.DateLayout + " 15:04")
}

func (p localizedPortal) addChrome(data *localizedPage) {
	product := linkModule{Title: p.text("footer.product"), Links: []moduleLink{
		p.localizedLink(p.text("nav.search"), "/consultar/"),
		p.localizedLink(p.text("nav.enforcement"), "/fiscalizacao/"),
	}}
	if p.hasForum() {
		product.Links = append(product.Links, p.localizedLink(p.text("nav.forum"), "/forum/"))
	}
	data.TopNav = append(data.TopNav, p.localizedLink(p.text("nav.search"), "/consultar/"))
	product.Links = append(product.Links, p.localizedLink(p.text("nav.methodology"), "/metodologia/"))
	data.Footer = append(data.Footer, product, linkModule{
		Title: p.text("nav.data"),
		Links: []moduleLink{
			p.localizedLink(p.text("nav.prices"), "/precos/"),
			p.localizedLink(p.text("title.data"), "/dados/"),
			p.localizedLink(p.text("nav.insights"), "/curiosidades/"),
		},
	})
	project := linkModule{Title: p.text("footer.project")}
	if p.opts.LandingURL != "" {
		guides, tools, about, editorial := "/en/guides/", "/en/tools/", "/en/about/", "/en/editorial-policy/"
		if p.Code == "es" {
			guides, tools, about, editorial = "/es/guias/", "/es/herramientas/", "/es/sobre/", "/es/politica-editorial/"
		}
		learn := linkModule{Title: p.text("footer.learn"), Links: []moduleLink{
			{Label: p.text("nav.help"), Href: p.landingPrefix + p.route("page", p.Segments["como-usar"], "", "")},
			{Label: p.text("nav.guides"), Href: p.landingPrefix + guides},
			{Label: p.text("nav.tools"), Href: p.landingPrefix + tools},
			{Label: p.text("nav.app"), Href: p.landingPrefix + p.Prefix + "/#aplicativo"},
		}}
		data.TopNav = append(data.TopNav, learn.Links[3], learn.Links[0], learn.Links[1], learn.Links[2])
		data.Footer = append(data.Footer, learn)
		project.Links = []moduleLink{
			{Label: p.text("nav.about"), Href: p.landingPrefix + about},
			{Label: p.text("nav.editorial"), Href: p.landingPrefix + editorial},
		}
	}
	const github = "https://github.com/AlexandreZanata/abastevo"
	project.Links = append(project.Links,
		moduleLink{Label: "GitHub", Href: github},
		moduleLink{Label: p.text("nav.contact"), Href: github + "/issues"},
		moduleLink{Label: p.text("nav.trademarks"), Href: github + "/blob/main/TRADEMARKS.md"},
	)
	data.Footer = append(data.Footer, project)
	if p.opts.LandingURL != "" {
		data.Footer = append(data.Footer, linkModule{Title: p.text("nav.privacy"), Links: []moduleLink{
			{Label: p.text("nav.privacy"), Href: p.landingPrefix + p.route("page", p.Segments["privacidade"], "", "")},
			{Label: p.text("nav.delete"), Href: p.landingPrefix + p.route("page", p.Segments["excluir-conta"], "", "")},
			{Label: p.text("nav.terms"), Href: p.landingPrefix + p.route("page", p.Segments["termos"], "", "")},
		}})
	}
}

func (p localizedPortal) stationCount(count int) string {
	key := "explorer.stations"
	if count == 1 {
		key = "explorer.station"
	}
	return p.integer(count) + " " + p.text(key)
}

func (p localizedPortal) addMunicipalities(data *localizedPage, sampled, registry []municipalityLink) {
	for _, city := range sampled {
		link := p.localizedLink(placeLabel(city.Name, city.UF), city.Href)
		if link.Note != "" {
			link.Note += " · "
		}
		link.Note += p.text("sample.count", "stations", p.stationCount(city.Stations))
		if city.Insufficient {
			link.Note += " · " + p.text("sample.minimum", "minimum", p.integer(minMunicipality))
		}
		data.appendModule(p.text("section.cities"), link)
	}
	if len(sampled) == 0 {
		data.Notes = append(data.Notes, localizedNote{Title: p.text("section.cities"), Body: p.text("sample.noCities")})
	}
	if len(registry) > 0 {
		title := p.text("section.registryOnly")
		data.Notes = append(data.Notes, localizedNote{Title: title, Body: p.text("sample.registryOnly")})
		for _, city := range registry {
			link := p.localizedLink(p.text("section.cityStations", "place", placeLabel(city.Name, city.UF)), city.Href)
			if link.Note != "" {
				link.Note += " · "
			}
			link.Note += p.stationCount(city.Stations)
			data.appendModule(title, link)
		}
	}
}

func (p localizedPortal) addCity(data *localizedPage, page municipalityPage) {
	place := placeLabel(page.Name, page.UF)
	data.Lead = p.text("sample.cityLead", "uf", page.UF, "code", page.Code, "stations", p.stationCount(page.Population))
	data.NoPrices = page.NoData
	switch {
	case page.NoData:
		data.Notes = append(data.Notes, localizedNote{Title: p.text("price.empty"), Body: p.text("sample.registryOnly")})
	case page.Insufficient:
		table := p.table("section.unranked", "station", "price", "date", "status")
		table.ID = "unranked-observations"
		table.Detail = p.text("sample.cityInsufficient", "stations", p.stationCount(page.Population), "minimum", p.integer(minMunicipality))
		for _, row := range page.Rows {
			table.Rows = append(table.Rows, []localizedCell{
				p.rankingStation(row), {Text: p.money(row.Amount)}, {Text: p.date(row.CollectedISO)}, {Text: p.operationStatus(row.SourceStatus)},
			})
		}
		data.addTable(table)
	default:
		data.addTable(p.prices(page.Summaries, "", ""))
		data.addTable(p.priorSurvey(page.Summaries, analytics.LevelMunicipality, page.Code))
		data.addRankings(p, page.Rankings, "")
		if len(page.FAQ) > 0 {
			answer := []string{p.text("faq.reference", "period", data.Period)}
			for _, row := range page.Summaries {
				answer = append(answer, p.text("faq.range", "fuel", p.fuel(row.Code), "min", p.money(row.Min), "max", p.money(row.Max)))
			}
			data.FAQ = []faqItem{
				{Question: p.text("faq.city", "place", place), Answer: strings.Join(answer, " ")},
				p.surveyFAQ(data.Period),
			}
		}
	}
	p.addRatio(data, page.Parity)
	stationHeader := p.text("section.cityStations", "place", page.Name)
	if page.StationsTotal > len(page.Stations) {
		data.Notes = append(data.Notes, localizedNote{Title: stationHeader, Body: p.text("sample.stationLimit",
			"shown", p.integer(len(page.Stations)), "total", p.integer(page.StationsTotal))})
	}
	for _, station := range page.Stations {
		link := p.localizedLink(station.Label, station.Href)
		if station.Note != "" {
			link.Note += " · " + station.Note
		}
		data.appendModule(stationHeader, link)
	}
	p.addFuelStates(data, page.UF)
}

func (p localizedPortal) addFuelStates(data *localizedPage, uf string) {
	data.appendModule(p.text("section.states"), p.localizedLink(ufName(uf), stateRoute(uf)))
	for _, hub := range p.orderedFuelHubs() {
		data.appendModule(p.text("section.fuels"), p.localizedLink(p.fuel(hub.Code), p.fuelStateHref(hub.Code, uf)))
	}
}

func (p localizedPortal) operationStatus(status string) string {
	if status == "active" || status == "suspended" || status == "revoked" {
		return p.text("status." + status)
	}
	return p.text("unknown")
}

func (p localizedPortal) rankingStation(row rankingRow) localizedCell {
	link := p.localizedLink(row.Name, "/postos/"+row.StationID+"/")
	cell := localizedCell{Text: link.Label, Href: link.Href}
	if row.LegalName != "" {
		cell.Details = append(cell.Details, row.LegalName)
	}
	if row.CNPJ != "" {
		cell.Details = append(cell.Details, row.CNPJ)
	}
	return cell
}

func (data *localizedPage) addRankings(p localizedPortal, rankings []rankingView, emptyKey string) {
	if len(rankings) == 0 && emptyKey != "" {
		data.NoRanking = p.text(emptyKey)
	}
	for _, ranking := range rankings {
		table := p.table("section.rankings", "position", "station", "place", "price", "date", "status")
		table.Title = p.text("ranking.fuel", "fuel", p.fuel(ranking.Code), "unit", p.unit(ranking.Unit))
		table.Detail = p.text("ranking.sample", "stations", p.stationCount(ranking.Population))
		if ranking.Truncated {
			table.Detail += " " + p.text("ranking.limit", "shown", p.integer(len(ranking.Rows)))
		}
		if ranking.TiesAtCutoff > 0 {
			table.Detail += " " + p.text("ranking.cutoff", "ties", p.integer(ranking.TiesAtCutoff))
		}
		for _, row := range ranking.Rows {
			position := p.integer(row.Position)
			if row.TieCount > 1 {
				position = p.text("ranking.tie", "position", position, "ties", p.integer(row.TieCount))
			}
			table.Rows = append(table.Rows, []localizedCell{
				{Text: position}, p.rankingStation(row), {Text: row.Place}, {Text: p.money(row.Amount)},
				{Text: p.date(row.CollectedISO)}, {Text: p.operationStatus(row.SourceStatus)},
			})
		}
		data.Rankings = append(data.Rankings, table)
	}
}

func (p localizedPortal) addRatio(data *localizedPage, ratio string) {
	if ratio == "" {
		return
	}
	if p.Code == "en" {
		ratio = strings.Replace(ratio, ",", ".", 1)
	}
	data.Notes = append(data.Notes, localizedNote{
		Title: p.text("ratio.link"), Body: p.text("ratio.summary", "ratio", ratio), SourceHref: parityRoute,
	})
}

func (p localizedPortal) surveyFAQ(period string) faqItem {
	return faqItem{
		Question: p.text("faq.period.question"),
		Answer:   p.text("faq.period.answer", "period", period),
	}
}

func (p localizedPortal) addFuelReading(data *localizedPage, row summaryRow, area string, states int, comparison *comparisonView) {
	fuel, stations := p.fuel(row.Code), p.stationCount(row.Stations)
	data.Lead = p.text("price.fuelLead", "fuel", fuel, "place", area, "unit", p.unit(row.Unit))
	reading := []string{p.text("reading.coverage", "period", data.Period, "stations", stations, "fuel", fuel, "place", area)}
	if states > 0 {
		reading = append(reading, p.text("reading.states", "states", p.integer(states)))
	}
	switch {
	case row.Stations == 1:
		reading = append(reading, p.text("reading.single", "price", p.money(row.Min)))
	case row.Min == row.Max:
		reading = append(reading, p.text("reading.flat", "price", p.money(row.Min)))
	default:
		reading = append(reading, p.text("reading.range", "min", p.money(row.Min), "max", p.money(row.Max)))
	}
	if row.HasMean {
		reading = append(reading, p.text("reading.mean", "mean", p.money(row.Mean)))
	}
	data.Reading = strings.Join(reading, " ")
	if comparison != nil {
		for _, change := range comparison.Rows {
			if change.Code != row.Code {
				continue
			}
			if !change.Comparable {
				data.Reading += " " + p.text("reading.changed")
			} else if change.HasMeanDelta {
				data.Reading += " " + p.text("reading.comparison", "movement", p.movement(&change.MeanDelta))
			}
		}
	}
	data.FAQ = []faqItem{{Question: p.text("faq.fuel", "fuel", fuel, "place", area),
		Answer: p.text("faq.reference", "period", data.Period) + " " + data.Reading}}
	if states == 0 {
		data.FAQ = append(data.FAQ, faqItem{
			Question: p.text("faq.minimum.question", "fuel", fuel, "place", area),
			Answer:   p.text("faq.minimum.answer", "period", data.Period, "price", p.money(row.Min)),
		})
	}
	data.FAQ = append(data.FAQ, p.surveyFAQ(data.Period))
	if row.HasMean {
		data.FAQ = append(data.FAQ, faqItem{
			Question: p.text("faq.mean.question"),
			Answer:   p.text("faq.mean.answer", "stations", stations),
			LinkHref: p.routes["/metodologia/"] + "#media-da-amostra", LinkLabel: p.text("nav.methodology"),
		})
	}
}

func (p localizedPortal) addMethodology(data *localizedPage) {
	data.Lead = p.text("methodology.lead")
	for _, section := range []struct{ key, id string }{
		{"source", ""}, {"dates", ""}, {"sample", ""}, {"mean", "media-da-amostra"},
		{"calculations", ""}, {"revisions", ""}, {"privacy", ""}, {"limits", ""},
	} {
		data.Notes = append(data.Notes, localizedNote{
			ID: section.id, Title: p.text("methodology." + section.key + ".title"),
			Body: p.text("methodology." + section.key + ".body"),
		})
	}
	data.Notes = append(data.Notes, localizedNote{
		Title: p.text("section.publication"), Body: p.text("publication.snapshot", "hash", data.InputHash,
			"version", analytics.MetricsVersion, "publication", data.PublishedAt),
	})
}

func (p localizedPortal) addOfficialInsights(data *localizedPage) {
	if p.official == nil {
		return
	}
	m := p.official.Manifest
	census := m.AssociationCensus
	data.Notes = append(data.Notes, localizedNote{
		Title: p.text("insights.official.count.title"), SourceHref: m.SourceURL, Detail: p.text("insights.official.count.detail"),
		Body: p.text("insights.official.count.body", "period", valueOr(m.SourcePeriod, m.Edition), "total", p.integer(m.Rows),
			"linked", p.integer(census.Linked), "unlinked", p.integer(census.Unmatched+census.WrongBranch+census.SuccessionReview),
			"ambiguous", p.integer(census.Ambiguous)),
	}, localizedNote{
		Title: p.text("insights.official.scope.title"), SourceHref: m.SourceURL, Detail: p.text("insights.official.scope.detail"),
		Body: p.text("insights.official.scope.body", "period", valueOr(m.SourcePeriod, m.Edition),
			"extraction", valueOr(m.SourceClockValue, p.text("unknown")), "precision", valueOr(m.SourceClockPrecision, p.text("unknown"))),
	}, localizedNote{
		Title: p.text("insights.official.link.title"), SourceHref: m.SourceURL, Detail: p.text("insights.official.link.detail"),
		Body: p.text("insights.official.link.body", "day", p.date(m.AssociationDecisionDay)),
	})
}
