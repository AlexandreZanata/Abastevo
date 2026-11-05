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
