package http

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/charging/adapters/read"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/charging/domain"
)

func TestPlaceSourceAttributionIsAdditiveAndCacheBound(t *testing.T) {
	place := read.Place{ID: "synthetic", Name: "Synthetic Charge"}
	legacy := placeFacts(place)
	if _, present := legacy["sources"]; present {
		t.Fatal("legacy place unexpectedly acquired source terms")
	}
	groups := []map[string]any{}
	legacyETag := placeDetailETag(place, groups)
	provider := int64(40)
	place.Sources = []domain.SourceAttribution{
		{Source: "OSM", ExternalID: "node:101", Revision: "synthetic-osm", License: "ODbL-1.0", Attribution: "© OpenStreetMap contributors (ODbL)"},
		{Source: "OCM", ExternalID: "101", Revision: "synthetic-ocm", ProviderID: &provider, License: "CC-0", Attribution: "Easy Volt"},
	}
	wire, err := json.Marshal(placeFacts(place))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"sources":[`, `"provider_id":40`, `"license":"CC-0"`, `"attribution":"Easy Volt"`, `"license":"ODbL-1.0"`} {
		if !strings.Contains(string(wire), want) {
			t.Errorf("missing %s in %s", want, wire)
		}
	}
	if strings.Count(string(wire), `"provider_id"`) != 1 {
		t.Errorf("provider assigned to the wrong source: %s", wire)
	}
	tag := placeDetailETag(place, groups)
	place.Sources[1].Attribution = "Changed synthetic attribution"
	if tag == legacyETag || tag == placeDetailETag(place, groups) {
		t.Fatal("attribution was not bound to the detail cache tag")
	}
}
