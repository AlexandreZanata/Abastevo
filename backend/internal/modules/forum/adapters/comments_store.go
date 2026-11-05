package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	forumsql "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/forum"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
	identitydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
)

func commentQuotaSubjectHourly(accountID string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("forum:quota:comment:hour:%s", accountID)))
	return hex.EncodeToString(sum[:])
}

func commentQuotaSubjectDaily(accountID string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("forum:quota:comment:day:%s", accountID)))
	return hex.EncodeToString(sum[:])
}

func mapCommentRowToDomain(r forumsql.ForumComment) *domain.Comment {
	var parentID *string
	if r.ParentID.Valid {
		pid := uuidString(r.ParentID)
		parentID = &pid
	}
	var pubAt *time.Time
	if r.PublishedAt.Valid {
		t := r.PublishedAt.Time.UTC()
		pubAt = &t
	}
	return &domain.Comment{
		ID:                uuidString(r.ID),
		PostID:            uuidString(r.PostID),
		AuthorAccountID:   uuidString(r.AuthorAccountID),
		PublicAlias:       r.PublicAlias,
		ParentID:          parentID,
		Depth:             r.Depth,
		Sequence:          r.Sequence,
		BodyText:          r.BodyText,
		Revision:          int(r.Revision),
		PublicationState:  domain.PublicationState(r.PublicationState),
		Visibility:        domain.Visibility(r.Visibility),
		CreatedAt:         r.CreatedAt.Time.UTC(),
		UpdatedAt:         r.UpdatedAt.Time.UTC(),
		PublishedAt:       pubAt,
		CreateOperationID: uuidString(r.CreateOperationID),
		BodySHA256:        r.BodySha256,
	}
}

// CreateComment atomically verifies quotas, locks post and parent, assigns scoped sequence,
// and inserts a new pending_review comment. Replays return the existing comment if content matches.
func (s *Store) CreateComment(ctx context.Context, cmd application.CreateCommentCommand) (application.CommentResult, error) {
	postUUID, err := mustUUID(cmd.PostID)
	if err != nil {
		return application.CommentResult{}, domain.ErrPostNotFound
	}
	authorUUID, err := mustUUID(cmd.AuthorAccountID)
	if err != nil {
		return application.CommentResult{}, domain.ErrMissingAuthor
	}
	opUUID, err := mustUUID(cmd.OperationID)
	if err != nil {
		return application.CommentResult{}, fmt.Errorf("forum: invalid operation id: %w", err)
	}

	bodySHA := sha256Hex(cmd.BodyText)

	// Idempotency check: see if (author_account_id, create_operation_id) already exists
	existing, err := s.queries.GetCommentByOperationID(ctx, forumsql.GetCommentByOperationIDParams{
		AuthorAccountID:   authorUUID,
		CreateOperationID: opUUID,
	})
	if err == nil {
		if existing.BodySha256 == bodySHA {
			return application.CommentResult{
				Comment:  mapCommentRowToDomain(existing),
				Replayed: true,
			}, nil
		}
		return application.CommentResult{}, domain.ErrConflictingOperation
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return application.CommentResult{}, fmt.Errorf("forum: check existing comment: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return application.CommentResult{}, fmt.Errorf("forum: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	qtx := s.queries.WithTx(tx)

	// 1. Quota check (if limiter configured)
	if s.limiter != nil {
		subjectHour := commentQuotaSubjectHourly(cmd.AuthorAccountID)
		if _, err := s.limiter.CheckTx(ctx, tx, subjectHour, identitydomain.OperationWrite); err != nil {
			return application.CommentResult{}, err
		}
		subjectDay := commentQuotaSubjectDaily(cmd.AuthorAccountID)
		if _, err := s.limiter.CheckTx(ctx, tx, subjectDay, identitydomain.OperationWrite); err != nil {
			return application.CommentResult{}, err
		}
	}

	// 2. Lock post and verify state/comments_enabled
	postRow, err := qtx.LockPostForComment(ctx, postUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return application.CommentResult{}, domain.ErrPostNotFound
		}
		return application.CommentResult{}, fmt.Errorf("forum: lock post: %w", err)
	}
	if postRow.PublicationState != string(domain.StatePublished) || postRow.Visibility != string(domain.VisibilityVisible) {
		return application.CommentResult{}, domain.ErrPostNotCommentable
	}
	if !postRow.CommentsEnabled {
		return application.CommentResult{}, domain.ErrPostCommentsDisabled
	}

	// 3. Lock parent comment (if reply)
	var parentUUID pgtype.UUID
	var depth int16
	if cmd.ParentID != nil && *cmd.ParentID != "" {
		pUUID, err := mustUUID(*cmd.ParentID)
		if err != nil {
			return application.CommentResult{}, domain.ErrParentCommentNotFound
		}
		parentRow, err := qtx.LockParentComment(ctx, pUUID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return application.CommentResult{}, domain.ErrParentCommentNotFound
			}
			return application.CommentResult{}, fmt.Errorf("forum: lock parent comment: %w", err)
		}
		if parentRow.PostID != postUUID {
			return application.CommentResult{}, domain.ErrCrossPostParent
		}
		if parentRow.Depth != 0 {
			return application.CommentResult{}, domain.ErrInvalidCommentDepth
		}
		if parentRow.PublicationState != string(domain.StatePublished) || parentRow.Visibility != string(domain.VisibilityVisible) {
			return application.CommentResult{}, domain.ErrParentCommentNotVisible
		}
		parentUUID = pUUID
		depth = 1
	}

	// 4. Check live comment count for author across forum
	liveCount, err := qtx.CountLiveCommentsByAuthor(ctx, authorUUID)
	if err != nil {
		return application.CommentResult{}, fmt.Errorf("forum: count live comments: %w", err)
	}
	if liveCount >= application.MaxLiveCommentsPerAccount {
		return application.CommentResult{}, domain.ErrCommentLiveQuotaExceeded
	}

	// 5. Sequence assignment & bump
	assignedSeq := postRow.NextCommentSequence
	if err := qtx.BumpPostCommentSequence(ctx, postUUID); err != nil {
		return application.CommentResult{}, fmt.Errorf("forum: bump post sequence: %w", err)
	}

	// 6. Insert comment
	commentUUID := newUUIDBytes()
	now := time.Now().UTC()
	created, err := qtx.InsertComment(ctx, forumsql.InsertCommentParams{
		ID:                commentUUID,
		PostID:            postUUID,
		AuthorAccountID:   authorUUID,
		PublicAlias:       cmd.PublicAlias,
		ParentID:          parentUUID,
		Depth:             depth,
		Sequence:          assignedSeq,
		BodyText:          cmd.BodyText,
		Revision:          1,
		PublicationState:  string(domain.StatePendingReview),
		Visibility:        string(domain.VisibilityVisible),
		CreatedAt:         pgtype.Timestamptz{Time: now, Valid: true},
		UpdatedAt:         pgtype.Timestamptz{Time: now, Valid: true},
		PublishedAt:       pgtype.Timestamptz{},
		CreateOperationID: opUUID,
		BodySha256:        bodySHA,
	})
	if err != nil {
		return application.CommentResult{}, fmt.Errorf("forum: insert comment: %w", err)
	}

	// 7. Record change in forum_changes
	changeID := newUUIDBytes()
	if err := qtx.RecordForumChange(ctx, forumsql.RecordForumChangeParams{
		ID:             changeID,
		TargetType:     "COMMENT",
		TargetID:       commentUUID,
		Revision:       1,
		Action:         "CREATE",
		ActorKind:      "AUTHOR",
		ActorAccountID: authorUUID,
		AdminAuditRef:  "",
		PolicyVersion:  1,
		ReasonCode:     "author-submission",
		PriorState:     "",
		NewState:       string(domain.StatePendingReview),
		CreatedAt:      pgtype.Timestamptz{Time: now, Valid: true},
	}); err != nil {
		return application.CommentResult{}, fmt.Errorf("forum: record change: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return application.CommentResult{}, fmt.Errorf("forum: commit tx: %w", err)
	}

	return application.CommentResult{
		Comment:  mapCommentRowToDomain(created),
		Replayed: false,
	}, nil
}

// EditComment updates the comment text under an optimistic concurrency CAS check.
func (s *Store) EditComment(ctx context.Context, cmd application.EditCommentCommand) (*domain.Comment, error) {
	commentUUID, err := mustUUID(cmd.CommentID)
	if err != nil {
		return nil, domain.ErrCommentNotFound
	}
	authorUUID, err := mustUUID(cmd.AuthorAccountID)
	if err != nil {
		return nil, domain.ErrMissingAuthor
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("forum: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	qtx := s.queries.WithTx(tx)

	existing, err := qtx.GetCommentByIDForUpdate(ctx, commentUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrCommentNotFound
		}
		return nil, fmt.Errorf("forum: lock comment: %w", err)
	}

	if existing.PublicationState == string(domain.StateRemoved) {
		return nil, domain.ErrRemovedIsTerminal
	}
	if existing.AuthorAccountID != authorUUID {
		return nil, domain.ErrCommentAuthorMismatch
	}
	if existing.Revision != int32(cmd.ExpectedRevision) {
		return nil, domain.ErrRevisionMismatch
	}

	bodySHA := sha256Hex(cmd.BodyText)
	updated, err := qtx.UpdateCommentCAS(ctx, forumsql.UpdateCommentCASParams{
		ID:              commentUUID,
		AuthorAccountID: authorUUID,
		Revision:        int32(cmd.ExpectedRevision),
		BodyText:        cmd.BodyText,
		BodySha256:      bodySHA,
	})
	if err != nil {
		return nil, fmt.Errorf("forum: update comment: %w", err)
	}

	now := time.Now().UTC()
	changeID := newUUIDBytes()
	if err := qtx.RecordForumChange(ctx, forumsql.RecordForumChangeParams{
		ID:             changeID,
		TargetType:     "COMMENT",
		TargetID:       commentUUID,
		Revision:       updated.Revision,
		Action:         "EDIT",
		ActorKind:      "AUTHOR",
		ActorAccountID: authorUUID,
		AdminAuditRef:  "",
		PolicyVersion:  1,
		ReasonCode:     "author-edit",
		PriorState:     existing.PublicationState,
		NewState:       string(domain.StatePendingReview),
		CreatedAt:      pgtype.Timestamptz{Time: now, Valid: true},
	}); err != nil {
		return nil, fmt.Errorf("forum: record change: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("forum: commit tx: %w", err)
	}

	return mapCommentRowToDomain(updated), nil
}

// RemoveComment performs tombstone removal on an authored comment.
func (s *Store) RemoveComment(ctx context.Context, cmd application.RemoveCommentCommand) (*domain.Comment, error) {
	commentUUID, err := mustUUID(cmd.CommentID)
	if err != nil {
		return nil, domain.ErrCommentNotFound
	}
	authorUUID, err := mustUUID(cmd.AuthorAccountID)
	if err != nil {
		return nil, domain.ErrMissingAuthor
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("forum: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	qtx := s.queries.WithTx(tx)

	existing, err := qtx.GetCommentByIDForUpdate(ctx, commentUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrCommentNotFound
		}
		return nil, fmt.Errorf("forum: lock comment: %w", err)
	}

	if existing.PublicationState == string(domain.StateRemoved) {
		return nil, domain.ErrRemovedIsTerminal
	}
	if existing.AuthorAccountID != authorUUID {
		return nil, domain.ErrCommentAuthorMismatch
	}
	if existing.Revision != int32(cmd.ExpectedRevision) {
		return nil, domain.ErrRevisionMismatch
	}

	removed, err := qtx.TombstoneComment(ctx, forumsql.TombstoneCommentParams{
		ID:              commentUUID,
		AuthorAccountID: authorUUID,
		Revision:        int32(cmd.ExpectedRevision),
	})
	if err != nil {
		return nil, fmt.Errorf("forum: tombstone comment: %w", err)
	}

	now := time.Now().UTC()
	changeID := newUUIDBytes()
	if err := qtx.RecordForumChange(ctx, forumsql.RecordForumChangeParams{
		ID:             changeID,
		TargetType:     "COMMENT",
		TargetID:       commentUUID,
		Revision:       removed.Revision,
		Action:         "REMOVE",
		ActorKind:      "AUTHOR",
		ActorAccountID: authorUUID,
		AdminAuditRef:  "",
		PolicyVersion:  1,
		ReasonCode:     "author-removal",
		PriorState:     existing.PublicationState,
		NewState:       string(domain.StateRemoved),
		CreatedAt:      pgtype.Timestamptz{Time: now, Valid: true},
	}); err != nil {
		return nil, fmt.Errorf("forum: record change: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("forum: commit tx: %w", err)
	}

	return mapCommentRowToDomain(removed), nil
}

// GetCommentByID retrieves a single comment by ID.
func (s *Store) GetCommentByID(ctx context.Context, id string) (*domain.Comment, error) {
	commentUUID, err := mustUUID(id)
	if err != nil {
		return nil, domain.ErrCommentNotFound
	}

	row, err := s.queries.GetCommentByID(ctx, commentUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrCommentNotFound
		}
		return nil, fmt.Errorf("forum: get comment: %w", err)
	}

	return mapCommentRowToDomain(row), nil
}

// ListCommentsByPost retrieves visible published comments for a post.
func (s *Store) ListCommentsByPost(ctx context.Context, query application.ListPostCommentsQuery) ([]domain.Comment, error) {
	postUUID, err := mustUUID(query.PostID)
	if err != nil {
		return nil, domain.ErrPostNotFound
	}

	limit := int32(query.Limit)
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	var cursorSeq pgtype.Int8
	if query.CursorSequence != nil {
		cursorSeq = pgtype.Int8{Int64: *query.CursorSequence, Valid: true}
	}

	rows, err := s.queries.ListCommentsByPost(ctx, forumsql.ListCommentsByPostParams{
		PostID:         postUUID,
		Limit:          limit,
		CursorSequence: cursorSeq,
	})
	if err != nil {
		return nil, fmt.Errorf("forum: list comments by post: %w", err)
	}

	out := make([]domain.Comment, 0, len(rows))
	for _, r := range rows {
		out = append(out, *mapCommentRowToDomain(r))
	}
	return out, nil
}

// ListRepliesByParent retrieves visible published replies for a parent comment.
func (s *Store) ListRepliesByParent(ctx context.Context, query application.ListCommentRepliesQuery) ([]domain.Comment, error) {
	parentUUID, err := mustUUID(query.ParentID)
	if err != nil {
		return nil, domain.ErrParentCommentNotFound
	}

	// Verify parent exists and is published+visible (suppresses replies if parent is hidden/removed)
	parent, err := s.queries.GetCommentByID(ctx, parentUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrParentCommentNotFound
		}
		return nil, fmt.Errorf("forum: check parent: %w", err)
	}
	if parent.PublicationState != string(domain.StatePublished) || parent.Visibility != string(domain.VisibilityVisible) {
		// Return empty slice when parent is hidden or removed, per B-BR-FOR-09
		return []domain.Comment{}, nil
	}

	limit := int32(query.Limit)
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	var cursorSeq pgtype.Int8
	if query.CursorSequence != nil {
		cursorSeq = pgtype.Int8{Int64: *query.CursorSequence, Valid: true}
	}

	rows, err := s.queries.ListRepliesByParent(ctx, forumsql.ListRepliesByParentParams{
		ParentID:       parentUUID,
		Limit:          limit,
		CursorSequence: cursorSeq,
	})
	if err != nil {
		return nil, fmt.Errorf("forum: list replies by parent: %w", err)
	}

	out := make([]domain.Comment, 0, len(rows))
	for _, r := range rows {
		out = append(out, *mapCommentRowToDomain(r))
	}
	return out, nil
}

// PublishComment approves a pending_review comment and marks it published.
func (s *Store) PublishComment(ctx context.Context, commentID string) (*domain.Comment, error) {
	commentUUID, err := mustUUID(commentID)
	if err != nil {
		return nil, domain.ErrCommentNotFound
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("forum: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	qtx := s.queries.WithTx(tx)

	existing, err := qtx.GetCommentByIDForUpdate(ctx, commentUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrCommentNotFound
		}
		return nil, fmt.Errorf("forum: lock comment: %w", err)
	}

	if existing.PublicationState != string(domain.StatePendingReview) {
		return nil, fmt.Errorf("forum: comment not in pending_review state: %s", existing.PublicationState)
	}

	published, err := qtx.PublishComment(ctx, commentUUID)
	if err != nil {
		return nil, fmt.Errorf("forum: publish comment: %w", err)
	}

	now := time.Now().UTC()
	changeID := newUUIDBytes()
	if err := qtx.RecordForumChange(ctx, forumsql.RecordForumChangeParams{
		ID:             changeID,
		TargetType:     "COMMENT",
		TargetID:       commentUUID,
		Revision:       published.Revision,
		Action:         "PUBLISH",
		ActorKind:      "ADMIN",
		ActorAccountID: pgtype.UUID{},
		AdminAuditRef:  "editorial-approval",
		PolicyVersion:  1,
		ReasonCode:     "editorial-publish",
		PriorState:     existing.PublicationState,
		NewState:       string(domain.StatePublished),
		CreatedAt:      pgtype.Timestamptz{Time: now, Valid: true},
	}); err != nil {
		return nil, fmt.Errorf("forum: record change: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("forum: commit tx: %w", err)
	}

	return mapCommentRowToDomain(published), nil
}

// RejectComment rejects a pending_review comment and marks it removed.
func (s *Store) RejectComment(ctx context.Context, commentID string) (*domain.Comment, error) {
	commentUUID, err := mustUUID(commentID)
	if err != nil {
		return nil, domain.ErrCommentNotFound
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("forum: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	qtx := s.queries.WithTx(tx)

	existing, err := qtx.GetCommentByIDForUpdate(ctx, commentUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrCommentNotFound
		}
		return nil, fmt.Errorf("forum: lock comment: %w", err)
	}

	if existing.PublicationState != string(domain.StatePendingReview) {
		return nil, fmt.Errorf("forum: comment not in pending_review state: %s", existing.PublicationState)
	}

	rejected, err := qtx.RejectComment(ctx, commentUUID)
	if err != nil {
		return nil, fmt.Errorf("forum: reject comment: %w", err)
	}

	now := time.Now().UTC()
	changeID := newUUIDBytes()
	if err := qtx.RecordForumChange(ctx, forumsql.RecordForumChangeParams{
		ID:             changeID,
		TargetType:     "COMMENT",
		TargetID:       commentUUID,
		Revision:       rejected.Revision,
		Action:         "REJECT",
		ActorKind:      "ADMIN",
		ActorAccountID: pgtype.UUID{},
		AdminAuditRef:  "editorial-rejection",
		PolicyVersion:  1,
		ReasonCode:     "editorial-reject",
		PriorState:     existing.PublicationState,
		NewState:       string(domain.StateRemoved),
		CreatedAt:      pgtype.Timestamptz{Time: now, Valid: true},
	}); err != nil {
		return nil, fmt.Errorf("forum: record change: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("forum: commit tx: %w", err)
	}

	return mapCommentRowToDomain(rejected), nil
}
