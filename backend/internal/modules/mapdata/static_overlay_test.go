package mapdata

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func overlayRows() []staticOverlayRow {
	return []staticOverlayRow{
		{
			UF: "PR",
			Assertion: StaticOverlayAssertion{
				ID: "b", Source: "OSM", Kind: "FUEL_STATION", Name: "Posto B",
				CNPJ: "01122233300000", Quality: "osm-mapped", Services: []string{"FUEL"},
			},
		},
		{
			UF: "SP",
			Assertion: StaticOverlayAssertion{
				ID: "a", Source: "OSM", Kind: "FUEL_STATION", Name: "Posto A",
				CNPJ: "05938540000134", Quality: "osm-mapped", Services: []string{"FUEL"},
			},
		},
	}
}

func overlayPrices() map[string][]StaticOverlayPrice {
	return map[string][]StaticOverlayPrice{
		"01122233300000": {{
			Source: "ANP", FuelProduct: "GASOLINE_REGULAR", Unit: "L", Condition: "STANDARD",
			AmountMilliBRL: 5890, Currency: "BRL", CollectedOn: "2026-10-18",
			SurveyWeekStart: "2026-10-11", SurveyWeekEnd: "2026-10-17", RevisionID: "rev-1",
		}},
	}
}

func TestAssembleStaticOverlayGroupsByUFAndEmbedsPrices(t *testing.T) {
	priceWeek := &StaticPriceWeek{SurveyStart: "2026-10-11", SurveyEnd: "2026-10-17"}
	files, manifest, result, err := assembleStaticOverlay(
		"gen-1", "gen-id", "source-1", time.Date(2026, 10, 26, 23, 4, 3, 0, time.UTC),
		overlayRows(), overlayPrices(), priceWeek,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || len(manifest.Files) != 2 {
		t.Fatalf("files=%d manifest=%d", len(files), len(manifest.Files))
	}
	if result.Counts.Assertions != 2 || result.Counts.WithPrices != 1 || result.Counts.UFs != 2 {
		t.Fatalf("counts=%+v", result.Counts)
	}
	if manifest.Files["PR.json"].WithPrices != 1 || manifest.Files["SP.json"].WithPrices != 0 {
		t.Fatalf("file counts=%+v", manifest.Files)
	}
	var pr staticOverlayUF
	if err := json.Unmarshal(files["PR.json"], &pr); err != nil {
		t.Fatal(err)
	}
	if len(pr.Assertions) != 1 || pr.Assertions[0].ID != "b" {
		t.Fatalf("pr assertions=%+v", pr.Assertions)
	}
	if len(pr.Assertions[0].Prices) != 1 || pr.Assertions[0].Prices[0].AmountMilliBRL != 5890 {
		t.Fatalf("pr prices=%+v", pr.Assertions[0].Prices)
	}
	if pr.PriceWeek == nil || pr.PriceWeek.SurveyEnd != "2026-10-17" {
		t.Fatalf("price week=%+v", pr.PriceWeek)
	}
	if len(manifest.ContentSHA256) != 64 || !strings.HasPrefix(result.Release, "gen-1-") {
		t.Fatalf("manifest hash/release=%q/%q", manifest.ContentSHA256, result.Release)
	}
	if manifest.Release != result.Release {
		t.Fatalf("release mismatch %q/%q", manifest.Release, result.Release)
	}
}

func TestAssembleStaticOverlayDeterministic(t *testing.T) {
	priceWeek := &StaticPriceWeek{SurveyStart: "2026-10-11", SurveyEnd: "2026-10-17"}
	firstFiles, firstManifest, firstResult, err := assembleStaticOverlay(
		"gen-1", "gen-id", "source-1", time.Date(2026, 10, 26, 23, 4, 3, 0, time.UTC),
		overlayRows(), overlayPrices(), priceWeek,
	)
	if err != nil {
		t.Fatal(err)
	}
	secondFiles, secondManifest, secondResult, err := assembleStaticOverlay(
		"gen-1", "gen-id", "source-1", time.Date(2026, 10, 26, 23, 4, 3, 0, time.UTC),
		overlayRows(), overlayPrices(), priceWeek,
	)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstFiles["PR.json"]) != string(secondFiles["PR.json"]) {
		t.Fatal("per-UF bytes differ")
	}
	if firstManifest.ContentSHA256 != secondManifest.ContentSHA256 || firstResult.Release != secondResult.Release {
		t.Fatal("manifest hash/release differ")
	}
}

func TestAssembleStaticOverlayUnknownState(t *testing.T) {
	rows := []staticOverlayRow{{
		UF: "",
		Assertion: StaticOverlayAssertion{
			ID: "a", Source: "OSM", Kind: "FUEL_STATION", Name: "Posto A",
			Quality: "osm-mapped", Services: []string{"FUEL"},
		},
	}}
	files, manifest, result, err := assembleStaticOverlay(
		"gen-1", "gen-id", "source-1", time.Date(2026, 10, 26, 23, 4, 3, 0, time.UTC),
		rows, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := files["??.json"]; !ok {
		t.Fatal("unknown state file missing")
	}
	if result.Counts.SkippedNoState != 1 {
		t.Fatalf("skipped=%d", result.Counts.SkippedNoState)
	}
	if manifest.PriceWeek != nil {
		t.Fatalf("price week=%+v", manifest.PriceWeek)
	}
}

func TestSanitizeRelease(t *testing.T) {
	if got := sanitizeRelease("gen 1/2"); got != "gen-1-2" {
		t.Fatalf("sanitize=%q", got)
	}
	if got := sanitizeRelease("..."); got != "release" {
		t.Fatalf("empty sanitize=%q", got)
	}
}
