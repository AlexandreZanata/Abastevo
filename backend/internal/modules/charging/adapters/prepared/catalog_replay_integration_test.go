//go:build integration

package prepared

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyRecoveredNationalCatalog(t *testing.T) {
	dir := os.Getenv("ANPFUEL_TEST_NATIONAL_REPLAY")
	if dir == "" {
		t.Skip("explicit owner-local national replay required")
	}
	bundle, err := VerifyNationalBundle(os.DirFS(filepath.Join(dir, "rights")),
		"52713c1cc28af777d53627a50486339ed3a31fa1c548faed0c5e5f278496215c",
		"d3b5d0ab7b0b0b05933decd285d012a052210391c4f46ea10a93d2befb80f44c")
	if err != nil {
		t.Fatal(err)
	}
	places, err := ProjectNationalCatalog(bundle)
	if err != nil {
		t.Fatal(err)
	}
	geo, err := os.ReadFile(filepath.Join(dir, "geography.json"))
	if err != nil {
		t.Fatal(err)
	}
	_, counts, err := BindCatalogGeography(bundle, places, geo,
		"a8f120b421613f7ae7a6f088f80a87cb8eae173588d5a579b785a776bdecfc3d",
		"5491bd548fe381cd388ba81c9edfbf4cf2749891e2fd32c1079ac71f7aba1c10")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "conflated", "generation", "uf-census.json"))
	if err != nil {
		t.Fatal(err)
	}
	var audited struct {
		ByUF map[string]struct {
			Sites int64 `json:"catalog_sites"`
		} `json:"by_uf"`
	}
	if err := json.Unmarshal(raw, &audited); err != nil {
		t.Fatal(err)
	}
	if len(audited.ByUF) != 28 || len(places) != 1658 || counts["UNASSIGNED"] != 11 {
		t.Fatalf("unexpected replay census: buckets=%d sites=%d unassigned=%d", len(audited.ByUF), len(places), counts["UNASSIGNED"])
	}
	for uf, row := range audited.ByUF {
		if counts[uf] != row.Sites {
			t.Errorf("UF %s = %d, audited %d", uf, counts[uf], row.Sites)
		}
	}
}
