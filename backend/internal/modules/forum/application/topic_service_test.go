package application

import (
	"context"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
)

type mockTopicStore struct {
	createFunc func(ctx context.Context, cmd CreateTopicCommand) (TopicResult, error)
	editFunc   func(ctx context.Context, cmd EditTopicCommand) (*domain.Post, error)
	removeFunc func(ctx context.Context, cmd RemoveTopicCommand) (*domain.Post, error)
	getFunc    func(ctx context.Context, id string) (*domain.Post, error)
}

func (m *mockTopicStore) CreateTopic(ctx context.Context, cmd CreateTopicCommand) (TopicResult, error) {
	if m.createFunc != nil {
		return m.createFunc(ctx, cmd)
	}
	return TopicResult{}, nil
}

func (m *mockTopicStore) EditTopic(ctx context.Context, cmd EditTopicCommand) (*domain.Post, error) {
	if m.editFunc != nil {
		return m.editFunc(ctx, cmd)
	}
	return nil, nil
}

func (m *mockTopicStore) RemoveTopic(ctx context.Context, cmd RemoveTopicCommand) (*domain.Post, error) {
	if m.removeFunc != nil {
		return m.removeFunc(ctx, cmd)
	}
	return nil, nil
}

func (m *mockTopicStore) GetTopicByID(ctx context.Context, id string) (*domain.Post, error) {
	if m.getFunc != nil {
		return m.getFunc(ctx, id)
	}
	return nil, nil
}

func TestTopicService_CreateTopic(t *testing.T) {
	ctx := context.Background()
	authorID := "11111111-1111-4111-8111-111111111111"
	opID := "22222222-2222-4222-8222-222222222222"

	t.Run("valid creation delegates to store with trimmed text", func(t *testing.T) {
		var receivedCmd CreateTopicCommand
		store := &mockTopicStore{
			createFunc: func(ctx context.Context, cmd CreateTopicCommand) (TopicResult, error) {
				receivedCmd = cmd
				return TopicResult{
					Topic: &domain.Post{
						ID:               "topic-1",
						Kind:             domain.KindTopic,
						Category:         cmd.Category,
						Title:            cmd.Title,
						BodyText:         cmd.BodyText,
						AuthorAccountID:  cmd.AuthorAccountID,
						PublicationState: domain.StatePendingReview,
						Visibility:       domain.VisibilityVisible,
						Revision:         1,
						LocalCreatedAt:   time.Now().UTC(),
					},
					Replayed: false,
				}, nil
			},
		}

		service := NewTopicService(store, nil)
		res, err := service.CreateTopic(ctx, CreateTopicCommand{
			AuthorAccountID: authorID,
			Category:        domain.CategoryFuels,
			Title:           "  Preço do diesel no Paraná  ",
			BodyText:        "  Relato sobre a média de preços nos postos de Curitiba.  ",
			OperationID:     opID,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Replayed {
			t.Errorf("expected Replayed = false")
		}
		if receivedCmd.Title != "Preço do diesel no Paraná" {
			t.Errorf("expected trimmed title, got %q", receivedCmd.Title)
		}
		if receivedCmd.BodyText != "Relato sobre a média de preços nos postos de Curitiba." {
			t.Errorf("expected trimmed body, got %q", receivedCmd.BodyText)
		}
	})

	t.Run("missing author rejects", func(t *testing.T) {
		service := NewTopicService(&mockTopicStore{}, nil)
		_, err := service.CreateTopic(ctx, CreateTopicCommand{
			AuthorAccountID: "",
			Category:        domain.CategoryFuels,
			Title:           "Título",
			BodyText:        "Corpo",
			OperationID:     opID,
		})
		if err != domain.ErrMissingAuthor {
			t.Fatalf("expected ErrMissingAuthor, got %v", err)
		}
	})

	t.Run("missing operation id rejects", func(t *testing.T) {
		service := NewTopicService(&mockTopicStore{}, nil)
		_, err := service.CreateTopic(ctx, CreateTopicCommand{
			AuthorAccountID: authorID,
			Category:        domain.CategoryFuels,
			Title:           "Título",
			BodyText:        "Corpo",
			OperationID:     "",
		})
		if err == nil {
			t.Fatalf("expected error for missing operation id, got nil")
		}
	})

	t.Run("empty title rejects", func(t *testing.T) {
		service := NewTopicService(&mockTopicStore{}, nil)
		_, err := service.CreateTopic(ctx, CreateTopicCommand{
			AuthorAccountID: authorID,
			Category:        domain.CategoryFuels,
			Title:           "   ",
			BodyText:        "Corpo válido",
			OperationID:     opID,
		})
		if err != domain.ErrTopicTitleEmpty {
			t.Fatalf("expected ErrTopicTitleEmpty, got %v", err)
		}
	})
}

func TestTopicService_EditTopic(t *testing.T) {
	ctx := context.Background()
	authorID := "11111111-1111-4111-8111-111111111111"
	topicID := "22222222-2222-4222-8222-222222222222"

	t.Run("valid edit delegates with trimmed text", func(t *testing.T) {
		var receivedCmd EditTopicCommand
		store := &mockTopicStore{
			editFunc: func(ctx context.Context, cmd EditTopicCommand) (*domain.Post, error) {
				receivedCmd = cmd
				return &domain.Post{
					ID:               topicID,
					Title:            cmd.Title,
					BodyText:         cmd.BodyText,
					Revision:         cmd.ExpectedRevision + 1,
					PublicationState: domain.StatePendingReview,
				}, nil
			},
		}

		service := NewTopicService(store, nil)
		post, err := service.EditTopic(ctx, EditTopicCommand{
			TopicID:          topicID,
			AuthorAccountID:  authorID,
			ExpectedRevision: 1,
			Category:         domain.CategoryFuels,
			Title:            "  Título editado  ",
			BodyText:         "  Corpo editado com atualização.  ",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if post.Revision != 2 {
			t.Errorf("expected revision 2, got %d", post.Revision)
		}
		if receivedCmd.Title != "Título editado" {
			t.Errorf("expected trimmed title, got %q", receivedCmd.Title)
		}
	})

	t.Run("missing topic id rejects", func(t *testing.T) {
		service := NewTopicService(&mockTopicStore{}, nil)
		_, err := service.EditTopic(ctx, EditTopicCommand{
			TopicID:          "",
			AuthorAccountID:  authorID,
			ExpectedRevision: 1,
			Category:         domain.CategoryFuels,
			Title:            "Título",
			BodyText:         "Corpo",
		})
		if err != domain.ErrTopicNotFound {
			t.Fatalf("expected ErrTopicNotFound, got %v", err)
		}
	})

	t.Run("stale revision rejects", func(t *testing.T) {
		service := NewTopicService(&mockTopicStore{}, nil)
		_, err := service.EditTopic(ctx, EditTopicCommand{
			TopicID:          topicID,
			AuthorAccountID:  authorID,
			ExpectedRevision: 0,
			Category:         domain.CategoryFuels,
			Title:            "Título",
			BodyText:         "Corpo",
		})
		if err != domain.ErrRevisionMismatch {
			t.Fatalf("expected ErrRevisionMismatch, got %v", err)
		}
	})
}

func TestTopicService_RemoveTopic(t *testing.T) {
	ctx := context.Background()
	authorID := "11111111-1111-4111-8111-111111111111"
	topicID := "22222222-2222-4222-8222-222222222222"

	t.Run("valid remove delegates to store", func(t *testing.T) {
		called := false
		store := &mockTopicStore{
			removeFunc: func(ctx context.Context, cmd RemoveTopicCommand) (*domain.Post, error) {
				called = true
				return &domain.Post{
					ID:               topicID,
					PublicationState: domain.StateRemoved,
					Visibility:       domain.VisibilityHidden,
					Revision:         2,
				}, nil
			},
		}

		service := NewTopicService(store, nil)
		post, err := service.RemoveTopic(ctx, RemoveTopicCommand{
			TopicID:          topicID,
			AuthorAccountID:  authorID,
			ExpectedRevision: 1,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !called {
			t.Errorf("expected removeFunc to be called")
		}
		if post.PublicationState != domain.StateRemoved {
			t.Errorf("expected state REMOVED, got %s", post.PublicationState)
		}
	})
}
