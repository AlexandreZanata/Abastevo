package portalgen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLinkGraphOfTheGeneratedPortal(t *testing.T) {
	out := generateSEOPortal(t, true)
	if _, err := Verify(out, VerifyOptions{SiteURL: seoSite}); err != nil {
		t.Fatal(err)
	}
	result, err := VerifyLinkGraph(out, LinkGraphOptions{SiteURL: seoSite})
	if err != nil {
		t.Fatal(err)
	}
	if result.SitemapURLs != result.Indexable || result.Indexable < 25 {
		t.Fatalf("link graph = %+v", result)
	}
	for _, family := range []string{"hubs", "combustiveis", "estados", "cidades"} {
		if result.MinInbound[family] < DefaultMinInboundLinks {
			t.Errorf("%s minimum inbound = %d, want >= %d", family, result.MinInbound[family], DefaultMinInboundLinks)
		}
	}
	// Sorriso and Belo Horizonte have one indexable station each and
	// Rondonópolis two: those four are held to the min(3, n) city bound;
	// every other indexable station has at least three inbound body links.
	if result.StationsBelowTarget != 4 || result.MinInbound["postos"] < 1 {
		t.Errorf("station bound = %d below target, min inbound %d", result.StationsBelowTarget, result.MinInbound["postos"])
	}
	again, err := VerifyLinkGraph(generateSEOPortal(t, true), LinkGraphOptions{SiteURL: seoSite})
	if err != nil || again.Digest != result.Digest {
		t.Fatalf("link graph is not deterministic: %v %s vs %s", err, again.Digest, result.Digest)
	}
}

func TestLinkingModulesOnCityAndStationPages(t *testing.T) {
	out := generateSEOPortal(t, true)
	city := readPortal(t, out, "precos/mt/cuiaba/index.html")
	for _, want := range []string{
		"<h2>Cidades próximas</h2>", `href="/precos/mt/varzea-grande/">Preços em Várzea Grande (MT)</a>`,
		"<h2>Outras cidades de Mato Grosso</h2>", `href="/precos/mt/sorriso/">Preços em Sorriso (MT)</a>`,
		`href="/precos/mt/">Preço dos combustíveis em Mato Grosso</a>`,
		"<h2>Postos em Cuiabá</h2>", `href="/postos/` + seoID(4) + `/">Posto de Combustiveis São João</a>`,
		`href="/precos/etanol/">Preço do etanol hidratado por estado</a>`,
		`href="/ferramentas/etanol-ou-gasolina/">Etanol ou gasolina em Cuiabá: calcule qual compensa</a>`,
		`href="/guias/como-ler-precos/"`, `href="/ferramentas/encher-o-tanque/"`,
	} {
		if !strings.Contains(city, want) {
			t.Errorf("Cuiabá page missing %s", want)
		}
	}
	if strings.Contains(city, `href="/postos/`+seoID(6)+`/"`) {
		t.Error("stale, unsampled station listed on the city page")
	}
	nearby := city[strings.Index(city, "<h2>Cidades próximas</h2>"):]
	nearby = nearby[:strings.Index(nearby, "</section>")]
	if strings.Index(nearby, "Várzea Grande") > strings.Index(nearby, "Sorriso") || strings.Contains(nearby, "Rondonópolis") {
		t.Errorf("nearby cities are not distance-ordered data cities: %s", nearby)
	}
	station := readPortal(t, out, "postos/"+seoID(2)+"/index.html")
	for _, want := range []string{
		"<h2>Outros postos em Cuiabá</h2>", `href="/postos/` + seoID(3) + `/"`,
		`href="/precos/mt/cuiaba/">Preços de combustíveis em Cuiabá (MT)</a>`,
		`href="/precos/gasolina-comum/">Preço da gasolina comum por estado</a>`,
	} {
		if !strings.Contains(station, want) {
			t.Errorf("station page missing %s", want)
		}
	}
	if strings.Count(station[strings.Index(station, "<h2>Outros postos em Cuiabá</h2>"):], `href="/postos/`) > siblingStations {
		t.Error("sibling list is not bounded")
	}
	rondon := readPortal(t, out, "precos/mt/rondonopolis/index.html")
	if head := parseHead(rondon); head.Robots != "noindex,follow" || !strings.Contains(rondon, "Sem preço na pesquisa ANP") ||
		!strings.Contains(rondon, `href="/postos/`+seoID(12)+`/"`) || !strings.Contains(rondon, `href="/postos/`+seoID(18)+`/"`) {
		t.Errorf("no-data city page = %+v", head)
	}
	rondonStation := readPortal(t, out, "postos/"+seoID(12)+"/index.html")
	if !strings.Contains(rondonStation, `<a href="/precos/mt/rondonopolis/">Rondonópolis</a>`) || parseHead(rondonStation).Robots != "index,follow" {
		t.Error("recent station in a no-data city lost its breadcrumb or gate")
	}
	state := readPortal(t, out, "precos/mt/index.html")
	if !strings.Contains(state, `href="/precos/mt/rondonopolis/">Postos em Rondonópolis</a>`) || !strings.Contains(state, `href="/precos/sp/">Preço dos combustíveis em São Paulo</a>`) {
		t.Error("state page lacks no-data cities or other states")
	}
	for _, landing := range []string{`href="/sobre/"`, `href="/politica-editorial/"`, `href="/ferramentas/"`, `href="/guias/"`, `href="/privacidade/"`} {
		if !strings.Contains(city, landing) {
			t.Errorf("shared chrome lacks %s", landing)
		}
	}
}

func TestLinkGraphRejectsBrokenGraphs(t *testing.T) {
	rewriteAll := func(t *testing.T, out, old, new string) {
		t.Helper()
		for path, body := range portalPages(t, out) {
			if strings.Contains(body, old) {
				if err := os.WriteFile(filepath.Join(out, path), []byte(strings.ReplaceAll(body, old, new)), 0o600); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	cases := map[string]struct {
		old, new, want string
	}{
		"missing route":  {`href="/precos/etanol/"`, `href="/precos/etanol-velho/"`, "missing route"},
		"orphaned hub":   {`href="/precos/etanol/"`, `href="/precos/"`, "inbound body links"},
		"generic anchor": {`>Preços em Sorriso (MT)</a>`, `>clique aqui</a>`, "generic anchor"},
		"retired route":  {`href="/precos/mt/sorriso/"`, `href="/precos/uf/MT/municipio/5107925/"`, "missing route"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			out := generateSEOPortal(t, true)
			rewriteAll(t, out, tc.old, tc.new)
			if _, err := VerifyLinkGraph(out, LinkGraphOptions{SiteURL: seoSite}); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("broken graph accepted or wrong error: %v", err)
			}
		})
	}
}
