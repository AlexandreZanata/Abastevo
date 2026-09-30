package application

import (
	"context"
	"strings"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/feedback/domain"
)

// VoteStore is the persistence port behind vote use cases. Atomicity
// lives here: CastVote converges, rewrites or inserts under one
// lock/transaction while refreshing the revision tally, so
// concurrent voters cannot fork or inflate a denominator. Tallies
// recompute from live rows; a lost unique race re-reads the winner.
type VoteStore interface {
	CastVote(ctx context.Context, rec domain.StoredVote) (domain.StoredVote, bool, error)
	RemoveVote(ctx context.Context, accountID, commentID string, revision int, nowUnix int64) (bool, error)
	Tally(ctx context.Context, commentID string, revision int) (domain.VoteTally, bool, error)
	RebuildTally(ctx context.Context, commentID string, revision int, nowUnix int64) (domain.VoteTally, error)
}

// VoteResult is the outcome of casting one validity vote.
type VoteResult struct {
	Vote    domain.StoredVote
	Tally   domain.VoteTally
	Created bool
}

// Vote records one changeable vote per account/comment/revision
// (F05). Equal choices converge idempotently; changed choices
// rewrite in place; authors never vote on their own comments and
// tombstoned targets refuse. Every path refreshes the exact
// revision tally.
func (s *Service) Vote(ctx context.Context, accountID, commentID, choice string) (VoteResult, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return VoteResult{}, domain.ErrTargetInvalid
	}
	if !domain.ValidVote(choice) {
		return VoteResult{}, domain.ErrVoteChoiceInvalid
	}
	target, err := s.voteTarget(ctx, accountID, commentID)
	if err != nil {
		return VoteResult{}, err
	}
	id, err := s.IDGen()
	if err != nil {
		return VoteResult{}, err
	}
	stored, created, err := s.voteStore().CastVote(ctx, domain.StoredVote{
		ID:              id,
		AccountID:       accountID,
		CommentID:       target.ID,
		CommentRevision: target.Revision,
		Choice:          choice,
		CreatedAt:       s.Clock.NowUnix(),
	})
	if err != nil {
		return VoteResult{}, err
	}
	tally, found, err := s.voteStore().Tally(ctx, target.ID, target.Revision)
	if err != nil {
		return VoteResult{}, err
	}
	if !found {
		return VoteResult{}, domain.ErrStatsMissing
	}
	return VoteResult{Vote: stored, Tally: tally, Created: created}, nil
}

// RemoveVote tombstones the caller's live vote on the comment's
// current revision. Absent votes converge as a no-op success, so
// retries never fail; history stays for audit.
func (s *Service) RemoveVote(ctx context.Context, accountID, commentID string) error {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return domain.ErrTargetInvalid
	}
	if err := s.authorize(ctx, accountID); err != nil {
		return err
	}
	// No self-vote check here: removal is cleanup, and absent votes
	// converge as a no-op success either way.
	target, err := s.liveTarget(ctx, commentID)
	if err != nil {
		return err
	}
	_, err = s.voteStore().RemoveVote(ctx, accountID, target.ID, target.Revision, s.Clock.NowUnix())
	return err
}

// Tally reads the exact per-revision count snapshot for display.
// Zero votes surface through the null agreement, never 0%.
func (s *Service) Tally(ctx context.Context, commentID string) (domain.VoteTally, error) {
	target, err := s.liveTarget(ctx, strings.TrimSpace(commentID))
	if err != nil {
		return domain.VoteTally{}, err
	}
	tally, found, err := s.voteStore().Tally(ctx, target.ID, target.Revision)
	if err != nil {
		return domain.VoteTally{}, err
	}
	if !found {
		return domain.VoteTally{CommentID: target.ID, Revision: target.Revision}, nil
	}
	return tally, nil
}

// RebuildTally recomputes one comment's current-revision tally from
// live rows for reconciliation.
func (s *Service) RebuildTally(ctx context.Context, commentID string) (domain.VoteTally, error) {
	target, err := s.liveTarget(ctx, strings.TrimSpace(commentID))
	if err != nil {
		return domain.VoteTally{}, err
	}
	return s.voteStore().RebuildTally(ctx, target.ID, target.Revision, s.Clock.NowUnix())
}

func (s *Service) voteStore() VoteStore {
	return s.Votes
}

// voteTarget authorizes and resolves the live vote target: active
// account, existing live comment, author excluded. Self-votes refuse
// before any store mutation.
func (s *Service) voteTarget(ctx context.Context, accountID, commentID string) (domain.StoredComment, error) {
	if err := s.authorize(ctx, accountID); err != nil {
		return domain.StoredComment{}, err
	}
	target, err := s.liveTarget(ctx, commentID)
	if err != nil {
		return domain.StoredComment{}, err
	}
	if target.AccountID == accountID {
		return domain.StoredComment{}, domain.ErrSelfVote
	}
	return target, nil
}

func (s *Service) liveTarget(ctx context.Context, commentID string) (domain.StoredComment, error) {
	if commentID == "" {
		return domain.StoredComment{}, domain.ErrCommentNotFound
	}
	target, found, err := s.Comments.GetComment(ctx, commentID)
	if err != nil {
		return domain.StoredComment{}, err
	}
	if !found || !target.Live() {
		return domain.StoredComment{}, domain.ErrCommentNotFound
	}
	return target, nil
}
