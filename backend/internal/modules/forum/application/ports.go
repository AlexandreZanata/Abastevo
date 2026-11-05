package application

import (
	"context"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
)

// Clock abstracts system time for deterministic testing.
type Clock interface {
	Now() time.Time
}

// SystemClock implements Clock using time.Now().UTC().
type SystemClock struct{}

// Now returns current UTC time.
func (SystemClock) Now() time.Time {
	return time.Now().UTC()
}

// IDGenerator abstracts UUID generation.
type IDGenerator interface {
	NewID() string
}

// ListPublicPostsQuery represents input parameters for feed queries.
type ListPublicPostsQuery struct {
	Category *domain.Category
	Kind     *domain.PostKind
	Limit    int
	Cursor   *string
}

// CategoriesResult represents categories with current taxonomy version.
type CategoriesResult struct {
	Items           []domain.CategoryItem
	TaxonomyVersion int
}

// CategoryReader defines read access to forum categories.
type CategoryReader interface {
	ListCategories(ctx context.Context) ([]domain.CategoryItem, int, error)
}

// PostReader defines read access to forum posts and feed snapshots.
type PostReader interface {
	GetPostByID(ctx context.Context, id string) (*domain.Post, error)
	ListPublicPosts(ctx context.Context, query ListPublicPostsQuery) (domain.FeedPage, error)
	GetFeedSnapshot(ctx context.Context) (string, error)
}

// FeedItem represents a candidate publication parsed from an external feed.
type FeedItem struct {
	GUID        string
	Title       string
	Link        string
	Excerpt     string
	PublishedAt time.Time
	Categories  []string
}

// FetchRequest represents bounded parameters for a conditional feed fetch.
type FetchRequest struct {
	SourceID     string
	URL          string
	ETag         string
	LastModified string
}

// FetchResponse represents the result of a conditional feed fetch.
type FetchResponse struct {
	NotModified  bool
	StatusCode   int
	ETag         string
	LastModified string
	Body         []byte
}

// SourceFetcher abstracts safe network retrieval of external candidate feeds.
type SourceFetcher interface {
	Fetch(ctx context.Context, req FetchRequest) (FetchResponse, error)
}

// FeedParser abstracts decoding and sanitization of raw RSS/Atom feeds.
type FeedParser interface {
	Parse(raw []byte, now time.Time) ([]FeedItem, error)
}
