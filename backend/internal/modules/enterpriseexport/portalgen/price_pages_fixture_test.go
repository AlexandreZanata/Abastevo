package portalgen

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	directoryapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport/analytics"
)

// The price pages fixture has three survey weeks (3 is the current one) and
// three cities, built so every sample rule has a case:
//
//   - Cuiabá (MT), 6 stations. Regular gasoline and ethanol in weeks 2 and 3
//     with the same stations (comparable; gasoline rose R$ 0,100, ethanol did
//     not move). Gasoline in week 1 with 5 of them. Diesel S10 with 4 stations
//     in week 2 and 5 in week 3 (sample changed). GNV with 2 stations and GLP
//     with 1 (below the minimum).
//   - Várzea Grande (MT), 4 stations: below the municipal minimum.
//   - São Paulo (SP), 5 stations, week 3 only: no previous survey.
//
// Sample means in week 3: Cuiabá gasoline 6,250 and ethanol 4,250 (68.0%);
// Mato Grosso 6,230 and 4,230 (67.9%); São Paulo 6,000 and 4,500 (75.0%);
// Brazil 6,153 and 4,320 (70.2%).
const (
	ppCuiaba       = "5103403"
	ppVarzeaGrande = "5108402"
	ppSaoPaulo     = "3550308"
)

func ppID(n int) string { return fmt.Sprintf("00000000-0000-4000-b000-%012d", n) }

type ppStation struct {
	n              int
	name, district string
	city, uf       string
	lat, lon       float64
	prices         map[int]map[string]int64
	street         string
}

func pricePagesFixture() ([]enterpriseexport.Station, []enterpriseexport.Revision, []enterpriseexport.Price) {
	defs := []ppStation{}
	districts := []string{"CENTRO", "JARDIM DAS AMÉRICAS", "", "PORTO", "COXIPÓ", "CPA"}
	for i := 0; i < 6; i++ {
		step := int64(100 * i)
		prices := map[int]map[string]int64{
			2: {"GASOLINE_REGULAR": 5900 + step, "ETHANOL": 4000 + step},
			3: {"GASOLINE_REGULAR": 6000 + step, "ETHANOL": 4000 + step},
		}
		if i < 5 {
			prices[1] = map[string]int64{"GASOLINE_REGULAR": 5850 + step}
			prices[3]["DIESEL_S10"] = 6800 + step
		}
		if i < 4 {
			prices[2]["DIESEL_S10"] = 6750 + step
		}
		if i < 2 {
			prices[3]["CNG"] = 4590 + step
		}
		if i == 0 {
			prices[3]["LPG_P13"] = 130000
		}
		defs = append(defs, ppStation{
			n: 1 + i, name: fmt.Sprintf("POSTO CUIABANO %d", 1+i), district: districts[i], city: ppCuiaba, uf: "MT",
			lat: -15.60 + float64(i)/100, lon: -56.10, prices: prices, street: fmt.Sprintf("AVENIDA DO CPA %d", i),
		})
	}
	for i := 0; i < 4; i++ {
		step := int64(100 * i)
		defs = append(defs, ppStation{
			n: 11 + i, name: fmt.Sprintf("POSTO VARZEA %d", 1+i), district: "CENTRO", city: ppVarzeaGrande, uf: "MT",
			lat: -15.65 + float64(i)/100, lon: -56.13, street: fmt.Sprintf("AVENIDA DA FEB %d", i),
			prices: map[int]map[string]int64{
				2: {"GASOLINE_REGULAR": 6050 + step},
				3: {"GASOLINE_REGULAR": 6050 + step, "ETHANOL": 4050 + step},
			},
		})
	}
	for i := 0; i < 5; i++ {
		step := int64(100 * i)
		defs = append(defs, ppStation{
			n: 21 + i, name: fmt.Sprintf("POSTO PAULISTANO %d", 1+i), district: "BELA VISTA", city: ppSaoPaulo, uf: "SP",
			lat: -23.56 + float64(i)/100, lon: -46.65, street: fmt.Sprintf("AVENIDA PAULISTA %d", i),
			prices: map[int]map[string]int64{3: {"GASOLINE_REGULAR": 5800 + step, "ETHANOL": 4300 + step}},
		})
	}

	revisions := make([]enterpriseexport.Revision, 0, 3)
	for week := 1; week <= 3; week++ {
		start := addDays("2026-09-27", 7*(week-1))
		revisions = append(revisions, enterpriseexport.Revision{
			RevisionID: fmt.Sprintf("00000000-0000-4000-b000-%012d", 900+week), SurveyStart: start, SurveyEnd: addDays(start, 6),
			PublishedAt: addDays(start, 7) + "T12:00:00Z", SourceURL: fmt.Sprintf("https://example.test/anp-pp-%02d.xlsx", week),
			SourceChecksum: fmt.Sprintf("sha256:pp%02d", week), ParserVersion: "anp-v1",
		})
	}
	stations := make([]enterpriseexport.Station, 0, len(defs))
	prices := []enterpriseexport.Price{}
	for _, def := range defs {
		address := map[string]string{"uf": def.uf, "street": def.street, "number": fmt.Sprintf("%d", 100+def.n)}
		if def.district != "" {
			address["district"] = def.district
		}
		stations = append(stations, enterpriseexport.Station{
			StationID: ppID(def.n), DisplayName: def.name, State: def.uf, MunicipalityCode: def.city,
			OperationStatus: "active", LocationQuality: "reviewed", Address: address,
			Coordinates: &directoryapp.LatLon{Lat: def.lat, Lon: def.lon},
		})
		for week := 1; week <= 3; week++ {
			for _, fuel := range sortedKeys(def.prices[week]) {
				unit, _ := productUnit(fuel)
				prices = append(prices, enterpriseexport.Price{
					PriceID: fmt.Sprintf("pp-%02d-%d-%s", def.n, week, fuel), RevisionID: revisions[week-1].RevisionID,
					StationID: ppID(def.n), FuelProduct: fuel, Unit: unit, AmountMilliBRL: def.prices[week][fuel],
					Currency: "BRL", CollectedOn: addDays(revisions[week-1].SurveyStart, 2),
				})
			}
		}
	}
	return stations, revisions, prices
}

// writePricePagesSnapshot writes the fixture as an export snapshot; mutate
// may change the rows first.
func writePricePagesSnapshot(t *testing.T, mutate func(*[]enterpriseexport.Station, *[]enterpriseexport.Price)) string {
	t.Helper()
	dir := t.TempDir()
	stations, revisions, prices := pricePagesFixture()
	if mutate != nil {
		mutate(&stations, &prices)
	}
	writeJSON(t, filepath.Join(dir, "manifest.json"), enterpriseexport.Manifest{
		SchemaVersion: enterpriseexport.SchemaVersion, ContentVersion: enterpriseexport.ContentVersion,
		PublicFieldPolicy: enterpriseexport.PublicFieldPolicy,
		BuiltAt:           "2026-10-24T12:00:00Z", ContentSHA256: "sha256:price-pages-fixture",
	})
	writeJSONL(t, filepath.Join(dir, "stations.jsonl"), stations)
	writeJSONL(t, filepath.Join(dir, "revisions.jsonl"), revisions)
	writeJSONL(t, filepath.Join(dir, "prices.jsonl"), prices)
	projection, err := analytics.Compute(analytics.Input{
		ContentSHA256: "sha256:price-pages-fixture", Stations: stations, Revisions: revisions, Prices: prices,
	}, analytics.Options{})
	if err != nil {
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(dir, "analytics-v1.json"), projection)
	return dir
}

// pricePagesProjection is the analytics projection of the unmodified fixture.
func pricePagesProjection(t *testing.T) analytics.Projection {
	t.Helper()
	stations, revisions, prices := pricePagesFixture()
	projection, err := analytics.Compute(analytics.Input{
		ContentSHA256: "sha256:price-pages-fixture", Stations: stations, Revisions: revisions, Prices: prices,
	}, analytics.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return projection
}

// generatePricePortal builds the fixture indexable, with the landing on the
// portal's own origin, as the production worker does.
func generatePricePortal(t *testing.T, mutate func(*[]enterpriseexport.Station, *[]enterpriseexport.Price)) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "portal")
	if _, err := Generate(Options{
		SnapshotDir: writePricePagesSnapshot(t, mutate), OutputDir: out, Indexable: true, SiteURL: seoSite, LandingURL: seoSite,
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

var (
	htmlRow  = regexp.MustCompile(`(?s)<tr>(.*?)</tr>`)
	htmlCell = regexp.MustCompile(`(?s)<t[dh][^>]*>(.*?)</t[dh]>`)
	htmlTag  = regexp.MustCompile(`<[^>]+>`)
)

// plainText strips tags and collapses whitespace.
func plainText(fragment string) string {
	return strings.Join(strings.Fields(htmlTag.ReplaceAllString(fragment, " ")), " ")
}

// sectionAfter returns the page from the first occurrence of marker to the
// next <h2>, or "" when the marker is absent.
func sectionAfter(body, marker string) string {
	start := strings.Index(body, marker)
	if start < 0 {
		return ""
	}
	rest := body[start+len(marker):]
	if end := strings.Index(rest, "<h2"); end >= 0 {
		rest = rest[:end]
	}
	return rest
}

// tableRow returns the plain cells of the first table row of fragment whose
// first cell equals first, or nil.
func tableRow(fragment, first string) []string {
	for _, row := range htmlRow.FindAllStringSubmatch(fragment, -1) {
		cells := []string{}
		for _, cell := range htmlCell.FindAllStringSubmatch(row[1], -1) {
			cells = append(cells, plainText(cell[1]))
		}
		if len(cells) > 0 && cells[0] == first {
			return cells
		}
	}
	return nil
}
