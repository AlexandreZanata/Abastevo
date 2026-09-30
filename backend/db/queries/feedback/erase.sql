-- Owned by feedback. Account-footprint export and erasure (P14-T05B,
-- B-BR-F02/F07). Export lists one account's rows for the owner archive;
-- erasure tombstones live rows by account and lets the caller rebuild the
-- affected aggregates from live rows. History stays for audit (deleted_at),
-- aggregates read live rows only. No cross-account rows ever match.

-- name: ListRatingsByAccount :many
SELECT id, account_id, station_id, product, stars, revision, created_at, deleted_at
FROM feedback_ratings
WHERE account_id = @account_id
ORDER BY station_id, product, created_at, id;

-- name: ListCommentsByAccount :many
SELECT id, account_id, station_id, product, parent_id, depth, text,
    revision, created_at, updated_at, deleted_at, visibility
FROM feedback_comments
WHERE account_id = @account_id
ORDER BY created_at, id;

-- name: ListVotesByAccount :many
SELECT id, account_id, comment_id, comment_revision, choice, created_at, deleted_at
FROM feedback_votes
WHERE account_id = @account_id
ORDER BY comment_id, comment_revision, created_at, id;
