package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	forumsql "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/forum"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
)

// maxOwnCommentReactions bounds one private owner read of a conversation.
const maxOwnCommentReactions = 1000

func reactionQuotaSubjectHourly(accountID string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("forum:quota:reaction:hour:%s", accountID)))
	return hex.EncodeToString(sum[:])
}

// SetReaction stores one account's reaction on a public post or comment
// (B-BR-FOR-17). Database triggers move the counts and announce the change on
// commit, so a repeated value is a no-op and counts never double count.
func (s *Store) SetReaction(ctx context.Context, cmd application.SetReactionCommand) (application.ReactionResult, error) {
	targetUUID, err := mustUUID(cmd.TargetID)
	if err != nil {
		return application.ReactionResult{}, notFoundFor(cmd.TargetType)
	}
	accountUUID, err := mustUUID(cmd.AccountID)
	if err != nil {
		return application.ReactionResult{}, domain.ErrMissingAuthor
	}
	if cmd.Reaction != domain.ReactionNone && cmd.Reaction != domain.ReactionLike && cmd.Reaction != domain.ReactionDislike {
		return application.ReactionResult{}, domain.ErrInvalidReaction
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return application.ReactionResult{}, fmt.Errorf("forum: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if cmd.Guard != nil {
		if err := cmd.Guard(ctx, tx); err != nil {
			return application.ReactionResult{}, err
		}
	}
	if err := checkQuota(ctx, tx, s.quotas.ReactionHourly, reactionQuotaSubjectHourly(cmd.AccountID)); err != nil {
		return application.ReactionResult{}, err
	}

	qtx := s.queries.WithTx(tx)
	result := application.ReactionResult{TargetType: cmd.TargetType, TargetID: cmd.TargetID, Reaction: cmd.Reaction}

	switch cmd.TargetType {
	case domain.ReactionTargetComment:
		target, err := qtx.GetPublicCommentReactionTarget(ctx, targetUUID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return application.ReactionResult{}, domain.ErrCommentNotFound
			}
			return application.ReactionResult{}, fmt.Errorf("forum: reaction target: %w", err)
		}
		// Revoked source rights or an inactive topic author hide the whole conversation.
		if _, err := qtx.GetPublicPostByID(ctx, target.PostID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return application.ReactionResult{}, domain.ErrCommentNotFound
			}
			return application.ReactionResult{}, fmt.Errorf("forum: reaction post: %w", err)
		}
		if cmd.Reaction == domain.ReactionNone {
			err = qtx.DeleteCommentReaction(ctx, forumsql.DeleteCommentReactionParams{CommentID: targetUUID, AccountID: accountUUID})
		} else {
			err = qtx.UpsertCommentReaction(ctx, forumsql.UpsertCommentReactionParams{
				CommentID: targetUUID, AccountID: accountUUID, PostID: target.PostID, Value: int16(cmd.Reaction),
			})
		}
		if err != nil {
			return application.ReactionResult{}, fmt.Errorf("forum: set comment reaction: %w", err)
		}
		counts, err := qtx.GetCommentReactionCounts(ctx, targetUUID)
		if err != nil {
			return application.ReactionResult{}, fmt.Errorf("forum: comment reaction counts: %w", err)
		}
		result.PostID = uuidString(target.PostID)
		result.Counts = domain.ReactionCounts{Likes: counts.LikeCount, Dislikes: counts.DislikeCount}

	case domain.ReactionTargetPost:
		if _, err := qtx.GetPublicPostByID(ctx, targetUUID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return application.ReactionResult{}, domain.ErrPostNotFound
			}
			return application.ReactionResult{}, fmt.Errorf("forum: reaction post: %w", err)
		}
		if cmd.Reaction == domain.ReactionNone {
			err = qtx.DeletePostReaction(ctx, forumsql.DeletePostReactionParams{PostID: targetUUID, AccountID: accountUUID})
		} else {
			err = qtx.UpsertPostReaction(ctx, forumsql.UpsertPostReactionParams{
				PostID: targetUUID, AccountID: accountUUID, Value: int16(cmd.Reaction),
			})
		}
		if err != nil {
			return application.ReactionResult{}, fmt.Errorf("forum: set post reaction: %w", err)
		}
		counts, err := qtx.GetPostReactionCounts(ctx, targetUUID)
		if err != nil {
			return application.ReactionResult{}, fmt.Errorf("forum: post reaction counts: %w", err)
		}
		result.PostID = cmd.TargetID
		result.Counts = domain.ReactionCounts{Likes: counts.LikeCount, Dislikes: counts.DislikeCount}

	default:
		return application.ReactionResult{}, domain.ErrInvalidReaction
	}

	if err := tx.Commit(ctx); err != nil {
		return application.ReactionResult{}, fmt.Errorf("forum: commit reaction: %w", err)
	}
	return result, nil
}

// OwnReactions reads the caller's reactions in one conversation.
func (s *Store) OwnReactions(ctx context.Context, accountID, postID string) (application.OwnReactions, error) {
	accountUUID, err := mustUUID(accountID)
	if err != nil {
		return application.OwnReactions{}, domain.ErrMissingAuthor
	}
	postUUID, err := mustUUID(postID)
	if err != nil {
		return application.OwnReactions{}, domain.ErrPostNotFound
	}
	own := application.OwnReactions{Comments: map[string]domain.Reaction{}}
	value, err := s.queries.GetOwnPostReaction(ctx, forumsql.GetOwnPostReactionParams{PostID: postUUID, AccountID: accountUUID})
	switch {
	case err == nil:
		own.Post = domain.Reaction(value)
	case !errors.Is(err, pgx.ErrNoRows):
		return application.OwnReactions{}, fmt.Errorf("forum: own post reaction: %w", err)
	}
	rows, err := s.queries.ListOwnCommentReactionsForPost(ctx, forumsql.ListOwnCommentReactionsForPostParams{
		AccountID: accountUUID, PostID: postUUID, Limit: maxOwnCommentReactions,
	})
	if err != nil {
		return application.OwnReactions{}, fmt.Errorf("forum: own comment reactions: %w", err)
	}
	for _, r := range rows {
		own.Comments[uuidString(r.CommentID)] = domain.Reaction(r.Value)
	}
	return own, nil
}

// ListReactionsByAccount returns every reaction of the account for privacy export.
func (s *Store) ListReactionsByAccount(ctx context.Context, accountID string) ([]application.OwnReactionRecord, error) {
	accountUUID, err := mustUUID(accountID)
	if err != nil {
		return nil, domain.ErrMissingAuthor
	}
	rows, err := s.queries.ListOwnReactionsByAccount(ctx, accountUUID)
	if err != nil {
		return nil, fmt.Errorf("forum: list reactions by account: %w", err)
	}
	out := make([]application.OwnReactionRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, application.OwnReactionRecord{
			TargetType: domain.ReactionTargetType(r.TargetType),
			TargetID:   uuidString(r.TargetID),
			Reaction:   domain.Reaction(r.Value).Wire(),
			UpdatedAt:  r.UpdatedAt.Time.UTC(),
		})
	}
	return out, nil
}

func notFoundFor(target domain.ReactionTargetType) error {
	if target == domain.ReactionTargetPost {
		return domain.ErrPostNotFound
	}
	return domain.ErrCommentNotFound
}
