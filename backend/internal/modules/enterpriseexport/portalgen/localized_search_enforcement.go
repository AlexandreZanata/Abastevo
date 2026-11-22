package portalgen

import (
	"encoding/json"
	"strings"
	"time"
)

type localizedSearchPage struct {
	Release, Copy, StationPrefix string
	States                       []moduleLink
}

type localizedFiscalPage struct {
	HasData                     bool
	Release, Copy, RecordPrefix string
	Lead, Edition, Publication  string
	DirectoryTitle              string
	Stats                       []localizedFiscalStat
	Records                     localizedTable
	PrevHref, NextHref          string
}

type localizedFiscalStat struct{ Label, Count, Detail string }

type localizedFiscalRecordPage struct {
	Status, CNPJ, Association, Reason string
	Station                           moduleLink
	Edition, Extraction, Publication  string
	Fields                            []localizedFiscalField
	SourceURL, Metadata, BackHref     string
}

type localizedFiscalField struct{ Label, Value string }

func (p localizedPortal) searchCopy(prefix string) string {
	copy := map[string]string{}
	for _, key := range []string{"area", "country", "state", "city", "remove", "station", "stations"} {
		copy[key] = p.catalog.Messages["explorer."+key]
	}
	for key, value := range p.catalog.Messages {
		if name, ok := strings.CutPrefix(key, prefix+"."); ok {
			copy[name] = value
		}
	}
	raw, _ := json.Marshal(copy) // A map of strings always encodes.
	return string(raw)
}

func (p localizedPortal) searchStationPrefix() string {
	const marker = "00000000-0000-4000-8000-000000000001"
	return strings.TrimSuffix(p.route("stations", marker, "", ""), marker+"/")
}

func (p localizedPortal) addSearch(data *localizedPage, page consultarPage) {
	search := &localizedSearchPage{
		Release: page.SearchRelease, StationPrefix: p.searchStationPrefix(),
		Copy: p.searchCopy("search.runtime"),
	}
	for _, state := range page.States {
		link := p.localizedLink(placeLabel(state.Name, state.Code), state.Href)
		link.Note = p.text("sample.stateCount", "stations", p.integer(state.Stations), "fuels", p.integer(state.Fuels))
		search.States = append(search.States, link)
	}
	data.ScriptURL = page.ScriptURL
	data.Search = search
	data.Lead = p.text("search.lead")
	data.addTable(p.prices(page.National, "", "search.national"))
}

func (p localizedPortal) fiscalDate(value string) string {
	day, err := time.Parse("02/01/2006", value)
	if err != nil {
		return value
	}
	return day.Format(p.DateLayout)
}

func (p localizedPortal) fiscalStatus(value string) string {
	return valueOr(p.catalog.Messages["fiscal.runtime.status."+strings.ToLower(strings.TrimSpace(value))], value)
}

func (p localizedPortal) fiscalReason(value string) string {
	return valueOr(p.catalog.Messages["fiscal.reason."+value], value)
}

func (p localizedPortal) fiscalAssociation(value string) string {
	return valueOr(p.catalog.Messages["fiscal.association."+value], p.text("unknown"))
}

func (p localizedPortal) fiscalEdition(manifest officialPin) string {
	return p.text("fiscal.edition", "edition", manifest.Edition, "period", valueOr(manifest.SourcePeriod, p.text("unknown")))
}

func (p localizedPortal) addEnforcement(data *localizedPage, page fiscalizacaoPage) {
	fiscal := &localizedFiscalPage{HasData: page.HasData}
	data.Enforcement = fiscal
	data.ShowPeriod = false
	if !page.HasData {
		data.Lead = p.text("fiscal.unavailable.lead")
		return
	}
	fiscal.Lead = p.text("fiscal.lead")
	fiscal.Edition = p.fiscalEdition(page.Manifest) + " · " + p.text("fiscal.sourceClock",
		"clock", valueOr(page.Manifest.SourceClockValue, p.text("unknown")),
		"precision", valueOr(p.catalog.Messages["fiscal.precision."+page.Manifest.SourceClockPrecision], valueOr(page.Manifest.SourceClockPrecision, p.text("unknown"))))
	fiscal.Publication = p.text("fiscal.publication", "revision", page.Manifest.RevisionID, "lease", page.Manifest.LeaseID,
		"checksum", page.Manifest.SourceChecksum, "policy", page.Manifest.AssociationPolicy, "scope", page.Manifest.AssociationScope,
		"decision", p.date(page.Manifest.AssociationDecisionDay))
	fiscal.Stats = []localizedFiscalStat{
		{p.text("fiscal.total"), p.integer(page.Total), p.text("fiscal.total.detail")},
		{p.text("fiscal.linked"), p.integer(page.Linked), p.text("fiscal.linked.detail")},
		{p.text("fiscal.unlinked"), p.integer(page.Unmatched), p.text("fiscal.unlinked.detail")},
		{p.text("fiscal.ambiguous"), p.integer(page.Ambiguous), p.text("fiscal.ambiguous.detail")},
	}
	if page.Page == 1 && page.SearchRelease != "" {
		fiscal.Release, fiscal.Copy = page.SearchRelease, p.searchCopy("fiscal.runtime")
		const marker = "000000000000000000000000"
		fiscal.RecordPrefix = strings.TrimSuffix(p.route("enforcementRecord", marker, "", ""), marker+"/")
		data.ScriptURL = page.ScriptURL
	}
	fiscal.DirectoryTitle = p.text("fiscal.directory")
	if page.Pages > 1 {
		fiscal.DirectoryTitle = p.text("fiscal.directory.page", "page", p.integer(page.Page), "total", p.integer(page.Pages))
	}
	fiscal.PrevHref = p.localizedLink("", page.PrevHref).Href
	fiscal.NextHref = p.localizedLink("", page.NextHref).Href
	fiscal.Records = p.table("fiscal.table", "cnpj", "sourceStatus", "proceeding", "amount", "due", "decision", "association", "details")
	for _, row := range page.Records {
		status := localizedCell{Text: p.fiscalStatus(row.Status)}
		if row.Status != status.Text {
			status.Details = []string{row.Status}
		}
		association := localizedCell{Text: p.fiscalAssociation(row.AssociationState)}
		if row.StationHref != "" {
			association.Text, association.Href = row.StationName, p.localizedLink("", row.StationHref).Href
			if row.StationLegalName != "" {
				association.Details = []string{row.StationLegalName}
			}
		}
		fiscal.Records.Rows = append(fiscal.Records.Rows, []localizedCell{
			{Text: row.CNPJ}, status, {Text: valueOr(row.Proceeding, p.text("missing"))},
			{Text: valueOr(row.Amount, p.text("missing"))}, {Text: valueOr(p.fiscalDate(row.DueOn), p.text("missing"))},
			{Text: valueOr(p.fiscalDate(valueOr(row.DecidedOn, row.AsOf)), p.text("missing"))},
			association, {Text: p.text("fiscal.open"), Href: p.route("enforcementRecord", row.RecordID, "", "")},
		})
	}
}

func (p localizedPortal) addEnforcementRecord(data *localizedPage, page fiscalizacaoRecordPage) {
	record := page.Record
	view := &localizedFiscalRecordPage{
		Status: p.fiscalStatus(record.Status), CNPJ: record.CNPJ,
		Association: p.fiscalAssociation(record.AssociationState), Reason: p.fiscalReason(record.AssociationReason),
		Station: p.localizedLink(record.StationName, record.StationHref),
		Edition: p.fiscalEdition(page.Manifest), Extraction: valueOr(record.ExtractionMonth, p.text("unknown")),
		Publication: data.PublishedAt, SourceURL: record.SourceURL, BackHref: p.routes["/fiscalizacao/"],
		Metadata: p.text("fiscal.record.metadata", "revision", record.RevisionID, "lease", record.LeaseID,
			"checksum", record.SourceChecksum, "publication", page.Manifest.PublishedAt),
	}
	if record.StationLegalName != "" {
		view.Station.Note = record.StationLegalName
	}
	data.Title = view.Status
	data.EnforcementRecord = view
	data.ShowPeriod = false
	for _, field := range []localizedFiscalField{
		{p.text("col.cnpj"), view.CNPJ}, {p.text("col.sourceStatus"), record.Status},
		{p.text("col.proceeding"), record.Proceeding}, {p.text("col.amount"), record.Amount},
		{p.text("col.due"), p.fiscalDate(record.DueOn)}, {p.text("col.decision"), p.fiscalDate(record.DecidedOn)},
		{p.text("fiscal.record.asof"), p.fiscalDate(record.AsOf)},
	} {
		field.Value = valueOr(field.Value, p.text("unknown"))
		view.Fields = append(view.Fields, field)
	}
	if record.OccurredOn != "" {
		view.Fields = append(view.Fields, localizedFiscalField{p.text("fiscal.record.occurred"), p.fiscalDate(record.OccurredOn)})
	}
	view.Fields = append(view.Fields, localizedFiscalField{p.text("fiscal.record.evidence"), record.Evidence})
}
