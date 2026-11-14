package portalgen

import (
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

// stationRecentSurveyWeeks is the D-SEO-02 window: a station page is
// indexable only with an official price in one of the most recent N distinct
// survey weeks of the snapshot.
const stationRecentSurveyWeeks = 8

type pageFamily string

const (
	familyHub           pageFamily = "hub"
	familyFiscalHub     pageFamily = "fiscal-hub"
	familyFiscalPage    pageFamily = "fiscal-page"
	familyFiscalRecord  pageFamily = "fiscal-record"
	familyFuel          pageFamily = "fuel"
	familyState         pageFamily = "state"
	familyCity          pageFamily = "city"
	familyStation       pageFamily = "station"
	familyForumHub      pageFamily = "forum-hub"
	familyForumCategory pageFamily = "forum-category"
	familyForumPost     pageFamily = "forum-post"
	familyNotFound      pageFamily = "not-found"
)

// indexFacts are the data facts the indexation policy reads.
type indexFacts struct {
	Family pageFamily
	// HasCurrentData: state/city/fuel pages have observations in the current
	// survey; the fiscalização hub has the admitted official slice.
	HasCurrentData      bool
	RecentOfficialPrice bool
	CompleteAddress     bool
	// PublishedEditorial: a forum post of the export's editorial projection
	// (published, visible, approved Abastevo source; B-BR-SEO-08).
	PublishedEditorial bool
}

// pageIndexable is the single indexation decision (B-BR-SEO-01): robots meta
// and sitemap inclusion both read it. The global switch is applied on top by
// the generator.
func pageIndexable(facts indexFacts) bool {
	switch facts.Family {
	case familyHub:
		return true
	case familyFiscalHub, familyFuel, familyState, familyCity:
		return facts.HasCurrentData
	case familyStation:
		return facts.RecentOfficialPrice && facts.CompleteAddress
	case familyForumHub, familyForumCategory:
		return facts.HasCurrentData
	case familyForumPost:
		return facts.PublishedEditorial
	default:
		return false
	}
}

// robotsValue renders the robots meta: a build with the global switch off is
// hidden everywhere; gated pages keep their links followable.
func robotsValue(globalIndexable, pageIndexable bool) string {
	switch {
	case !globalIndexable:
		return "noindex,nofollow"
	case pageIndexable:
		return "index,follow"
	default:
		return "noindex,follow"
	}
}

func parseDay(value string) (time.Time, error) {
	return time.Parse("2006-01-02", value)
}

// recentSurveyRevisions returns the revisions whose survey week is among the
// newest weeks distinct survey starts (corrections of one week share it).
func recentSurveyRevisions(revisions map[string]enterpriseexport.Revision, weeks int) map[string]bool {
	starts := map[string]bool{}
	for _, revision := range revisions {
		starts[revision.SurveyStart] = true
	}
	ordered := sortedKeys(starts)
	sort.Sort(sort.Reverse(sort.StringSlice(ordered)))
	if len(ordered) > weeks {
		ordered = ordered[:weeks]
	}
	keep := map[string]bool{}
	for _, start := range ordered {
		keep[start] = true
	}
	out := map[string]bool{}
	for id, revision := range revisions {
		if keep[revision.SurveyStart] {
			out[id] = true
		}
	}
	return out
}

type stationFacts struct {
	RecentOfficialPrice bool
	CompleteAddress     bool
	// LastObserved is the newest valid official collection day, or "".
	LastObserved string
}

func productUnit(fuel string) (string, error) {
	unit, err := kernel.Product(fuel).Unit()
	return string(unit), err
}

// validOfficialPrice reports whether price is a BRL observation in the
// canonical unit, collected inside its own revision's survey week.
func validOfficialPrice(price enterpriseexport.Price, revision enterpriseexport.Revision) bool {
	if price.Currency != "BRL" {
		return false
	}
	unit, err := kernel.Product(price.FuelProduct).Unit()
	if err != nil || price.Unit != string(unit) {
		return false
	}
	collected, err := parseDay(price.CollectedOn)
	if err != nil {
		return false
	}
	start, errStart := parseDay(revision.SurveyStart)
	end, errEnd := parseDay(revision.SurveyEnd)
	return errStart == nil && errEnd == nil && !collected.Before(start) && !collected.After(end)
}

// stationIndexFacts evaluates the D-SEO-02 gate for one station from its own
// prices.
func stationIndexFacts(station enterpriseexport.Station, prices []enterpriseexport.Price, revisions map[string]enterpriseexport.Revision, recent map[string]bool) stationFacts {
	facts := stationFacts{CompleteAddress: completeAddress(station)}
	for _, price := range prices {
		revision, ok := revisions[price.RevisionID]
		if !ok || !validOfficialPrice(price, revision) {
			continue
		}
		if recent[price.RevisionID] {
			facts.RecentOfficialPrice = true
		}
		if price.CollectedOn > facts.LastObserved {
			facts.LastObserved = price.CollectedOn
		}
	}
	return facts
}

// completeAddress requires a street with letters, a municipality name and a
// known UF.
func completeAddress(station enterpriseexport.Station) bool {
	return stationStreet(station) != "" && stationMunicipalityName(station) != "" && knownUF(station.State)
}

func knownUF(uf string) bool {
	return isUF(uf) && ufName(uf) != uf
}

// stationStreet is the public street (with number when present), using the
// same safety rules as addressLine.
func stationStreet(station enterpriseexport.Station) string {
	street := strings.TrimRight(addressComponent(station.Address["street"]), " ,;")
	if !strings.ContainsFunc(street, unicode.IsLetter) || addressMunicipalityCode.MatchString(street) {
		return ""
	}
	if number := addressComponent(station.Address["number"]); number != "" {
		street += ", " + number
	}
	return street
}

func stationMunicipalityName(station enterpriseexport.Station) string {
	municipality := addressComponent(station.Address["municipio"])
	if addressMunicipalityCode.MatchString(municipality) {
		municipality = ""
	}
	return municipalityLabel(station.MunicipalityCode, municipality)
}
