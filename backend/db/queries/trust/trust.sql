-- Owned by trust. Decisions are insert-only; the current view
-- upserts to the latest verdict in the same transaction. No UPDATE or
-- DELETE path exists in this file: blocks, reversals and
-- rehabilitations all arrive as new decision rows.

-- name: InsertDecision :one
INSERT INTO trust_decisions
    (id, contributor_ref, tier, reason, case_refs, policy_version,
     occurred_at)
VALUES (@id, @contributor_ref, @tier, @reason, @case_refs,
    @policy_version, @occurred_at)
RETURNING id;

-- name: ListDecisions :many
SELECT id, contributor_ref, tier, reason, case_refs, policy_version,
    occurred_at
FROM trust_decisions
WHERE contributor_ref = @contributor_ref
ORDER BY occurred_at ASC, id ASC;

-- name: UpsertCurrent :one
INSERT INTO trust_current (contributor_ref, tier, version, updated_at)
VALUES (@contributor_ref, @tier, 1, @updated_at)
ON CONFLICT (contributor_ref) DO UPDATE SET
    tier = EXCLUDED.tier,
    version = trust_current.version + 1,
    updated_at = EXCLUDED.updated_at
RETURNING contributor_ref;

-- name: GetCurrent :one
SELECT contributor_ref, tier, version, updated_at
FROM trust_current
WHERE contributor_ref = @contributor_ref;

-- Erasure (P07-T04, B-BR-016): drop the rebuildable current view so the
-- erased owner reads as NEW, and unlink decision history to an opaque
-- token (reviewed-outcome counts survive, identity links do not). Only
-- these queries may remove trust rows; verdict recording stays
-- append-only.

-- name: DeleteTrustCurrent :execrows
DELETE FROM trust_current
WHERE contributor_ref = @contributor_ref;

-- name: UnlinkTrustDecisions :execrows
UPDATE trust_decisions
SET contributor_ref = @anon_ref
WHERE contributor_ref = @contributor_ref;
