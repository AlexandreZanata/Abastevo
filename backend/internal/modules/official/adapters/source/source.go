package source

import (
	"errors"
	"net/url"
	"path"
	"strconv"
	"strings"
)

// Entry kinds. Listing pages are change-probed; downloads carry a filename
// shape so a compromised listing cannot redirect the fetcher to an
// unexpected object on the same host.
const (
	KindListing       = "listing"
	KindStationDetail = "station-detail"
	KindWeeklySummary = "weekly-summary"
	KindHistoricalCSV = "historical-csv"
)

// Entry is one approved origin. Host matches exactly (subdomains never
// inherit approval); PathPrefix is a cleaned absolute prefix; FileGlob, when
// set, restricts the final path segment for downloads; Ports, when set,
// allows those explicit ports (empty means implicit 443 only, so loopback
// test servers declare theirs without weakening production entries).
type Entry struct {
	ID         string
	Host       string
	PathPrefix string
	Kind       string
	FileGlob   string
	Ports      []int
}

// DefaultAllowlist mirrors docs/data-sources.md: the ANP price pages and
// download patterns on www.gov.br. Nothing else is fetchable until an
// explicit entry exists.
func DefaultAllowlist() []Entry {
	lpc := "/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/"
	return []Entry{
		{ID: "anp-weekly-page", Host: "www.gov.br", PathPrefix: lpc, Kind: KindListing},
		{ID: "anp-lpc-detail", Host: "www.gov.br", PathPrefix: lpc + "arquivos-lpc/",
			Kind: KindStationDetail, FileGlob: "revendas_lpc_*.xlsx"},
		{ID: "anp-lpc-summary", Host: "www.gov.br", PathPrefix: lpc + "arquivos-lpc/",
			Kind: KindWeeklySummary, FileGlob: "resumo_semanal_lpc_*.xlsx"},
		{ID: "anp-history-page", Host: "www.gov.br",
			PathPrefix: "/anp/pt-br/centrais-de-conteudo/dados-abertos/",
			Kind:       KindHistoricalCSV},
	}
}

var (
	ErrURLRejected  = errors.New("source: URL not allowlisted")
	ErrScheme       = errors.New("source: only plain https allowed")
	ErrHostMismatch = errors.New("source: host not allowlisted")
)

// Match validates a URL against the allowlist and returns its entry.
// Query strings, fragments, credentials and explicit ports are refused:
// documented ANP patterns carry none, and each is a smuggling surface.
func Match(allow []Entry, rawURL string) (Entry, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return Entry{}, ErrURLRejected
	}
	if u.Scheme != "https" {
		return Entry{}, ErrScheme
	}
	if u.User != nil {
		return Entry{}, ErrURLRejected
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return Entry{}, ErrURLRejected
	}
	cleaned := path.Clean("/" + strings.TrimPrefix(u.EscapedPath(), "/"))
	// File-shaped URLs (final segment carries an extension) only match
	// entries with a filename glob, so an unexpected object on an approved
	// host can never ride on a page entry.
	looksFile := strings.Contains(path.Base(cleaned), ".")
	for _, e := range allow {
		if !strings.EqualFold(u.Hostname(), e.Host) {
			continue
		}
		if port := u.Port(); port != "" {
			listed := false
			for _, ep := range e.Ports {
				if strconv.Itoa(ep) == port {
					listed = true
					break
				}
			}
			if !listed {
				continue
			}
		}
		if !strings.HasPrefix(cleaned+"/", e.PathPrefix) {
			continue
		}
		if looksFile && e.FileGlob == "" {
			continue
		}
		if e.FileGlob != "" {
			base := path.Base(cleaned)
			ok, gerr := path.Match(e.FileGlob, base)
			if gerr != nil || !ok {
				continue
			}
		}
		return e, nil
	}
	return Entry{}, ErrHostMismatch
}
