-- Owned by community. Helpfulness votes converge retries on the
-- contributor+submission natural key and keep one live row per
-- contributor+observation+sequence; tallies move in the same transaction
-- as the vote write and rebuild from live rows. Choice changes and
-- removals only touch live rows; this file holds no destructive path.

-- name: GetHelpfulnessTarget :one
SELECT o.id, o.contributor_ref, o.hidden_at, o.deleted_at,
    COALESCE((SELECT d.to_state FROM community_observation_decisions d
      WHERE d.observation_id = o.id ORDER BY d.sequence DESC LIMIT 1), '')::text AS state,
    (SELECT COALESCE(MAX(d.sequence), 0)::bigint FROM community_observation_decisions d
      WHERE d.observation_id = o.id) AS latest_sequence
FROM community_observations o
WHERE o.id = @observation_id;

-- name: LockHelpfulnessVote :one
SELECT id, contributor_ref, observation_id, decision_sequence,
    choice, client_submission_id, created_at, deleted_at
FROM community_helpfulness_votes
WHERE contributor_ref = @contributor_ref AND observation_id = @observation_id
    AND decision_sequence = @decision_sequence AND deleted_at IS NULL
FOR UPDATE;

-- name: GetHelpfulnessVoteBySubmission :one
SELECT id, contributor_ref, observation_id, decision_sequence,
    choice, client_submission_id, created_at, deleted_at
FROM community_helpfulness_votes
WHERE contributor_ref = @contributor_ref AND client_submission_id = @client_submission_id;

-- name: InsertHelpfulnessVote :exec
INSERT INTO community_helpfulness_votes
    (id, contributor_ref, observation_id, decision_sequence,
     choice, client_submission_id, created_at)
VALUES (@id, @contributor_ref, @observation_id, @decision_sequence,
    @choice, @client_submission_id, @created_at);

-- name: UpdateHelpfulnessChoice :exec
UPDATE community_helpfulness_votes
SET choice = @choice
WHERE id = @id AND deleted_at IS NULL;

-- name: TombstoneHelpfulnessVote :execrows
UPDATE community_helpfulness_votes
SET deleted_at = @deleted_at
WHERE contributor_ref = @contributor_ref AND observation_id = @observation_id
    AND decision_sequence = @decision_sequence AND deleted_at IS NULL;

-- name: BumpHelpfulnessTally :execrows
UPDATE community_helpfulness_tallies
SET helpful = helpful + @helpful_delta,
    not_helpful = not_helpful + @not_helpful_delta,
    updated_at = @updated_at
WHERE observation_id = @observation_id AND decision_sequence = @decision_sequence;

-- name: InsertHelpfulnessTally :exec
INSERT INTO community_helpfulness_tallies
    (observation_id, decision_sequence, helpful, not_helpful, updated_at)
VALUES (@observation_id, @decision_sequence, @helpful, @not_helpful, @updated_at);

-- name: LockHelpfulnessTarget :exec
SELECT pg_advisory_xact_lock(hashtext(@observation_id::text || ':' || @decision_sequence::text));

-- name: GetHelpfulnessTally :one
SELECT observation_id, decision_sequence, helpful, not_helpful
FROM community_helpfulness_tallies
WHERE observation_id = @observation_id AND decision_sequence = @decision_sequence;

-- name: RebuildHelpfulnessTally :one
WITH live AS (
    SELECT
        COUNT(*) FILTER (WHERE choice = 'HELPFUL') AS helpful,
        COUNT(*) FILTER (WHERE choice = 'NOT_HELPFUL') AS not_helpful
    FROM community_helpfulness_votes
    WHERE observation_id = @observation_id AND decision_sequence = @decision_sequence
        AND deleted_at IS NULL
)
INSERT INTO community_helpfulness_tallies
    (observation_id, decision_sequence, helpful, not_helpful, updated_at)
SELECT @observation_id, @decision_sequence, live.helpful, live.not_helpful, @updated_at
FROM live
ON CONFLICT (observation_id, decision_sequence) DO UPDATE SET
    helpful = EXCLUDED.helpful,
    not_helpful = EXCLUDED.not_helpful,
    updated_at = EXCLUDED.updated_at
RETURNING observation_id, decision_sequence, helpful, not_helpful;

-- name: GetHelpfulnessChoice :one
SELECT choice
FROM community_helpfulness_votes
WHERE contributor_ref = @contributor_ref AND observation_id = @observation_id
    AND decision_sequence = @decision_sequence AND deleted_at IS NULL;

-- name: EraseHelpfulnessVotes :execrows
UPDATE community_helpfulness_votes
SET deleted_at = @deleted_at
WHERE contributor_ref = @contributor_ref AND deleted_at IS NULL;
