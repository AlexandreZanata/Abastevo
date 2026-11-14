package portalgen

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
)

const (
	forumPost1 = "forum/combustiveis/etanol-ou-gasolina-em-cuiaba-a-regra-dos-70-f0000001/index.html"
	forumPost2 = "forum/eletromobilidade/recarga-eletrica-nas-rodovias-paulistas-f0000002/index.html"
	forumPost3 = "forum/combustiveis/diesel-s10-em-varzea-grande-f0000003/index.html"
	forumPost4 = "forum/dados-abastevo/como-lemos-a-pesquisa-da-anp-script-alert-x-script-aspas-f0000004/index.html"
)

func forumRow(id, category, name, title string) enterpriseexport.ForumPost {
	return enterpriseexport.ForumPost{
		PostID: id, Category: category, CategoryName: name, Title: title, Body: "Texto de " + title + ".",
		PublishedAt: "2026-10-10T12:00:00Z", UpdatedAt: "2026-10-10T12:00:00Z", SourceName: "Equipe Abastevo",
	}
}

func planForum(t *testing.T, rows ...enterpriseexport.ForumPost) *forumPlan {
	t.Helper()
	g := &generator{}
	if err := g.prepareForum(rows); err != nil {
		t.Fatal(err)
	}
	return g.forum
}

func TestForumSlugsFollowTheSharedRuleWithACap(t *testing.T) {
	for in, want := range map[string]string{
		"Etanol ou Gasolina? A regra dos 70% em Cuiabá": "etanol-ou-gasolina-a-regra-dos-70-em-cuiaba",
		"Alta Floresta D'Oeste: preços":                 "alta-floresta-doeste-precos",
		"Combustíveis":                                  "combustiveis",
		"Eletromobilidade":                              "eletromobilidade",
		"Legislação de Trânsito":                        "legislacao-de-transito",
		"Veículos e Tecnologia":                         "veiculos-e-tecnologia",
		"Rodovias e Mobilidade":                         "rodovias-e-mobilidade",
		"Economia do Motorista":                         "economia-do-motorista",
		"Dados Abastevo":                                "dados-abastevo",
		"!!! ???":                                       "",
	} {
		if got := forumSlug(in); got != want {
			t.Errorf("forumSlug(%q) = %q, want %q", in, got, want)
		}
	}
	long := strings.Repeat("Preço médio da gasolina ", 5)
	full, capped := slugify(long), forumSlug(long)
	if len(capped) > forumSlugMaxRunes || !strings.HasPrefix(full, capped) || strings.HasSuffix(capped, "-") ||
		full[len(capped)] != '-' {
		t.Errorf("capped slug %q is not a word-boundary prefix of %q", capped, full)
	}
}

func TestForumRoutesAreStableUniqueAndOrderIndependent(t *testing.T) {
	a := forumRow("abcdef12-0000-4000-8000-000000000001", "FUELS", "Combustíveis", "Preço da gasolina")
	plan := planForum(t, a)
	if got := plan.Posts[0].Route; got != "/forum/combustiveis/preco-da-gasolina-abcdef12/" {
		t.Fatalf("route = %q", got)
	}
	edited := a
	edited.Title = "Preço da gasolina sobe em outubro"
	if got := planForum(t, edited).Posts[0].Route; got != "/forum/combustiveis/preco-da-gasolina-sobe-em-outubro-abcdef12/" {
		t.Errorf("edited route = %q: the id suffix must stay, only the title slug moves", got)
	}
	symbols := forumRow("0badc0de-0000-4000-8000-000000000001", "FUELS", "Combustíveis", "?!")
	if got := planForum(t, symbols).Posts[0].Route; got != "/forum/combustiveis/publicacao-0badc0de/" {
		t.Errorf("symbol-only title route = %q", got)
	}
	twin := forumRow("abcdef12-0000-4000-8000-000000000002", "FUELS", "Combustíveis", "Preço da gasolina")
	other := forumRow("12345678-0000-4000-8000-000000000003", "FUELS", "Combustíveis", "Preço da gasolina")
	elsewhere := forumRow("abcdef12-0000-4000-8000-000000000004", "ELECTRIC", "Eletromobilidade", "Preço da gasolina")
	forward := planForum(t, a, twin, other, elsewhere)
	backward := planForum(t, elsewhere, other, twin, a)
	routes := map[string]string{}
	for _, post := range forward.Posts {
		routes[post.Row.PostID] = post.Route
	}
	want := map[string]string{
		a.PostID:         "/forum/combustiveis/preco-da-gasolina-" + a.PostID + "/",
		twin.PostID:      "/forum/combustiveis/preco-da-gasolina-" + twin.PostID + "/",
		other.PostID:     "/forum/combustiveis/preco-da-gasolina-12345678/",
		elsewhere.PostID: "/forum/eletromobilidade/preco-da-gasolina-abcdef12/",
	}
	for id, route := range want {
		if routes[id] != route {
			t.Errorf("route of %s = %q, want %q", id, routes[id], route)
		}
	}
	for _, post := range backward.Posts {
		if routes[post.Row.PostID] != post.Route {
			t.Errorf("route of %s depends on input order", post.Row.PostID)
		}
	}
}

func TestForumHeadsAreUniqueAndBounded(t *testing.T) {
	a := forumRow("aaaaaaaa-0000-4000-8000-000000000001", "FUELS", "Combustíveis", "Preço da gasolina")
	b := forumRow("bbbbbbbb-0000-4000-8000-000000000002", "FUELS", "Combustíveis", "Preço da gasolina")
	c := forumRow("cccccccc-0000-4000-8000-000000000003", "ELECTRIC", "Eletromobilidade", "Preço da gasolina")
	short := forumRow("dddddddd-0000-4000-8000-000000000004", "ELECTRIC", "Eletromobilidade", "Oi")
	short.Body = ""
	long := forumRow("eeeeeeee-0000-4000-8000-000000000005", "ELECTRIC", "Eletromobilidade", "Longo")
	long.Excerpt = strings.Repeat("Um texto editorial longo sobre recarga. ", 10)
	b.Body, c.Body = a.Body, a.Body
	plan := planForum(t, a, b, c, short, long)
	titles, descriptions := map[string]bool{}, map[string]bool{}
	for _, post := range plan.Posts {
		if titles[post.HeadTitle] || descriptions[post.Description] {
			t.Errorf("duplicate head for %s: %q / %q", post.Row.PostID, post.HeadTitle, post.Description)
		}
		titles[post.HeadTitle], descriptions[post.Description] = true, true
		if n := utf8.RuneCountInString(post.Description); n < descriptionMin || n > descriptionMax {
			t.Errorf("%s description has %d runes: %q", post.Row.PostID, n, post.Description)
		}
		if !strings.HasSuffix(post.HeadTitle, " | Fórum abastevo") {
			t.Errorf("title %q lacks the forum suffix", post.HeadTitle)
		}
	}
	if !titles["Preço da gasolina – Eletromobilidade | Fórum abastevo"] {
		t.Errorf("category disambiguation missing: %v", titles)
	}
}

func TestForumFuelProductsMapToFuelHubs(t *testing.T) {
	for wire, product := range forumFuelProducts {
		if fuelSlug(product) == "" {
			t.Errorf("forum fuel %s maps to %s without a hub slug", wire, product)
		}
	}
	if forumFuelProducts["GASOLINE_PREMIUM"] != "GASOLINE_ADDITIVED" {
		t.Error("forum GASOLINE_PREMIUM is the persisted additive legacy")
	}
}

func TestForumPagesOnTheGeneratedPortal(t *testing.T) {
	out := generateSEOPortal(t, true)
	pages := portalPages(t, out)
	forumPages := []string{"forum/index.html", "forum/combustiveis/index.html", "forum/eletromobilidade/index.html",
		"forum/dados-abastevo/index.html", forumPost1, forumPost2, forumPost3, forumPost4}
	for _, path := range forumPages {
		body, ok := pages[path]
		if !ok {
			t.Fatalf("missing forum page %s", path)
		}
		if head := parseHead(body); head.Robots != "index,follow" {
			t.Errorf("%s robots = %q", path, head.Robots)
		}
	}
	post := pages[forumPost1]
	for _, want := range []string{
		"<h1>Etanol ou gasolina em Cuiabá? A regra dos 70%</h1>",
		`Por <a href="/sobre/">Equipe abastevo</a> · Publicado em <time datetime="2026-10-20T13:00:00Z">20/10/2026</time> · Atualizado em <time datetime="2026-10-22T09:30:00Z">22/10/2026</time>`,
		`<a class="tag" href="/precos/mt/">Preço dos combustíveis em Mato Grosso</a>`,
		`<a class="tag" href="/precos/mt/cuiaba/">Preços em Cuiabá (MT)</a>`,
		`<a class="tag" href="/precos/etanol/">Preço do etanol hidratado por estado</a>`,
		`<a class="tag" href="/postos/` + seoID(1) + `/">Preços do posto Auto Posto Cuiabá Ltda</a>`,
		"<p>O etanol compensa quando custa até 70% da gasolina.</p>",
		"<h2>Como fazer a conta</h2>",
		"<ul><li>Divida o preço do etanol pelo da gasolina.</li><li>Até 0,70: etanol.</li></ul>",
		"<p>Veja o preço de cada posto<br>na página da cidade.</p>",
		"<h2>Outras publicações</h2>", `href="/` + strings.TrimSuffix(forumPost3, "index.html") + `">Diesel S10 em Várzea Grande</a>`,
		"<h2>Categorias do fórum</h2>", `href="/forum/eletromobilidade/">Publicações sobre Eletromobilidade</a>`,
		`<a href="/#aplicativo">Acompanhe o fórum e os preços no aplicativo abastevo</a>`,
		`<meta property="og:type" content="article">`, `<meta property="article:modified_time" content="2026-10-22T09:30:00Z">`,
	} {
		if !strings.Contains(post, want) {
			t.Errorf("post page lacks %s", want)
		}
	}
	statewide := pages[forumPost2]
	if strings.Contains(statewide, "Atualizado em") || !strings.Contains(statewide, `<a class="tag" href="/precos/sp/">`) ||
		strings.Contains(statewide, `class="tag" href="/precos/sp/sao-paulo/"`) || strings.Contains(statewide, `class="tag" href="/postos/`) {
		t.Error("statewide post shows an update or links data it is not about")
	}
	if diesel := pages[forumPost3]; !strings.Contains(diesel, `<a class="tag" href="/precos/diesel-s10/">Preço do diesel S10 por estado</a>`) ||
		!strings.Contains(diesel, `<a class="tag" href="/precos/mt/varzea-grande/">Preços em Várzea Grande (MT)</a>`) {
		t.Error("diesel post lacks its fuel hub or city link")
	}

	graph := pageGraph(t, forumPost1, post)
	posting := graph["DiscussionForumPosting"]
	author, _ := posting["author"].(map[string]any)
	partOf, _ := posting["isPartOf"].(map[string]any)
	canonical := seoSite + "/" + strings.TrimSuffix(forumPost1, "index.html")
	if posting["headline"] != "Etanol ou gasolina em Cuiabá? A regra dos 70%" || posting["url"] != canonical ||
		posting["datePublished"] != "2026-10-20T13:00:00Z" || posting["dateModified"] != "2026-10-22T09:30:00Z" ||
		author["@type"] != "Organization" || author["name"] != "Equipe abastevo" || author["url"] != seoSite+"/sobre/" ||
		partOf["url"] != seoSite+"/forum/" {
		t.Errorf("DiscussionForumPosting = %v", posting)
	}
	text, _ := posting["text"].(string)
	if text == "" || !strings.HasPrefix(text, "O etanol compensa") || !strings.Contains(html.UnescapeString(stripTags(post)), "Até 0,70: etanol.") {
		t.Errorf("posting text is not an excerpt of the visible body: %q", text)
	}
	crumbs := []string{}
	for _, item := range graph["BreadcrumbList"]["itemListElement"].([]any) {
		crumbs = append(crumbs, item.(map[string]any)["name"].(string))
	}
	if strings.Join(crumbs, " › ") != "Início › Fórum › Combustíveis › Etanol ou gasolina em Cuiabá? A regra dos 70%" {
		t.Errorf("post breadcrumb = %v", crumbs)
	}
	for _, path := range []string{"forum/index.html", "forum/combustiveis/index.html"} {
		collection := pageGraph(t, path, pages[path])["CollectionPage"]
		elements := collection["mainEntity"].(map[string]any)["itemListElement"].([]any)
		if len(elements) == 0 {
			t.Fatalf("%s ItemList is empty", path)
		}
		for _, element := range elements {
			href := strings.TrimPrefix(element.(map[string]any)["url"].(string), seoSite)
			if !strings.Contains(pages[path], `href="`+href+`"`) {
				t.Errorf("%s ItemList entry %s is not visible", path, href)
			}
		}
	}

	moduleLinks := func(path string) string {
		body := pages[path]
		start := strings.Index(body, "<h2>No fórum")
		if start < 0 {
			return ""
		}
		return body[start : start+strings.Index(body[start:], "</section>")]
	}
	route := func(path string) string { return `href="/` + strings.TrimSuffix(path, "index.html") + `"` }
	for _, check := range []struct {
		page    string
		want    []string
		notWant []string
	}{
		{"precos/mt/index.html", []string{route(forumPost1), route(forumPost3)}, []string{route(forumPost2)}},
		{"precos/mt/cuiaba/index.html", []string{route(forumPost1)}, []string{route(forumPost3), route(forumPost2)}},
		{"precos/mt/varzea-grande/index.html", []string{route(forumPost3)}, []string{route(forumPost1)}},
		{"precos/sp/sao-paulo/index.html", []string{route(forumPost2)}, nil},
		{"precos/index.html", []string{route(forumPost1), route(forumPost2), route(forumPost3), `href="/forum/combustiveis/"`, `href="/forum/"`}, []string{route(forumPost4)}},
	} {
		module := moduleLinks(check.page)
		for _, want := range check.want {
			if !strings.Contains(module, want) {
				t.Errorf("%s forum module lacks %s", check.page, want)
			}
		}
		for _, notWant := range check.notWant {
			if strings.Contains(module, notWant) {
				t.Errorf("%s forum module links %s", check.page, notWant)
			}
		}
	}
	if moduleLinks("precos/mg/index.html") != "" {
		t.Error("a state without forum posts shows an empty forum module")
	}
	if !strings.Contains(pages["consultar/index.html"], `<a href="/forum/" >Fórum</a>`) && !strings.Contains(pages["consultar/index.html"], `<a href="/forum/">Fórum</a>`) {
		t.Error("portal navigation lacks the forum")
	}
	if !strings.Contains(pages["forum/index.html"], `<a href="/forum/" aria-current="page">Fórum</a>`) {
		t.Error("forum hub is not the current navigation entry")
	}

	sitemap := readPortal(t, out, "sitemaps/portal-forum-1.xml")
	for _, check := range []struct{ route, lastmod string }{
		{"/forum/", "2026-10-22"}, {"/forum/combustiveis/", "2026-10-22"}, {"/forum/eletromobilidade/", "2026-10-18"},
		{"/" + strings.TrimSuffix(forumPost1, "index.html"), "2026-10-22"}, {"/" + strings.TrimSuffix(forumPost4, "index.html"), "2026-10-10"},
	} {
		if !strings.Contains(sitemap, "<loc>"+seoSite+check.route+"</loc>\n    <lastmod>"+check.lastmod+"</lastmod>") {
			t.Errorf("forum sitemap lacks %s with lastmod %s", check.route, check.lastmod)
		}
	}
	if strings.Count(sitemap, "<loc>") != len(forumPages) || !strings.Contains(readPortal(t, out, "sitemap.xml"), "/sitemaps/portal-forum-1.xml") {
		t.Errorf("forum sitemap family = %s", sitemap)
	}
	result, err := VerifyLinkGraph(out, LinkGraphOptions{SiteURL: seoSite})
	if err != nil || result.MinInbound["forum"] < DefaultMinInboundLinks {
		t.Fatalf("forum link graph = %+v, %v", result.MinInbound, err)
	}
}

func stripTags(body string) string {
	return tagPattern.ReplaceAllString(body, " ")
}

func TestForumEscapesHostileText(t *testing.T) {
	out := generateSEOPortal(t, true)
	body := readPortal(t, out, forumPost4)
	for _, forbidden := range []string{"<script>alert", "<img src=x", "<b>negrito", "</script><b>"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("hostile text rendered as markup: %s", forbidden)
		}
	}
	for _, want := range []string{
		"<h1>Como lemos a pesquisa da ANP &lt;script&gt;alert(&#34;x&#34;)&lt;/script&gt; &amp; &#34;aspas&#34;</h1>",
		"<p>Texto com &lt;img src=x onerror=alert(1)&gt; e javascript:alert(1) que fica como texto.</p>",
		"<h2>&lt;/script&gt;&lt;b&gt;negrito?&lt;/b&gt;</h2>",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("escaped text missing: %s", want)
		}
	}
	posting := pageGraph(t, forumPost4, body)["DiscussionForumPosting"]
	if posting["headline"] != `Como lemos a pesquisa da ANP <script>alert("x")</script> & "aspas"` {
		t.Errorf("JSON-LD headline = %q", posting["headline"])
	}
	if _, err := Verify(out, VerifyOptions{SiteURL: seoSite}); err != nil {
		t.Fatal(err)
	}
}

// TestForumPostLeavesTheNextRelease regenerates the fixture without one
// post, as the next release after a hide does: the page is gone (404), the
// sitemap and every module drop it, and no link points at it.
func TestForumPostLeavesTheNextRelease(t *testing.T) {
	snapshot := writeSEOFixtureWithoutForum(t)
	posts := seoForumFixture()
	writeForumSection(t, snapshot, append(posts[:2:2], posts[3]))
	out := filepath.Join(t.TempDir(), "portal")
	if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: out, Indexable: true, SiteURL: seoSite, LandingURL: seoSite}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, forumPost3)); !os.IsNotExist(err) {
		t.Fatal("hidden post still has a page")
	}
	hidden := "/" + strings.TrimSuffix(forumPost3, "index.html")
	for path, body := range portalPages(t, out) {
		if strings.Contains(body, hidden) {
			t.Errorf("%s still links the hidden post", path)
		}
	}
	if strings.Contains(readSitemaps(t, out), hidden) {
		t.Error("hidden post still in the sitemap")
	}
	if _, err := Verify(out, VerifyOptions{SiteURL: seoSite}); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyLinkGraph(out, LinkGraphOptions{SiteURL: seoSite}); err != nil {
		t.Fatal(err)
	}
}

func TestPortalWithoutForumHasNoForumRoutes(t *testing.T) {
	out := filepath.Join(t.TempDir(), "portal")
	if _, err := Generate(Options{SnapshotDir: writeSEOFixtureWithoutForum(t), OutputDir: out, Indexable: true, SiteURL: seoSite, LandingURL: seoSite}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, "forum")); !os.IsNotExist(err) {
		t.Fatal("forum routes without forum posts")
	}
	for path, body := range portalPages(t, out) {
		if strings.Contains(body, `"/forum/`) {
			t.Errorf("%s links the forum without posts", path)
		}
	}
	if _, err := os.Stat(filepath.Join(out, "sitemaps", "portal-forum-1.xml")); !os.IsNotExist(err) {
		t.Error("forum sitemap without posts")
	}
	if _, err := VerifyLinkGraph(out, LinkGraphOptions{SiteURL: seoSite}); err != nil {
		t.Fatal(err)
	}
}

func TestOfficialOnlyBuildHasNoForum(t *testing.T) {
	snapshot := writeSEOFixture(t)
	writeOfficialFixture(t, snapshot)
	out := filepath.Join(t.TempDir(), "portal")
	if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: out, OfficialOnly: true, Indexable: true, SiteURL: seoSite}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, "forum")); !os.IsNotExist(err) {
		t.Fatal("official-only build emitted the forum")
	}
	if _, err := Verify(out, VerifyOptions{SiteURL: seoSite, OfficialOnly: true}); err != nil {
		t.Fatal(err)
	}
}

func TestForumSectionMustMatchTheExportManifest(t *testing.T) {
	cases := map[string]func(t *testing.T, dir string){
		"count mismatch": func(t *testing.T, dir string) {
			writeForumSection(t, dir, seoForumFixture())
			writeJSONL(t, filepath.Join(dir, enterpriseexport.ForumFile), seoForumFixture()[:3])
		},
		"unbound file": func(t *testing.T, dir string) {
			writeJSONL(t, filepath.Join(dir, enterpriseexport.ForumFile), seoForumFixture())
		},
		"missing file": func(t *testing.T, dir string) {
			writeForumSection(t, dir, seoForumFixture())
			if err := os.Remove(filepath.Join(dir, enterpriseexport.ForumFile)); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, tamper := range cases {
		t.Run(name, func(t *testing.T) {
			snapshot := writeSEOFixtureWithoutForum(t)
			tamper(t, snapshot)
			_, err := Generate(Options{SnapshotDir: snapshot, OutputDir: filepath.Join(t.TempDir(), "portal")})
			if err == nil || !strings.Contains(err.Error(), "forum") {
				t.Fatalf("tampered forum section accepted: %v", err)
			}
		})
	}
}

func TestForumInvalidRowsFailClosed(t *testing.T) {
	for name, mutate := range map[string]func(*enterpriseexport.ForumPost){
		"duplicate id":            nil,
		"upper-case id":           func(p *enterpriseexport.ForumPost) { p.PostID = strings.ToUpper(p.PostID) },
		"blank title":             func(p *enterpriseexport.ForumPost) { p.Title = " " },
		"bad clock":               func(p *enterpriseexport.ForumPost) { p.PublishedAt = "ontem" },
		"bad uf":                  func(p *enterpriseexport.ForumPost) { p.RegionUF = "mt" },
		"municipality without uf": func(p *enterpriseexport.ForumPost) { p.RegionUF = "" },
		"bad station":             func(p *enterpriseexport.ForumPost) { p.StationID = "../../etc" },
	} {
		t.Run(name, func(t *testing.T) {
			posts := seoForumFixture()
			if mutate == nil {
				posts[1].PostID = posts[0].PostID
			} else {
				mutate(&posts[0])
			}
			snapshot := writeSEOFixtureWithoutForum(t)
			writeForumSection(t, snapshot, posts)
			if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: filepath.Join(t.TempDir(), "portal")}); err == nil {
				t.Fatal("invalid forum row accepted")
			}
		})
	}
}

// TestForumAtScaleFitsTheBudgets renders 400 long posts in one category:
// the category hub is bounded, every page fits the HTML gzip budget and
// every post keeps at least three inbound body links.
func TestForumAtScaleFitsTheBudgets(t *testing.T) {
	posts := make([]enterpriseexport.ForumPost, 0, 400)
	body := strings.TrimSpace(strings.Repeat("Parágrafo editorial longo sobre preços e consumo de combustível. ", 60))
	if utf8.RuneCountInString(body) > 4000 {
		body = string([]rune(body)[:3990]) + "."
	}
	for i := 0; i < 400; i++ {
		post := forumRow(fmt.Sprintf("%08x-0000-4000-8000-%012d", i+1, i+1), "FUELS", "Combustíveis", fmt.Sprintf("Publicação editorial número %d sobre combustíveis", i+1))
		post.Body = body
		post.PublishedAt = fmt.Sprintf("2026-%02d-%02dT12:00:00Z", 1+i%9, 1+i%28)
		post.UpdatedAt = post.PublishedAt
		posts = append(posts, post)
	}
	snapshot := writeSEOFixtureWithoutForum(t)
	writeForumSection(t, snapshot, posts)
	out := filepath.Join(t.TempDir(), "portal")
	if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: out, Indexable: true, SiteURL: seoSite, LandingURL: seoSite}); err != nil {
		t.Fatal(err)
	}
	category := readPortal(t, out, "forum/combustiveis/index.html")
	if !strings.Contains(category, "Exibindo as 300 publicações mais recentes de 400.") {
		t.Error("category hub is not bounded")
	}
	if _, err := Verify(out, VerifyOptions{SiteURL: seoSite}); err != nil {
		t.Fatal(err)
	}
	result, err := VerifyLinkGraph(out, LinkGraphOptions{SiteURL: seoSite})
	if err != nil || result.MinInbound["forum"] < DefaultMinInboundLinks {
		t.Fatalf("forum link graph at scale = %+v, %v", result.MinInbound, err)
	}
}

func TestForumSinglePostKeepsThreeInboundLinks(t *testing.T) {
	snapshot := writeSEOFixtureWithoutForum(t)
	writeForumSection(t, snapshot, seoForumFixture()[3:])
	out := filepath.Join(t.TempDir(), "portal")
	if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: out, Indexable: true, SiteURL: seoSite, LandingURL: seoSite}); err != nil {
		t.Fatal(err)
	}
	result, err := VerifyLinkGraph(out, LinkGraphOptions{SiteURL: seoSite})
	if err != nil || result.MinInbound["forum"] < DefaultMinInboundLinks {
		t.Fatalf("single-post forum link graph = %+v, %v", result.MinInbound, err)
	}
}
