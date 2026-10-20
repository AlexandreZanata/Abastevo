package native

import (
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// MaxListingBytes bounds one listing page before any parsing.
const MaxListingBytes = 4 << 20

// ArchiveRef is one verified official PMQC archive link. The period is
// derived from the archive filename in the href, never from page anchor
// text; a mislabeled link cannot invent a period the source does not serve.
type ArchiveRef struct {
	URL   string
	Year  string
	Month string
}

const pmqcListingPrefix = "/anp/pt-br/centrais-de-conteudo/dados-abertos/"

var hrefPattern = regexp.MustCompile(`href\s*=\s*"([^"<>]{1,1000})"`)

// ParseListingPage extracts every verified archive href from the official
// PMQC page. Relative hrefs resolve against the page URL; anything failing
// the archive URL contract (host, prefix, filename shape, query/fragment)
// is ignored as unrelated page furniture. A page that yields no archive is
// refused instead of silently stopping discovery, and duplicate hrefs
// collapse so aliased links cannot double-import.
func ParseListingPage(pageURL string, body []byte) ([]ArchiveRef, error) {
	if len(body) == 0 || len(body) > MaxListingBytes {
		return nil, refused("listing body")
	}
	base, e := url.Parse(pageURL)
	if e != nil || base.Scheme != "https" || base.Host != "www.gov.br" || base.User != nil || base.RawQuery != "" || base.Fragment != "" || !strings.HasPrefix(base.EscapedPath(), pmqcListingPrefix) {
		return nil, refused("listing page identity")
	}
	seen := map[string]bool{}
	refs := make([]ArchiveRef, 0, 16)
	for _, match := range hrefPattern.FindAllSubmatch(body, -1) {
		href := strings.TrimSpace(string(match[1]))
		if href == "" {
			continue
		}
		link, e := url.Parse(href)
		if e != nil {
			continue
		}
		resolved := base.ResolveReference(link)
		raw := resolved.String()
		if ValidateURL(raw) != nil || seen[raw] {
			continue
		}
		year, month, e := archivePeriod(raw)
		if e != nil {
			continue
		}
		seen[raw] = true
		refs = append(refs, ArchiveRef{URL: raw, Year: year, Month: month})
	}
	if len(refs) == 0 {
		return nil, refused("listing archives missing")
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].URL < refs[j].URL })
	return refs, nil
}

// IsAnnual reports whether one reference names a whole-year archive whose
// monthly members must be enumerated by a bounded inspection.
func (r ArchiveRef) IsAnnual() bool { return r.Month == "" }

// Cursor returns the stable scan identity of one reference.
func (r ArchiveRef) Cursor() string { return r.URL }
