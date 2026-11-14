package portalgen

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
)

// TestDenseCityAndStatePagesFitTheHTMLBudget builds a São Paulo-like city
// (1,200 sampled stations, all fuels) and a UF with 700 unsampled
// registry cities, then verifies the default per-page gzip budget and the
// link graph still hold.
func TestDenseCityAndStatePagesFitTheHTMLBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("dense portal build")
	}
	codes := []string{}
	for code, entry := range ibgeTable() {
		if entry.U == "SP" && code != "3550308" {
			codes = append(codes, code)
		}
	}
	sort.Strings(codes)
	snapshot := writeFixtureWith(t, func(stations *[]enterpriseexport.Station, prices *[]enterpriseexport.Price) {
		fuels := []string{"GASOLINE_REGULAR", "GASOLINE_ADDITIVED", "ETHANOL", "DIESEL_S10", "DIESEL_S500", "CNG"}
		for i := 0; i < 1200; i++ {
			id := fmt.Sprintf("00000000-0000-4000-9000-%012d", i)
			*stations = append(*stations, enterpriseexport.Station{
				StationID: id, DisplayName: fmt.Sprintf("AUTO POSTO PAULISTANO NÚMERO %04d LTDA", i), CNPJ: "",
				State: "SP", MunicipalityCode: "3550308", OperationStatus: "active", LocationQuality: "unknown",
				Address: map[string]string{"street": fmt.Sprintf("AVENIDA DOS BANDEIRANTES QUADRA %d", i), "number": "1000", "district": "VILA OLÍMPIA", "uf": "SP"},
			})
			for j, fuel := range fuels {
				unit, _ := productUnit(fuel)
				*prices = append(*prices, enterpriseexport.Price{
					PriceID: fmt.Sprintf("dense-%d-%d", i, j), RevisionID: r2, StationID: id, FuelProduct: fuel, Unit: unit,
					AmountMilliBRL: int64(5000 + (i*7+j*13)%2000), Currency: "BRL", CollectedOn: "2026-10-15",
				})
			}
		}
		for i, code := range codes[:700] {
			*stations = append(*stations, enterpriseexport.Station{
				StationID: fmt.Sprintf("00000000-0000-4000-a000-%012d", i), DisplayName: fmt.Sprintf("POSTO INTERIOR %04d", i),
				State: "SP", MunicipalityCode: code, OperationStatus: "active", LocationQuality: "unknown",
				Address: map[string]string{"street": "RUA PRINCIPAL", "number": "1", "uf": "SP"},
			})
		}
	})
	out := filepath.Join(t.TempDir(), "portal")
	if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: out, Indexable: true, SiteURL: seoSite, LandingURL: seoSite}); err != nil {
		t.Fatal(err)
	}
	verified, err := Verify(out, VerifyOptions{SiteURL: seoSite})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("dense portal: pages=%d largest_html_gzip=%d", verified.Pages, verified.LargestHTMLGzipBytes)
	if verified.LargestHTMLGzipBytes > DefaultMaxHTMLGzip/2 {
		t.Errorf("largest page gzip %d leaves under half of the %d budget", verified.LargestHTMLGzipBytes, DefaultMaxHTMLGzip)
	}
	graph, err := VerifyLinkGraph(out, LinkGraphOptions{SiteURL: seoSite})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(graph.MinInbound)
	t.Logf("dense link graph: indexable=%d min_inbound=%s below_target=%d", graph.Indexable, raw, graph.StationsBelowTarget)
}
