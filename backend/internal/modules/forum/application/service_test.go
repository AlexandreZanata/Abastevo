package application

import (
	"context"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
)

type mockCategoryReader struct {
	categories []domain.CategoryItem
	version    int
	err        error
}

func (m *mockCategoryReader) ListCategories(ctx context.Context) ([]domain.CategoryItem, int, error) {
	return m.categories, m.version, m.err
}

type mockPostReader struct {
	post     *domain.Post
	feed     domain.FeedPage
	snapshot string
	err      error
}

func (m *mockPostReader) GetPostByID(ctx context.Context, id string) (*domain.Post, error) {
	return m.post, m.err
}

func (m *mockPostReader) ListPublicPosts(ctx context.Context, query ListPublicPostsQuery) (domain.FeedPage, error) {
	return m.feed, m.err
}

func (m *mockPostReader) GetFeedSnapshot(ctx context.Context) (string, error) {
	return m.snapshot, m.err
}

type fixedClock struct {
	now time.Time
}

func (c *fixedClock) Now() time.Time {
	return c.now
}

func TestForumServiceDefaultOff(t *testing.T) {
	cfg := domain.DefaultConfig() // All flags false
	clk := &fixedClock{now: time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)}
	svc := NewService(cfg, &mockCategoryReader{}, &mockPostReader{}, clk)

	ctx := context.Background()

	t.Run("categories_refused_when_reads_disabled", func(t *testing.T) {
		_, err := svc.GetCategories(ctx)
		if err != domain.ErrForumDisabled {
			t.Fatalf("GetCategories() error = %v, want ErrForumDisabled", err)
		}
	})

	t.Run("feed_refused_when_reads_disabled", func(t *testing.T) {
		_, err := svc.GetFeed(ctx, ListPublicPostsQuery{Limit: 20})
		if err != domain.ErrForumDisabled {
			t.Fatalf("GetFeed() error = %v, want ErrForumDisabled", err)
		}
	})

	t.Run("detail_refused_when_reads_disabled", func(t *testing.T) {
		_, err := svc.GetPostDetail(ctx, "any-id")
		if err != domain.ErrForumDisabled {
			t.Fatalf("GetPostDetail() error = %v, want ErrForumDisabled", err)
		}
	})
}

func TestForumServiceReadsEnabled(t *testing.T) {
	cfg := domain.Config{
		ReadsEnabled:    true,
		TaxonomyVersion: 1,
	}
	clk := &fixedClock{now: time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)}

	now := clk.Now()
	visiblePost := &domain.Post{
		ID:                  "post-123",
		Kind:                domain.KindEditorial,
		Category:            domain.CategoryFuels,
		Title:               "Noticia Editorial",
		Excerpt:             "Resumo",
		BodyText:            "Texto completo",
		SourceID:            "src-1",
		Source:              &domain.Source{Name: "Abastevo", RightsStatus: domain.RightsApproved},
		FirstPublishedAt:    &now,
		LocalCreatedAt:      now,
		LocalUpdatedAt:      now,
		FirstPublicationSeq: 1,
		PublicationState:    domain.StatePublished,
		Visibility:          domain.VisibilityVisible,
		Revision:            1,
		CommentsEnabled:     true,
	}

	catReader := &mockCategoryReader{
		categories: []domain.CategoryItem{
			{Code: domain.CategoryFuels, Name: "Combustiveis", Description: "Descricao"},
		},
		version: 1,
	}
	postReader := &mockPostReader{
		post: visiblePost,
		feed: domain.FeedPage{
			Items:      []domain.PostSummary{},
			NextCursor: nil,
			Snapshot:   "rev-1",
			FetchedAt:  now,
		},
		snapshot: "rev-1",
	}

	svc := NewService(cfg, catReader, postReader, clk)
	ctx := context.Background()

	t.Run("categories_succeed", func(t *testing.T) {
		res, err := svc.GetCategories(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res.Items) != 1 || res.TaxonomyVersion != 1 {
			t.Fatalf("unexpected result: %+v", res)
		}
	})

	t.Run("feed_succeeds", func(t *testing.T) {
		feed, err := svc.GetFeed(ctx, ListPublicPostsQuery{Limit: 20})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if feed.Snapshot != "rev-1" {
			t.Fatalf("feed snapshot = %q, want rev-1", feed.Snapshot)
		}
	})

	t.Run("detail_succeeds_for_visible_post", func(t *testing.T) {
		detail, err := svc.GetPostDetail(ctx, "post-123")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if detail.ID != "post-123" {
			t.Fatalf("detail ID = %q, want post-123", detail.ID)
		}
	})

	t.Run("detail_refused_for_hidden_post", func(t *testing.T) {
		hiddenPost := *visiblePost
		hiddenPost.Visibility = domain.VisibilityHidden
		hiddenPostReader := &mockPostReader{post: &hiddenPost}
		hiddenSvc := NewService(cfg, catReader, hiddenPostReader, clk)

		_, err := hiddenSvc.GetPostDetail(ctx, "post-123")
		if err != domain.ErrNotPubliclyVisible {
			t.Fatalf("GetPostDetail() error = %v, want ErrNotPubliclyVisible", err)
		}
	})
}
