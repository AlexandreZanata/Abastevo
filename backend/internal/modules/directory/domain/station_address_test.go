package domain

import "testing"

func TestStationAddressPrecedenceAndHonestFallback(t *testing.T) {
	candidates := []StationAddress{
		{Source: "municipality", Components: map[string]string{"municipio": "[P25-TEST] City", "uf": "SP"}},
		{Source: "cnefe", Components: map[string]string{"street": "[P25-TEST] CNEFE"}},
		{Source: "osm", Components: map[string]string{"street": "[P25-TEST] OSM"}},
		{Source: "rfb", Components: map[string]string{"street": "[P25-TEST] RFB"}},
		{Source: "anp", Components: map[string]string{"street": "[P25-TEST] ANP"}},
	}
	for end := 1; end <= len(candidates); end++ {
		got := SelectStationAddress(candidates[:end]...)
		if got.Source != candidates[end-1].Source {
			t.Fatal(got)
		}
	}
	got := SelectStationAddress(StationAddress{Source: "municipality", Components: map[string]string{"municipio_ibge": "3550308", "uf": "SP"}}, StationAddress{Source: "invented", Components: map[string]string{"street": "-23, -46"}})
	if got.Source != "" || len(got.Components) != 0 {
		t.Fatal("invented address", got)
	}
	got = SelectStationAddress(candidates[4])
	got.Components["street"] = "changed"
	if candidates[4].Components["street"] != "[P25-TEST] ANP" {
		t.Fatal("source components mutated")
	}
}

func TestStationAddressNeverSubstitutesCodeCoordinatesOrInvalidUF(t *testing.T) {
	for _, components := range []map[string]string{
		{"municipio": "3550308", "uf": "SP"},
		{"latitude": "-23.5", "longitude": "-46.5", "uf": "SP"},
		{"municipio": "[P25-TEST] City", "uf": "S1"},
	} {
		if got := ANPStationAddress(components); got.Source != "" {
			t.Fatal("invalid fallback", got)
		}
	}
}
