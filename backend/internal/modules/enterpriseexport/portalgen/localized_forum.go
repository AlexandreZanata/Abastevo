package portalgen

import (
	"strings"
	"time"
	"unicode/utf8"
)

type localizedForumPage struct {
	Kind, Brand, Team, ListTitle, Limit, AppHref string
	Categories                                   []moduleLink
	Posts                                        []localizedForumItem
	Post                                         *localizedForumArticle
}

type localizedForumItem struct {
	Title, Excerpt, Route, Category, CategoryRoute, Date, Language string
}

type localizedForumArticle struct {
	Title, Category, CategoryHref, Author, AboutHref, Language string
	PublishedISO, PublishedDay, UpdatedISO, UpdatedDay         string
	Blocks                                                     []forumBlock
	Related                                                    []moduleLink
}

type forumContent struct {
	Title, Excerpt, Language string
	Blocks                   []forumBlock
}

func (p localizedPortal) forumCopy(post *forumPost) forumContent {
	if translated, ok := reviewedForumCopy(post, p.Code); ok {
		return translated
	}
	return forumContent{Title: post.Row.Title, Excerpt: strings.TrimSpace(post.Row.Excerpt), Language: "pt-BR", Blocks: post.Blocks}
}

func (p localizedPortal) forumCategory(category *forumCategory) string {
	if name := p.catalog.Messages["forum.category."+category.Code]; name != "" {
		return name
	}
	return category.Name + " (" + p.text("language.portuguese") + ")"
}

func (p localizedPortal) forumPostCount(count int) string {
	key := "forum.posts"
	if count == 1 {
		key = "forum.post"
	}
	return p.text(key, "count", p.integer(count))
}

func (p localizedPortal) forumDate(date time.Time) string {
	return date.In(forumZone()).Format(p.DateLayout)
}

func (p localizedPortal) forumItems(posts []*forumPost) []localizedForumItem {
	items := make([]localizedForumItem, 0, len(posts))
	for _, post := range posts {
		copy := p.forumCopy(post)
		items = append(items, localizedForumItem{
			Title: copy.Title, Excerpt: copy.Excerpt, Language: copy.Language,
			Route: p.routes[post.Route], Category: p.forumCategory(post.Category),
			CategoryRoute: p.routes[post.Category.Route], Date: p.forumDate(post.Published),
		})
	}
	return items
}

func (p localizedPortal) addRecentForumPosts(data *localizedPage) {
	if !p.hasForum() {
		return
	}
	for _, item := range p.forumItems(p.forum.Posts[:min(forumPrecosLatest, len(p.forum.Posts))]) {
		link := moduleLink{Label: item.Title, Href: item.Route, Note: item.Category + " · " + item.Date}
		if item.Language == "pt-BR" {
			link.Note += " · " + p.text("language.portuguese")
		}
		data.appendModule(p.text("forum.latest"), link)
		data.appendModule(p.text("forum.categories"), moduleLink{Label: item.Category, Href: item.CategoryRoute})
	}
}

func (p localizedPortal) forumCategories(except *forumCategory) []moduleLink {
	links := make([]moduleLink, 0, len(p.forum.Categories))
	for _, category := range p.forum.Categories {
		if category != except {
			link := p.localizedLink(p.forumCategory(category), category.Route)
			link.Note = p.forumPostCount(len(category.Posts))
			links = append(links, link)
		}
	}
	return links
}

func (p localizedPortal) forumPage(data *localizedPage, kind string) *localizedForumPage {
	view := &localizedForumPage{Kind: kind, Brand: p.text("title.forum"), Team: p.text("forum.team")}
	data.Forum = view
	data.ShowPeriod = false
	data.FeedHref = p.forumFeed
	if p.opts.LandingURL != "" {
		view.AppHref = p.landingPrefix + p.Prefix + "/#aplicativo"
	}
	if kind != "hub" {
		data.appendModule(p.text("section.related"), p.localizedLink(view.Brand, "/forum/"))
	}
	return view
}

func (p localizedPortal) addForumHub(data *localizedPage) ldNode {
	view := p.forumPage(data, "hub")
	posts := p.forum.Posts[:min(forumHubLatest, len(p.forum.Posts))]
	view.Posts, view.Categories = p.forumItems(posts), p.forumCategories(nil)
	view.ListTitle = p.text("forum.latest")
	data.Lead = p.text("forum.lead")
	if len(posts) < len(p.forum.Posts) {
		view.Limit = p.text("forum.hubLimit", "shown", p.integer(len(posts)), "total", p.integer(len(p.forum.Posts)))
	}
	return p.forumCollection(data, view.Posts)
}

func (p localizedPortal) addForumCategory(data *localizedPage, category *forumCategory) ldNode {
	view := p.forumPage(data, "category")
	view.ListTitle = p.text("forum.publications")
	posts := category.Posts[:min(forumCategoryListCap, len(category.Posts))]
	view.Posts = p.forumItems(posts)
	data.Lead = p.catalog.Messages["forum.categoryDescription."+category.Code]
	data.Breadcrumbs = []crumb{
		{Label: p.text("home"), Href: data.Home},
		{Label: view.Brand, Href: p.routes["/forum/"]},
		{Label: data.Title, Current: true},
	}
	if len(posts) < len(category.Posts) {
		view.Limit = p.text("forum.categoryLimit", "shown", p.integer(len(posts)), "total", p.integer(len(category.Posts)))
	}
	for _, link := range p.forumCategories(category) {
		data.appendModule(p.text("forum.otherCategories"), link)
	}
	return p.forumCollection(data, view.Posts)
}

func (p localizedPortal) forumCollection(data *localizedPage, items []localizedForumItem) ldNode {
	links := make([]listItem, 0, len(items))
	for _, item := range items {
		links = append(links, listItem{Name: item.Title, Href: item.Route})
	}
	return p.collectionNode(data.pageBase, links)
}

func (p localizedPortal) addForumPost(data *localizedPage, page forumPostPage) ldNode {
	post := page.Post
	copy := p.forumCopy(post)
	view := p.forumPage(data, "post")
	article := &localizedForumArticle{
		Title: copy.Title, Language: copy.Language, Category: p.forumCategory(post.Category), CategoryHref: p.routes[post.Category.Route],
		Author: post.Author, Blocks: copy.Blocks, PublishedISO: page.PublishedISO, PublishedDay: p.forumDate(post.Published),
	}
	view.Post = article
	data.Title = article.Title
	if post.Author == forumEditorialLabel {
		article.Author = view.Team
		if p.opts.LandingURL != "" {
			article.AboutHref = p.landingPrefix + p.Prefix + "/about/"
			if p.Code == "es" {
				article.AboutHref = p.landingPrefix + p.Prefix + "/sobre/"
			}
		}
	}
	data.OGType, data.ArticlePublished, data.ArticleModified = "article", page.PublishedISO, page.UpdatedISO
	if page.ShowUpdated {
		article.UpdatedISO, article.UpdatedDay = page.UpdatedISO, p.forumDate(post.Updated)
	}
	if copy.Language != "pt-BR" && copy.Excerpt != "" {
		data.Description = p.priceDescription("description.forumExcerpt", "excerpt", copy.Excerpt)
	}
	data.Breadcrumbs = []crumb{
		{Label: p.text("home"), Href: data.Home},
		{Label: view.Brand, Href: p.routes["/forum/"]},
		{Label: article.Category, Href: article.CategoryHref},
		{Label: article.Title, Current: true},
	}
	article.Related = p.localizedForumRelated(post)
	for _, link := range p.forumCategories(nil) {
		data.appendModule(p.text("forum.categories"), link)
	}
	siblings := p.forumSiblings(post)
	for _, link := range siblings {
		for _, candidate := range p.forum.Posts {
			if candidate.Route == link.Href {
				content := p.forumCopy(candidate)
				link = p.localizedLink(content.Title, candidate.Route)
				link.Note = p.forumCategory(candidate.Category) + " · " + p.forumDate(candidate.Published)
				if content.Language == "pt-BR" {
					link.Note += " · " + p.text("language.portuguese")
				}
				data.appendModule(p.text("forum.otherPosts"), link)
				break
			}
		}
	}
	node := p.discussionNode(data.pageBase, post, article.AboutHref)
	node["headline"], node["inLanguage"], node["articleSection"] = article.Title, article.Language, article.Category
	node["author"].(ldNode)["name"] = article.Author
	text := forumPlainText(article.Blocks)
	if utf8.RuneCountInString(text) > forumTextExcerpt {
		text = truncateWords(text, forumTextExcerpt-1) + "…"
	}
	if text == "" {
		text = article.Title
	}
	node["text"] = text
	hub := p.absoluteURL(p.routes["/forum/"])
	node["isPartOf"] = ldNode{"@type": "CollectionPage", "@id": hub + "#webpage", "url": hub, "name": view.Brand}
	return node
}

func (p localizedPortal) localizedForumRelated(post *forumPost) []moduleLink {
	row := post.Row
	var links []moduleLink
	if row.LandingRoute != "" && LandingRoutes[row.LandingRoute] && p.opts.LandingURL != "" {
		links = append(links, p.localizedEditorialResource(row.LandingRoute))
	}
	if row.RegionUF != "" && p.statePages[row.RegionUF] {
		links = append(links, p.localizedLink(p.text("station.pricesIn", "place", ufName(row.RegionUF)), stateRoute(row.RegionUF)))
	}
	if city := p.cities[row.RegionMunicipality]; city != nil && city.UF == row.RegionUF {
		links = append(links, p.localizedLink(p.text("station.pricesIn", "place", placeLabel(city.Name, city.UF)), city.Route))
	}
	if hub, ok := p.fuelHubs[forumFuelProducts[row.FuelProduct]]; ok {
		links = append(links, p.localizedLink(p.fuel(hub.Code), hub.Href))
	}
	if i, ok := p.stationIndex[row.StationID]; ok {
		station := p.entries[i]
		links = append(links, p.localizedLink(p.text("forum.station", "station", station.name), "/postos/"+row.StationID+"/"))
	}
	return links
}
