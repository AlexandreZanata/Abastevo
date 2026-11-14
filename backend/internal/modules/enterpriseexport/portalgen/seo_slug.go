package portalgen

import (
	"sort"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// slugify implements the URL slug shared with the landing build (D-SEO-01):
// Unicode NFD, drop combining marks, lowercase, remove apostrophes (' and
// U+2019), replace every run outside [a-z0-9] with '-', trim '-'.
func slugify(value string) string {
	var out strings.Builder
	dash := false
	for _, r := range norm.NFD.String(value) {
		if unicode.Is(unicode.Mn, r) || r == '\'' || r == '’' {
			continue
		}
		r = unicode.ToLower(r)
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			if dash && out.Len() > 0 {
				out.WriteByte('-')
			}
			dash = false
			out.WriteRune(r)
			continue
		}
		dash = true
	}
	return out.String()
}

func stateRoute(uf string) string {
	return "/precos/" + strings.ToLower(uf) + "/"
}

// ibgeCitySlug is the slug of the IBGE name for code, or "" when the code is
// not in the embedded IBGE table.
func ibgeCitySlug(code string) string {
	entry, ok := ibgeTable()[code]
	if !ok || entry.N == "" {
		return ""
	}
	return slugify(entry.N)
}

type cityKey struct {
	Code string
	UF   string
	Name string
}

// assignCityRoutes maps each municipality code to /precos/{uf}/{slug}/. The
// IBGE name is the slug source (unique per UF over the full table); a code
// outside the table or a slug that still collides inside the build gets the
// -{code} suffix on every colliding entry, so the result never depends on
// iteration order.
func assignCityRoutes(cities []cityKey) map[string]string {
	slugs := make(map[string]string, len(cities))
	suffixed := map[string]bool{}
	groups := map[string][]string{}
	for _, city := range cities {
		slug := ibgeCitySlug(city.Code)
		if slug == "" {
			slug = slugify(city.Name)
			suffixed[city.Code] = true
		}
		slugs[city.Code] = slug
		if !suffixed[city.Code] {
			key := city.UF + "/" + slug
			groups[key] = append(groups[key], city.Code)
		}
	}
	for _, codes := range groups {
		if len(codes) > 1 {
			for _, code := range codes {
				suffixed[code] = true
			}
		}
	}
	routes := make(map[string]string, len(cities))
	for _, city := range cities {
		slug := slugs[city.Code]
		if suffixed[city.Code] {
			if slug == "" {
				slug = city.Code
			} else {
				slug += "-" + city.Code
			}
		}
		routes[city.Code] = stateRoute(city.UF) + slug + "/"
	}
	return routes
}

// sortedKeys returns the keys of a string-keyed map in ascending order.
func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
