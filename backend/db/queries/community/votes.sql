-- Owned by community. Votes and reports are insert-only: retries
-- converge on natural keys, duplicate support conflicts on the
-- observation+contributor pair, and active repeated reports converge on
-- the open triple. Resolution history lands with moderation (P07); no
-- UPDATE or DELETE path exists in this file.

-- name: InsertConfirmation :one
INSERT INTO community_confirmations
    (id, observation_id, contributor_ref, client_submission_id,
     received_at, policy_version)
VALUES (@id, @observation_id, @contributor_ref, @client_submission_id,
    @received_at, @policy_version)
ON CONFLICT (contributor_ref, client_submission_id) DO NOTHING
RETURNING id;

-- name: GetConfirmationByNaturalKey :one
SELECT id, observation_id, contributor_ref, client_submission_id,
    received_at, policy_version
FROM community_confirmations
WHERE contributor_ref = @contributor_ref AND client_submission_id = @client_submission_id;

-- name: InsertDispute :one
INSERT INTO community_disputes
    (id, target_observation_id, contributor_ref, client_submission_id,
     reason, detail, replacement_id, status, received_at, policy_version)
VALUES (@id, @target_observation_id, @contributor_ref,
    @client_submission_id, @reason, @detail, @replacement_id, @status,
    @received_at, @policy_version)
ON CONFLICT (contributor_ref, client_submission_id) DO NOTHING
RETURNING id;

-- name: GetDisputeByNaturalKey :one
SELECT id, target_observation_id, contributor_ref, client_submission_id,
    reason, detail, replacement_id, status, received_at, policy_version
FROM community_disputes
WHERE contributor_ref = @contributor_ref AND client_submission_id = @client_submission_id;

-- name: GetOpenDispute :one
SELECT id, target_observation_id, contributor_ref, client_submission_id,
    reason, detail, replacement_id, status, received_at, policy_version
FROM community_disputes
WHERE contributor_ref = @contributor_ref
    AND target_observation_id = @target_observation_id
    AND reason = @reason AND status = 'OPEN';
