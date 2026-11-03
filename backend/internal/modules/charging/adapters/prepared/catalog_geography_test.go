package prepared

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestBindCatalogGeographyBindsEachPlaceAndComputesCensus(t *testing.T) {
	catalog, boundary := strings.Repeat("a", 64), strings.Repeat("b", 64)
	bundle := NationalBundle{Sites: 2, OfferSHA256: catalog}
	places := []NationalPlace{{Identity: CatalogIdentity{Key: "one"}}, {Identity: CatalogIdentity{Key: "two"}}}
	makeArtifact := func(rows []map[string]string) ([]byte, string) {
		raw, err := json.Marshal(map[string]any{
			"format": "abastevo.ev-catalog-geography-v1", "catalog_sha256": catalog,
			"boundary_sha256": boundary, "sites": rows,
		})
		if err != nil {
			t.Fatal(err)
		}
		return raw, fmt.Sprintf("%x", sha256.Sum256(raw))
	}
	first := map[string]string{"key": "one", "uf": "SP", "reason": "ibge_polygon"}
	second := map[string]string{"key": "two", "uf": "UNASSIGNED", "reason": "boundary_ambiguous"}
	raw, digest := makeArtifact([]map[string]string{second, first})
	bound, counts, err := BindCatalogGeography(bundle, places, raw, digest, boundary)
	if err != nil || len(bound) != 2 || bound[0].UF != "SP" || bound[1].UF != "UNASSIGNED" ||
		places[0].UF != "" || !reflect.DeepEqual(counts, map[string]int64{"SP": 1, "UNASSIGNED": 1}) {
		t.Fatalf("geography=%+v census=%+v error=%v", bound, counts, err)
	}
	for _, rows := range [][]map[string]string{
		{first}, {first, first}, {first, {"key": "other", "uf": "SP", "reason": "ibge_polygon"}},
		{first, {"key": "two", "uf": "ZZ", "reason": "ibge_polygon"}},
		{first, {"key": "two", "uf": "SP", "reason": "no_boundary_match"}},
	} {
		bad, pin := makeArtifact(rows)
		if _, _, err := BindCatalogGeography(bundle, places, bad, pin, boundary); err == nil {
			t.Fatalf("bad geography accepted: %+v", rows)
		}
	}
	if _, _, err := BindCatalogGeography(bundle, places, raw, catalog, boundary); err == nil {
		t.Fatal("wrong file hash accepted")
	}
	if _, _, err := BindCatalogGeography(bundle, places, raw, digest, catalog); err == nil {
		t.Fatal("wrong boundary hash accepted")
	}
}
