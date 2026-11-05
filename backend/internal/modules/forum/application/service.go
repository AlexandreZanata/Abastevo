package application

import (
	"context"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
)

// Service coordinates read use cases for Forum Abastevo.
type Service struct {
	cfg            domain.Config
	categoryReader CategoryReader
	postReader     PostReader
	clock          Clock
}

// NewService creates a new forum application service.
func NewService(
	cfg domain.Config,
	categoryReader CategoryReader,
	postReader PostReader,
	clock Clock,
) *Service {
	return &Service{
		cfg:            cfg,
		categoryReader: categoryReader,
		postReader:     postReader,
		clock:          clock,
	}
}

// GetCategories returns active forum categories, checking the read feature gate.
func (s *Service) GetCategories(ctx context.Context) (CategoriesResult, error) {
	if !s.cfg.ReadsAllowed() {
		return CategoriesResult{}, domain.ErrForumDisabled
	}
	items, version, err := s.categoryReader.ListCategories(ctx)
	if err != nil {
		return CategoriesResult{}, err
	}
	return CategoriesResult{
		Items:           items,
		TaxonomyVersion: version,
	}, nil
}

// GetFeed returns a paginated page of public post summaries.
func (s *Service) GetFeed(ctx context.Context, query ListPublicPostsQuery) (domain.FeedPage, error) {
	if !s.cfg.ReadsAllowed() {
		return domain.FeedPage{}, domain.ErrForumDisabled
	}

	if query.Limit <= 0 {
		query.Limit = 20
	} else if query.Limit > 50 {
		query.Limit = 50
	}

	page, err := s.postReader.ListPublicPosts(ctx, query)
	if err != nil {
		return domain.FeedPage{}, err
	}

	if page.FetchedAt.IsZero() && s.clock != nil {
		page.FetchedAt = s.clock.Now()
	}

	return page, nil
}

// GetPostDetail returns public detail for a single post.
func (s *Service) GetPostDetail(ctx context.Context, id string) (*domain.PostDetail, error) {
	if !s.cfg.ReadsAllowed() {
		return nil, domain.ErrForumDisabled
	}

	post, err := s.postReader.GetPostByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if post == nil {
		return nil, domain.ErrPostNotFound
	}

	return post.ToDetail()
}
