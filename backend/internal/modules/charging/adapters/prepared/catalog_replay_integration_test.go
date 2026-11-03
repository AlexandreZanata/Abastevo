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
	const rights = "52713c1cc28af777d53627a50486339ed3a31fa1c548faed0c5e5f278496215c"
	const manifest = "d3b5d0ab7b0b0b05933decd285d012a052210391c4f46ea10a93d2befb80f44c"
	const geography = "a8f120b421613f7ae7a6f088f80a87cb8eae173588d5a579b785a776bdecfc3d"
	const boundary = "5491bd548fe381cd388ba81c9edfbf4cf2749891e2fd32c1079ac71f7aba1c10"
	rightsFS := os.DirFS(filepath.Join(dir, "rights"))
	bundle, err := VerifyNationalBundle(rightsFS, rights, manifest)
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
	_, counts, err := BindCatalogGeography(bundle, places, geo, geography, boundary)
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
	inventory, err := VerifyNationalSourceInventory(
		os.DirFS(filepath.Join(dir, "conflated", "generation")),
		os.DirFS(filepath.Join(dir, "normalized", "generation")), manifest)
	if err != nil {
		t.Fatal(err)
	}
	input := NationalImport{Files: rightsFS, RightsSHA256: rights, ManifestSHA256: manifest, Geography: geo,
		GeographySHA256: geography, BoundarySHA256: boundary}
	first, err := CompareNationalCatalogs(nil, input, inventory)
	if err != nil || first.InventoryRecords != 1775 || len(first.Added) != 1658 || len(first.Groups) != 2094 {
		t.Fatalf("first replay: inventory=%d added=%d groups=%d err=%v",
			first.InventoryRecords, len(first.Added), len(first.Groups), err)
	}
	replay, err := CompareNationalCatalogs(&input, input, inventory)
	if err != nil || replay.CandidateSHA256 != first.CandidateSHA256 || replay.PreviousSHA256 != first.CandidateSHA256 ||
		len(replay.Added)+len(replay.Removed)+len(replay.Groups)+len(replay.Moved)+len(replay.Sources)+len(replay.Review) != 0 {
		t.Fatalf("repeat replay failed: err=%v", err)
	}
	t.Logf("verified %d source decisions, %d published sites, %d source groups and an empty repeat diff",
		first.InventoryRecords, len(first.Added), len(first.Groups))

}
