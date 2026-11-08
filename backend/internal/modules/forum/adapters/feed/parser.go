package feed

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
)

const (
	MaxFeedBodyBytes      = 2 * 1024 * 1024 // 2 MiB
	MaxEntryMetadataBytes = 16 * 1024       // 16 KiB per entry
	MaxEntriesPerFeed     = 100
	MaxXMLDepth           = 32
	MaxTitleRunes         = 120
	MaxExcerptRunes       = 400
	MaxFutureSkew         = 5 * time.Minute
)

var (
	htmlTagRegex = regexp.MustCompile(`(?i)<[^>]*>`)
	spaceRegex   = regexp.MustCompile(`\s+`)

	// rssDateFormats supports standard RFC-822, RFC-1123 and variants with timezone offset.
	rssDateFormats = []string{
		time.RFC1123Z,
		time.RFC1123,
		time.RFC822Z,
		time.RFC822,
		"Mon, 2 Jan 2006 15:04:05 -0700",
		"Mon, 2 Jan 2006 15:04:05 MST",
		"2 Jan 2006 15:04:05 -0700",
		"2006-01-02T15:04:05-07:00",
		"2006-01-02T15:04:05Z",
		time.RFC3339,
		time.RFC3339Nano,
	}

	atomDateFormats = []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05-07:00",
		"2006-01-02T15:04:05Z",
		time.RFC1123Z,
	}
)

// DefaultFeedParser implements application.FeedParser using Go stdlib encoding/xml.
type DefaultFeedParser struct{}

// NewFeedParser constructs a new DefaultFeedParser.
func NewFeedParser() *DefaultFeedParser {
	return &DefaultFeedParser{}
}

// Parse decodes, validates and sanitizes a raw RSS 2.0 or Atom 1.0 feed document.
func (p *DefaultFeedParser) Parse(raw []byte, now time.Time) ([]application.FeedItem, error) {
	if len(raw) == 0 {
		return nil, domain.ErrFeedMalformed
	}
	if len(raw) > MaxFeedBodyBytes {
		return nil, domain.ErrFeedOversized
	}

	// 1. Strict XXE Guard: refuse DTD and ENTITY definitions immediately
	if bytes.Contains(raw, []byte("<!DOCTYPE")) || bytes.Contains(raw, []byte("<!doctype")) ||
		bytes.Contains(raw, []byte("<!ENTITY")) || bytes.Contains(raw, []byte("<!entity")) {
		return nil, domain.ErrFeedSecurityRefusal
	}

	// 2. Tokenize and enforce maximum nesting depth to prevent XML bomb / stack exhaustion
	dec := xml.NewDecoder(bytes.NewReader(raw))
	depth := 0
	var rootName string

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: %v", domain.ErrFeedMalformed, err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			if depth > MaxXMLDepth {
				return nil, domain.ErrFeedSecurityRefusal
			}
			if rootName == "" {
				rootName = strings.ToLower(t.Name.Local)
			}
		case xml.EndElement:
			depth--
		}
	}

	// 3. Dispatch parser according to root document type
	switch rootName {
	case "rss":
		return parseRSS2(raw, now)
	case "feed":
		return parseAtom1(raw, now)
	default:
		return nil, fmt.Errorf("%w: unsupported root element %q", domain.ErrFeedMalformed, rootName)
	}
}

// --- RSS 2.0 Schema & Parser ---

type rss2Channel struct {
	Title string     `xml:"title"`
	Link  string     `xml:"link"`
	Items []rss2Item `xml:"item"`
}

type rss2Item struct {
	Title       string   `xml:"title"`
	Link        string   `xml:"link"`
	Description string   `xml:"description"`
	PubDate     string   `xml:"pubDate"`
	GUID        rss2GUID `xml:"guid"`
	Categories  []string `xml:"category"`
}

type rss2GUID struct {
	Value       string `xml:",chardata"`
	IsPermaLink string `xml:"isPermaLink,attr"`
}

type rss2Doc struct {
	XMLName xml.Name    `xml:"rss"`
	Channel rss2Channel `xml:"channel"`
}

func parseRSS2(raw []byte, now time.Time) ([]application.FeedItem, error) {
	var doc rss2Doc
	if err := xml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("%w: invalid rss 2.0: %v", domain.ErrFeedMalformed, err)
	}

	count := len(doc.Channel.Items)
	if count > MaxEntriesPerFeed {
		count = MaxEntriesPerFeed
	}

	items := make([]application.FeedItem, 0, count)
	for i := 0; i < count; i++ {
		item := doc.Channel.Items[i]
		parsed, ok := sanitizeFeedItem(
			item.GUID.Value,
			item.Title,
			item.Link,
			item.Description,
			item.PubDate,
			rssDateFormats,
			item.Categories,
			now,
		)
		if ok {
			items = append(items, parsed)
		}
	}

	return items, nil
}

// --- Atom 1.0 Schema & Parser ---

type atomEntry struct {
	Title     string     `xml:"title"`
	ID        string     `xml:"id"`
	Updated   string     `xml:"updated"`
	Published string     `xml:"published"`
	Summary   string     `xml:"summary"`
	Content   string     `xml:"content"`
	Links     []atomLink `xml:"link"`
	Category  []struct {
		Term string `xml:"term,attr"`
	} `xml:"category"`
}

type atomLink struct {
	Rel  string `xml:"rel,attr"`
	Href string `xml:"href,attr"`
}

type atomDoc struct {
	XMLName xml.Name    `xml:"feed"`
	Entries []atomEntry `xml:"entry"`
}

func parseAtom1(raw []byte, now time.Time) ([]application.FeedItem, error) {
	var doc atomDoc
	if err := xml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("%w: invalid atom 1.0: %v", domain.ErrFeedMalformed, err)
	}

	count := len(doc.Entries)
	if count > MaxEntriesPerFeed {
		count = MaxEntriesPerFeed
	}

	items := make([]application.FeedItem, 0, count)
	for i := 0; i < count; i++ {
		entry := doc.Entries[i]

		// Find canonical link
		var chosenLink string
		for _, l := range entry.Links {
			if l.Rel == "alternate" || l.Rel == "" {
				chosenLink = l.Href
				break
			}
		}
		if chosenLink == "" && len(entry.Links) > 0 {
			chosenLink = entry.Links[0].Href
		}

		desc := entry.Summary
		if strings.TrimSpace(desc) == "" {
			desc = entry.Content
		}

		dateStr := entry.Updated
		if strings.TrimSpace(dateStr) == "" {
			dateStr = entry.Published
		}

		cats := make([]string, 0, len(entry.Category))
		for _, c := range entry.Category {
			if term := strings.TrimSpace(c.Term); term != "" {
				cats = append(cats, term)
			}
		}

		parsed, ok := sanitizeFeedItem(
			entry.ID,
			entry.Title,
			chosenLink,
			desc,
			dateStr,
			atomDateFormats,
			cats,
			now,
		)
		if ok {
			items = append(items, parsed)
		}
	}

	return items, nil
}

// --- Normalization & Sanitization Helpers ---

func sanitizeFeedItem(
	rawGUID, rawTitle, rawLink, rawDesc, rawDate string,
	dateFormats []string,
	rawCategories []string,
	now time.Time,
) (application.FeedItem, bool) {
	// 1. Clean Title: strip HTML, unescape entities, trim
	title := cleanText(rawTitle)
	if title == "" {
		return application.FeedItem{}, false
	}
	if utf8.RuneCountInString(title) > MaxTitleRunes {
		runes := []rune(title)
		title = string(runes[:MaxTitleRunes])
	}

	// 2. Validate Link: must be HTTPS:443 only, valid URL, no credentials
	link := strings.TrimSpace(rawLink)
	u, err := url.Parse(link)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return application.FeedItem{}, false
	}
	if u.Port() != "" && u.Port() != "443" {
		return application.FeedItem{}, false
	}

	// 3. Normalize GUID: fallback to link if GUID is missing
	guid := strings.TrimSpace(rawGUID)
	if guid == "" {
		guid = link
	}

	// 4. Clean Excerpt: strip HTML, unescape, trim, bounded to 400 runes
	excerpt := cleanText(rawDesc)
	if utf8.RuneCountInString(excerpt) > MaxExcerptRunes {
		runes := []rune(excerpt)
		excerpt = string(runes[:MaxExcerptRunes])
	}

	// 5. Parse Publication Date
	pubDate := parseTimeWithFormats(rawDate, dateFormats)
	if pubDate.IsZero() {
		return application.FeedItem{}, false
	}
	// Future skew guard: reject timestamps more than 5 minutes ahead of clock
	if pubDate.After(now.Add(MaxFutureSkew)) {
		return application.FeedItem{}, false
	}

	// 6. Clean Categories
	cats := make([]string, 0, len(rawCategories))
	for _, c := range rawCategories {
		clean := cleanText(c)
		if clean != "" {
			cats = append(cats, clean)
		}
	}

	// 7. Enforce per-entry metadata ceiling (16 KiB)
	metadataSize := len(guid) + len(title) + len(link) + len(excerpt)
	if metadataSize > MaxEntryMetadataBytes {
		return application.FeedItem{}, false
	}

	return application.FeedItem{
		GUID:        guid,
		Title:       title,
		Link:        link,
		Excerpt:     excerpt,
		PublishedAt: pubDate.UTC(),
		Categories:  cats,
	}, true
}

func cleanText(s string) string {
	if s == "" {
		return ""
	}
	// Strip HTML tags
	noTags := htmlTagRegex.ReplaceAllString(s, " ")
	// Unescape HTML & XML entities (&amp;, &lt;, &gt;, &quot;, &#39;, etc.)
	unescaped := html.UnescapeString(noTags)
	// Normalize whitespace
	singleSpace := spaceRegex.ReplaceAllString(unescaped, " ")
	return strings.TrimSpace(singleSpace)
}

func parseTimeWithFormats(s string, formats []string) time.Time {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return time.Time{}
	}
	for _, layout := range formats {
		if t, err := time.Parse(layout, trimmed); err == nil {
			return t
		}
	}
	return time.Time{}
}
