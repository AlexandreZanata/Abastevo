package portalgen

import (
	"encoding/json"
	"testing"
)

func TestSlugifyFollowsTheSharedAlgorithm(t *testing.T) {
	for input, want := range map[string]string{
		"ALTA FLORESTA D'OESTE":           "alta-floresta-doeste",
		"SÃO PAULO":                       "sao-paulo",
		"CUIABÁ":                          "cuiaba",
		"MOGI DAS CRUZES":                 "mogi-das-cruzes",
		"Espigão D’Oeste":                 "espigao-doeste",
		"GUAJARÁ-MIRIM":                   "guajara-mirim",
		"  --Sant'Ana  do  Livramento-- ": "santana-do-livramento",
		"Itapajé / Ceará":                 "itapaje-ceara",
		"ÑANDÚ ç Ü":                       "nandu-c-u",
		"":                                "",
		"'’":                              "",
	} {
		if got := slugify(input); got != want {
			t.Errorf("slugify(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestIBGESlugsAreUniquePerState(t *testing.T) {
	raw := map[string]struct {
		N string `json:"n"`
		U string `json:"u"`
	}{}
	if err := json.Unmarshal(ibgeMunicipalitiesRaw, &raw); err != nil {
		t.Fatal(err)
	}
	if len(raw) < 5000 {
		t.Fatalf("IBGE asset has %d municipalities, want the full table", len(raw))
	}
	seen := map[string]string{}
	for code, entry := range raw {
		slug := slugify(entry.N)
		if slug == "" || !isUF(entry.U) {
			t.Fatalf("IBGE %s (%q/%q) has no usable slug or UF", code, entry.N, entry.U)
		}
		key := entry.U + "/" + slug
		if other, dup := seen[key]; dup {
			t.Errorf("IBGE slug collision in %s: %s and %s -> %s", entry.U, other, code, slug)
		}
		seen[key] = code
		if got := ibgeCitySlug(code); got != slug {
			t.Errorf("ibgeCitySlug(%s) = %q, want %q", code, got, slug)
		}
	}
}

func TestGeoRoutes(t *testing.T) {
	if got := stateRoute("MT"); got != "/precos/mt/" {
		t.Errorf("stateRoute = %q", got)
	}
	routes := assignCityRoutes([]cityKey{
		{Code: "5103403", UF: "MT", Name: "Cuiabá"},
		{Code: "1100015", UF: "RO", Name: "Alta Floresta d'Oeste"},
		{Code: "9999998", UF: "MT", Name: "Vila Nova"},
		{Code: "9999999", UF: "MT", Name: "Vila Nova"},
		{Code: "9999997", UF: "MT", Name: "Cuiabá"},
	})
	for code, want := range map[string]string{
		"5103403": "/precos/mt/cuiaba/",
		"1100015": "/precos/ro/alta-floresta-doeste/",
		"9999998": "/precos/mt/vila-nova-9999998/",
		"9999999": "/precos/mt/vila-nova-9999999/",
		"9999997": "/precos/mt/cuiaba-9999997/",
	} {
		if routes[code] != want {
			t.Errorf("city route %s = %q, want %q", code, routes[code], want)
		}
	}
}
