-- Owned by safety (B-BR-SAFE-01…09). Private per-account blocks and user
-- reports. Account identifiers stay in these rows; no query projects them
-- to a caller other than the owning account's own blocked set.

-- name: LockSafetySubject :exec
-- Serializes one account's block/report admission (active cap, replay
-- and quota checks) without locking the accounts row.
SELECT pg_advisory_xact_lock(hashtextextended(@subject::text, 0));

-- name: GetBlockByPair :one
SELECT id, label, source_kind, created_at
FROM community_user_blocks
WHERE blocker_account_id = @blocker_account_id AND blocked_account_id = @blocked_account_id;

-- name: CountActiveBlocks :one
SELECT count(*)::int AS active
FROM community_user_blocks
WHERE blocker_account_id = @blocker_account_id;

-- name: InsertBlock :one
INSERT INTO community_user_blocks
    (id, blocker_account_id, blocked_account_id, label, source_kind, created_at)
VALUES (@id, @blocker_account_id, @blocked_account_id, @label, @source_kind, @created_at)
RETURNING id, label, source_kind, created_at;

-- name: ListBlocks :many
SELECT id, label, source_kind, created_at
FROM community_user_blocks
WHERE blocker_account_id = @blocker_account_id
ORDER BY created_at DESC, id DESC
LIMIT @row_limit;

-- name: ListBlockedAccounts :many
SELECT blocked_account_id
FROM community_user_blocks
WHERE blocker_account_id = @blocker_account_id;

-- name: DeleteBlock :execrows
DELETE FROM community_user_blocks
WHERE id = @id AND blocker_account_id = @blocker_account_id;

-- name: GetReportByPair :one
SELECT id, reason, created_at
FROM community_user_reports
WHERE reporter_account_id = @reporter_account_id AND reported_account_id = @reported_account_id;

-- name: CountReportsSince :one
SELECT count(*)::int AS reports
FROM community_user_reports
WHERE reporter_account_id = @reporter_account_id AND created_at > @since;

-- name: InsertReport :one
INSERT INTO community_user_reports
    (id, reporter_account_id, reported_account_id, source_kind, source_id, reason, detail, created_at)
VALUES (@id, @reporter_account_id, @reported_account_id, @source_kind, @source_id, @reason, @detail, @created_at)
RETURNING id, reason, created_at;

-- name: SetReportCase :exec
UPDATE community_user_reports
SET moderation_case_id = @moderation_case_id
WHERE id = @id;

-- name: DeleteBlocksForErasure :execrows
-- Account erasure removes blocks the account made or received, bounded.
DELETE FROM community_user_blocks
WHERE id IN (
    SELECT b.id FROM community_user_blocks b
    WHERE b.blocker_account_id = @account_id OR b.blocked_account_id = @account_id
    ORDER BY b.id
    LIMIT @row_limit
);

-- name: ScrubReportDetailForErasure :execrows
-- Account erasure scrubs free text of reports the account filed, bounded.
UPDATE community_user_reports
SET detail = ''
WHERE id IN (
    SELECT r.id FROM community_user_reports r
    WHERE r.reporter_account_id = @account_id AND r.detail <> ''
    ORDER BY r.id
    LIMIT @row_limit
);

-- name: HasPendingSafetyErasure :one
SELECT EXISTS (
    SELECT 1 FROM community_user_blocks
    WHERE blocker_account_id = @account_id OR blocked_account_id = @account_id
) OR EXISTS (
    SELECT 1 FROM community_user_reports
    WHERE reporter_account_id = @account_id AND detail <> ''
) AS pending;
