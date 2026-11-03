package prepared

import (
	"crypto/sha256"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestNationalDiffTracksSourceLossAndRefusesFalseRemoval(t *testing.T) {
	before := []NationalPlace{
		diffPlace("old-match", "0", "osm:node:1", "ocm:poi:1"),
		diffPlace("removed", "0", "osm:node:2"),
		diffPlace("unresolved", "0", "ocm:poi:3"),
		diffPlace("split", "0", "osm:node:5", "ocm:poi:5"),
	}
	after := []NationalPlace{
		diffPlace("new-key", "0.0005", "osm:node:1"),
		diffPlace("added", "0", "osm:node:4"),
		diffPlace("split-osm", "0", "osm:node:5"),
		diffPlace("split-ocm", "0", "ocm:poi:5"),
	}
	diff := compareNationalPlaces(before, after, map[string]string{"ocm:poi:3": "QUARANTINED"})
	if !reflect.DeepEqual(diff.Added, []string{"added"}) || !reflect.DeepEqual(diff.Removed, []string{"removed"}) {
		t.Fatalf("added=%v removed=%v", diff.Added, diff.Removed)
	}
	if len(diff.Sources) != 1 || !reflect.DeepEqual(diff.Sources[0].Lost, []string{"ocm:poi:1"}) ||
		diff.Sources[0].Before != "old-match" || diff.Sources[0].After != "new-key" {
		t.Fatalf("source changes=%+v", diff.Sources)
	}
	if len(diff.Moved) != 1 || diff.Moved[0].Before != "old-match" || diff.Moved[0].DistanceM <= 50 {
		t.Fatalf("moves=%+v", diff.Moved)
	}
	if len(diff.Review) != 2 || diff.Review[0].Reason != "identity-split" ||
		diff.Review[1].Reason != "root-still-in-source-inventory" {
		t.Fatalf("review=%+v", diff.Review)
	}
	slices.Reverse(before)
	slices.Reverse(after)
	if reordered := compareNationalPlaces(before, after, map[string]string{"ocm:poi:3": "QUARANTINED"}); !reflect.DeepEqual(diff, reordered) {
		t.Fatalf("report depends on input order: %+v", reordered)
	}
}

func TestNationalDiffComparesGroupFactsAndStrictMovementThreshold(t *testing.T) {
	before := diffPlace("same", "0", "osm:node:1")
	after := diffPlace("same", "0.000449", "osm:node:1")
	oldPower, equalPower, newPower := int64(22000), int64(22000), int64(50000)
	before.Groups = []NationalGroup{
		{SourceKey: "osm:node:1", Key: "changed", PowerW: &oldPower},
		{SourceKey: "osm:node:1", Key: "same", PowerW: &oldPower},
		{SourceKey: "osm:node:1", Key: "removed", Family: "CCS2"},
	}
	after.Groups = []NationalGroup{
		{SourceKey: "osm:node:1", Key: "changed", PowerW: &newPower},
		{SourceKey: "osm:node:1", Key: "same", PowerW: &equalPower},
		{SourceKey: "osm:node:1", Key: "added", Family: "TYPE2"},
	}
	diff := compareNationalPlaces([]NationalPlace{before}, []NationalPlace{after}, nil)
	if len(diff.Moved) != 0 || len(diff.Groups) != 3 {
		t.Fatalf("moves=%v groups=%v", diff.Moved, diff.Groups)
	}
	if diff.Groups[0].Change != "added" || diff.Groups[1].Change != "changed" ||
		diff.Groups[2].Change != "removed" || diff.Groups[1].Source != "osm:node:1" {
		t.Fatalf("groups=%+v", diff.Groups)
	}
}

func TestNationalDiffVerifiesBundlesAndInventory(t *testing.T) {
	raw := []byte(syntheticNationalCatalog())
	files, rights, manifest := nationalFixtureWithCatalog(t, raw)
	boundary := strings.Repeat("c", 64)
	geography := fmt.Appendf(nil, `{"format":"abastevo.ev-catalog-geography-v1","catalog_sha256":"%x","boundary_sha256":"%s","sites":[{"key":"ev-site-v1:%s","uf":"SP","reason":"ibge_polygon"}]}`,
		sha256.Sum256(raw), boundary, strings.Repeat("a", 64))
	input := NationalImport{Files: files, RightsSHA256: rights, ManifestSHA256: manifest, Geography: geography,
		GeographySHA256: fmt.Sprintf("%x", sha256.Sum256(geography)), BoundarySHA256: boundary}
	bundle, err := admitNationalImport(input)
	if err != nil {
		t.Fatal(err)
	}
	inventory := NationalSourceInventory{manifest, ArtifactDescriptor{Bytes: int64(len(raw)), Records: 1,
		SHA256: fmt.Sprintf("%x", sha256.Sum256(raw))}, map[string]string{}}
	for _, source := range bundle.places[0].Sources {
		inventory.states[source.Key] = "ACCEPTED"
	}
	if _, err := CompareNationalCatalogs(nil, input, NationalSourceInventory{}); err == nil {
		t.Fatal("incomplete inventory accepted")
	}
	for key := range inventory.states {
		inventory.states[key] = "QUARANTINED"
		if _, err := CompareNationalCatalogs(nil, input, inventory); err == nil {
			t.Fatal("quarantined projection accepted")
		}
		inventory.states[key] = "ACCEPTED"
		break
	}
	wrongCatalog := inventory
	wrongCatalog.catalog.Bytes++
	if _, err := CompareNationalCatalogs(nil, input, wrongCatalog); err == nil {
		t.Fatal("unrelated inventory catalog accepted")
	}
	first, err := CompareNationalCatalogs(nil, input, inventory)
	if err != nil || len(first.Added) != 1 || first.CandidateSHA256 != bundle.plan.BatchSHA256 {
		t.Fatalf("initial diff=%+v err=%v", first, err)
	}
	same, err := CompareNationalCatalogs(&input, input, inventory)
	if err != nil || same.PreviousSHA256 != same.CandidateSHA256 ||
		len(same.Added)+len(same.Removed)+len(same.Moved)+len(same.Sources)+len(same.Groups)+len(same.Review) != 0 {
		t.Fatalf("repeat diff=%+v err=%v", same, err)
	}
	unverified := input
	unverified.RightsSHA256 = strings.Repeat("d", 64)
	if _, err := CompareNationalCatalogs(&unverified, input, inventory); err == nil {
		t.Fatal("unverified baseline accepted")
	}
}

func diffPlace(key, longitude string, roots ...string) NationalPlace {
	return NationalPlace{Identity: CatalogIdentity{Key: key, SourceKeys: roots}, Latitude: "0", Longitude: longitude}
}
