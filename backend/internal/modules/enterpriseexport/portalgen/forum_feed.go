package portalgen

import (
	"encoding/xml"
	"time"
)

// The Atom feed of the editorial forum posts (SEO-22). It exists only in an
// indexable build with a forum, so every entry is also a sitemap URL.
const (
	forumFeedPath    = "forum/feed.xml"
	forumFeedEntries = 20
	atomXMLNS        = "http://www.w3.org/2005/Atom"
)

type atomLink struct {
	Rel  string `xml:"rel,attr"`
	Type string `xml:"type,attr"`
	Href string `xml:"href,attr"`
}

type atomAuthor struct {
	Name string `xml:"name"`
}

type atomCategory struct {
	Term string `xml:"term,attr"`
}

type atomEntry struct {
	ID        string       `xml:"id"`
	Title     string       `xml:"title"`
	Link      atomLink     `xml:"link"`
	Published string       `xml:"published"`
	Updated   string       `xml:"updated"`
	Author    atomAuthor   `xml:"author"`
	Category  atomCategory `xml:"category"`
	Summary   string       `xml:"summary"`
}

type atomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	XMLNS   string      `xml:"xmlns,attr"`
	Lang    string      `xml:"xml:lang,attr"`
	ID      string      `xml:"id"`
	Title   string      `xml:"title"`
	Updated string      `xml:"updated"`
	Links   []atomLink  `xml:"link"`
	Entries []atomEntry `xml:"entry"`
}

// forumFeedHref is the root-relative feed link of the forum pages, or ""
// when this build has no feed.
func (g *generator) forumFeedHref() string {
	if !g.hasForum() || !g.indexable || g.siteURL == "" {
		return ""
	}
	return "/" + forumFeedPath
}

// writeForumFeed writes the newest editorial posts as Atom 1.0. The feed
// date is the newest entry date, never the build time, so identical inputs
// produce identical bytes.
func (g *generator) writeForumFeed() error {
	href := g.forumFeedHref()
	if href == "" {
		return nil
	}
	posts := g.forum.Posts[:min(forumFeedEntries, len(g.forum.Posts))]
	feed := atomFeed{
		XMLNS: atomXMLNS, Lang: "pt-BR", ID: g.siteURL + href, Title: forumBrand,
		Links: []atomLink{
			{Rel: "self", Type: "application/atom+xml", Href: g.siteURL + href},
			{Rel: "alternate", Type: "text/html", Href: g.siteURL + "/forum/"},
		},
		Entries: make([]atomEntry, 0, len(posts)),
	}
	var newest time.Time
	for _, post := range posts {
		url := g.siteURL + post.Route
		feed.Entries = append(feed.Entries, atomEntry{
			ID: url, Title: post.Row.Title,
			Link:      atomLink{Rel: "alternate", Type: "text/html", Href: url},
			Published: post.Published.Format(time.RFC3339), Updated: post.Updated.Format(time.RFC3339),
			Author: atomAuthor{Name: post.Author}, Category: atomCategory{Term: post.Category.Name},
			Summary: post.Description,
		})
		if post.Updated.After(newest) {
			newest = post.Updated
		}
	}
	feed.Updated = newest.Format(time.RFC3339)
	body, err := marshalSitemap(feed)
	if err != nil {
		return err
	}
	return g.writeAsset(forumFeedPath, body)
}
