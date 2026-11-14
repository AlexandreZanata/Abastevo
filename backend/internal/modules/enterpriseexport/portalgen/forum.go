package portalgen

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

// Forum page bounds (B-BR-SEO-08).
const (
	forumHubLatest        = 30
	forumCategoryListCap  = 300
	forumSameCategory     = 3
	forumSameRegion       = 2
	forumGlobalRing       = 2
	forumRegionModuleSize = 3
	forumPrecosLatest     = 3
	forumSlugMaxRunes     = 80
	forumTextExcerpt      = 500
	forumEditorialLabel   = "Equipe abastevo"
	forumBrand            = "Fórum abastevo"
)

// forumFuelProducts maps the forum fuel wire enum to the survey product
// codes of the fuel hubs; GASOLINE_PREMIUM is the persisted additive legacy
// (Android FuelProduct.GASOLINE_PREMIUM ↔ GASOLINE_ADDITIVED).
var forumFuelProducts = map[string]string{
	"GASOLINE_REGULAR": "GASOLINE_REGULAR",
	"GASOLINE_PREMIUM": "GASOLINE_ADDITIVED",
	"ETHANOL":          "ETHANOL",
	"DIESEL_S10":       "DIESEL_S10",
	"DIESEL_S500":      "DIESEL_S500",
	"GLP":              "LPG_P13",
	"GNV":              "CNG",
}

// forumFillers pad a short forum description up to descriptionMin; the
// last one is short enough to fit after any 119-character text.
var forumFillers = []string{
	"Publicação da Equipe abastevo no fórum abastevo.",
	"Leia o texto completo, a data e os dados relacionados.",
	"Conteúdo editorial sem afiliação à ANP.",
	"Veja também outras publicações do fórum.",
}

type forumCategory struct {
	Code, Name, Description, Slug, Route string
	Posts                                []*forumPost
	LastMod                              string
}

type forumPost struct {
	Row                      enterpriseexport.ForumPost
	ID8                      string
	Route                    string
	Category                 *forumCategory
	Published, Updated       time.Time
	PublishedDay, UpdatedDay string
	LastMod                  string
	Blocks                   []forumBlock
	HeadTitle, Description   string
	Author                   string
}

// forumPlan is the whole web forum of one build, prepared before any page
// is written so price pages can link posts and posts can link each other.
type forumPlan struct {
	Posts      []*forumPost // newest first
	Categories []*forumCategory
	LastMod    string
	position   map[*forumPost]int
}

func forumZone() *time.Location {
	if loc, ok := kernel.LoadTimeZone("America/Sao_Paulo"); ok {
		return loc
	}
	return time.UTC
}

// forumAuthor is the visible editorial attribution: D-SEO-09 writes the
// Abastevo team as "Equipe abastevo"; another source keeps its own name.
func forumAuthor(sourceName string) string {
	name := strings.TrimSpace(sourceName)
	if name == "" || strings.EqualFold(name, forumEditorialLabel) {
		return forumEditorialLabel
	}
	return name
}

// forumSlug is the B-BR-SEO-03 slug capped at forumSlugMaxRunes, cut at a
// '-' boundary.
func forumSlug(value string) string {
	slug := slugify(value)
	if len(slug) <= forumSlugMaxRunes {
		return slug
	}
	slug = slug[:forumSlugMaxRunes]
	if i := strings.LastIndexByte(slug, '-'); i > 0 {
		slug = slug[:i]
	}
	return strings.Trim(slug, "-")
}

// forumDescription fits text after an optional prefix within descriptionMax
// runes and pads it with forum fillers up to descriptionMin.
func forumDescription(prefix, text string) string {
	text = strings.Join(strings.Fields(text), " ")
	room := descriptionMax
	if prefix != "" {
		room -= utf8.RuneCountInString(prefix) + 1
	}
	if utf8.RuneCountInString(text) > room {
		text = truncateWords(text, room-1) + "…"
	}
	out := strings.TrimSpace(prefix + " " + text)
	for _, filler := range forumFillers {
		if utf8.RuneCountInString(out) >= descriptionMin {
			break
		}
		if candidate := out + " " + filler; utf8.RuneCountInString(candidate) <= descriptionMax {
			out = candidate
		}
	}
	return out
}

// prepareForum validates the snapshot's editorial posts and plans their
// routes, order, heads and links. An empty forum plans no page.
func (g *generator) prepareForum(rows []enterpriseexport.ForumPost) error {
	if len(rows) == 0 {
		return nil
	}
	zone := forumZone()
	categories := map[string]*forumCategory{}
	seen := map[string]bool{}
	plan := &forumPlan{position: map[*forumPost]int{}}
	for _, row := range rows {
		if !isUUID(row.PostID) || row.PostID != strings.ToLower(row.PostID) || seen[row.PostID] {
			return fmt.Errorf("portalgen: invalid or duplicate forum post id")
		}
		seen[row.PostID] = true
		published, errPublished := time.Parse(time.RFC3339, row.PublishedAt)
		updated, errUpdated := time.Parse(time.RFC3339, row.UpdatedAt)
		if errPublished != nil || errUpdated != nil || strings.TrimSpace(row.Title) == "" || row.Category == "" ||
			(row.RegionUF != "" && !isUF(row.RegionUF)) || (row.RegionMunicipality != "" && (!isIBGE(row.RegionMunicipality) || row.RegionUF == "")) ||
			(row.StationID != "" && !isUUID(row.StationID)) {
			return fmt.Errorf("portalgen: invalid forum post %s", row.PostID)
		}
		category, ok := categories[row.Category]
		if !ok {
			name := strings.TrimSpace(row.CategoryName)
			if name == "" {
				name = row.Category
			}
			category = &forumCategory{Code: row.Category, Name: name, Description: strings.TrimSpace(row.CategoryDescription)}
			categories[row.Category] = category
		}
		post := &forumPost{
			Row: row, ID8: row.PostID[:8], Category: category,
			Published: published.UTC(), Updated: updated.UTC(),
			PublishedDay: published.In(zone).Format("02/01/2006"), UpdatedDay: updated.In(zone).Format("02/01/2006"),
			LastMod: updated.In(zone).Format("2006-01-02"),
			Blocks:  parseForumBody(row.Body), Author: forumAuthor(row.SourceName),
		}
		category.Posts = append(category.Posts, post)
		plan.Posts = append(plan.Posts, post)
	}
	newestFirst := func(posts []*forumPost) {
		sort.Slice(posts, func(i, j int) bool {
			if !posts[i].Published.Equal(posts[j].Published) {
				return posts[i].Published.After(posts[j].Published)
			}
			return posts[i].Row.PostID < posts[j].Row.PostID
		})
	}
	newestFirst(plan.Posts)
	for i, post := range plan.Posts {
		plan.position[post] = i
		plan.LastMod = max(plan.LastMod, post.LastMod)
	}
	slugs := map[string][]*forumCategory{}
	for _, code := range sortedKeys(categories) {
		category := categories[code]
		category.Slug = forumSlug(category.Name)
		if category.Slug == "" {
			category.Slug = forumSlug(code)
		}
		slugs[category.Slug] = append(slugs[category.Slug], category)
	}
	for _, group := range slugs {
		if len(group) > 1 {
			for _, category := range group {
				category.Slug += "-" + forumSlug(category.Code)
			}
		}
	}
	for _, code := range sortedKeys(categories) {
		category := categories[code]
		category.Route = "/forum/" + category.Slug + "/"
		newestFirst(category.Posts)
		for _, post := range category.Posts {
			category.LastMod = max(category.LastMod, post.LastMod)
		}
		plan.Categories = append(plan.Categories, category)
	}
	sort.Slice(plan.Categories, func(i, j int) bool { return plan.Categories[i].Slug < plan.Categories[j].Slug })
	assignForumRoutes(plan.Posts)
	planForumHeads(plan.Posts)
	g.forum = plan
	return nil
}

// assignForumRoutes gives every post /forum/{categoria}/{titulo}-{id8}/; a
// route that still collides uses the full id on every colliding post, so
// the result never depends on input order.
func assignForumRoutes(posts []*forumPost) {
	base := func(post *forumPost, suffix string) string {
		slug := forumSlug(post.Row.Title)
		if slug == "" {
			slug = "publicacao"
		}
		return post.Category.Route + slug + "-" + suffix + "/"
	}
	groups := map[string][]*forumPost{}
	for _, post := range posts {
		post.Route = base(post, post.ID8)
		groups[post.Route] = append(groups[post.Route], post)
	}
	for _, group := range groups {
		if len(group) > 1 {
			for _, post := range group {
				post.Route = base(post, post.Row.PostID)
			}
		}
	}
}

// planForumHeads makes every post title and description unique: the
// category, then the publication day and the short id disambiguate.
func planForumHeads(posts []*forumPost) {
	for _, post := range posts {
		post.HeadTitle = strings.TrimSpace(post.Row.Title) + " | " + forumBrand
		post.Description = forumDescription("", forumSummary(post))
	}
	titleSuffixes := []func(*forumPost) string{
		func(p *forumPost) string { return p.Category.Name },
		func(p *forumPost) string { return p.ID8 },
	}
	for _, suffix := range titleSuffixes {
		for _, group := range duplicateForum(posts, func(p *forumPost) string { return p.HeadTitle }) {
			for _, post := range group {
				post.HeadTitle = strings.TrimSuffix(post.HeadTitle, " | "+forumBrand) + " – " + suffix(post) + " | " + forumBrand
			}
		}
	}
	prefixes := []func(*forumPost) string{
		func(p *forumPost) string { return p.Category.Name + ", " + p.PublishedDay + ":" },
		func(p *forumPost) string { return p.Category.Name + ", " + p.PublishedDay + ", " + p.ID8 + ":" },
	}
	for _, prefix := range prefixes {
		for _, group := range duplicateForum(posts, func(p *forumPost) string { return p.Description }) {
			for _, post := range group {
				post.Description = forumDescription(prefix(post), forumSummary(post))
			}
		}
	}
}

func duplicateForum(posts []*forumPost, key func(*forumPost) string) [][]*forumPost {
	groups := map[string][]*forumPost{}
	for _, post := range posts {
		groups[key(post)] = append(groups[key(post)], post)
	}
	out := [][]*forumPost{}
	for _, value := range sortedKeys(groups) {
		if len(groups[value]) > 1 {
			out = append(out, groups[value])
		}
	}
	return out
}

// forumSummary is the editor's introduction, else the body, else the title.
func forumSummary(post *forumPost) string {
	if excerpt := strings.TrimSpace(post.Row.Excerpt); excerpt != "" {
		return excerpt
	}
	if text := forumPlainText(post.Blocks); text != "" {
		return text
	}
	return post.Row.Title
}

func (g *generator) hasForum() bool {
	return g.forum != nil && len(g.forum.Posts) > 0
}

func (g *generator) forumPostLink(post *forumPost, note string) moduleLink {
	return moduleLink{Label: post.Row.Title, Href: post.Route, Note: note}
}

// forumRegionModule lists up to forumRegionModuleSize posts of a region:
// for a city, posts about that municipality, then statewide posts of its
// UF; for a state, every post of the UF.
func (g *generator) forumRegionModule(uf, code string) linkModule {
	if !g.hasForum() || uf == "" {
		return linkModule{}
	}
	picked := []*forumPost{}
	seen := map[*forumPost]bool{}
	add := func(match func(enterpriseexport.ForumPost) bool) {
		for _, post := range g.forum.Posts {
			if len(picked) == forumRegionModuleSize {
				return
			}
			if !seen[post] && match(post.Row) {
				seen[post] = true
				picked = append(picked, post)
			}
		}
	}
	if code != "" {
		add(func(row enterpriseexport.ForumPost) bool { return row.RegionMunicipality == code && row.RegionUF == uf })
		add(func(row enterpriseexport.ForumPost) bool { return row.RegionUF == uf && row.RegionMunicipality == "" })
	} else {
		add(func(row enterpriseexport.ForumPost) bool { return row.RegionUF == uf })
	}
	links := make([]moduleLink, 0, len(picked))
	for _, post := range picked {
		links = append(links, g.forumPostLink(post, post.Category.Name+" · "+post.PublishedDay))
	}
	return linkModule{Title: "No fórum", Links: links}
}

// forumPrecosModule links the latest posts and their categories from the
// national prices hub.
func (g *generator) forumPrecosModule() linkModule {
	if !g.hasForum() {
		return linkModule{}
	}
	links := []moduleLink{}
	categories := []*forumCategory{}
	seen := map[*forumCategory]bool{}
	for _, post := range g.forum.Posts[:min(forumPrecosLatest, len(g.forum.Posts))] {
		links = append(links, g.forumPostLink(post, post.Category.Name+" · "+post.PublishedDay))
		if !seen[post.Category] {
			seen[post.Category] = true
			categories = append(categories, post.Category)
		}
	}
	for _, category := range categories {
		links = append(links, moduleLink{Label: "Mais publicações sobre " + category.Name + " no fórum", Href: category.Route})
	}
	links = append(links, moduleLink{Label: "Todas as publicações do fórum abastevo", Href: "/forum/"})
	return linkModule{Title: "No fórum do abastevo", Links: links}
}

// forumSiblings is "Outras publicações": a ring over the post's category,
// then posts of the same region, then a ring over all posts. The global ring
// guarantees every post a link from each of its two predecessors.
func (g *generator) forumSiblings(post *forumPost) []moduleLink {
	picked := []*forumPost{}
	seen := map[*forumPost]bool{post: true}
	add := func(other *forumPost) bool {
		if seen[other] {
			return false
		}
		seen[other] = true
		picked = append(picked, other)
		return true
	}
	category := post.Category.Posts
	for i, other := range category {
		if other == post {
			for _, next := range ringAfter(category, i, forumSameCategory) {
				add(next)
			}
			break
		}
	}
	if post.Row.RegionUF != "" {
		added := 0
		// Same municipality (or statewide) first, then the rest of the UF.
		for _, sameMunicipality := range []bool{true, false} {
			for _, other := range g.forum.Posts {
				if added == forumSameRegion {
					break
				}
				if other.Row.RegionUF != post.Row.RegionUF || (sameMunicipality && other.Row.RegionMunicipality != post.Row.RegionMunicipality) {
					continue
				}
				if add(other) {
					added++
				}
			}
		}
	}
	all, added := g.forum.Posts, 0
	start := g.forum.position[post]
	for step := 1; step < len(all) && added < forumGlobalRing; step++ {
		if add(all[(start+step)%len(all)]) {
			added++
		}
	}
	links := make([]moduleLink, 0, len(picked))
	for _, other := range picked {
		links = append(links, g.forumPostLink(other, other.Category.Name+" · "+other.PublishedDay))
	}
	return links
}

func (g *generator) forumCategoryLinks(except *forumCategory) []moduleLink {
	links := []moduleLink{}
	for _, category := range g.forum.Categories {
		if category == except {
			continue
		}
		links = append(links, moduleLink{Label: "Publicações sobre " + category.Name, Href: category.Route,
			Note: fmt.Sprintf("%d %s", len(category.Posts), plural(len(category.Posts), "publicação", "publicações"))})
	}
	return links
}

// forumRelated links the data pages a post is about, only when those pages
// exist in this build.
func (g *generator) forumRelated(post *forumPost) []moduleLink {
	links := []moduleLink{}
	uf, code := post.Row.RegionUF, post.Row.RegionMunicipality
	if uf != "" && g.statePages[uf] {
		links = append(links, moduleLink{Label: "Preço dos combustíveis em " + ufName(uf), Href: stateRoute(uf)})
	}
	if city, ok := g.cities[code]; ok && code != "" && city.UF == uf {
		links = append(links, moduleLink{Label: "Preços em " + placeLabel(city.Name, city.UF), Href: city.Route})
	}
	if product := forumFuelProducts[post.Row.FuelProduct]; product != "" {
		if hub, ok := g.fuelHubs[product]; ok {
			words := fuelVocabulary[product]
			links = append(links, moduleLink{Label: "Preço " + words.Article + " " + words.Name + " por estado", Href: hub.Href})
		}
	}
	if index, ok := g.stationIndex[post.Row.StationID]; ok && post.Row.StationID != "" {
		entry := g.entries[index]
		links = append(links, moduleLink{Label: "Preços do posto " + entry.name, Href: "/postos/" + entry.station.StationID + "/"})
	}
	return links
}

type forumListItem struct {
	Title, Route, Day, Excerpt, CategoryName, CategoryRoute string
}

func forumListItems(posts []*forumPost) []forumListItem {
	items := make([]forumListItem, 0, len(posts))
	for _, post := range posts {
		items = append(items, forumListItem{
			Title: post.Row.Title, Route: post.Route, Day: post.PublishedDay,
			Excerpt: strings.TrimSpace(post.Row.Excerpt), CategoryName: post.Category.Name, CategoryRoute: post.Category.Route,
		})
	}
	return items
}

func forumCollection(items []forumListItem) []listItem {
	out := make([]listItem, 0, len(items))
	for _, item := range items {
		out = append(out, listItem{Name: item.Title, Href: item.Route})
	}
	return out
}

type forumCategoryLink struct {
	Name, Route string
	Count       int
}

type forumHubPage struct {
	pageBase
	Posts      []forumListItem
	Categories []forumCategoryLink
	Total      int
	AppHref    string
}

type forumCategoryPage struct {
	pageBase
	Name, Summary string
	Posts         []forumListItem
	Total         int
	Truncated     bool
}

type forumPostPage struct {
	pageBase
	PostTitle                  string
	CategoryName, CategoryHref string
	Author, AboutHref          string
	PublishedISO, PublishedDay string
	UpdatedISO, UpdatedDay     string
	ShowUpdated                bool
	Related                    []moduleLink
	Blocks                     []forumBlock
	AppHref                    string
}

func (g *generator) forumLandingHref(path string) string {
	if g.opts.LandingURL == "" {
		return ""
	}
	return g.landingPrefix + path
}

// generateForum writes /forum/, the category hubs and the post pages.
func (g *generator) generateForum() error {
	if !g.hasForum() {
		return nil
	}
	if err := g.generateForumHub(); err != nil {
		return err
	}
	for _, category := range g.forum.Categories {
		if err := g.generateForumCategory(category); err != nil {
			return err
		}
	}
	for _, post := range g.forum.Posts {
		if err := g.generateForumPost(post); err != nil {
			return err
		}
	}
	return nil
}

func (g *generator) generateForumHub() error {
	total := len(g.forum.Posts)
	names := make([]string, 0, len(g.forum.Categories))
	categories := make([]forumCategoryLink, 0, len(g.forum.Categories))
	for _, category := range g.forum.Categories {
		names = append(names, category.Name)
		categories = append(categories, forumCategoryLink{Name: category.Name, Route: category.Route, Count: len(category.Posts)})
	}
	description := forumDescription("", fmt.Sprintf("%s: %d %s da Equipe abastevo sobre combustíveis, mobilidade e dados. Categorias: %s.",
		forumBrand, total, plural(total, "publicação", "publicações"), joinPT(names)))
	data := forumHubPage{
		pageBase: g.newBase(pageSpec{
			Route: "/forum/", Facts: indexFacts{Family: familyForumHub, HasCurrentData: total > 0},
			Title: forumBrand, HeadTitle: forumBrand + ": publicações sobre combustíveis e mobilidade",
			Description: description, Active: "forum", Crumbs: []crumb{{Label: "Fórum", Current: true}}, LastMod: g.forum.LastMod,
		}),
		Posts: forumListItems(g.forum.Posts[:min(forumHubLatest, total)]), Categories: categories, Total: total,
		AppHref: g.forumLandingHref("/#aplicativo"),
	}
	data.Modules = []linkModule{g.hubModule("/forum/")}
	var err error
	if data.pageBase, err = g.collectionBase(data.pageBase, forumCollection(data.Posts)); err != nil {
		return err
	}
	return g.writeRoute("forumHub", data)
}

func (g *generator) generateForumCategory(category *forumCategory) error {
	total := len(category.Posts)
	lead := fmt.Sprintf("%s no %s: %d %s da Equipe abastevo.", category.Name, forumBrand, total, plural(total, "publicação", "publicações"))
	if category.Description != "" {
		lead += " " + category.Description
	}
	data := forumCategoryPage{
		pageBase: g.newBase(pageSpec{
			Route: category.Route, Facts: indexFacts{Family: familyForumCategory, HasCurrentData: total > 0},
			Title: category.Name, HeadTitle: category.Name + " | " + forumBrand,
			Description: forumDescription("", lead), Active: "forum",
			Crumbs:  []crumb{{Label: "Fórum", Href: "/forum/"}, {Label: category.Name, Current: true}},
			LastMod: category.LastMod,
		}),
		Name: category.Name, Summary: category.Description, Total: total,
		Posts: forumListItems(category.Posts[:min(forumCategoryListCap, total)]), Truncated: total > forumCategoryListCap,
	}
	data.Modules = []linkModule{{Title: "Outras categorias do fórum", Links: g.forumCategoryLinks(category)}}
	var err error
	if data.pageBase, err = g.collectionBase(data.pageBase, forumCollection(data.Posts)); err != nil {
		return err
	}
	return g.writeRoute("forumCategory", data)
}

func (g *generator) generateForumPost(post *forumPost) error {
	base := g.newBase(pageSpec{
		Route: post.Route, Facts: indexFacts{Family: familyForumPost, PublishedEditorial: true},
		Title: post.Row.Title, HeadTitle: post.HeadTitle, Description: post.Description, Active: "forum",
		Crumbs:  []crumb{{Label: "Fórum", Href: "/forum/"}, {Label: post.Category.Name, Href: post.Category.Route}, {Label: post.Row.Title, Current: true}},
		LastMod: post.LastMod,
	})
	base.OGType = "article"
	base.ArticlePublished = post.Published.Format(time.RFC3339)
	base.ArticleModified = post.Updated.Format(time.RFC3339)
	base.Modules = []linkModule{
		{Title: "Outras publicações", Links: g.forumSiblings(post)},
		{Title: "Categorias do fórum", Links: g.forumCategoryLinks(nil)},
	}
	data := forumPostPage{
		pageBase: base, PostTitle: post.Row.Title, CategoryName: post.Category.Name, CategoryHref: post.Category.Route,
		Author: post.Author, PublishedISO: base.ArticlePublished, PublishedDay: post.PublishedDay,
		UpdatedISO: base.ArticleModified, UpdatedDay: post.UpdatedDay, ShowUpdated: post.UpdatedDay != post.PublishedDay,
		Related: g.forumRelated(post), Blocks: post.Blocks, AppHref: g.forumLandingHref("/#aplicativo"),
	}
	if post.Author == forumEditorialLabel {
		data.AboutHref = g.forumLandingHref("/sobre/")
	}
	var err error
	if data.pageBase, err = g.withStructuredData(data.pageBase, g.discussionNode(data.pageBase, post, data.AboutHref)); err != nil {
		return err
	}
	return g.writeRoute("forumPost", data)
}

// discussionNode describes one editorial post with visible facts only: the
// title, an excerpt of the rendered body, the visible dates and author.
func (g *generator) discussionNode(base pageBase, post *forumPost, aboutHref string) ldNode {
	author := ldNode{"@type": "Organization", "name": post.Author}
	if aboutHref != "" {
		author["url"] = g.absoluteURL(aboutHref)
	}
	text := forumPlainText(post.Blocks)
	if utf8.RuneCountInString(text) > forumTextExcerpt {
		text = truncateWords(text, forumTextExcerpt-1) + "…"
	}
	if text == "" {
		text = post.Row.Title
	}
	return ldNode{
		"@type": "DiscussionForumPosting", "@id": base.Canonical + "#post", "url": base.Canonical,
		"mainEntityOfPage": base.Canonical, "headline": post.Row.Title, "text": text,
		"datePublished": base.ArticlePublished, "dateModified": base.ArticleModified,
		"inLanguage": "pt-BR", "articleSection": post.Category.Name, "author": author,
		"isPartOf": ldNode{"@type": "CollectionPage", "@id": g.siteURL + "/forum/#webpage", "url": g.siteURL + "/forum/", "name": forumBrand},
	}
}
