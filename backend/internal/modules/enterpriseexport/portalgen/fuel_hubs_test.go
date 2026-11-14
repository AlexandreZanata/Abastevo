package portalgen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport/analytics"
)

func TestFuelHubsComeFromSurveyProductsInTheirOwnUnit(t *testing.T) {
	summary := func(fuel, unit string) analytics.Summary {
		return analytics.Summary{Geography: analytics.Geography{Level: analytics.LevelNational, Code: "BR"}, Fuel: fuel, Unit: unit, MinMilliBRL: 1, MaxMilliBRL: 2}
	}
	hubs := planFuelHubs([]analytics.Summary{
		summary("GASOLINE_REGULAR", "L"), summary("CNG", "M3"), summary("CNG", "L"),
		summary("UNKNOWN_FUEL", "L"), {Geography: analytics.Geography{Level: analytics.LevelState, Code: "MT"}, Fuel: "ETHANOL", Unit: "L"},
	})
	if len(hubs) != 2 || hubs["GASOLINE_REGULAR"].Href != "/precos/gasolina-comum/" || hubs["CNG"].Href != "/precos/gnv/" || hubs["CNG"].Unit != "M3" {
		t.Fatalf("hubs = %+v", hubs)
	}
	for code, slug := range map[string]string{
		"GASOLINE_REGULAR": "gasolina-comum", "GASOLINE_ADDITIVED": "gasolina-aditivada", "ETHANOL": "etanol",
		"DIESEL_S10": "diesel-s10", "DIESEL_S500": "diesel-s500", "CNG": "gnv", "LPG_P13": "glp",
	} {
		if got := fuelSlug(code); got != slug || len(got) <= 2 {
			t.Errorf("fuelSlug(%s) = %q, want %q (longer than a UF)", code, got, slug)
		}
	}
}

func TestGeneratedFuelHubs(t *testing.T) {
	out := generateSEOPortal(t, true)
	for _, slug := range []string{"gasolina-comum", "etanol", "diesel-s10", "diesel-s500", "gnv"} {
		if _, err := os.Stat(filepath.Join(out, "precos", slug, "index.html")); err != nil {
			t.Errorf("fuel hub %s missing: %v", slug, err)
		}
	}
	for _, absent := range []string{"glp", "gasolina-aditivada"} {
		if _, err := os.Stat(filepath.Join(out, "precos", absent)); !os.IsNotExist(err) {
			t.Errorf("fuel hub %s generated without survey data", absent)
		}
	}
	gnv := readPortal(t, out, "precos/gnv/index.html")
	if !strings.Contains(gnv, "R$ por metro cúbico (m³)") || strings.Contains(gnv, "por litro") {
		t.Error("GNV hub does not keep its own unit")
	}
	gas := readPortal(t, out, "precos/gasolina-comum/index.html")
	for _, want := range []string{
		`<h1>Preço da gasolina comum no Brasil</h1>`, `href="/precos/mt/">Mato Grosso (MT)</a>`, `href="/precos/sp/">São Paulo (SP)</a>`,
		`href="/precos/mt/cuiaba/">Cuiabá (MT)</a>`, `href="/precos/mt/sorriso/">Sorriso (MT)</a>`, "amostra pequena",
		`href="/precos/etanol/"`,
	} {
		if !strings.Contains(gas, want) {
			t.Errorf("gasolina hub missing %s", want)
		}
	}
	if head := parseHead(gas); head.Title != "Preço da gasolina comum no Brasil por estado – ANP 17/10/2026" || head.Robots != "index,follow" {
		t.Errorf("gasolina hub head = %+v", head)
	}
	precos := readPortal(t, out, "precos/index.html")
	city := readPortal(t, out, "precos/mt/cuiaba/index.html")
	for _, page := range []string{precos, city, readPortal(t, out, "precos/mt/index.html")} {
		if !strings.Contains(page, `href="/precos/gasolina-comum/"`) {
			t.Error("price page does not link the gasolina hub")
		}
	}
	if _, err := Verify(out, VerifyOptions{SiteURL: seoSite}); err != nil {
		t.Fatal(err)
	}
}
