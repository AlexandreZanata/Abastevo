package adapters

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	community "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/community"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/domain"
)

// HelpfulnessStore is the PostgreSQL owner of helpfulness votes. One
// live row per contributor+observation+sequence converges concurrent
// casts, flips, removals and lost-response retries; tallies move in the
// same transaction and rebuild from live rows. The assertion below keeps
// store and application port in agreement at compile time.
var _ application.HelpfulnessStore = (*HelpfulnessStore)(nil)

// HelpfulnessStore persists helpfulness votes and exact tallies.
type HelpfulnessStore struct {
	pool *pgxpool.Pool
}

// NewHelpfulnessStore opens the helpfulness vote store over pool.
func NewHelpfulnessStore(pool *pgxpool.Pool) *HelpfulnessStore {
	return &HelpfulnessStore{pool: pool}
}

// Target resolves the vote target: author, visibility and the latest
// decision sequence. Unknown observations fail before any mutation.
func (s *HelpfulnessStore) Target(ctx context.Context, observationID string) (application.HelpfulnessTarget, error) {
	uid, err := mustUUID(observationID)
	if err != nil {
		return application.HelpfulnessTarget{}, application.ErrTargetNotFound
	}
	row, err := community.New(s.pool).GetHelpfulnessTarget(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return application.HelpfulnessTarget{}, application.ErrTargetNotFound
		}
		return application.HelpfulnessTarget{}, err
	}
	return application.HelpfulnessTarget{
		ObservationID:  uuidString(row.ID),
		AuthorRef:      row.ContributorRef,
		State:          row.State,
		Hidden:         row.HiddenAt.Valid,
		Deleted:        row.DeletedAt.Valid,
		LatestSequence: row.LatestSequence,
	}, nil
}

// CastVote converges, rewrites or inserts under one lock/transaction
// while refreshing the exact tally, so concurrent voters cannot fork
// or inflate a denominator. A lost unique race re-reads the winner.
func (s *HelpfulnessStore) CastVote(ctx context.Context, rec domain.StoredVote) (domain.StoredVote, bool, error) {
	id, err := mustUUID(rec.ID)
	if err != nil {
		return domain.StoredVote{}, false, err
	}
	obs, err := mustUUID(rec.ObservationID)
	if err != nil {
		return domain.StoredVote{}, false, err
	}
	for attempt := 0; attempt < 2; attempt++ {
		stored, created, retry, err := s.castOnce(ctx, id, obs, rec)
		if err != nil {
			return domain.StoredVote{}, false, err
		}
		if !retry {
			return stored, created, nil
		}
	}
	return domain.StoredVote{}, false, application.ErrTargetNotFound
}

func (s *HelpfulnessStore) castOnce(ctx context.Context, id, obs pgtype.UUID, rec domain.StoredVote) (domain.StoredVote, bool, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.StoredVote{}, false, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := community.New(tx)
	// Lost-response retry: the same contributor+submission identity
	// replays the committed vote instead of opening a second row.
	if replay, err := q.GetHelpfulnessVoteBySubmission(ctx, community.GetHelpfulnessVoteBySubmissionParams{
		ContributorRef: rec.ContributorRef, ClientSubmissionID: rec.ClientSubmissionID,
	}); err == nil {
		if err := tx.Commit(ctx); err != nil {
			return domain.StoredVote{}, false, false, err
		}
		return toHelpfulnessVote(replay), false, false, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return domain.StoredVote{}, false, false, err
	}
	row, err := q.LockHelpfulnessVote(ctx, community.LockHelpfulnessVoteParams{
		ContributorRef: rec.ContributorRef, ObservationID: obs, DecisionSequence: rec.DecisionSequence,
	})
	if err == nil {
		existing := toHelpfulnessVote(row)
		if existing.Choice == rec.Choice {
			if err := tx.Commit(ctx); err != nil {
				return domain.StoredVote{}, false, false, err
			}
			return existing, false, false, nil
		}
		if err := q.UpdateHelpfulnessChoice(ctx, community.UpdateHelpfulnessChoiceParams{
			Choice: string(rec.Choice), ID: row.ID,
		}); err != nil {
			return domain.StoredVote{}, false, false, err
		}
		oldH, oldN := helpfulDelta(existing.Choice)
		newH, newN := helpfulDelta(rec.Choice)
		retry, err := bumpHelpfulnessTally(ctx, q, obs, rec.DecisionSequence, newH-oldH, newN-oldN, rec.CreatedAt)
		if err != nil || retry {
			return domain.StoredVote{}, false, retry, err
		}
		existing.Choice = rec.Choice
		if err := tx.Commit(ctx); err != nil {
			if isUniqueViolation(err) {
				return domain.StoredVote{}, false, true, nil
			}
			return domain.StoredVote{}, false, false, err
		}
		return existing, false, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.StoredVote{}, false, false, err
	}
	if err := q.InsertHelpfulnessVote(ctx, community.InsertHelpfulnessVoteParams{
		ID: id, ContributorRef: rec.ContributorRef, ObservationID: obs,
		DecisionSequence: rec.DecisionSequence, Choice: string(rec.Choice),
		ClientSubmissionID: rec.ClientSubmissionID, CreatedAt: pgTime(rec.CreatedAt),
	}); err != nil {
		if isUniqueViolation(err) {
			return domain.StoredVote{}, false, true, nil
		}
		return domain.StoredVote{}, false, false, err
	}
	newH, newN := helpfulDelta(rec.Choice)
	retry, err := bumpHelpfulnessTally(ctx, q, obs, rec.DecisionSequence, newH, newN, rec.CreatedAt)
	if err != nil || retry {
		return domain.StoredVote{}, false, retry, err
	}
	if err := tx.Commit(ctx); err != nil {
		if isUniqueViolation(err) {
			return domain.StoredVote{}, false, true, nil
		}
		return domain.StoredVote{}, false, false, err
	}
	rec.ID = uuidString(id)
	return rec, true, false, nil
}

// helpfulDelta maps a stored choice onto (helpful, not-helpful) deltas.
func helpfulDelta(choice domain.Choice) (int64, int64) {
	if choice == domain.ChoiceHelpful {
		return 1, 0
	}
	return 0, 1
}

// bumpHelpfulnessTally moves the stored tally by signed deltas with a
// plain UPDATE: PostgreSQL evaluates CHECK constraints on the proposed
// row before ON CONFLICT resolution, so a delta upsert would reject
// negative moves even against a sufficient balance. A missing tally row
// takes the exact path for its sign: non-negative first-cast deltas
// insert directly (a concurrent insert races into one more UPDATE
// attempt), while negative moves rebuild from live rows under an
// advisory target lock so concurrent rebuilds never overwrite each
// other with stale snapshots. Nothing here clamps silently.
func bumpHelpfulnessTally(ctx context.Context, q *community.Queries, obs pgtype.UUID, seq, helpfulDelta, notHelpfulDelta int64, at time.Time) (bool, error) {
	for attempt := 0; attempt < 3; attempt++ {
		n, err := q.BumpHelpfulnessTally(ctx, community.BumpHelpfulnessTallyParams{
			ObservationID: obs, DecisionSequence: seq,
			HelpfulDelta: helpfulDelta, NotHelpfulDelta: notHelpfulDelta,
			UpdatedAt: pgTime(at),
		})
		if err != nil {
			return false, err
		}
		if n == 1 {
			return false, nil
		}
		if helpfulDelta >= 0 && notHelpfulDelta >= 0 {
			err = q.InsertHelpfulnessTally(ctx, community.InsertHelpfulnessTallyParams{
				ObservationID: obs, DecisionSequence: seq,
				Helpful: helpfulDelta, NotHelpful: notHelpfulDelta, UpdatedAt: pgTime(at),
			})
			if err == nil {
				return false, nil
			}
			if !isUniqueViolation(err) {
				return false, err
			}
			continue
		}
		if err := lockHelpfulnessTarget(ctx, q, obs, seq); err != nil {
			return false, err
		}
		_, err = q.RebuildHelpfulnessTally(ctx, community.RebuildHelpfulnessTallyParams{
			ObservationID: obs, DecisionSequence: seq, UpdatedAt: pgTime(at),
		})
		if err != nil {
			if isUniqueViolation(err) {
				return true, nil
			}
			return false, err
		}
		return false, nil
	}
	return true, nil
}

func lockHelpfulnessTarget(ctx context.Context, q *community.Queries, obs pgtype.UUID, seq int64) error {
	return q.LockHelpfulnessTarget(ctx, community.LockHelpfulnessTargetParams{
		ObservationID: uuidString(obs), DecisionSequence: strconv.FormatInt(seq, 10),
	})
}

// RemoveVote tombstones the live vote and reconciles the tally from
// live rows in the same transaction, so concurrent flips and removals
// always land on exact counts.
func (s *HelpfulnessStore) RemoveVote(ctx context.Context, contributorRef, observationID string, sequence int64) (bool, error) {
	obs, err := mustUUID(observationID)
	if err != nil {
		return false, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := community.New(tx)
	n, err := q.TombstoneHelpfulnessVote(ctx, community.TombstoneHelpfulnessVoteParams{
		ContributorRef: contributorRef, ObservationID: obs, DecisionSequence: sequence,
		DeletedAt: pgTime(time.Now()),
	})
	if err != nil {
		return false, err
	}
	if n == 0 {
		if err := tx.Commit(ctx); err != nil {
			return false, err
		}
		return false, nil
	}
	if _, err := s.rebuildTallyTx(ctx, q, obs, sequence, time.Now()); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

// Tally reads the exact stored per-revision snapshot.
func (s *HelpfulnessStore) Tally(ctx context.Context, observationID string, sequence int64) (domain.Tally, bool, error) {
	obs, err := mustUUID(observationID)
	if err != nil {
		return domain.Tally{}, false, err
	}
	row, err := community.New(s.pool).GetHelpfulnessTally(ctx, community.GetHelpfulnessTallyParams{
		ObservationID: obs, DecisionSequence: sequence,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Tally{}, false, nil
		}
		return domain.Tally{}, false, err
	}
	return domain.Tally{
		ObservationID: uuidString(row.ObservationID), DecisionSequence: row.DecisionSequence,
		Helpful: row.Helpful, NotHelpful: row.NotHelpful,
	}, true, nil
}

// MyVote reads the caller's live choice for the owner overlay.
func (s *HelpfulnessStore) MyVote(ctx context.Context, contributorRef, observationID string, sequence int64) (domain.Choice, bool, error) {
	obs, err := mustUUID(observationID)
	if err != nil {
		return "", false, err
	}
	choice, err := community.New(s.pool).GetHelpfulnessChoice(ctx, community.GetHelpfulnessChoiceParams{
		ContributorRef: contributorRef, ObservationID: obs, DecisionSequence: sequence,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	return domain.Choice(choice), true, nil
}

// RebuildTally recomputes one target's tally from live rows for
// reconciliation after crashes, replays or administrative repair.
func (s *HelpfulnessStore) RebuildTally(ctx context.Context, observationID string, sequence int64) (domain.Tally, error) {
	obs, err := mustUUID(observationID)
	if err != nil {
		return domain.Tally{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Tally{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tally, err := s.rebuildTallyTx(ctx, community.New(tx), obs, sequence, time.Now())
	if err != nil {
		return domain.Tally{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Tally{}, err
	}
	return tally, nil
}

func (s *HelpfulnessStore) rebuildTallyTx(ctx context.Context, q *community.Queries, obs pgtype.UUID, sequence int64, at time.Time) (domain.Tally, error) {
	if err := lockHelpfulnessTarget(ctx, q, obs, sequence); err != nil {
		return domain.Tally{}, err
	}
	row, err := q.RebuildHelpfulnessTally(ctx, community.RebuildHelpfulnessTallyParams{
		ObservationID: obs, DecisionSequence: sequence, UpdatedAt: pgTime(at),
	})
	if err != nil {
		return domain.Tally{}, err
	}
	return domain.Tally{
		ObservationID: uuidString(row.ObservationID), DecisionSequence: row.DecisionSequence,
		Helpful: row.Helpful, NotHelpful: row.NotHelpful,
	}, nil
}

// EraseContributorVotes tombstones every live helpfulness vote of one
// contributor for account erasure. Rows stay for audit; the caller
// rebuilds affected tallies afterwards.
func (s *HelpfulnessStore) EraseContributorVotes(ctx context.Context, contributorRef string, at time.Time) (int64, error) {
	return community.New(s.pool).EraseHelpfulnessVotes(ctx, community.EraseHelpfulnessVotesParams{
		ContributorRef: contributorRef, DeletedAt: pgTime(at),
	})
}

func toHelpfulnessVote(row community.CommunityHelpfulnessVote) domain.StoredVote {
	var deleted *time.Time
	if row.DeletedAt.Valid {
		t := row.DeletedAt.Time
		deleted = &t
	}
	return domain.StoredVote{
		ID: uuidString(row.ID), ContributorRef: row.ContributorRef,
		ObservationID: uuidString(row.ObservationID), DecisionSequence: row.DecisionSequence,
		Choice: domain.Choice(row.Choice), ClientSubmissionID: row.ClientSubmissionID,
		CreatedAt: row.CreatedAt.Time, DeletedAt: deleted,
	}
}
