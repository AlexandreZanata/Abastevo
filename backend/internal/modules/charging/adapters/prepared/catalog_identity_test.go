package prepared

import (
	"reflect"
	"strings"
	"testing"
)

func TestResolveCatalogIdentitiesRetainsHistoryAndRefusesSplit(t *testing.T) {
	legacy := "11111111-1111-4111-8111-111111111111"
	oldOSM := "22222222-2222-4222-8222-222222222222"
	older := "33333333-3333-4333-8333-333333333333"
	key := func(c string) string { return "ev-site-v1:" + strings.Repeat(c, 64) }
	old := map[string]CatalogPublicIdentity{
		"ocm:poi:7":    {ID: legacy, Aliases: []string{older}},
		"osm:node:101": {ID: oldOSM},
	}
	merged := CatalogIdentity{Key: key("a"), SourceKeys: []string{"osm:node:101", "ocm:poi:7"}}
	got, err := ResolveCatalogIdentities([]CatalogIdentity{merged}, old)
	want := CatalogPublicIdentity{ID: legacy, Aliases: []string{oldOSM, older}}
	if err != nil || len(got) != 1 || !reflect.DeepEqual(got[0], want) {
		t.Fatalf("merge=%+v err=%v want=%+v", got, err, want)
	}
	continued := map[string]CatalogPublicIdentity{"ocm:poi:7": want, "osm:node:101": want}
	osmOnly := CatalogIdentity{Key: key("b"), SourceKeys: []string{"osm:node:101"}}
	got, err = ResolveCatalogIdentities([]CatalogIdentity{osmOnly}, continued)
	if err != nil || len(got) != 1 || !reflect.DeepEqual(got[0], want) {
		t.Fatalf("source disappearance lost identity: %+v, %v", got, err)
	}
	ocmOnly := CatalogIdentity{Key: key("c"), SourceKeys: []string{"ocm:poi:7"}}
	if _, err := ResolveCatalogIdentities([]CatalogIdentity{osmOnly, ocmOnly}, continued); err == nil {
		t.Fatal("one previous public identity bound to two sites")
	}
	fresh := CatalogIdentity{Key: key("d"), SourceKeys: []string{"osm:way:901"}}
	first, err := ResolveCatalogIdentities([]CatalogIdentity{fresh}, old)
	if err != nil || len(first) != 1 || first[0].ID == "" || first[0].ID == oldOSM {
		t.Fatalf("fresh identity unavailable: %+v, %v", first, err)
	}
	again, err := ResolveCatalogIdentities([]CatalogIdentity{fresh}, nil)
	if err != nil || !reflect.DeepEqual(first, again) {
		t.Fatalf("fresh identity changed on replay: %+v, %v", again, err)
	}
}

func TestResolveCatalogIdentitiesRefusesDuplicateAndInvalidSources(t *testing.T) {
	a := CatalogIdentity{Key: "ev-site-v1:" + strings.Repeat("a", 64), SourceKeys: []string{"ocm:poi:7"}}
	b := CatalogIdentity{Key: "ev-site-v1:" + strings.Repeat("b", 64), SourceKeys: []string{"ocm:poi:7"}}
	for _, sites := range [][]CatalogIdentity{{a, b}, {{Key: a.Key, SourceKeys: []string{"ocm:poi:07"}}}, {{Key: a.Key}}} {
		if _, err := ResolveCatalogIdentities(sites, nil); err == nil {
			t.Fatalf("invalid identity accepted: %+v", sites)
		}
	}
}
