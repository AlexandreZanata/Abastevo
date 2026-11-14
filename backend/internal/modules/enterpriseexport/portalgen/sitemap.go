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
var sitemapFamilies = []string{"hubs", "precos", "postos"}

var sitemapChildPath = regexp.MustCompile(`^sitemaps/portal-(hubs|precos|postos)-[1-9][0-9]*\.xml$`)

func sitemapFamily(family pageFamily) (string, error) {
	switch family {
	case familyHub, familyFiscalHub:
		return "hubs", nil
	case familyFuel, familyState, familyCity:
		return "precos", nil
	case familyStation:
		return "postos", nil
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

// buildSitemaps splits indexable entries into per-family child sitemaps of
// at most chunk URLs and MaxSitemapBytes each, and returns them with the
// sitemap index body. Entry lastmod values are data dates; an index entry
// carries the newest lastmod of its file.
func buildSitemaps(siteURL string, entries []sitemapEntry, chunk int) ([]sitemapFile, []byte, error) {
	if chunk <= 0 || chunk > MaxSitemapURLs {
		return nil, nil, fmt.Errorf("portalgen: invalid sitemap chunk")
	}
	byFamily := map[string][]sitemapEntry{}
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
		byFamily[family] = append(byFamily[family], entry)
	}
	if len(seen) == 0 {
		return nil, nil, fmt.Errorf("portalgen: empty sitemap")
	}
	index := sitemapIndexDoc{XMLNS: sitemapXMLNS}
	var files []sitemapFile
	for _, family := range sitemapFamilies {
		group := byFamily[family]
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
			path := fmt.Sprintf("sitemaps/portal-%s-%d.xml", family, n)
			files = append(files, sitemapFile{Path: path, Body: body})
			index.Sitemaps = append(index.Sitemaps, sitemapRef{Loc: siteURL + "/" + path, LastMod: newest})
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
