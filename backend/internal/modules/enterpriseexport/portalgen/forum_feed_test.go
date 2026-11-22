package portalgen

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const forumFeedHeadLink = `<link rel="alternate" type="application/atom+xml" title="Fórum abastevo" href="/forum/feed.xml">`

func TestForumFeedListsEditorialPostsNewestFirst(t *testing.T) {
	out := generateSEOPortal(t, true)
	raw := readPortal(t, out, forumFeedPath)
	var feed atomFeed
	if err := xml.Unmarshal([]byte(raw), &feed); err != nil {
		t.Fatalf("feed is not well-formed XML: %v", err)
	}
	if feed.XMLName.Space != atomXMLNS || feed.ID != seoSite+"/forum/feed.xml" || feed.Title != forumBrand {
		t.Fatalf("feed identity = %+v", feed)
	}
	if len(feed.Links) != 2 || feed.Links[0].Rel != "self" || feed.Links[0].Href != feed.ID ||
		feed.Links[1].Rel != "alternate" || feed.Links[1].Href != seoSite+"/forum/" {
		t.Fatalf("feed links = %+v", feed.Links)
	}
	if len(feed.Entries) != len(seoForumFixture()) {
		t.Fatalf("feed has %d entries, want %d", len(feed.Entries), len(seoForumFixture()))
	}

	sitemap := readPortal(t, out, "sitemaps/portal-forum-pt-BR-1.xml")
	var newest, previous time.Time
	for i, entry := range feed.Entries {
		published, err := time.Parse(time.RFC3339, entry.Published)
		if err != nil {
			t.Fatalf("entry %d published %q: %v", i, entry.Published, err)
		}
		updated, err := time.Parse(time.RFC3339, entry.Updated)
		if err != nil {
			t.Fatalf("entry %d updated %q: %v", i, entry.Updated, err)
		}
		if i > 0 && published.After(previous) {
			t.Fatalf("entry %d is newer than the entry before it", i)
		}
		previous = published
		if updated.After(newest) {
			newest = updated
		}
		if entry.ID != entry.Link.Href || !strings.Contains(sitemap, "<loc>"+entry.ID+"</loc>") {
			t.Fatalf("entry %d link %q is not a forum sitemap URL", i, entry.ID)
		}
		if entry.Title == "" || entry.Summary == "" || entry.Author.Name == "" || entry.Category.Term == "" {
			t.Fatalf("entry %d is missing a field: %+v", i, entry)
		}
		page := readPortal(t, out, strings.TrimPrefix(strings.TrimPrefix(entry.ID, seoSite), "/")+"index.html")
		if parseHead(page).Description != entry.Summary {
			t.Fatalf("entry %d summary differs from its page description", i)
		}
	}
	feedUpdated, err := time.Parse(time.RFC3339, feed.Updated)
	if err != nil || !feedUpdated.Equal(newest) {
		t.Fatalf("feed updated = %q, want the newest entry date %s", feed.Updated, newest.Format(time.RFC3339))
	}
	// The newest fixture post was edited on 2026-10-22; the build ran on 2026-10-24.
	if !feedUpdated.Equal(time.Date(2026, 10, 22, 9, 30, 0, 0, time.UTC)) {
		t.Fatalf("feed updated = %s, want the fixture edit instant, never the build time", feed.Updated)
	}
}

func TestForumFeedEscapesHostileText(t *testing.T) {
	raw := readPortal(t, generateSEOPortal(t, true), forumFeedPath)
	if strings.Contains(raw, "<script>") || strings.Contains(raw, `& "aspas"`) {
		t.Fatal("feed carries unescaped post text")
	}
	if !strings.Contains(raw, "&lt;script&gt;") || !strings.Contains(raw, "&amp;") {
		t.Fatal("feed lost the escaped post title")
	}
}

func TestForumFeedIsDeterministicAndPassesTheStrictVerifier(t *testing.T) {
	first, second := generateSEOPortal(t, true), generateSEOPortal(t, true)
	if readPortal(t, first, forumFeedPath) != readPortal(t, second, forumFeedPath) {
		t.Fatal("two builds of the same snapshot produced different feeds")
	}
	if _, err := Verify(first, VerifyOptions{SiteURL: seoSite}); err != nil {
		t.Fatalf("strict verifier refused a portal with a forum feed: %v", err)
	}
	if !strings.Contains(readPortal(t, first, "portal-manifest.json"), `"path": "forum/feed.xml"`) {
		t.Fatal("the portal manifest does not list the feed")
	}
}

func TestForumFeedIsAdvertisedOnlyOnForumPages(t *testing.T) {
	for route, body := range portalPages(t, generateSEOPortal(t, true)) {
		forumPage := strings.HasPrefix(route, "forum/")
		if strings.Contains(body, forumFeedHeadLink) != forumPage {
			t.Fatalf("%s: feed head link present = %t, want %t", route, !forumPage, forumPage)
		}
	}
}

func TestForumFeedNeedsAForumAndAnIndexableBuild(t *testing.T) {
	withoutForum := filepath.Join(t.TempDir(), "portal")
	if _, err := Generate(Options{
		SnapshotDir: writeSEOFixtureWithoutForum(t), OutputDir: withoutForum, Indexable: true, SiteURL: seoSite, LandingURL: seoSite,
	}); err != nil {
		t.Fatal(err)
	}
	for name, out := range map[string]string{"no forum": withoutForum, "hidden build": generateSEOPortal(t, false)} {
		if _, err := os.Stat(filepath.Join(out, forumFeedPath)); !os.IsNotExist(err) {
			t.Fatalf("%s: feed file exists (stat error %v)", name, err)
		}
		for route, body := range portalPages(t, out) {
			if strings.Contains(body, "application/atom+xml") {
				t.Fatalf("%s: %s advertises a feed that does not exist", name, route)
			}
		}
	}
}
