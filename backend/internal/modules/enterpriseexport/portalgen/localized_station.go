package portalgen

import (
	"strings"
	"time"
	"unicode/utf8"
)

type localizedStationPage struct {
	View           stationView
	Region         moduleLink
	Observations   localizedTable
	Histories      []localizedTable
	Source         sourceView
	Records        []localizedStationRecord
	RecordsSummary string
}

type localizedStationRecord struct {
	Link               moduleLink
	Proceeding, Amount string
	Date, Association  string
}

func (p localizedPortal) stationPlace(page stationPage) string {
	if page.City != "" {
		return placeLabel(page.City, page.UF)
	}
	if page.UF != "" {
		return ufName(page.UF)
	}
	return p.text("station.location.unknown")
}

func (p localizedPortal) addStation(data *localizedPage, page stationPage) ldNode {
	index := p.stationIndex[page.Station.ID]
	entry := p.entries[index]
	station := entry.station
	view := &localizedStationPage{View: page.Station, Source: page.Source}
	data.Station = view
	data.ShowPeriod = false
	data.Title = page.Station.Name
	if page.City != "" {
		data.Title += " – " + p.stationPlace(page)
	}
	// UUID text distinguishes independently registered stations with identical
	// display names. Keep the discriminator inside both metadata budgets.
	id := station.StationID
	suffix := " – " + id[len(id)-12:]
	data.HeadTitle = truncateWords(p.text("title.station", "station", page.Station.Name, "place", p.stationPlace(page)), titleMax-utf8.RuneCountInString(suffix)) + suffix
	idSentence := p.text("station.identifier", "id", id)
	data.Description = truncateWords(data.Description, descriptionMax-utf8.RuneCountInString(idSentence)-1) + " " + idSentence
	data.Breadcrumbs = data.Breadcrumbs[:1]
	data.Breadcrumbs = append(data.Breadcrumbs, crumb{Label: p.text("nav.prices"), Href: p.routes["/precos/"]})
	if page.Station.StateHref != "" {
		data.Breadcrumbs = append(data.Breadcrumbs, crumb{Label: page.Station.StateName, Href: p.localizedLink(page.Station.StateName, page.Station.StateHref).Href})
		view.Region = p.localizedLink(p.text("station.pricesIn", "place", page.Station.StateName), page.Station.StateHref)
	}
	if page.Station.MunicipalityHref != "" {
		city := p.localizedLink(p.text("station.pricesIn", "place", page.Station.Municipality), page.Station.MunicipalityHref)
		view.Region = city
		label := page.City
		if city.Note != "" {
			label += " (" + city.Note + ")"
		}
		data.Breadcrumbs = append(data.Breadcrumbs, crumb{Label: label, Href: city.Href})
	}
	data.Breadcrumbs = append(data.Breadcrumbs, crumb{Label: page.Station.Name, Current: true})
	view.View.Place = p.stationPlace(page)
	view.View.Status = p.operationStatus(station.OperationStatus)
	view.View.Location = p.text("station.location.unknown")
	switch station.LocationQuality {
	case "reviewed", "city-centroid":
		view.View.Location = p.text("station.location." + station.LocationQuality)
	}
	view.Observations = p.table("station.observationsCaption", "fuel", "unit", "condition", "price", "date", "source")
	for _, row := range page.Observations {
		view.Observations.rows(p.fuel(row.FuelCode), p.unit(row.Unit), p.text("station.spot"), p.money(row.Amount), p.date(row.CollectedISO), "ANP")
	}
	fuels := make([]string, 0, len(page.Histories))
	for _, history := range page.Histories {
		table := p.table("station.historyCaption", "period", "price", "date", "status")
		table.Detail = p.fuel(history.FuelCode) + " · " + p.unit(history.Unit)
		for _, point := range history.Points {
			value, date, status := p.text("missing"), p.text("missing"), p.text("station.historyMissing")
			if !point.Missing && point.Amount != nil {
				value, date, status = p.money(*point.Amount), p.date(point.CollectedISO), p.text("station.historyPublished")
			}
			table.rows(p.portalLocale.period(point.SurveyStart, point.SurveyEnd), value, date, status)
		}
		view.Histories = append(view.Histories, table)
		fuels = append(fuels, history.FuelCode)
		if hub, ok := p.fuelHubs[history.FuelCode]; ok {
			data.appendModule(p.text("station.fuels"), p.localizedLink(p.fuel(history.FuelCode), hub.Href))
		}
	}
	if city := p.cities[station.MunicipalityCode]; city != nil && page.Station.MunicipalityHref != "" {
		for _, sibling := range siblingLinks(city, &p.entries[index]) {
			link := p.localizedLink(sibling.Label, sibling.Href)
			link.Note = sibling.Note
			data.appendModule(p.text("station.siblings", "city", city.Name), link)
		}
	}
	if p.opts.LandingURL != "" && len(fuels) > 0 {
		for _, link := range stationGuides(fuels) {
			data.appendModule(p.text("section.related"), p.localizedEditorialResource(link.Href))
		}
	}
	view.Source.Period = data.Period
	if date, err := time.Parse("02/01/2006 15:04", view.Source.PublishedAt); err == nil {
		view.Source.PublishedAt = date.Format(p.DateLayout + " 15:04")
	}
	for _, record := range page.OfficialRecords {
		link := p.localizedLink(record.Status, "/fiscalizacao/registro/"+record.RecordID+"/")
		item := localizedStationRecord{Link: link, Proceeding: record.Proceeding, Amount: record.Amount,
			Association: p.fiscalAssociation(record.AssociationState)}
		if record.DueOn != "" {
			item.Date = p.text("station.recordDue", "date", p.fiscalDate(record.DueOn))
		} else if record.DecidedOn != "" {
			item.Date = p.text("station.recordDecided", "date", p.fiscalDate(record.DecidedOn))
		}
		view.Records = append(view.Records, item)
	}
	if page.OfficialMore {
		view.RecordsSummary = p.text("station.recordsLimit", "total", p.integer(page.OfficialCount), "shown", p.integer(len(view.Records)))
	}
	return p.gasStationNode(data.pageBase, entry)
}

var localizedEditorialPaths = map[string][2]string{
	"/guias/etanol-ou-gasolina/":                 {"guides/ethanol-or-gasoline", "guias/etanol-o-gasolina"},
	"/guias/como-ler-precos/":                    {"guides/how-to-read-fuel-prices", "guias/como-leer-los-precios"},
	"/guias/pesquisa-anp/":                       {"guides/anp-weekly-price-survey", "guias/encuesta-semanal-anp"},
	"/guias/combustivel-adulterado/":             {"guides/recognize-and-report-adulterated-fuel", "guias/identificar-y-denunciar-combustible-adulterado"},
	"/guias/como-calcular-consumo-medio/":        {"guides/measure-fuel-economy", "guias/calcular-consumo-medio"},
	"/guias/como-e-formado-o-preco-da-gasolina/": {"guides/how-gasoline-prices-are-formed", "guias/como-se-forma-el-precio-de-la-gasolina"},
	"/guias/como-economizar-combustivel/":        {"guides/save-fuel", "guias/ahorrar-combustible"},
	"/guias/diesel-s10-ou-s500/":                 {"guides/diesel-s10-or-s500", "guias/diesel-s10-o-s500"},
	"/guias/direitos-do-consumidor-no-posto/":    {"guides/fuel-station-consumer-rights", "guias/derechos-del-consumidor-en-la-estacion"},
	"/guias/gasolina-comum-ou-aditivada/":        {"guides/regular-or-additive-gasoline", "guias/gasolina-regular-o-con-aditivos"},
	"/guias/gnv-compensa/":                       {"guides/is-cng-worth-it", "guias/conviene-el-gnv"},
	"/guias/icms-sobre-combustiveis/":            {"guides/icms-fuel-tax-explained", "guias/icms-sobre-combustibles"},
	"/guias/precos-diferentes-entre-cidades/":    {"guides/why-fuel-prices-vary-by-city", "guias/por-que-varian-los-precios-entre-ciudades"},
	"/ferramentas/etanol-ou-gasolina/":           {"tools/ethanol-or-gasoline-calculator", "herramientas/calculadora-etanol-o-gasolina"},
	"/ferramentas/custo-de-viagem/":              {"tools/trip-fuel-cost-calculator", "herramientas/calculadora-costo-de-viaje"},
	"/ferramentas/consumo-medio/":                {"tools/fuel-consumption-calculator", "herramientas/calculadora-consumo-medio"},
	"/ferramentas/custo-por-km/":                 {"tools/cost-per-km-calculator", "herramientas/calculadora-costo-por-km"},
	"/ferramentas/encher-o-tanque/":              {"tools/fill-up-cost-calculator", "herramientas/calculadora-llenar-el-tanque"},
}

var localizedEditorialLandingRoutes = func() map[string]bool {
	routes := make(map[string]bool, 2*len(localizedEditorialPaths))
	for _, localized := range localizedEditorialPaths {
		routes["/en/"+localized[0]+"/"] = true
		routes["/es/"+localized[1]+"/"] = true
	}
	return routes
}()

func isLandingRoute(route string) bool {
	return LandingRoutes[route] || localizedEditorialLandingRoutes[route]
}

func (p localizedPortal) localizedEditorialResource(route string) moduleLink {
	label := p.text("forum.guide")
	switch route {
	case "/guias/como-ler-precos/":
		label = p.text("station.priceGuide")
	case "/ferramentas/etanol-ou-gasolina/":
		label = p.text("station.ethanolTool")
	case "/ferramentas/custo-por-km/":
		label = p.text("station.costTool")
	default:
		if strings.HasPrefix(route, "/ferramentas/") {
			label = p.text("forum.tool")
		}
	}
	if pair, ok := localizedEditorialPaths[route]; ok && p.Code == "en" {
		return moduleLink{Label: label, Href: p.landingPrefix + p.Prefix + "/" + pair[0] + "/"}
	} else if ok && p.Code == "es" {
		return moduleLink{Label: label, Href: p.landingPrefix + p.Prefix + "/" + pair[1] + "/"}
	}
	return p.localizedLink(label, p.landingPrefix+route)
}
