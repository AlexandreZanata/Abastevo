package prepared

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
)

func syntheticNationalCatalog() string {
	raw := strings.ReplaceAll(`{"key":"ev-site-v1:DIGEST","kind":"CHARGING_SITE","point":{"latitude_decimal":"-23.5","longitude_decimal":"-46.6","provenance":"source"},"name":null,"confidence":"two-source","field_sources":{"point":{"source_key":"osm:node:7","field":"point"}},"sources":[
{"key":"osm:node:7","kind":"CHARGING_SITE","access":"PERMISSIVE","point":{"latitude_decimal":"-23.5","longitude_decimal":"-46.6","provenance":"source"},"source":{"source":"OSM","external_id":"node:7","revision":"synthetic-osm","record_sha256":"DIGEST","license":"ODbL-1.0","attribution":"© OpenStreetMap contributors (ODbL)"},"groups":[{"key":"osm:node:7:socket:type2","family":"TYPE2","raw_socket":"type2","reported_quantity":2,"current":"UNKNOWN"}]},
{"key":"ocm:poi:9","kind":"CHARGING_SITE","access":"PUBLIC","point":{"latitude_decimal":"-23.5","longitude_decimal":"-46.6","provenance":"source"},"source":{"source":"OCM","external_id":"9","revision":"synthetic-ocm","record_sha256":"DIGEST","provider_id":40,"license":"CC-0","attribution":"Easy Volt"},"groups":[{"key":"ocm:group:9:1","family":"TYPE2","raw_socket":"25","reported_quantity":4,"current":"AC"}]}],"equipment":[]}`, "\n", "") + "\n"
	return strings.ReplaceAll(raw, "DIGEST", strings.Repeat("a", 64))
}

func TestProjectNationalCatalogKeepsSourceGroupsAndUnknownName(t *testing.T) {
	makeBundle := func(raw string) NationalBundle {
		return NationalBundle{Catalog: []byte(raw), Sites: 1, OfferSHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(raw)))}
	}
	raw := syntheticNationalCatalog()
	got, err := ProjectNationalCatalog(makeBundle(raw))
	if err != nil || len(got) != 1 {
		t.Fatalf("projection refused: %v", err)
	}
	site := got[0]
	if site.Name != nil || site.Confidence != "two-source" || site.Access != "PERMISSIVE" || len(site.Groups) != 2 ||
		site.Groups[0].SourceKey != "osm:node:7" || *site.Groups[0].Quantity != 2 ||
		site.Groups[1].SourceKey != "ocm:poi:9" || *site.Groups[1].Quantity != 4 {
		t.Fatalf("source facts changed: %+v", site)
	}
	for _, changed := range []string{
		strings.Replace(raw, `"provider_id":40,`, "", 1),
		strings.Replace(raw, `"ocm:group:9:1"`, `"osm:node:7:socket:type2"`, 1),
		strings.Replace(raw, `"two-source"`, `"single-source"`, 1),
	} {
		if rows, err := ProjectNationalCatalog(makeBundle(changed)); err == nil || rows != nil {
			t.Fatal("malformed catalog returned rows")
		}
	}
}
