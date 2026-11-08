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

-- Actions are insert-only. The case status moves exactly once per
-- action through TransitionCase below: the only UPDATE path in this
-- file, guarded to actionable states so closed cases never reopen.

-- name: InsertAction :one
INSERT INTO moderation_actions
    (id, case_id, actor_id, action, reason, occurred_at, policy_version)
VALUES (@id, @case_id, @actor_id, @action, @reason, @occurred_at,
    @policy_version)
RETURNING id;

-- name: GetAction :one
SELECT id, case_id, actor_id, action, reason, occurred_at, policy_version
FROM moderation_actions
WHERE id = @id;

-- name: ListActionsByCase :many
SELECT id, case_id, actor_id, action, reason, occurred_at, policy_version
FROM moderation_actions
WHERE case_id = @case_id
ORDER BY occurred_at ASC, id ASC;

-- name: TransitionCase :one
UPDATE moderation_cases SET status = @status
WHERE id = @id AND status IN ('OPEN', 'IN_REVIEW')
RETURNING id;

-- Retention (P07-T05): purge long-closed cases with their audit rows.
-- Accountability retention is 12 months initially (SECURITY_PRIVACY):
-- only RESOLVED/REJECTED cases older than the cutoff go, oldest first
-- in bounded batches, audit rows before their cases.

-- name: OldestClosedCase :one
SELECT opened_at
FROM moderation_cases
WHERE status IN ('RESOLVED', 'REJECTED') AND opened_at < @cutoff
ORDER BY opened_at ASC
LIMIT 1;

-- name: PurgeClosedCaseActions :execrows
DELETE FROM moderation_actions
WHERE case_id IN (
    SELECT old.id FROM moderation_cases AS old
    WHERE old.status IN ('RESOLVED', 'REJECTED') AND old.opened_at < @cutoff
    ORDER BY old.opened_at ASC
    LIMIT @batch
);

-- name: PurgeClosedCases :execrows
DELETE FROM moderation_cases AS gone
WHERE gone.id IN (
    SELECT old.id FROM moderation_cases AS old
    WHERE old.status IN ('RESOLVED', 'REJECTED') AND old.opened_at < @cutoff
    ORDER BY old.opened_at ASC
    LIMIT @batch
);
