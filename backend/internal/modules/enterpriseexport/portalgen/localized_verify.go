package portalgen

import (
	"bytes"
	"fmt"
	"maps"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

func localeForPortalRoute(route string, locales map[string]portalLocale) portalLocale {
	for _, locale := range locales {
		if locale.Prefix != "" && strings.HasPrefix(route, locale.Prefix+"/") {
			return locale
		}
	}
	return locales[defaultPortalLocale]
}

// portalRouteIdentity resolves a potentially translated route to the
// Portuguese route of the same page. The inverse mapping admits only the
// translated families and keeps fuel, state and municipality identity.
func portalRouteIdentity(route string, locale, source portalLocale) string {
	for _, kind := range []string{"methodology", "data", "insights", "search", "enforcement", "forum"} {
		if route == locale.route(kind, "", "", "") {
			return source.route(kind, "", "", "")
		}
	}
	for _, kind := range []string{"stations", "forum", "enforcementPage", "enforcementRecord"} {
		prefix := locale.route(kind, "", "", "")
		if kind == "enforcementPage" || kind == "enforcementRecord" {
			prefix = strings.TrimSuffix(prefix, "/")
		}
		tail, ok := strings.CutPrefix(route, prefix)
		if !ok || !strings.HasSuffix(tail, "/") {
			continue
		}
		slug := strings.TrimSuffix(tail, "/")
		switch kind {
		case "stations":
			if !isUUID(slug) {
				return ""
			}
		case "forum":
			parts := strings.Split(slug, "/")
			if len(parts) > 2 || !routeSlug.MatchString(parts[0]) || len(parts) == 2 && !routeSlug.MatchString(parts[1]) {
				return ""
			}
		case "enforcementPage":
			page, err := strconv.Atoi(slug)
			if err != nil || page < 2 || strconv.Itoa(page) != slug {
				return ""
			}
		case "enforcementRecord":
			if !fiscalRecordIDShape.MatchString(slug) {
				return ""
			}
		}
		return source.route(kind, slug, "", "")
	}
	prices := locale.route("prices", "", "", "")
	if route == prices {
		return source.route("prices", "", "", "")
	}
	tail, ok := strings.CutPrefix(route, prices)
	if !ok || !strings.HasSuffix(tail, "/") {
		return ""
	}
	parts := strings.Split(strings.TrimSuffix(tail, "/"), "/")
	if len(parts) > 2 {
		return ""
	}
	fuel := ""
	for canonical, translated := range locale.FuelSlugs {
		if parts[0] == translated {
			fuel = canonical
			break
		}
	}
	area := parts[0]
	if fuel != "" {
		if len(parts) == 1 {
			return source.route("prices", fuel, "", "")
		}
		area = parts[1]
	}
	if len(area) != 2 || area != strings.ToLower(area) || !knownUF(strings.ToUpper(area)) {
		return ""
	}
	if fuel != "" || len(parts) == 1 {
		return source.route("prices", fuel, area, "")
	}
	city := parts[1]
	if !routeSlug.MatchString(city) || locale.Code != defaultPortalLocale && !localizedCityRouteAllowed(area, city) {
		return ""
	}
	return source.route("prices", "", area, city)
}

func localizedCityRouteAllowed(area, city string) bool {
	uf := strings.ToUpper(area)
	for code := range localizedCityCodes {
		entry, exists := ibgeTable()[code]
		if !exists || entry.U != uf {
			continue
		}
		slug := ibgeCitySlug(code)
		if slug != "" && (city == slug || city == slug+"-"+code) {
			return true
		}
	}
	return false
}

var (
	headLinkTag  = regexp.MustCompile(`(?i)<link\b[^>]*>`)
	hreflangAttr = regexp.MustCompile(`(?i)\shreflang\s*=`)
	alternateTag = regexp.MustCompile(`^<link rel="alternate" hreflang="([^"]+)" href="([^"]+)">$`)
)

type portalLanguagePage struct {
	route         string
	identity      string
	locale        portalLocale
	indexable     bool
	forumOriginal bool
	canonical     string
	alternates    map[string]string
}

func collectPortalLanguagePage(route, canonical string, indexable bool, body []byte, locales map[string]portalLocale) (portalLanguagePage, error) {
	page := portalLanguagePage{route: route, canonical: canonical, indexable: indexable}
	page.locale = localeForPortalRoute(route, locales)
	page.identity = portalRouteIdentity(route, page.locale, locales[defaultPortalLocale])
	if page.locale.Code != defaultPortalLocale {
		if page.identity == "" {
			return page, fmt.Errorf("portalcheck: unsupported localized route %s", route)
		}
		if !page.locale.Enabled || !page.locale.reviewed() {
			return page, fmt.Errorf("portalcheck: route %s uses an inactive locale", route)
		}
		if strings.HasPrefix(page.identity, "/forum/") && strings.Count(page.identity, "/") == 4 {
			page.forumOriginal = bytes.Contains(body, []byte(`lang="pt-BR" data-forum-original`))
			if page.forumOriginal && page.indexable {
				return page, fmt.Errorf("portalcheck: untranslated forum content is indexable on %s", route)
			}
		}
	}
	head, _, _ := bytes.Cut(body, []byte("</head>"))
	if page.locale.Code != defaultPortalLocale {
		if !bytes.Contains(head, []byte(`<html lang="`+page.locale.HTMLLang+`">`)) {
			return page, fmt.Errorf("portalcheck: wrong HTML language on %s", route)
		}
		if canonical != "" && !bytes.Contains(head, []byte(`<meta property="og:locale" content="`+page.locale.OGLocale+`">`)) {
			return page, fmt.Errorf("portalcheck: wrong Open Graph locale on %s", route)
		}
	}
	if !hreflangAttr.Match(head) {
		return page, nil
	}
	page.alternates = map[string]string{}
	for _, link := range headLinkTag.FindAll(head, -1) {
		if !hreflangAttr.Match(link) {
			continue
		}
		match := alternateTag.FindSubmatch(link)
		if match == nil {
			return page, fmt.Errorf("portalcheck: invalid hreflang on %s", route)
		}
		tag := string(match[1])
		if _, duplicate := page.alternates[tag]; duplicate {
			return page, fmt.Errorf("portalcheck: duplicate hreflang %s on %s", tag, route)
		}
		page.alternates[tag] = string(match[2])
	}
	return page, nil
}

func checkPortalLanguagePages(pages map[string]portalLanguagePage, siteURL string) error {
	groups := map[string]map[string]string{}
	for _, route := range sortedKeys(pages) {
		page := pages[route]
		if page.locale.Code != defaultPortalLocale {
			source, ok := pages[page.identity]
			if !ok || (source.indexable != page.indexable && !(page.forumOriginal && !page.indexable)) {
				return fmt.Errorf("portalcheck: %s must share the Portuguese counterpart's indexation", route)
			}
			if source.canonical != "" || page.canonical != "" || page.indexable {
				origin := siteURL
				if origin == "" && source.canonical != "" {
					origin = strings.TrimSuffix(source.canonical, source.route)
				}
				if err := checkLocalizedCanonical(page, origin); err != nil {
					return err
				}
			}
		}
		if !page.indexable || page.identity == "" {
			if len(page.alternates) > 0 {
				return fmt.Errorf("portalcheck: hreflang on unsupported or noindex route %s", route)
			}
			continue
		}
		group := groups[page.identity]
		if group == nil {
			group = map[string]string{}
			groups[page.identity] = group
		}
		group[page.locale.Hreflang] = page.canonical
		if page.locale.Code == defaultPortalLocale {
			group["x-default"] = page.canonical
		}
	}
	for _, route := range sortedKeys(pages) {
		page := pages[route]
		group := groups[page.identity]
		if !page.indexable || len(group) <= 2 {
			if len(page.alternates) > 0 {
				return fmt.Errorf("portalcheck: unexpected hreflang on %s", route)
			}
			continue
		}
		if err := checkLocalizedCanonical(page, siteURL); err != nil {
			return err
		}
		if !maps.Equal(page.alternates, group) {
			return fmt.Errorf("portalcheck: incomplete or invalid reciprocal hreflang on %s", route)
		}
	}
	return nil
}

func checkLocalizedCanonical(page portalLanguagePage, siteURL string) error {
	origin := strings.TrimRight(siteURL, "/")
	if origin == "" {
		parsed, err := url.Parse(page.canonical)
		if err == nil && parsed.Host != "" && parsed.User == nil {
			origin = parsed.Scheme + "://" + parsed.Host
		}
	}
	if origin == "" || !validOrigin(origin) || page.canonical != origin+page.route {
		return fmt.Errorf("portalcheck: %s is not self-canonical", page.route)
	}
	return nil
}
