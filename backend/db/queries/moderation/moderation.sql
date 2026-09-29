-- Owned by moderation. Cases are insert-only in T01: retries converge
-- on the open target, duplicate reports return the open case, and the
-- queue lists actionable cases by priority then age. No UPDATE or DELETE
-- path exists in this file: status moves through audited actions in
-- P07-T02, and history is never edited.

-- name: InsertCase :one
INSERT INTO moderation_cases
    (id, target_type, target_id, status, priority, reason, detail,
     evidence_id, opened_at, policy_version)
VALUES (@id, @target_type, @target_id, @status, @priority, @reason,
    @detail, @evidence_id, @opened_at, @policy_version)
ON CONFLICT DO NOTHING
RETURNING id;

-- name: GetCase :one
SELECT id, target_type, target_id, status, priority, reason, detail,
    evidence_id, opened_at, policy_version
FROM moderation_cases
WHERE id = @id;

-- name: GetOpenCase :one
SELECT id, target_type, target_id, status, priority, reason, detail,
    evidence_id, opened_at, policy_version
FROM moderation_cases
WHERE target_type = @target_type AND target_id = @target_id
    AND status IN ('OPEN', 'IN_REVIEW');

-- name: ListOpenCases :many
SELECT id, target_type, target_id, status, priority, reason, detail,
    evidence_id, opened_at, policy_version
FROM moderation_cases
WHERE status IN ('OPEN', 'IN_REVIEW')
    AND (
        CASE priority WHEN 'P1' THEN 0 WHEN 'P2' THEN 1 ELSE 2 END,
        opened_at, id
    ) > (sqlc.arg(cursor_rank)::INT, sqlc.arg(cursor_opened_at)::TIMESTAMPTZ, sqlc.arg(cursor_id)::UUID)
ORDER BY
    CASE priority WHEN 'P1' THEN 0 WHEN 'P2' THEN 1 ELSE 2 END ASC,
    opened_at ASC, id ASC
LIMIT sqlc.arg(page_limit);
