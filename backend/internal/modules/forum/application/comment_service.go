package application

import (
	"context"
	"fmt"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
)

// CommentService coordinates discussion comment and reply use cases.
type CommentService struct {
	store CommentStore
	clock Clock
}

// NewCommentService creates a new CommentService.
func NewCommentService(store CommentStore, clock Clock) *CommentService {
	if clock == nil {
		clock = SystemClock{}
	}
	return &CommentService{
		store: store,
		clock: clock,
	}
}

// CreateComment validates input text and creates or replays a discussion comment.
func (s *CommentService) CreateComment(ctx context.Context, cmd CreateCommentCommand) (CommentResult, error) {
	if cmd.PostID == "" {
		return CommentResult{}, domain.ErrPostNotFound
	}
	if cmd.AuthorAccountID == "" {
		return CommentResult{}, domain.ErrMissingAuthor
	}
	if cmd.OperationID == "" {
		return CommentResult{}, fmt.Errorf("forum: missing operation id")
	}

	cleanBody, err := domain.ValidateCommentBody(cmd.BodyText)
	if err != nil {
		return CommentResult{}, err
	}
	cmd.BodyText = cleanBody

	return s.store.CreateComment(ctx, cmd)
}

// EditComment validates input text and performs a CAS edit on an authored comment.
func (s *CommentService) EditComment(ctx context.Context, cmd EditCommentCommand) (*domain.Comment, error) {
	if cmd.CommentID == "" {
		return nil, domain.ErrCommentNotFound
	}
	if cmd.AuthorAccountID == "" {
		return nil, domain.ErrMissingAuthor
	}
	if cmd.ExpectedRevision < 1 {
		return nil, domain.ErrRevisionMismatch
	}

	cleanBody, err := domain.ValidateCommentBody(cmd.BodyText)
	if err != nil {
		return nil, err
	}
	cmd.BodyText = cleanBody

	return s.store.EditComment(ctx, cmd)
}

// RemoveComment performs an authenticated tombstone removal on an authored comment.
func (s *CommentService) RemoveComment(ctx context.Context, cmd RemoveCommentCommand) (*domain.Comment, error) {
	if cmd.CommentID == "" {
		return nil, domain.ErrCommentNotFound
	}
	if cmd.AuthorAccountID == "" {
		return nil, domain.ErrMissingAuthor
	}
	if cmd.ExpectedRevision < 1 {
		return nil, domain.ErrRevisionMismatch
	}

	return s.store.RemoveComment(ctx, cmd)
}

// GetCommentByID retrieves a comment by its UUID.
func (s *CommentService) GetCommentByID(ctx context.Context, id string) (*domain.Comment, error) {
	if id == "" {
		return nil, domain.ErrCommentNotFound
	}
	return s.store.GetCommentByID(ctx, id)
}

// ListCommentsByPost retrieves visible published comments for a post.
func (s *CommentService) ListCommentsByPost(ctx context.Context, query ListPostCommentsQuery) ([]domain.Comment, error) {
	if query.PostID == "" {
		return nil, domain.ErrPostNotFound
	}
	if query.Limit <= 0 {
		query.Limit = 20
	}
	if query.Limit > 100 {
		query.Limit = 100
	}
	return s.store.ListCommentsByPost(ctx, query)
}

// ListRepliesByParent retrieves visible published replies for a parent comment.
func (s *CommentService) ListRepliesByParent(ctx context.Context, query ListCommentRepliesQuery) ([]domain.Comment, error) {
	if query.ParentID == "" {
		return nil, domain.ErrParentCommentNotFound
	}
	if query.Limit <= 0 {
		query.Limit = 20
	}
	if query.Limit > 100 {
		query.Limit = 100
	}
	return s.store.ListRepliesByParent(ctx, query)
}
