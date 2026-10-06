// P34-T03 bounded synthetic staging catalog spec test.
//
// The manifest pins a small deterministic station set and the
// source/community exercise cases for P35-P37. No national ingestion, no VPS
// seeding and no population claim happen here: seeding runs only with explicit
// write scope through existing mechanisms (ResolveCNPJ/RecordLocation and the
// validated ANP import), cleanup is restricted to owned station UUIDs, and
// live reads stay blocked until TLS trust + data availability resolve.
package apicontract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type p34Station struct {
	Key             string  `json:"key"`
	DisplayName     string  `json:"display_name"`
	CNPJNormalized  string  `json:"cnpj_normalized"`
	Municipality    string  `json:"municipality_code"`
	LocationQuality string  `json:"location_quality"`
	Lat             float64 `json:"lat"`
	Lon             float64 `json:"lon"`
	HasCoordinates  bool    `json:"has_coordinates"`
}

type p34Case struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	GoldenVector string `json:"golden_vector"`
}

type p34Catalog struct {
	ID            string       `json:"id"`
	Version       int          `json:"version"`
	Provenance    string       `json:"provenance"`
	OwnershipMark string       `json:"ownership_mark"`
	SeedMechanism string       `json:"seed_mechanism"`
	CleanupScope  string       `json:"cleanup_scope"`
	NoAdminAPI    bool         `json:"no_admin_api"`
	NoGlobalReset bool         `json:"no_global_reset"`
	NoPhotos      bool         `json:"no_private_photos"`
	Stations      []p34Station `json:"stations"`
	Cases         []p34Case    `json:"cases"`
}

func loadP34Catalog(t *testing.T) p34Catalog {
	t.Helper()
	path := filepath.Join(contractsDir(t), "testdata", "p34", "catalog.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read p34 catalog: %v", err)
	}
	var spec p34Catalog
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatalf("parse p34 catalog: %v", err)
	}
	return spec
}

func TestP34CatalogIsBoundedDeterministic(t *testing.T) {
	spec := loadP34Catalog(t)
	if spec.ID != "p34-staging-catalog" || spec.Version != 1 {
		t.Fatalf("unexpected id/version: %+v", spec)
	}
	if len(spec.Stations) != 3 {
		t.Fatalf("stations = %d, want 3", len(spec.Stations))
	}
	seen := map[string]bool{}
	for _, st := range spec.Stations {
		if !strings.Contains(st.DisplayName, spec.OwnershipMark) {
			t.Errorf("station %s display %q lacks ownership mark %q", st.Key, st.DisplayName, spec.OwnershipMark)
		}
		if st.CNPJNormalized == "" || seen[st.CNPJNormalized] {
			t.Errorf("station %s has empty/duplicate cnpj", st.Key)
		}
		seen[st.CNPJNormalized] = true
		if st.Municipality != "3550308" {
			t.Errorf("station %s municipality = %q, want 3550308", st.Key, st.Municipality)
		}
		switch st.LocationQuality {
		case "reviewed":
			if !st.HasCoordinates {
				t.Errorf("station %s reviewed without coordinates", st.Key)
			}
			if st.Lat < -90 || st.Lat > 90 || st.Lon < -180 || st.Lon > 180 {
				t.Errorf("station %s coordinates out of bounds", st.Key)
			}
		case "unknown":
			if st.HasCoordinates {
				t.Errorf("station %s unknown with coordinates", st.Key)
			}
		default:
			t.Errorf("station %s unknown quality %q", st.Key, st.LocationQuality)
		}
		if strings.Contains(st.DisplayName, "@") || strings.Contains(st.CNPJNormalized, "@") {
			t.Errorf("station %s carries identity data", st.Key)
		}
	}
	if !spec.NoAdminAPI || !spec.NoGlobalReset || !spec.NoPhotos {
		t.Errorf("seeding guards weakened: %+v", spec)
	}
	if spec.SeedMechanism == "" || spec.CleanupScope == "" {
		t.Errorf("seeding/cleanup mechanism undeclared: %+v", spec)
	}
}

func TestP34CatalogCasesMapToGoldenVectors(t *testing.T) {
	spec := loadP34Catalog(t)
	wantKinds := map[string]bool{
		"list": false, "nearby": false, "detail": false, "empty-price": false,
		"community-valid": false, "disputed": false, "stale": false,
		"invalid-lat": false, "unknown-uuid": false,
	}
	for _, c := range spec.Cases {
		if _, ok := wantKinds[c.Kind]; !ok {
			t.Errorf("case %s has unexpected kind %q", c.ID, c.Kind)
			continue
		}
		wantKinds[c.Kind] = true
		if c.GoldenVector != "" {
			if _, err := os.Stat(filepath.Join(contractsDir(t), "testdata", "api", c.GoldenVector)); err != nil {
				t.Errorf("case %s golden vector %s missing: %v", c.ID, c.GoldenVector, err)
			}
		}
	}
	for kind, seen := range wantKinds {
		if !seen {
			t.Errorf("case kind %q not covered", kind)
		}
	}
}
