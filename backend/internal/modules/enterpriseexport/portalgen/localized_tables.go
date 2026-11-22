package portalgen

import (
	"strings"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport/analytics"
)

func (p localizedPortal) table(title string, headers ...string) localizedTable {
	table := localizedTable{Title: p.text(title)}
	for _, header := range headers {
		table.Headers = append(table.Headers, p.text("col."+header))
	}
	return table
}

func (page *localizedPage) addTable(table localizedTable) {
	if len(table.Rows) > 0 {
		page.Tables = append(page.Tables, table)
	}
}

func (table *localizedTable) rows(texts ...string) {
	row := make([]localizedCell, len(texts))
	for i, text := range texts {
		row[i].Text = text
	}
	table.Rows = append(table.Rows, row)
}

func (p localizedPortal) optionalPrice(amount int64, show bool) string {
	if show {
		return p.money(amount)
	}
	return p.text("missing")
}

func (p localizedPortal) prices(rows []summaryRow, uf, title string) localizedTable {
	if title == "" {
		title = "section.summary"
	}
	table := p.table(title, "fuel", "unit", "stations", "observations", "min", "max", "mean", "spread")
	for _, row := range rows {
		href := p.routes[row.FuelHref]
		if area := p.fuelStateHref(row.Code, uf); uf != "" && area != "" {
			href = p.localizedLink("", area).Href
		}
		count := p.integer(row.Stations)
		if row.Insufficient {
			count += " · " + p.text("sample.small")
		}
		table.rows(
			p.fuel(row.Code), p.unit(row.Unit), count, p.integer(row.Observations),
			p.money(row.Min), p.money(row.Max), p.optionalPrice(row.Mean, row.HasMean), p.money(row.Spread),
		)
		table.Rows[len(table.Rows)-1][0].Href = href
	}
	return table
}

func (p localizedPortal) geo(title string, rows []fuelGeoRow) localizedTable {
	table := p.table(title, "place", "stations", "min", "max", "mean", "spread")
	for _, row := range rows {
		href := p.routes[row.Href]
		parts := strings.Split(strings.Trim(row.Href, "/"), "/")
		if href == "" && len(parts) == 4 {
			href = p.routes["/precos/"+parts[2]+"/"+parts[3]+"/"]
		}
		if href == "" {
			href = row.Href
		}
		count := p.integer(row.Stations)
		if row.Insufficient {
			count += " · " + p.text("sample.small")
		}
		table.rows(
			placeLabel(row.Name, row.UF), count, p.optionalPrice(row.Min, !row.Insufficient),
			p.optionalPrice(row.Max, !row.Insufficient), p.optionalPrice(row.Mean, row.HasMean),
			p.optionalPrice(row.Spread, !row.Insufficient),
		)
		table.Rows[len(table.Rows)-1][0].Href = href
	}
	return table
}

func (p localizedPortal) history(rows []fuelHistoryRow) localizedTable {
	table := p.table("section.history", "period", "stations", "min", "max", "mean")
	for i, row := range rows {
		week := p.weeks[i].revision
		count := p.integer(row.Stations)
		if row.Missing {
			count = p.text("unknown")
		}
		table.rows(
			p.portalLocale.period(week.SurveyStart, week.SurveyEnd), count, p.optionalPrice(row.Min, !row.Missing),
			p.optionalPrice(row.Max, !row.Missing), p.optionalPrice(row.Mean, row.HasMean),
		)
	}
	return table
}

func (p localizedPortal) priorSurvey(rows []summaryRow, level, code string) localizedTable {
	if p.previous == nil {
		return localizedTable{}
	}
	table := p.table("section.summary", "fuel", "previous", "current", "min", "max", "mean", "comparison")
	table.Title = p.text("section.comparison", "period", p.portalLocale.period(p.previous.SurveyStart, p.previous.SurveyEnd))
	for _, row := range rows {
		key := summaryKey{level, code, row.Code, row.Unit}
		before, okBefore := p.previousSummaries[key]
		now, okNow := p.current[key]
		if !okBefore || !okNow {
			continue
		}
		change, err := analytics.CompareSummaries(before, now)
		if err != nil || (!change.Comparable && !change.SampleChanged) {
			continue
		}
		var minimum, maximum string
		mean, situation := p.text("missing"), p.text("sample.changed")
		meanBefore, hasBefore := summaryMean(before)
		meanNow, hasNow := summaryMean(now)
		if change.Comparable {
			minimum, maximum, situation = p.movement(change.ChangeMinMilliBRL), p.movement(change.ChangeMaxMilliBRL), p.text("sample.same")
			if hasBefore && hasNow {
				delta := meanNow - meanBefore
				mean = p.movement(&delta)
			}
		} else {
			minimum = p.text("sample.before", "price", p.money(before.MinMilliBRL))
			maximum = p.text("sample.before", "price", p.money(before.MaxMilliBRL))
			if hasBefore {
				mean = p.text("sample.before", "price", p.money(meanBefore))
			}
		}
		table.rows(p.fuel(row.Code), p.integer(before.StationCount), p.integer(now.StationCount), minimum, maximum, mean, situation)
	}
	return table
}

func (p localizedPortal) movement(amount *int64) string {
	switch {
	case amount == nil:
		return p.text("missing")
	case *amount > 0:
		return p.text("sample.up", "price", p.money(*amount))
	case *amount < 0:
		return p.text("sample.down", "price", p.money(-*amount))
	default:
		return p.text("sample.flat")
	}
}

func (p localizedPortal) stateFuelDirectory(page *localizedPage) {
	table := p.table("section.stateFuels", "place", "fuel")
	for _, uf := range sortedKeys(p.statePages) {
		for _, hub := range p.orderedFuelHubs() {
			if href := p.localizedLink("", p.fuelStateHref(hub.Code, uf)).Href; href != "" {
				table.Rows = append(table.Rows, []localizedCell{
					{Text: ufName(uf)}, {Text: p.fuel(hub.Code), Href: href},
				})
			}
		}
	}
	page.addTable(table)
}
