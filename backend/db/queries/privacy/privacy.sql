-- Owned by privacy. Requests are insert-once: retries converge on the
-- owner natural key, and outcomes move forward only (REQUESTED to READY
-- or FAILED) through the guarded transitions below. No mutation exists
-- outside those guards and no removal path exists in this file: expiry
-- purges arrive with the retention scheduler (P07-T05), and erasure
-- removes payloads through the deletion workflow (P07-T04), never by
-- editing export history.

-- name: InsertRequest :one
INSERT INTO privacy_requests
    (id, contributor_id, contributor_ref, client_submission_id, type,
     status, requested_at, policy_version)
VALUES (@id, @contributor_id, @contributor_ref, @client_submission_id,
    @type, @status, @requested_at, @policy_version)
ON CONFLICT DO NOTHING
RETURNING id;

-- name: GetRequest :one
SELECT id, contributor_id, contributor_ref, client_submission_id, type,
    status, archive, archive_sha256, requested_at, ready_at, expires_at,
    completed_at, policy_version
FROM privacy_requests
WHERE id = @id;

-- name: GetRequestByNaturalKey :one
SELECT id, contributor_id, contributor_ref, client_submission_id, type,
    status, archive, archive_sha256, requested_at, ready_at, expires_at,
    completed_at, policy_version
FROM privacy_requests
WHERE contributor_id = @contributor_id AND client_submission_id = @client_submission_id;

-- name: MarkReady :one
UPDATE privacy_requests
SET status = 'READY', archive = @archive, archive_sha256 = @archive_sha256,
    ready_at = @ready_at, expires_at = @expires_at,
    completed_at = @completed_at
WHERE id = @id AND status = 'REQUESTED'
RETURNING id;

-- name: MarkFailed :one
UPDATE privacy_requests
SET status = 'FAILED', completed_at = @completed_at
WHERE id = @id AND status = 'REQUESTED'
RETURNING id;

-- name: MarkDeletionReady :one
UPDATE privacy_requests
SET status = 'READY', ready_at = @ready_at, completed_at = @completed_at
WHERE id = @id AND status = 'REQUESTED' AND type = 'DELETION'
RETURNING id;

-- Deletion replay ledger (P07-T04): insert-once per contributor and
-- scope, contributor-scoped reads, replay marks. No DELETE path: the
-- ledger ages out with the backup horizon under retention (P07-T05).

-- name: InsertLedgerEntry :one
INSERT INTO privacy_deletion_ledger
    (id, contributor_id, contributor_ref, scope, reason, occurred_at,
     policy_version)
VALUES (@id, @contributor_id, @contributor_ref, @scope, @reason,
    @occurred_at, @policy_version)
ON CONFLICT DO NOTHING
RETURNING id;

-- name: ListLedgerByContributor :many
SELECT id, contributor_id, contributor_ref, scope, reason, occurred_at,
    replayed_at, policy_version
FROM privacy_deletion_ledger
WHERE contributor_id = @contributor_id
ORDER BY scope ASC;

-- name: MarkLedgerReplayed :execrows
UPDATE privacy_deletion_ledger
SET replayed_at = @replayed_at
WHERE id = @id;

-- name: PurgeOwnerArchives :execrows
UPDATE privacy_requests
SET archive = NULL
WHERE contributor_id = @contributor_id AND archive IS NOT NULL;
