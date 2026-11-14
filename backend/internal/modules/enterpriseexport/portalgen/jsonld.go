package portalgen

import (
	"encoding/json"
	"html/template"
	"strings"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
)

// maxItemListEntries bounds an ItemList to the first visible entries.
const maxItemListEntries = 100

// listItem is one visible link of a collection page.
type listItem struct {
	Name string
	Href string
}

type ldNode = map[string]any

// absoluteURL resolves a root-relative portal or landing link against the
// site origin; absolute links are kept.
func (g *generator) absoluteURL(href string) string {
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		return href
	}
	return g.siteURL + href
}

func (g *generator) breadcrumbNode(base pageBase) ldNode {
	items := make([]ldNode, 0, len(base.Breadcrumbs))
	for i, item := range base.Breadcrumbs {
		target := base.Canonical
		if !item.Current {
			target = g.absoluteURL(item.Href)
		}
		items = append(items, ldNode{"@type": "ListItem", "position": i + 1, "name": item.Label, "item": target})
	}
	return ldNode{"@type": "BreadcrumbList", "@id": base.Canonical + "#breadcrumb", "itemListElement": items}
}

// collectionNode describes a hub, fuel, state or city page and the first
// visible entries of its main list.
func (g *generator) collectionNode(base pageBase, items []listItem) ldNode {
	elements := make([]ldNode, 0, min(len(items), maxItemListEntries))
	for i, item := range items {
		if i == maxItemListEntries {
			break
		}
		elements = append(elements, ldNode{"@type": "ListItem", "position": i + 1, "name": item.Name, "url": g.absoluteURL(item.Href)})
	}
	return ldNode{
		"@type": "CollectionPage", "@id": base.Canonical + "#webpage", "url": base.Canonical,
		"name": base.HeadTitle, "description": base.Description, "inLanguage": "pt-BR",
		"isPartOf":   ldNode{"@type": "WebSite", "name": "abastevo", "url": g.siteURL + "/"},
		"breadcrumb": ldNode{"@id": base.Canonical + "#breadcrumb"},
		"mainEntity": ldNode{"@type": "ItemList", "numberOfItems": len(elements), "itemListElement": elements},
	}
}

// gasStationNode carries only the visible identity and address; no rating,
// offer or price is emitted.
func (g *generator) gasStationNode(base pageBase, entry stationEntry) ldNode {
	node := ldNode{"@type": "GasStation", "@id": base.Canonical + "#station", "name": entry.name, "url": base.Canonical}
	if legal := stationLegalName(entry.station); legal != "" {
		node["legalName"] = legal
	}
	address := ldNode{"@type": "PostalAddress", "addressCountry": "BR"}
	if street := stationStreet(entry.station); street != "" {
		address["streetAddress"] = street
	}
	if entry.city != "" {
		address["addressLocality"] = entry.city
	}
	if knownUF(entry.station.State) {
		address["addressRegion"] = entry.station.State
	}
	if postal := addressComponent(entry.station.Address["postal_code"]); postal != "" {
		address["postalCode"] = postal
	}
	if len(address) > 2 {
		node["address"] = address
	}
	return node
}

// datasetNode describes the /dados/ downloads with the license text shown on
// the page and the survey's temporal and spatial coverage.
func (g *generator) datasetNode(base pageBase, files []downloadFile, manifest downloadFile, latest *enterpriseexport.Revision) ldNode {
	distribution := make([]ldNode, 0, len(files)+1)
	for _, file := range files {
		distribution = append(distribution, ldNode{"@type": "DataDownload", "name": file.Label, "encodingFormat": "text/csv", "contentUrl": g.absoluteURL(file.Href)})
	}
	if manifest.Href != "" {
		distribution = append(distribution, ldNode{"@type": "DataDownload", "name": manifest.Label, "encodingFormat": "application/json", "contentUrl": g.absoluteURL(manifest.Href)})
	}
	node := ldNode{
		"@type": "Dataset", "@id": base.Canonical + "#dataset", "url": base.Canonical,
		"name":        "Preços de combustíveis da pesquisa ANP (dados derivados do abastevo)",
		"description": base.Description, "inLanguage": "pt-BR", "isAccessibleForFree": true,
		"creator":         ldNode{"@type": "Organization", "name": "abastevo", "url": g.siteURL + "/"},
		"license":         ldNode{"@type": "CreativeWork", "name": dataLicenseNote, "url": base.Canonical + "#notas"},
		"spatialCoverage": ldNode{"@type": "Place", "name": "Brasil"},
		"distribution":    distribution,
	}
	if latest != nil {
		node["temporalCoverage"] = latest.SurveyStart + "/" + latest.SurveyEnd
		if latest.SourceURL != "" {
			node["isBasedOn"] = latest.SourceURL
		}
	}
	return node
}

// withStructuredData renders the page's JSON-LD graph: the breadcrumb plus
// the family nodes. json.Marshal escapes <, > and &, so the block cannot
// close its script element.
func (g *generator) withStructuredData(base pageBase, nodes ...ldNode) (pageBase, error) {
	if base.Canonical == "" {
		return base, nil
	}
	graph := append([]ldNode{g.breadcrumbNode(base)}, nodes...)
	raw, err := json.Marshal(ldNode{"@context": "https://schema.org", "@graph": graph})
	if err != nil {
		return base, err
	}
	base.JSONLD = template.JS(raw)
	return base, nil
}
