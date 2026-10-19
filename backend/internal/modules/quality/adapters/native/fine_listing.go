package native

import (
	"context"
	"html"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/prepared"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
)

const FineListingPage = "https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/multas-aplicadas-com-vencimento-a-partir-de-2016"

type FineRef struct{ URL, Year string }

var fineHref = regexp.MustCompile(`(?i)\bhref\s*=\s*(?:"([^"<>]{1,1000})"|'([^'<>]{1,1000})')`)

// ParseFineListingPage returns only published annual CSV hrefs supported by
// the native profile. Grouped periods and spreadsheets remain explicit coverage
// gaps, never guessed annual URLs. The source page labels are not year evidence.
func ParseFineListingPage(page string, body []byte) ([]FineRef, error) {
	if page != FineListingPage || len(body) == 0 || len(body) > MaxListingBytes {
		return nil, refused("fine listing identity or budget")
	}
	base, _ := url.Parse(page)
	seen := map[string]bool{}
	var out []FineRef
	for _, match := range fineHref.FindAllSubmatch(body, -1) {
		href := string(match[1])
		if href == "" {
			href = string(match[2])
		}
		u, e := url.Parse(html.UnescapeString(strings.TrimSpace(href)))
		if e != nil {
			continue
		}
		raw := base.ResolveReference(u).String()
		year, e := FineURLYear(raw)
		if e != nil || seen[raw] {
			continue
		}
		seen[raw] = true
		out = append(out, FineRef{URL: raw, Year: strconv.Itoa(year)})
	}
	if len(out) == 0 {
		return nil, refused("fine annual CSV hrefs unavailable")
	}
	sort.Slice(out, func(i, j int) bool { return out[i].URL < out[j].URL })
	return out, nil
}

// InspectFineFile verifies the private file against its fetch identity before
// discovery freezes that identity into an import job. It parses no private rows.
func InspectFineFile(ctx context.Context, path string, ref FineRef, identity prepared.InputIdentity) (application.InventoryEntry, error) {
	if ValidateFineSelection(ref.URL, ref.Year) != nil {
		return application.InventoryEntry{}, refused("fine inspection selection")
	}
	info, e := os.Lstat(path)
	if e != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return application.InventoryEntry{}, refused("fine inspection file")
	}
	f, e := os.Open(path)
	if e != nil {
		return application.InventoryEntry{}, refused("fine inspection unavailable")
	}
	defer f.Close()
	current, e := f.Stat()
	if e != nil || !os.SameFile(info, current) {
		return application.InventoryEntry{}, refused("fine inspection replacement")
	}
	if e = verify(ctx, f, identity); e != nil {
		return application.InventoryEntry{}, e
	}
	return application.InventoryEntry{URL: ref.URL, Label: ref.Year, SHA256: identity.SHA256, Bytes: uint64(identity.Bytes)}, nil
}
