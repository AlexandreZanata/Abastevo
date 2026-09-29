-- Owned by community. Facts and decisions are insert-only: this file
-- contains no UPDATE or DELETE against community tables, and review
-- tooling must keep it that way. Corrections append new facts.

-- name: InsertObservation :one
INSERT INTO community_observations
    (id, contributor_ref, client_submission_id, station_id, fuel_product,
     unit, amount_milli_brl, raw_price_text, condition_kind, qualifier_key,
     evidence_id, received_at, claimed_captured_at, supersedes_id, policy_version)
VALUES (@id, @contributor_ref, @client_submission_id, @station_id,
    @fuel_product, @unit, @amount_milli_brl, @raw_price_text,
    @condition_kind, @qualifier_key, @evidence_id, @received_at,
    @claimed_captured_at, @supersedes_id, @policy_version)
ON CONFLICT (contributor_ref, client_submission_id) DO NOTHING
RETURNING id;

-- name: GetObservationByNaturalKey :one
SELECT id, contributor_ref, client_submission_id, station_id, fuel_product,
    unit, amount_milli_brl, raw_price_text, condition_kind, qualifier_key,
    evidence_id, received_at, claimed_captured_at, supersedes_id, policy_version
FROM community_observations
WHERE contributor_ref = @contributor_ref AND client_submission_id = @client_submission_id;

-- name: GetObservation :one
SELECT id, contributor_ref, client_submission_id, station_id, fuel_product,
    unit, amount_milli_brl, raw_price_text, condition_kind, qualifier_key,
    evidence_id, received_at, claimed_captured_at, supersedes_id, policy_version
FROM community_observations
WHERE id = @id;

-- name: AppendDecision :one
INSERT INTO community_observation_decisions
    (id, observation_id, sequence, to_state, reason_codes, policy_version,
     occurred_at, actor_ref)
VALUES (@id, @observation_id, @sequence, @to_state, @reason_codes,
    @policy_version, @occurred_at, @actor_ref)
RETURNING id;

-- name: ListDecisions :many
SELECT id, observation_id, sequence, to_state, reason_codes, policy_version,
    occurred_at, actor_ref
FROM community_observation_decisions
WHERE observation_id = @observation_id
ORDER BY sequence ASC;
