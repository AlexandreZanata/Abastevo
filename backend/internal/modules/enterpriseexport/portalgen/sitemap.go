package portalgen

import (
	"encoding/xml"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const (
	sitemapXMLNS = "http://www.sitemaps.org/schemas/sitemap/0.9"
	// SitemapChunkURLs keeps every child sitemap well under the 50,000 URL
	// protocol limit (B-BR-SEO-04).
	SitemapChunkURLs = 45_000
)

// sitemapFamilies fixes the child file families and their order.
var sitemapFamilies = []string{"hubs", "precos", "postos", "forum"}

// The optional locale keeps previously published Portuguese artifacts verifiable.
var sitemapChildPath = regexp.MustCompile(`^sitemaps/portal-(hubs|precos|postos|forum)(?:-(pt-BR|[a-z]{2}))?-[1-9][0-9]*\.xml$`)

func sitemapFamily(family pageFamily) (string, error) {
	switch family {
	case familyHub, familyFiscalHub:
		return "hubs", nil
	case familyFuel, familyState, familyCity, familyFuelState, familyFuelCity, familyParity:
		return "precos", nil
	case familyStation:
		return "postos", nil
	case familyForumHub, familyForumCategory, familyForumPost:
		return "forum", nil
	default:
		return "", fmt.Errorf("portalgen: page family %s cannot be in a sitemap", family)
	}
}

type sitemapRef struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod,omitempty"`
}

type sitemapIndexDoc struct {
	XMLName  xml.Name     `xml:"sitemapindex"`
	XMLNS    string       `xml:"xmlns,attr"`
	Sitemaps []sitemapRef `xml:"sitemap"`
}

type sitemapFile struct {
	Path string
	Body []byte
}

func marshalSitemap(value any) ([]byte, error) {
	body, err := xml.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	body = append([]byte(xml.Header), body...)
	return append(body, '\n'), nil
}

// buildSitemaps splits indexable entries into per-family and per-locale sitemaps of
// at most chunk URLs and MaxSitemapBytes each, and returns them with the
// sitemap index body. Entry lastmod values are data dates; an index entry
// carries the newest lastmod of its file.
func buildSitemaps(siteURL string, entries []sitemapEntry, chunk int) ([]sitemapFile, []byte, error) {
	if chunk <= 0 || chunk > MaxSitemapURLs {
		return nil, nil, fmt.Errorf("portalgen: invalid sitemap chunk")
	}
	type partition struct{ family, locale string }
	byPartition := map[partition][]sitemapEntry{}
	localeCodes := map[string]bool{}
	seen := map[string]bool{}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Route, "/") || !strings.HasSuffix(entry.Route, "/") {
			return nil, nil, fmt.Errorf("portalgen: invalid sitemap route")
		}
		if seen[entry.Route] {
			return nil, nil, fmt.Errorf("portalgen: duplicate sitemap route")
		}
		seen[entry.Route] = true
		family, err := sitemapFamily(entry.Family)
		if err != nil {
			return nil, nil, err
		}
		locale := entry.Locale
		if locale == "" {
			locale = defaultPortalLocale
		}
		localeCodes[locale] = true
		key := partition{family, locale}
		byPartition[key] = append(byPartition[key], entry)
	}
	if len(seen) == 0 {
		return nil, nil, fmt.Errorf("portalgen: empty sitemap")
	}
	index := sitemapIndexDoc{XMLNS: sitemapXMLNS}
	var files []sitemapFile
	delete(localeCodes, defaultPortalLocale)
	localeOrder := append([]string{defaultPortalLocale}, sortedKeys(localeCodes)...)
	for _, family := range sitemapFamilies {
		for _, locale := range localeOrder {
			group := byPartition[partition{family, locale}]
			sort.Slice(group, func(i, j int) bool { return group[i].Route < group[j].Route })
			for start, n := 0, 1; start < len(group); start, n = start+chunk, n+1 {
				end := min(start+chunk, len(group))
				set := sitemapSet{XMLNS: sitemapXMLNS, URLs: make([]sitemapURL, 0, end-start)}
				newest := ""
				for _, entry := range group[start:end] {
					set.URLs = append(set.URLs, sitemapURL{Loc: siteURL + entry.Route, LastMod: entry.LastMod})
					newest = max(newest, entry.LastMod)
				}
				body, err := marshalSitemap(set)
				if err != nil {
					return nil, nil, err
				}
				if int64(len(body)) > MaxSitemapBytes {
					return nil, nil, &BudgetError{Msg: "portalgen: sitemap bytes budget exceeded"}
				}
				path := fmt.Sprintf("sitemaps/portal-%s-%s-%d.xml", family, locale, n)
				files = append(files, sitemapFile{Path: path, Body: body})
				index.Sitemaps = append(index.Sitemaps, sitemapRef{Loc: siteURL + "/" + path, LastMod: newest})
			}
		}
	}
	body, err := marshalSitemap(index)
	if err != nil {
		return nil, nil, err
	}
	return files, body, nil
}

// writeSitemap writes the sitemap index and its children; non-indexable
// builds have none.
func (g *generator) writeSitemap() error {
	if !g.indexable {
		return nil
	}
	files, index, err := buildSitemaps(g.siteURL, g.sitemap, SitemapChunkURLs)
	if err != nil {
		return err
	}
	for _, file := range files {
		if err := g.writeAsset(file.Path, file.Body); err != nil {
			return err
		}
	}
	if err := g.writeAsset("sitemap.xml", index); err != nil {
		return err
	}
	g.hasSitemap = true
	return nil
}

func checkSitemapPartition(route, family, language string, locales map[string]portalLocale) error {
	locale := localeForPortalRoute(route, locales)
	if locale.Code != language {
		return fmt.Errorf("portalcheck: %s is listed in a sitemap for %s", route, language)
	}
	identity := route
	if locale.Code != defaultPortalLocale {
		identity = portalRouteIdentity(route, locale, locales[defaultPortalLocale])
	}
	expected := ""
	switch {
	case identity == "/consultar/" || identity == "/precos/" || identity == "/metodologia/" ||
		identity == "/curiosidades/" || identity == "/dados/" || identity == "/fiscalizacao/":
		expected = "hubs"
	case strings.HasPrefix(identity, "/precos/"):
		expected = "precos"
	case strings.HasPrefix(identity, "/postos/"):
		expected = "postos"
	case strings.HasPrefix(identity, "/forum/"):
		expected = "forum"
	}
	if family != expected {
		return fmt.Errorf("portalcheck: %s is listed in the %s sitemap family", route, family)
	}
	return nil
}
