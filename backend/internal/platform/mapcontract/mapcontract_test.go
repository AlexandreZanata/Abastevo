package mapcontract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "contracts", "testdata", "map", name))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func corpusMap(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func assertionsOf(doc map[string]any) []any {
	return doc["assertions"].([]any)
}

func TestValidAssertionCorpus(t *testing.T) {
	corpus, err := ParseAssertions(fixture(t, "poi-assertions-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := len(corpus.Assertions); got != 5 {
		t.Fatalf("assertions %d", got)
	}
	anp := corpus.Assertions[0]
	if anp.Prices[0].AmountMilliBRL != 5890 || anp.Prices[0].Unit != "L" || anp.Prices[0].Product != "GASOLINE_REGULAR" {
		t.Fatalf("ANP price drifted: %+v", anp.Prices[0])
	}
	if anp.Identities.CNPJ == nil || *anp.Identities.CNPJ != "01122233300018" {
		t.Fatalf("CNPJ leading zero lost: %+v", anp.Identities.CNPJ)
	}
	ocm := corpus.Assertions[2]
	if ocm.Charging == nil || ocm.Charging.PowerW != nil || ocm.Charging.TariffMilliBRLPerKWh != nil {
		t.Fatalf("unknown charging facts invented: %+v", ocm.Charging)
	}
	community := corpus.Assertions[1]
	if community.Charging == nil || *community.Charging.TariffMilliBRLPerKWh != 1850 {
		t.Fatalf("community tariff drifted: %+v", community.Charging)
	}
}

func TestAssertionRejections(t *testing.T) {
	base := corpusMap(t, fixture(t, "poi-assertions-v1.json"))
	cases := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"unsupported-version", func(d map[string]any) { d["version"] = 2 }},
		{"duplicate-id", func(d map[string]any) {
			list := assertionsOf(d)
			list[1].(map[string]any)["id"] = list[0].(map[string]any)["id"]
		}},
		{"price-overflow", func(d map[string]any) {
			assertionsOf(d)[0].(map[string]any)["prices"].([]any)[0].(map[string]any)["amount_milli_brl"] = 1000001
		}},
		{"wrong-product-unit", func(d map[string]any) {
			price := assertionsOf(d)[0].(map[string]any)["prices"].([]any)[0].(map[string]any)
			price["product"] = "LPG_P13"
			price["unit"] = "L"
		}},
		{"city-centroid-pin", func(d map[string]any) {
			loc := assertionsOf(d)[3].(map[string]any)["location"].(map[string]any)
			loc["latitude_deg"] = "-15.79"
			loc["longitude_deg"] = "-47.88"
			loc["quality"] = "city-centroid"
			loc["display_point"] = "station"
			assertionsOf(d)[3].(map[string]any)["capabilities"].(map[string]any)["pin_eligible"] = true
		}},
		{"rights-source-mismatch", func(d map[string]any) {
			assertionsOf(d)[0].(map[string]any)["rights"].(map[string]any)["source"] = "COMMUNITY"
		}},
		{"charging-on-anp", func(d map[string]any) {
			community := assertionsOf(d)[1].(map[string]any)["charging"]
			assertionsOf(d)[0].(map[string]any)["charging"] = community
		}},
		{"unknown-field", func(d map[string]any) {
			assertionsOf(d)[0].(map[string]any)["invented"] = true
		}},
		{"missing-station-identity", func(d map[string]any) {
			assertionsOf(d)[0].(map[string]any)["identities"].(map[string]any)["station_uuid"] = nil
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := corpusMap(t, fixture(t, "poi-assertions-v1.json"))
			// Rebuild from base is unnecessary because fixture decode is fresh;
			// mutate the freshly decoded document only.
			tc.mutate(doc)
			raw, err := json.Marshal(doc)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ParseAssertions(raw); err == nil {
				t.Fatal("invalid corpus accepted")
			}
		})
	}
	_ = base
	if _, err := ParseAssertions(fixture(t, "poi-assertions-v1.json")[:100]); err == nil {
		t.Fatal("truncated corpus accepted")
	}
}

func TestValidNativeManifest(t *testing.T) {
	manifest, err := ParseNativeManifest(fixture(t, "native-map-manifest-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Engine.UpstreamSHA != "4587b8ad4d12c9f59982709fdaf03cc925948827" {
		t.Fatalf("engine pin drifted: %s", manifest.Engine.UpstreamSHA)
	}
	if len(manifest.Dependencies) != 2 || manifest.Rights.License != "ODbL-1.0" {
		t.Fatalf("manifest drifted: %+v", manifest)
	}
}

func TestNativeManifestRejections(t *testing.T) {
	base := corpusMap(t, fixture(t, "native-map-manifest-v1.json"))
	cases := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"missing-dependency", func(d map[string]any) { d["dependencies"] = []any{"world", "br-sp"} }},
		{"unsafe-path", func(d map[string]any) {
			d["packages"].([]any)[0].(map[string]any)["path"] = "../br-df.mwm"
		}},
		{"bad-digest", func(d map[string]any) {
			d["packages"].([]any)[0].(map[string]any)["sha256"] = "short"
		}},
		{"rights-denied", func(d map[string]any) {
			d["rights"].(map[string]any)["commercial"] = false
		}},
		{"engine-unpinned", func(d map[string]any) {
			d["engine"].(map[string]any)["upstream_sha"] = "main"
		}},
		{"bbox-inverted", func(d map[string]any) {
			d["region"].(map[string]any)["bbox"].(map[string]any)["min_lon_deg"] = 10
			d["region"].(map[string]any)["bbox"].(map[string]any)["max_lon_deg"] = -10
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := corpusMap(t, fixture(t, "native-map-manifest-v1.json"))
			tc.mutate(doc)
			raw, err := json.Marshal(doc)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ParseNativeManifest(raw); err == nil {
				t.Fatal("invalid manifest accepted")
			}
		})
	}
	_ = base
}

func TestClocksPreserveFutureAndRejectMalformed(t *testing.T) {
	doc := corpusMap(t, fixture(t, "poi-assertions-v1.json"))
	assertionsOf(doc)[0].(map[string]any)["clocks"].(map[string]any)["fetched_at"] = "2099-01-01T00:00:00Z"
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	corpus, err := ParseAssertions(raw)
	if err != nil {
		t.Fatal(err)
	}
	if corpus.Assertions[0].Clocks.FetchedAt != "2099-01-01T00:00:00Z" {
		t.Fatal("future clock not preserved")
	}
	assertionsOf(doc)[0].(map[string]any)["clocks"].(map[string]any)["fetched_at"] = "not-a-time"
	raw, err = json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseAssertions(raw); err == nil {
		t.Fatal("malformed clock accepted")
	}
}

func TestPackagePathVectors(t *testing.T) {
	var vectors struct {
		Valid   []string `json:"valid"`
		Invalid []string `json:"invalid"`
	}
	if err := json.Unmarshal(fixture(t, "package-path-vectors-v1.json"), &vectors); err != nil {
		t.Fatal(err)
	}
	if len(vectors.Valid) == 0 || len(vectors.Invalid) == 0 {
		t.Fatal("empty path vectors")
	}
	for _, path := range vectors.Valid {
		if !ValidPackagePath(path) {
			t.Errorf("valid path refused: %q", path)
		}
	}
	for _, path := range vectors.Invalid {
		if ValidPackagePath(path) {
			t.Errorf("invalid path accepted: %q", path)
		}
	}
}

func TestNativeManifestAcceptsCountryIDWithSpaces(t *testing.T) {
	doc := corpusMap(t, fixture(t, "native-map-manifest-v1.json"))
	doc["packages"].([]any)[0].(map[string]any)["path"] = "2601/Brazil_Mato Grosso.mwm"
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := ParseNativeManifest(raw)
	if err != nil {
		t.Fatalf("spaced country id refused: %v", err)
	}
	if manifest.Packages[0].Path != "2601/Brazil_Mato Grosso.mwm" {
		t.Fatalf("path drifted: %q", manifest.Packages[0].Path)
	}
	for _, bad := range []string{" Brazil.mwm", "Brazil.mwm ", "Brazil_Mato  Grosso.mwm", "2601/ Brazil.mwm"} {
		doc := corpusMap(t, fixture(t, "native-map-manifest-v1.json"))
		doc["packages"].([]any)[0].(map[string]any)["path"] = bad
		raw, err := json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ParseNativeManifest(raw); err == nil {
			t.Fatalf("unsafe spaced path accepted: %q", bad)
		}
	}
}
