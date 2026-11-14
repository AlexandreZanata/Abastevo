package portalgen

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"html"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// DefaultMinInboundLinks is the B-BR-SEO-07 target of body links from
// distinct pages into every indexable page.
const DefaultMinInboundLinks = 3

type LinkGraphOptions struct {
	SiteURL    string
	MinInbound int
}

type LinkGraphResult struct {
	Pages       int
	Indexable   int
	SitemapURLs int
	// MinInbound is the smallest inbound body-link count per page family
	// among indexable pages.
	MinInbound map[string]int
	// StationsBelowTarget counts indexable stations held to the min(target,
	// n) city bound because their city has fewer than target indexable
	// stations.
	StationsBelowTarget int
	// Digest binds the sorted link graph; identical builds have identical
	// digests.
	Digest string
}

var (
	anchorTag       = regexp.MustCompile(`(?s)<a\b[^>]*\bhref="([^"]*)"[^>]*>(.*?)</a>`)
	anyHref         = regexp.MustCompile(`\bhref="([^"]*)"`)
	tagPattern      = regexp.MustCompile(`<[^>]+>`)
	cityRoutePath   = regexp.MustCompile(`^/precos/[a-z]{2}/[a-z0-9-]+/$`)
	genericAnchors  = map[string]bool{"clique aqui": true, "aqui": true, "saiba mais": true, "leia mais": true, "veja mais": true, "link": true, "mais": true, "acesse": true}
	mainOpenMarker  = []byte(`<main id="conteudo"`)
	mainCloseMarker = []byte(`</main>`)
)

type graphPage struct {
	route      string
	robots     string
	canonical  string
	contextual map[string]bool
}

// VerifyLinkGraph checks the internal link graph of one generated portal
// (B-BR-SEO-07): every sitemap URL is an indexable, self-canonical page and
// every indexable page is in the sitemap; no root-relative link targets a
// missing route (landing routes are the LandingRoutes allowlist); no body
// anchor is generic; every indexable page has at least MinInbound body links
// from distinct pages, except stations, held to min(MinInbound, n) where n
// is the number of indexable stations linking the same city page.
func VerifyLinkGraph(dir string, opts LinkGraphOptions) (LinkGraphResult, error) {
	minInbound := opts.MinInbound
	if minInbound <= 0 {
		minInbound = DefaultMinInboundLinks
	}
	files, err := walkArtifact(dir)
	if err != nil {
		return LinkGraphResult{}, err
	}
	exists := map[string]bool{}
	for _, rel := range files {
		exists[rel] = true
	}
	pages := map[string]*graphPage{}
	for _, rel := range files {
		if !strings.HasSuffix(rel, "index.html") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			return LinkGraphResult{}, err
		}
		route := "/" + strings.TrimSuffix(rel, "index.html")
		page := &graphPage{route: route, contextual: map[string]bool{}}
		if match := metaRobots.FindSubmatch(body); match != nil {
			page.robots = string(match[1])
		}
		if match := canonicalLink.FindSubmatch(body); match != nil {
			page.canonical = string(match[1])
		}
		for _, match := range anyHref.FindAllSubmatch(body, -1) {
			if target, internal := internalTarget(string(match[1])); internal && !targetExists(target, exists) {
				return LinkGraphResult{}, fmt.Errorf("linkgraph: %s links missing route %s", route, target)
			}
		}
		main := mainContent(body)
		for _, match := range anchorTag.FindAllSubmatch(main, -1) {
			text := strings.ToLower(strings.TrimSpace(html.UnescapeString(string(tagPattern.ReplaceAll(match[2], nil)))))
			if genericAnchors[text] {
				return LinkGraphResult{}, fmt.Errorf("linkgraph: generic anchor %q on %s", text, route)
			}
			if target, internal := internalTarget(string(match[1])); internal && target != route && strings.HasSuffix(target, "/") {
				page.contextual[target] = true
			}
		}
		pages[route] = page
	}
	site := strings.TrimRight(opts.SiteURL, "/")
	result := LinkGraphResult{Pages: len(pages), MinInbound: map[string]int{}}
	indexable := map[string]bool{}
	for route, page := range pages {
		if page.robots != "index,follow" {
			continue
		}
		indexable[route] = true
		if site != "" && page.canonical != site+route {
			return LinkGraphResult{}, fmt.Errorf("linkgraph: %s is not self-canonical", route)
		}
	}
	result.Indexable = len(indexable)
	listed, err := sitemapRoutes(dir, exists)
	if err != nil {
		return LinkGraphResult{}, err
	}
	result.SitemapURLs = len(listed)
	for route, loc := range listed {
		page, ok := pages[route]
		if !ok || !indexable[route] || page.canonical != loc {
			return LinkGraphResult{}, fmt.Errorf("linkgraph: sitemap URL %s is not an indexable self-canonical page", loc)
		}
	}
	for route := range indexable {
		if _, ok := listed[route]; !ok && len(listed) > 0 {
			return LinkGraphResult{}, fmt.Errorf("linkgraph: indexable %s is absent from the sitemap", route)
		}
	}
	inbound := map[string]int{}
	cityStations := map[string]int{}
	stationCity := map[string]string{}
	for route, page := range pages {
		for target := range page.contextual {
			inbound[target]++
		}
		if indexable[route] && strings.HasPrefix(route, "/postos/") {
			for _, target := range sortedKeys(page.contextual) {
				if cityRoutePath.MatchString(target) {
					stationCity[route] = target
					cityStations[target]++
					break
				}
			}
		}
	}
	var violations []string
	for _, route := range sortedKeys(indexable) {
		required, family := minInbound, routeFamily(route)
		if family == "postos" {
			n := cityStations[stationCity[route]]
			if n == 0 {
				n = 1
			}
			if n < minInbound {
				required = n
				result.StationsBelowTarget++
			}
		}
		if inbound[route] < required {
			violations = append(violations, fmt.Sprintf("%s has %d inbound body links, needs %d", route, inbound[route], required))
		}
		if current, ok := result.MinInbound[family]; !ok || inbound[route] < current {
			result.MinInbound[family] = inbound[route]
		}
	}
	if len(violations) > 0 {
		if len(violations) > 5 {
			violations = append(violations[:5], fmt.Sprintf("and %d more", len(violations)-5))
		}
		return LinkGraphResult{}, fmt.Errorf("linkgraph: %s", strings.Join(violations, "; "))
	}
	hash := sha256.New()
	for _, route := range sortedKeys(pages) {
		fmt.Fprintf(hash, "%s %s %t\n", route, pages[route].robots, indexable[route])
		for _, target := range sortedKeys(pages[route].contextual) {
			fmt.Fprintf(hash, "  %s\n", target)
		}
	}
	result.Digest = "sha256:" + hex.EncodeToString(hash.Sum(nil))
	return result, nil
}

func routeFamily(route string) string {
	switch {
	case strings.HasPrefix(route, "/postos/"):
		return "postos"
	case cityRoutePath.MatchString(route):
		return "cidades"
	case strings.HasPrefix(route, "/precos/") && route != "/precos/":
		if len(strings.Trim(strings.TrimPrefix(route, "/precos/"), "/")) == 2 {
			return "estados"
		}
		return "combustiveis"
	default:
		return "hubs"
	}
}

// internalTarget normalizes a root-relative link to its path.
func internalTarget(href string) (string, bool) {
	if !strings.HasPrefix(href, "/") || strings.HasPrefix(href, "//") {
		return "", false
	}
	href = strings.SplitN(href, "#", 2)[0]
	href = strings.SplitN(href, "?", 2)[0]
	return href, href != ""
}

func targetExists(target string, exists map[string]bool) bool {
	if LandingRoutes[target] {
		return true
	}
	rel := strings.TrimPrefix(target, "/")
	if strings.HasSuffix(target, "/") {
		rel += "index.html"
	}
	return exists[rel]
}

func mainContent(body []byte) []byte {
	start := bytes.Index(body, mainOpenMarker)
	if start < 0 {
		return nil
	}
	end := bytes.Index(body[start:], mainCloseMarker)
	if end < 0 {
		return nil
	}
	return body[start : start+end]
}

// sitemapRoutes maps every child-sitemap URL path to its loc.
func sitemapRoutes(dir string, exists map[string]bool) (map[string]string, error) {
	out := map[string]string{}
	if !exists["sitemap.xml"] {
		return out, nil
	}
	raw, err := os.ReadFile(filepath.Join(dir, "sitemap.xml"))
	if err != nil {
		return nil, err
	}
	var index sitemapIndexDoc
	if err := xml.Unmarshal(raw, &index); err != nil {
		return nil, fmt.Errorf("linkgraph: invalid sitemap index")
	}
	for _, ref := range index.Sitemaps {
		parsed, err := url.Parse(ref.Loc)
		if err != nil {
			return nil, fmt.Errorf("linkgraph: invalid sitemap index entry")
		}
		body, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(strings.TrimPrefix(parsed.Path, "/"))))
		if err != nil {
			return nil, fmt.Errorf("linkgraph: child sitemap unavailable")
		}
		var set sitemapSet
		if err := xml.Unmarshal(body, &set); err != nil {
			return nil, fmt.Errorf("linkgraph: invalid child sitemap")
		}
		for _, entry := range set.URLs {
			loc, err := url.Parse(entry.Loc)
			if err != nil {
				return nil, fmt.Errorf("linkgraph: invalid sitemap URL")
			}
			out[loc.Path] = entry.Loc
		}
	}
	return out, nil
}
