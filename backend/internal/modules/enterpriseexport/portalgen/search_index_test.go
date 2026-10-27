package portalgen

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
)

func TestMunicipalityLabelUsesIBGENames(t *testing.T) {
	for _, tc := range []struct{ code, address, want string }{
		{"3550308", "", "São Paulo"},
		{"5107925", "", "Sorriso"},
		{"3549904", "", "São José dos Campos"},
		{"3545803", "", "Santa Bárbara d'Oeste"},
		{"3515103", "", "Embu-Guaçu"},
		{"9999999", "CIDADE TESTE", "Cidade Teste"},
		{"9999999", "", ""},
	} {
		if got := municipalityLabel(tc.code, tc.address); got != tc.want {
			t.Errorf("municipalityLabel(%q, %q) = %q, want %q", tc.code, tc.address, got, tc.want)
		}
	}
}

func TestPrefixKeysSkipStopwordsAndIndexCNPJ(t *testing.T) {
	keys := prefixKeys(enterpriseexport.Station{DisplayName: "AUTO POSTO SHELL DE SÃO JOSÉ LTDA 2", CNPJ: "04218406000104"})
	want := []string{"c-042", "n-jos", "n-sao", "n-she"}
	if strings.Join(keys, ",") != strings.Join(want, ",") {
		t.Fatalf("prefix keys = %v, want %v", keys, want)
	}
}

func readIndexJSON(t *testing.T, out, rel string, target any) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(out, "dados", "busca", "*", filepath.FromSlash(rel)))
	if err != nil || len(matches) != 1 {
		t.Fatalf("search file %s: %v %v", rel, matches, err)
	}
	raw, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		t.Fatal(err)
	}
}

func TestGenerateWritesNationalSearchIndex(t *testing.T) {
	out := filepath.Join(t.TempDir(), "portal")
	if _, err := Generate(Options{SnapshotDir: writeFixture(t), OutputDir: out}); err != nil {
		t.Fatal(err)
	}
	var index searchIndex
	readIndexJSON(t, out, "indice.json", &index)
	if index.Schema != SearchIndexSchema || index.PrefixLength != 3 || len(index.Stopwords) == 0 {
		t.Fatalf("index = %+v", index)
	}
	if len(index.Fuels) != 2 || index.Fuels[0].Code != "GASOLINE_REGULAR" || index.Fuels[1].Code != "ETHANOL" {
		t.Fatalf("fuels must start with regular gasoline: %+v", index.Fuels)
	}
	cities := map[string]searchMunicipality{}
	for _, city := range index.Municipalities {
		cities[city.Code] = city
	}
	if cities["3550308"].Name != "São Paulo" || cities["3550308"].Stations != 5 || cities["3106200"].Name != "Belo Horizonte" {
		t.Fatalf("municipalities = %+v", index.Municipalities)
	}
	var prefix searchPrefixShard
	readIndexJSON(t, out, "p/n-alf.json", &prefix)
	if prefix.Count != 1 || prefix.Stations[0].Name != "Posto Alfa" || prefix.Stations[0].MunicipalityName != "São Paulo" || prefix.Stations[0].Prices != nil {
		t.Fatalf("prefix shard = %+v", prefix)
	}
	var city searchShard
	readIndexJSON(t, out, "m/3550308.json", &city)
	if city.Municipality != "3550308" || city.UF != "SP" || city.Count != 5 {
		t.Fatalf("city shard = %+v", city)
	}
	var prices searchPriceShard
	readIndexJSON(t, out, "precos/GASOLINE_REGULAR.json", &prices)
	if prices.Count != 7 || prices.Rows[0].Milli > prices.Rows[len(prices.Rows)-1].Milli || prices.Unit != "L" {
		t.Fatalf("price shard not sorted cheapest first: %+v", prices)
	}
	consultar := readPortal(t, out, "consultar/index.html")
	if !strings.Contains(consultar, "data-portal-finder") || strings.Contains(consultar, "data-search-uf") {
		t.Fatal("consultar still renders the UF/city selects instead of the smart search")
	}
	if precos := readPortal(t, out, "precos/index.html"); !strings.Contains(precos, "data-portal-prices") || !strings.Contains(precos, `value="GASOLINE_REGULAR"`) {
		t.Fatal("prices page lacks the fuel explorer")
	}
	if _, err := Verify(out, VerifyOptions{}); err != nil {
		t.Fatalf("generated search index fails verification: %v", err)
	}
}

// TestSearchIndexChecksRefuseTampering calls the search-family checks
// directly, so a refusal comes from those rules and not from the manifest
// digest check that runs first in Verify.
func TestSearchIndexChecksRefuseTampering(t *testing.T) {
	cases := map[string]func(t *testing.T, base string, files map[string]FileDigest, release string){
		"untampered": nil,
		"private row key": func(t *testing.T, base string, _ map[string]FileDigest, _ string) {
			path := filepath.Join(base, "p", "n-alf.json")
			raw, _ := os.ReadFile(path)
			if err := os.WriteFile(path, []byte(strings.Replace(string(raw), `"n":"Posto Alfa"`, `"n":"Posto Alfa","cpf":"x"`, 1)), 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"missing city shard": func(t *testing.T, base string, files map[string]FileDigest, release string) {
			delete(files, "dados/busca/"+release+"/m/3106200.json")
		},
		"unexpected national file": func(t *testing.T, base string, files map[string]FileDigest, release string) {
			if err := os.WriteFile(filepath.Join(base, "p", "todos.json"), []byte("{}\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			files["dados/busca/"+release+"/p/todos.json"] = FileDigest{}
		},
		"price row without station id": func(t *testing.T, base string, _ map[string]FileDigest, _ string) {
			path := filepath.Join(base, "precos", "ETHANOL.json")
			raw, _ := os.ReadFile(path)
			if err := os.WriteFile(path, []byte(strings.Replace(string(raw), `"i":"`, `"i":"x`, 1)), 0o600); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, tamper := range cases {
		t.Run(name, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "portal")
			if _, err := Generate(Options{SnapshotDir: writeFixture(t), OutputDir: out}); err != nil {
				t.Fatal(err)
			}
			matches, _ := filepath.Glob(filepath.Join(out, "dados", "busca", "*"))
			if len(matches) != 1 {
				t.Fatalf("release dirs = %v", matches)
			}
			release := filepath.Base(matches[0])
			files := map[string]FileDigest{}
			_ = filepath.Walk(out, func(path string, info os.FileInfo, err error) error {
				if err == nil && !info.IsDir() {
					rel, _ := filepath.Rel(out, path)
					files[filepath.ToSlash(rel)] = FileDigest{}
				}
				return err
			})
			if tamper == nil {
				if err := checkSearchIndexes(out, files, release); err != nil {
					t.Fatalf("untampered index refused: %v", err)
				}
				return
			}
			tamper(t, matches[0], files, release)
			if err := checkSearchIndexes(out, files, release); err == nil {
				t.Fatal("tampered search index accepted")
			}
		})
	}
}
