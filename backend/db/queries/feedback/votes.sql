-- Owned by feedback. One live vote per account/comment/revision
-- (P14-T04, B-BR-F05). Changes rewrite the choice, removals
-- tombstone, tallies recompute per revision in the same transaction;
-- a lost unique race re-reads the winner.

-- name: LockVote :one
SELECT id, account_id, comment_id, comment_revision, choice, created_at, deleted_at
FROM feedback_votes
WHERE account_id = @account_id AND comment_id = @comment_id
    AND comment_revision = @comment_revision AND deleted_at IS NULL
FOR UPDATE;

-- name: InsertVote :exec
INSERT INTO feedback_votes
    (id, account_id, comment_id, comment_revision, choice, created_at, deleted_at)
VALUES (@id, @account_id, @comment_id, @comment_revision, @choice, @created_at, NULL);

-- name: UpdateVoteChoice :exec
UPDATE feedback_votes
SET choice = @choice
WHERE id = @id AND deleted_at IS NULL;

-- name: TombstoneVote :execrows
UPDATE feedback_votes
SET deleted_at = @now
WHERE account_id = @account_id AND comment_id = @comment_id
    AND comment_revision = @comment_revision AND deleted_at IS NULL;

-- name: CountVotes :one
SELECT count(*) FILTER (WHERE choice = 'VALID')::bigint AS valid_count,
    count(*) FILTER (WHERE choice = 'INVALID')::bigint AS invalid_count
FROM feedback_votes
WHERE comment_id = @comment_id AND comment_revision = @comment_revision
    AND deleted_at IS NULL;

-- name: UpsertTally :exec
INSERT INTO feedback_tallies
    (comment_id, revision, valid_count, invalid_count, updated_at)
VALUES (@comment_id, @revision, @valid_count, @invalid_count, @updated_at)
ON CONFLICT (comment_id, revision) DO UPDATE SET
    valid_count = @valid_count,
    invalid_count = @invalid_count,
    updated_at = @updated_at;

-- IncrementTally applies signed deltas atomically for the write path.
-- Recompute-in-transaction loses updates under concurrency (two
-- writers snapshot overlapping states and last-writer-wins stale
-- counts); the single-statement increment serializes on the row.
-- Full recomputation stays in RebuildTally for reconciliation.

-- name: IncrementTally :exec
INSERT INTO feedback_tallies
    (comment_id, revision, valid_count, invalid_count, updated_at)
VALUES (@comment_id, @revision, @valid_delta, @invalid_delta, @updated_at)
ON CONFLICT (comment_id, revision) DO UPDATE SET
    valid_count = feedback_tallies.valid_count + EXCLUDED.valid_count,
    invalid_count = feedback_tallies.invalid_count + EXCLUDED.invalid_count,
    updated_at = EXCLUDED.updated_at;

-- name: GetTally :one
SELECT comment_id, revision, valid_count, invalid_count, updated_at
FROM feedback_tallies
WHERE comment_id = @comment_id AND revision = @revision;
