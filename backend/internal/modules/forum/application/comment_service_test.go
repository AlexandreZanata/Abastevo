package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
)

type mockCommentStore struct {
	createFn       func(ctx context.Context, cmd CreateCommentCommand) (CommentResult, error)
	editFn         func(ctx context.Context, cmd EditCommentCommand) (*domain.Comment, error)
	removeFn       func(ctx context.Context, cmd RemoveCommentCommand) (*domain.Comment, error)
	getByIDFn      func(ctx context.Context, id string) (*domain.Comment, error)
	listByPostFn   func(ctx context.Context, query ListPostCommentsQuery) ([]domain.Comment, error)
	listByParentFn func(ctx context.Context, query ListCommentRepliesQuery) ([]domain.Comment, error)
	publishFn      func(ctx context.Context, id string) (*domain.Comment, error)
	rejectFn       func(ctx context.Context, id string) (*domain.Comment, error)
}

func (m *mockCommentStore) CreateComment(ctx context.Context, cmd CreateCommentCommand) (CommentResult, error) {
	if m.createFn != nil {
		return m.createFn(ctx, cmd)
	}
	return CommentResult{}, errors.New("unimplemented")
}

func (m *mockCommentStore) EditComment(ctx context.Context, cmd EditCommentCommand) (*domain.Comment, error) {
	if m.editFn != nil {
		return m.editFn(ctx, cmd)
	}
	return nil, errors.New("unimplemented")
}

func (m *mockCommentStore) RemoveComment(ctx context.Context, cmd RemoveCommentCommand) (*domain.Comment, error) {
	if m.removeFn != nil {
		return m.removeFn(ctx, cmd)
	}
	return nil, errors.New("unimplemented")
}

func (m *mockCommentStore) GetCommentByID(ctx context.Context, id string) (*domain.Comment, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, errors.New("unimplemented")
}

func (m *mockCommentStore) ListCommentsByPost(ctx context.Context, query ListPostCommentsQuery) ([]domain.Comment, error) {
	if m.listByPostFn != nil {
		return m.listByPostFn(ctx, query)
	}
	return nil, errors.New("unimplemented")
}

func (m *mockCommentStore) ListRepliesByParent(ctx context.Context, query ListCommentRepliesQuery) ([]domain.Comment, error) {
	if m.listByParentFn != nil {
		return m.listByParentFn(ctx, query)
	}
	return nil, errors.New("unimplemented")
}

func (m *mockCommentStore) PublishComment(ctx context.Context, id string) (*domain.Comment, error) {
	if m.publishFn != nil {
		return m.publishFn(ctx, id)
	}
	return nil, errors.New("unimplemented")
}

func (m *mockCommentStore) RejectComment(ctx context.Context, id string) (*domain.Comment, error) {
	if m.rejectFn != nil {
		return m.rejectFn(ctx, id)
	}
	return nil, errors.New("unimplemented")
}



func TestCommentService_CreateComment_Validation(t *testing.T) {
	store := &mockCommentStore{
		createFn: func(ctx context.Context, cmd CreateCommentCommand) (CommentResult, error) {
			return CommentResult{
				Comment: &domain.Comment{
					ID:               "11111111-1111-4111-8111-111111111111",
					PostID:           cmd.PostID,
					AuthorAccountID:  cmd.AuthorAccountID,
					PublicAlias:      cmd.PublicAlias,
					BodyText:         cmd.BodyText,
					Sequence:         1,
					Revision:         1,
					PublicationState: domain.StatePendingReview,
					Visibility:       domain.VisibilityVisible,
				},
				Replayed: false,
			}, nil
		},
	}
	svc := NewCommentService(store, &fixedClock{now: time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)})
	ctx := context.Background()

	// Missing PostID
	_, err := svc.CreateComment(ctx, CreateCommentCommand{
		AuthorAccountID: "22222222-2222-4222-8222-222222222222",
		BodyText:        "Comentário válido",
		OperationID:     "33333333-3333-4333-8333-333333333333",
	})
	if !errors.Is(err, domain.ErrPostNotFound) {
		t.Errorf("expected ErrPostNotFound, got %v", err)
	}

	// Missing AuthorAccountID
	_, err = svc.CreateComment(ctx, CreateCommentCommand{
		PostID:      "44444444-4444-4444-8444-444444444444",
		BodyText:    "Comentário válido",
		OperationID: "33333333-3333-4333-8333-333333333333",
	})
	if !errors.Is(err, domain.ErrMissingAuthor) {
		t.Errorf("expected ErrMissingAuthor, got %v", err)
	}

	// Missing OperationID
	_, err = svc.CreateComment(ctx, CreateCommentCommand{
		PostID:          "44444444-4444-4444-8444-444444444444",
		AuthorAccountID: "22222222-2222-4222-8222-222222222222",
		BodyText:        "Comentário válido",
	})
	if err == nil || !strings.Contains(err.Error(), "missing operation id") {
		t.Errorf("expected missing operation id error, got %v", err)
	}

	// Empty body
	_, err = svc.CreateComment(ctx, CreateCommentCommand{
		PostID:          "44444444-4444-4444-8444-444444444444",
		AuthorAccountID: "22222222-2222-4222-8222-222222222222",
		BodyText:        "   ",
		OperationID:     "33333333-3333-4333-8333-333333333333",
	})
	if !errors.Is(err, domain.ErrCommentBodyEmpty) {
		t.Errorf("expected ErrCommentBodyEmpty, got %v", err)
	}

	// Body exceeds 280 code points
	_, err = svc.CreateComment(ctx, CreateCommentCommand{
		PostID:          "44444444-4444-4444-8444-444444444444",
		AuthorAccountID: "22222222-2222-4222-8222-222222222222",
		BodyText:        strings.Repeat("X", 281),
		OperationID:     "33333333-3333-4333-8333-333333333333",
	})
	if !errors.Is(err, domain.ErrCommentBodyTooLong) {
		t.Errorf("expected ErrCommentBodyTooLong, got %v", err)
	}

	// Valid creation
	res, err := svc.CreateComment(ctx, CreateCommentCommand{
		PostID:          "44444444-4444-4444-8444-444444444444",
		AuthorAccountID: "22222222-2222-4222-8222-222222222222",
		PublicAlias:     "Motorista123",
		BodyText:        "Comentário válido e revisado",
		OperationID:     "33333333-3333-4333-8333-333333333333",
	})
	if err != nil {
		t.Fatalf("unexpected error on valid create: %v", err)
	}
	if res.Comment == nil || res.Comment.PublicationState != domain.StatePendingReview {
		t.Errorf("expected comment in pending_review state, got %v", res.Comment)
	}
}

func TestCommentService_EditComment_Validation(t *testing.T) {
	store := &mockCommentStore{
		editFn: func(ctx context.Context, cmd EditCommentCommand) (*domain.Comment, error) {
			return &domain.Comment{
				ID:               cmd.CommentID,
				AuthorAccountID:  cmd.AuthorAccountID,
				BodyText:         cmd.BodyText,
				Revision:         cmd.ExpectedRevision + 1,
				PublicationState: domain.StatePendingReview,
				Visibility:       domain.VisibilityVisible,
			}, nil
		},
	}
	svc := NewCommentService(store, nil)
	ctx := context.Background()

	// Missing CommentID
	_, err := svc.EditComment(ctx, EditCommentCommand{
		AuthorAccountID:  "22222222-2222-4222-8222-222222222222",
		ExpectedRevision: 1,
		BodyText:         "Edição válida",
	})
	if !errors.Is(err, domain.ErrCommentNotFound) {
		t.Errorf("expected ErrCommentNotFound, got %v", err)
	}

	// Missing Author
	_, err = svc.EditComment(ctx, EditCommentCommand{
		CommentID:        "11111111-1111-4111-8111-111111111111",
		ExpectedRevision: 1,
		BodyText:         "Edição válida",
	})
	if !errors.Is(err, domain.ErrMissingAuthor) {
		t.Errorf("expected ErrMissingAuthor, got %v", err)
	}

	// Invalid ExpectedRevision
	_, err = svc.EditComment(ctx, EditCommentCommand{
		CommentID:        "11111111-1111-4111-8111-111111111111",
		AuthorAccountID:  "22222222-2222-4222-8222-222222222222",
		ExpectedRevision: 0,
		BodyText:         "Edição válida",
	})
	if !errors.Is(err, domain.ErrRevisionMismatch) {
		t.Errorf("expected ErrRevisionMismatch, got %v", err)
	}

	// Valid edit
	edited, err := svc.EditComment(ctx, EditCommentCommand{
		CommentID:        "11111111-1111-4111-8111-111111111111",
		AuthorAccountID:  "22222222-2222-4222-8222-222222222222",
		ExpectedRevision: 1,
		BodyText:         "Texto editado e pronto para revisão",
	})
	if err != nil {
		t.Fatalf("unexpected error on valid edit: %v", err)
	}
	if edited.Revision != 2 || edited.PublicationState != domain.StatePendingReview {
		t.Errorf("expected revision 2 and pending_review state, got rev=%d state=%s", edited.Revision, edited.PublicationState)
	}
}

func TestCommentService_RemoveComment_Validation(t *testing.T) {
	store := &mockCommentStore{
		removeFn: func(ctx context.Context, cmd RemoveCommentCommand) (*domain.Comment, error) {
			return &domain.Comment{
				ID:               cmd.CommentID,
				AuthorAccountID:  cmd.AuthorAccountID,
				BodyText:         "",
				Revision:         cmd.ExpectedRevision + 1,
				PublicationState: domain.StateRemoved,
				Visibility:       domain.VisibilityHidden,
			}, nil
		},
	}
	svc := NewCommentService(store, nil)
	ctx := context.Background()

	// Missing CommentID
	_, err := svc.RemoveComment(ctx, RemoveCommentCommand{
		AuthorAccountID:  "22222222-2222-4222-8222-222222222222",
		ExpectedRevision: 1,
	})
	if !errors.Is(err, domain.ErrCommentNotFound) {
		t.Errorf("expected ErrCommentNotFound, got %v", err)
	}

	// Missing Author
	_, err = svc.RemoveComment(ctx, RemoveCommentCommand{
		CommentID:        "11111111-1111-4111-8111-111111111111",
		ExpectedRevision: 1,
	})
	if !errors.Is(err, domain.ErrMissingAuthor) {
		t.Errorf("expected ErrMissingAuthor, got %v", err)
	}

	// Invalid ExpectedRevision
	_, err = svc.RemoveComment(ctx, RemoveCommentCommand{
		CommentID:        "11111111-1111-4111-8111-111111111111",
		AuthorAccountID:  "22222222-2222-4222-8222-222222222222",
		ExpectedRevision: -1,
	})
	if !errors.Is(err, domain.ErrRevisionMismatch) {
		t.Errorf("expected ErrRevisionMismatch, got %v", err)
	}

	// Valid remove
	removed, err := svc.RemoveComment(ctx, RemoveCommentCommand{
		CommentID:        "11111111-1111-4111-8111-111111111111",
		AuthorAccountID:  "22222222-2222-4222-8222-222222222222",
		ExpectedRevision: 1,
	})
	if err != nil {
		t.Fatalf("unexpected error on valid remove: %v", err)
	}
	if removed.PublicationState != domain.StateRemoved || removed.Visibility != domain.VisibilityHidden || removed.BodyText != "" {
		t.Errorf("expected tombstoned comment, got %+v", removed)
	}
}
