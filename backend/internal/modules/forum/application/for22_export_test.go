package application_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
)

type for22MockFullStore struct {
	comments []domain.Comment
	topics   []domain.Post
}

func (m *for22MockFullStore) CreateComment(ctx context.Context, cmd application.CreateCommentCommand) (application.CommentResult, error) {
	return application.CommentResult{}, nil
}

func (m *for22MockFullStore) EditComment(ctx context.Context, cmd application.EditCommentCommand) (*domain.Comment, error) {
	return nil, nil
}

func (m *for22MockFullStore) RemoveComment(ctx context.Context, cmd application.RemoveCommentCommand) (*domain.Comment, error) {
	return nil, nil
}

func (m *for22MockFullStore) GetCommentByID(ctx context.Context, id string) (*domain.Comment, error) {
	return nil, nil
}

func (m *for22MockFullStore) ListCommentsByPost(ctx context.Context, query application.ListPostCommentsQuery) ([]domain.Comment, error) {
	return nil, nil
}

func (m *for22MockFullStore) ListRepliesByParent(ctx context.Context, query application.ListCommentRepliesQuery) ([]domain.Comment, error) {
	return nil, nil
}

func (m *for22MockFullStore) PublishComment(ctx context.Context, commentID string) (*domain.Comment, error) {
	return nil, nil
}

func (m *for22MockFullStore) RejectComment(ctx context.Context, commentID string) (*domain.Comment, error) {
	return nil, nil
}

func (m *for22MockFullStore) ListCommentsByAccount(ctx context.Context, accountID string) ([]domain.Comment, error) {
	out := make([]domain.Comment, 0)
	for _, c := range m.comments {
		if c.AuthorAccountID == accountID {
			out = append(out, c)
		}
	}
	return out, nil
}

func (m *for22MockFullStore) EraseAccountBatch(ctx context.Context, accountID string, limit int) (bool, int, error) {
	return true, 0, nil
}

func (m *for22MockFullStore) ListTopicsByAccount(ctx context.Context, accountID string) ([]domain.Post, error) {
	out := make([]domain.Post, 0)
	for _, t := range m.topics {
		if t.AuthorAccountID == accountID {
			out = append(out, t)
		}
	}
	return out, nil
}

func TestFOR22_ExportAccount_TopicsAndCommentsIsolation(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)

	authorA := "11111111-1111-4111-8111-000000000001"
	authorB := "22222222-2222-4222-8222-000000000002"

	ufSP := "SP"
	ibgeSP := 3550308
	fuelGas := "GASOLINA_COMUM"

	store := &for22MockFullStore{
		topics: []domain.Post{
			{
				ID:               "topic-a2",
				Kind:             domain.KindTopic,
				Category:         domain.CategoryFuels,
				Title:            "Topico 2 do Autor A",
				Excerpt:          "Resumo 2",
				BodyText:         "Corpo do topico 2",
				AuthorAccountID:  authorA,
				PublicationState: domain.StatePublished,
				Visibility:       domain.VisibilityVisible,
				Revision:         1,
				Region: &domain.Region{
					UF:               ufSP,
					MunicipalityIBGE: &ibgeSP,
				},
				FuelProduct:      &fuelGas,
				LocalCreatedAt:   now,
				LocalUpdatedAt:   now,
				FirstPublishedAt: &now,
			},
			{
				ID:               "topic-a1",
				Kind:             domain.KindTopic,
				Category:         domain.CategoryVehiclesTech,
				Title:            "Topico 1 do Autor A",
				Excerpt:          "Resumo 1",
				BodyText:         "Corpo do topico 1",
				AuthorAccountID:  authorA,
				PublicationState: domain.StatePendingReview,
				Visibility:       domain.VisibilityVisible,
				Revision:         1,
				LocalCreatedAt:   now,
				LocalUpdatedAt:   now,
			},
			{
				ID:               "topic-b1",
				Kind:             domain.KindTopic,
				Category:         domain.CategoryFuels,
				Title:            "Topico do Autor B",
				Excerpt:          "Resumo B",
				BodyText:         "Corpo B",
				AuthorAccountID:  authorB,
				PublicationState: domain.StatePublished,
				Visibility:       domain.VisibilityVisible,
				Revision:         1,
				LocalCreatedAt:   now,
				LocalUpdatedAt:   now,
			},
		},
		comments: []domain.Comment{
			{
				ID:               "comment-a1",
				PostID:           "topic-a1",
				AuthorAccountID:  authorA,
				PublicAlias:      "Alias A",
				Depth:            0,
				Sequence:         1,
				Revision:         1,
				PublicationState: domain.StatePublished,
				Visibility:       domain.VisibilityVisible,
				BodyText:         "Comentario A",
				CreatedAt:        now,
				UpdatedAt:        now,
			},
			{
				ID:               "comment-b1",
				PostID:           "topic-a1",
				AuthorAccountID:  authorB,
				PublicAlias:      "Alias B",
				Depth:            1,
				Sequence:         2,
				Revision:         1,
				PublicationState: domain.StatePublished,
				Visibility:       domain.VisibilityVisible,
				BodyText:         "Comentario B de resposta",
				CreatedAt:        now,
				UpdatedAt:        now,
			},
		},
	}

	// 1. Export Author A
	envA, err := application.ExportAccount(ctx, store, authorA, now)
	if err != nil {
		t.Fatalf("export author A: %v", err)
	}

	if envA.Footprint.TotalTopics != 2 {
		t.Fatalf("expected 2 topics for author A, got %d", envA.Footprint.TotalTopics)
	}
	if envA.Footprint.TotalComments != 1 {
		t.Fatalf("expected 1 comment for author A, got %d", envA.Footprint.TotalComments)
	}

	// Deterministic sorting verification: topic-a1 before topic-a2
	if envA.Footprint.Topics[0].ID != "topic-a1" || envA.Footprint.Topics[1].ID != "topic-a2" {
		t.Errorf("topics not sorted deterministically by ID: %v, %v", envA.Footprint.Topics[0].ID, envA.Footprint.Topics[1].ID)
	}

	// Field content verification
	top2 := envA.Footprint.Topics[1]
	if top2.Category != "FUELS" {
		t.Errorf("expected Category FUELS, got %s", top2.Category)
	}
	if top2.RegionUf == nil || *top2.RegionUf != "SP" {
		t.Errorf("expected RegionUf SP, got %v", top2.RegionUf)
	}
	if top2.RegionMunicipalityIbge == nil || *top2.RegionMunicipalityIbge != 3550308 {
		t.Errorf("expected RegionMunicipalityIbge 3550308, got %v", top2.RegionMunicipalityIbge)
	}
	if top2.FuelProduct == nil || *top2.FuelProduct != "GASOLINA_COMUM" {
		t.Errorf("expected FuelProduct GASOLINA_COMUM, got %v", top2.FuelProduct)
	}

	// Author B's data must never leak into Author A's export
	for _, top := range envA.Footprint.Topics {
		if top.ID == "topic-b1" {
			t.Errorf("privacy leak: topic-b1 found in author A export: %+v", top)
		}
	}
	for _, c := range envA.Footprint.Comments {
		if c.ID == "comment-b1" {
			t.Errorf("privacy leak: comment-b1 found in author A export: %+v", c)
		}
	}

	// 2. Export Author B
	envB, err := application.ExportAccount(ctx, store, authorB, now)
	if err != nil {
		t.Fatalf("export author B: %v", err)
	}
	if envB.Footprint.TotalTopics != 1 || envB.Footprint.Topics[0].ID != "topic-b1" {
		t.Errorf("unexpected topics for author B: %+v", envB.Footprint.Topics)
	}
	if envB.Footprint.TotalComments != 1 || envB.Footprint.Comments[0].ID != "comment-b1" {
		t.Errorf("unexpected comments for author B: %+v", envB.Footprint.Comments)
	}
}

func TestFOR22_ExportAccount_DeterministicJSON(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	author := "11111111-1111-4111-8111-000000000001"

	store := &for22MockFullStore{
		topics: []domain.Post{
			{
				ID:               "topic-z",
				Kind:             domain.KindTopic,
				Category:         domain.CategoryFuels,
				Title:            "Z Topic",
				AuthorAccountID:  author,
				PublicationState: domain.StatePublished,
				Visibility:       domain.VisibilityVisible,
				Revision:         1,
				LocalCreatedAt:   now,
				LocalUpdatedAt:   now,
			},
			{
				ID:               "topic-a",
				Kind:             domain.KindTopic,
				Category:         domain.CategoryVehiclesTech,
				Title:            "A Topic",
				AuthorAccountID:  author,
				PublicationState: domain.StatePublished,
				Visibility:       domain.VisibilityVisible,
				Revision:         1,
				LocalCreatedAt:   now,
				LocalUpdatedAt:   now,
			},
		},
	}

	bytes1, err := application.ExportAccountBytes(ctx, store, author, now)
	if err != nil {
		t.Fatalf("export bytes 1: %v", err)
	}
	bytes2, err := application.ExportAccountBytes(ctx, store, author, now)
	if err != nil {
		t.Fatalf("export bytes 2: %v", err)
	}

	if !bytes.Equal(bytes1, bytes2) {
		t.Errorf("export bytes are not byte-for-byte identical:\n1: %s\n2: %s", string(bytes1), string(bytes2))
	}
}
