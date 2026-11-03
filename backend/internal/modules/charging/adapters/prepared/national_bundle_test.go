package prepared

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"
)

func nationalFixture(t *testing.T) (fstest.MapFS, string, string) {
	return nationalFixtureWithCatalog(t, []byte("{\"key\":\"synthetic\"}\n"))
}

func nationalFixtureWithCatalog(t *testing.T, data []byte) (fstest.MapFS, string, string) {
	t.Helper()
	digest := func(raw []byte) string {
		hash := sha256.Sum256(raw)
		return hex.EncodeToString(hash[:])
	}
	dataDescriptor := map[string]any{"path": "derived-database.ndjson", "bytes": len(data), "sha256": digest(data), "records": 1}
	notice, err := json.Marshal(map[string]any{
		"format": "abastevo.ev-derived-database-offer-v1", "license": "ODbL-1.0", "data": dataDescriptor,
	})
	if err != nil {
		t.Fatal(err)
	}
	manifest := strings.Repeat("a", 64)
	receipt, err := json.Marshal(map[string]any{
		"format": "abastevo.ev-generation-rights-v1", "catalog_sha256": digest(data),
		"catalog_manifest_sha256": manifest, "catalog_sites": 1, "requires_derived_database_offer": true,
		"offer": map[string]any{
			"data":   dataDescriptor,
			"notice": map[string]any{"path": "derived-database-offer.json", "bytes": len(notice), "sha256": digest(notice)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return fstest.MapFS{
		"rights.json":                 {Data: receipt},
		"derived-database.ndjson":     {Data: data},
		"derived-database-offer.json": {Data: notice},
	}, digest(receipt), manifest
}

func TestVerifyNationalBundle(t *testing.T) {
	files, rights, manifest := nationalFixture(t)
	got, err := VerifyNationalBundle(files, rights, manifest)
	if err != nil || got.RightsSHA256 != rights || got.Sites != 1 ||
		string(got.Catalog) != string(files["derived-database.ndjson"].Data) {
		t.Fatalf("bundle = %+v, err = %v", got, err)
	}
	for _, name := range []string{"rights.json", "derived-database.ndjson", "derived-database-offer.json"} {
		t.Run(name, func(t *testing.T) {
			files, rights, manifest := nationalFixture(t)
			delete(files, name)
			if _, err := VerifyNationalBundle(files, rights, manifest); err == nil {
				t.Fatal("missing artifact admitted")
			}
			files[name] = &fstest.MapFile{Data: []byte("tampered")}
			if _, err := VerifyNationalBundle(files, rights, manifest); err == nil {
				t.Fatal("modified artifact admitted")
			}
		})
	}
	if _, err := VerifyNationalBundle(files, rights, strings.Repeat("0", 64)); err == nil {
		t.Fatal("wrong catalog manifest admitted")
	}
	files["review.json"] = &fstest.MapFile{Data: []byte("private fixture")}
	if _, err := VerifyNationalBundle(files, rights, manifest); err == nil {
		t.Fatal("extra artifact admitted")
	}
}

func TestVerifyNationalBundleRejectsOversizedDescriptor(t *testing.T) {
	files, _, manifest := nationalFixture(t)
	var receipt map[string]any
	if err := json.Unmarshal(files[rightsFile].Data, &receipt); err != nil {
		t.Fatal(err)
	}
	receipt["offer"].(map[string]any)["data"].(map[string]any)["bytes"] = offerLimit + 1
	changed, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	files[rightsFile].Data = changed
	hash := sha256.Sum256(changed)
	if _, err := VerifyNationalBundle(files, hex.EncodeToString(hash[:]), manifest); err == nil {
		t.Fatal("oversized offer admitted")
	}
}
