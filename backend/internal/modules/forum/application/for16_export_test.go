package application_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
)

type exportTestMockStore struct {
	comments []domain.Comment
}

func (m *exportTestMockStore) CreateComment(ctx context.Context, cmd application.CreateCommentCommand) (application.CommentResult, error) {
	return application.CommentResult{}, errors.New("unimplemented")
}
func (m *exportTestMockStore) EditComment(ctx context.Context, cmd application.EditCommentCommand) (*domain.Comment, error) {
	return nil, errors.New("unimplemented")
}
func (m *exportTestMockStore) RemoveComment(ctx context.Context, cmd application.RemoveCommentCommand) (*domain.Comment, error) {
	return nil, errors.New("unimplemented")
}
func (m *exportTestMockStore) GetCommentByID(ctx context.Context, id string) (*domain.Comment, error) {
	return nil, errors.New("unimplemented")
}
func (m *exportTestMockStore) ListCommentsByPost(ctx context.Context, query application.ListPostCommentsQuery) ([]domain.Comment, error) {
	return nil, errors.New("unimplemented")
}
func (m *exportTestMockStore) ListRepliesByParent(ctx context.Context, query application.ListCommentRepliesQuery) ([]domain.Comment, error) {
	return nil, errors.New("unimplemented")
}
func (m *exportTestMockStore) PublishComment(ctx context.Context, commentID string) (*domain.Comment, error) {
	return nil, errors.New("unimplemented")
}
func (m *exportTestMockStore) RejectComment(ctx context.Context, commentID string) (*domain.Comment, error) {
	return nil, errors.New("unimplemented")
}
func (m *exportTestMockStore) ListCommentsByAccount(ctx context.Context, accountID string) ([]domain.Comment, error) {
	var out []domain.Comment
	for _, c := range m.comments {
		if c.AuthorAccountID == accountID {
			out = append(out, c)
		}
	}
	return out, nil
}
func (m *exportTestMockStore) EraseAccountBatch(ctx context.Context, accountID string, limit int) (bool, int, error) {
	return true, 0, nil
}

func TestExportAccount_RedactsToOwner(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	parentID := "11111111-1111-4111-8111-000000000001"

	authorA := "aaaaaaaa-aaaa-4aaa-8aaa-000000000001"
	authorB := "bbbbbbbb-bbbb-4bbb-8bbb-000000000002"

	store := &exportTestMockStore{
		comments: []domain.Comment{
			{
				ID:               parentID,
				PostID:           "99999999-9999-4999-8999-000000000001",
				AuthorAccountID:  authorA,
				PublicAlias:      "Piloto_SP",
				Depth:            0,
				Sequence:         1,
				Revision:         1,
				PublicationState: domain.StatePublished,
				Visibility:       domain.VisibilityVisible,
				BodyText:         "Comentario publicado do autor A",
				CreatedAt:        now.Add(-2 * time.Hour),
				UpdatedAt:        now.Add(-2 * time.Hour),
				PublishedAt:      &now,
			},
			{
				ID:               "22222222-2222-4222-8222-000000000002",
				PostID:           "99999999-9999-4999-8999-000000000001",
				AuthorAccountID:  authorA,
				ParentID:         &parentID,
				PublicAlias:      "Piloto_SP",
				Depth:            1,
				Sequence:         2,
				Revision:         1,
				PublicationState: domain.StatePendingReview,
				Visibility:       domain.VisibilityVisible,
				BodyText:         "Resposta pendente de revisao do autor A",
				CreatedAt:        now.Add(-1 * time.Hour),
				UpdatedAt:        now.Add(-1 * time.Hour),
			},
			{
				ID:               "33333333-3333-4333-8333-000000000003",
				PostID:           "99999999-9999-4999-8999-000000000001",
				AuthorAccountID:  authorB,
				PublicAlias:      "Outro_Usuario",
				Depth:            0,
				Sequence:         3,
				Revision:         1,
				PublicationState: domain.StatePublished,
				Visibility:       domain.VisibilityVisible,
				BodyText:         "Comentario publicado de outro autor B",
				CreatedAt:        now.Add(-30 * time.Minute),
				UpdatedAt:        now.Add(-30 * time.Minute),
				PublishedAt:      &now,
			},
		},
	}

	// 1. Export for Author A
	envA, err := application.ExportAccount(ctx, store, authorA, now)
	if err != nil {
		t.Fatalf("export author A: %v", err)
	}

	if envA.Format != application.ForumExportFormat {
		t.Errorf("expected format %s, got %s", application.ForumExportFormat, envA.Format)
	}
	if envA.AccountID != authorA {
		t.Errorf("expected account ID %s, got %s", authorA, envA.AccountID)
	}
	if envA.Footprint.TotalComments != 2 || len(envA.Footprint.Comments) != 2 {
		t.Fatalf("expected 2 comments for author A, got %d", envA.Footprint.TotalComments)
	}

	// Invariant: author B comment must never appear in author A export
	for _, c := range envA.Footprint.Comments {
		if c.ID == "33333333-3333-4333-8333-000000000003" {
			t.Errorf("privacy leak: author B comment found in author A export: %+v", c)
		}
	}

	// 2. Export for Author B
	envB, err := application.ExportAccount(ctx, store, authorB, now)
	if err != nil {
		t.Fatalf("export author B: %v", err)
	}
	if envB.Footprint.TotalComments != 1 || len(envB.Footprint.Comments) != 1 {
		t.Fatalf("expected 1 comment for author B, got %d", envB.Footprint.TotalComments)
	}
	if envB.Footprint.Comments[0].ID != "33333333-3333-4333-8333-000000000003" {
		t.Errorf("unexpected comment for author B: %+v", envB.Footprint.Comments[0])
	}
}

func TestExportAccount_DeterministicJSON(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 5, 14, 0, 0, 0, time.UTC)
	author := "aaaaaaaa-aaaa-4aaa-8aaa-000000000001"

	store := &exportTestMockStore{
		comments: []domain.Comment{
			{
				ID:               "bbbbbbbb-bbbb-4bbb-8bbb-000000000002",
				PostID:           "99999999-9999-4999-8999-000000000001",
				AuthorAccountID:  author,
				PublicAlias:      "Alias",
				Depth:            0,
				Sequence:         2,
				Revision:         1,
				PublicationState: domain.StatePublished,
				Visibility:       domain.VisibilityVisible,
				BodyText:         "Segundo comentario",
				CreatedAt:        now,
				UpdatedAt:        now,
			},
			{
				ID:               "aaaaaaaa-aaaa-4aaa-8aaa-000000000001",
				PostID:           "99999999-9999-4999-8999-000000000001",
				AuthorAccountID:  author,
				PublicAlias:      "Alias",
				Depth:            0,
				Sequence:         1,
				Revision:         1,
				PublicationState: domain.StatePublished,
				Visibility:       domain.VisibilityVisible,
				BodyText:         "Primeiro comentario",
				CreatedAt:        now,
				UpdatedAt:        now,
			},
		},
	}

	bytes1, err := application.ExportAccountBytes(ctx, store, author, now)
	if err != nil {
		t.Fatalf("first export: %v", err)
	}

	bytes2, err := application.ExportAccountBytes(ctx, store, author, now)
	if err != nil {
		t.Fatalf("second export: %v", err)
	}

	if !bytes.Equal(bytes1, bytes2) {
		t.Errorf("export bytes are not deterministic:\n1: %s\n2: %s", string(bytes1), string(bytes2))
	}

	// Verify comments are sorted deterministically by ID (aaaaaaaa before bbbbbbbb)
	env, err := application.ExportAccount(ctx, store, author, now)
	if err != nil {
		t.Fatalf("export account: %v", err)
	}
	if env.Footprint.Comments[0].ID > env.Footprint.Comments[1].ID {
		t.Errorf("comments are not sorted by ID in export footprint")
	}
}

func TestExportAccount_EmptyOwner(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	ghostAuthor := "99999999-0000-4000-8000-000000000000"

	store := &exportTestMockStore{comments: nil}

	env, err := application.ExportAccount(ctx, store, ghostAuthor, now)
	if err != nil {
		t.Fatalf("export ghost author: %v", err)
	}
	if env.Footprint.TotalComments != 0 {
		t.Errorf("expected 0 comments, got %d", env.Footprint.TotalComments)
	}
	if env.Footprint.Comments == nil {
		t.Errorf("expected non-nil empty slice for comments, got nil")
	}
}

func TestExportAccount_InvalidAccount(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	store := &exportTestMockStore{}

	if _, err := application.ExportAccount(ctx, store, "", now); !errors.Is(err, domain.ErrTargetInvalid) {
		t.Errorf("expected ErrTargetInvalid for empty account, got %v", err)
	}
	if _, err := application.ExportAccount(ctx, store, "   ", now); !errors.Is(err, domain.ErrTargetInvalid) {
		t.Errorf("expected ErrTargetInvalid for whitespace account, got %v", err)
	}
}
