-- Owned by community. Facts and decisions are insert-only: this file
-- contains no UPDATE or DELETE against community tables, and review
-- tooling must keep it that way. Corrections append new facts.

-- name: InsertObservation :one
INSERT INTO community_observations
    (id, contributor_ref, client_submission_id, station_id, fuel_product,
     unit, amount_milli_brl, raw_price_text, condition_kind, qualifier_key,
     evidence_id, received_at, claimed_captured_at, supersedes_id, policy_version,
     location_verdict, location_proximity, location_reason)
VALUES (@id, @contributor_ref, @client_submission_id, @station_id,
    @fuel_product, @unit, @amount_milli_brl, @raw_price_text,
    @condition_kind, @qualifier_key, @evidence_id, @received_at,
    @claimed_captured_at, @supersedes_id, @policy_version,
    @location_verdict, @location_proximity, @location_reason)
ON CONFLICT (contributor_ref, client_submission_id) DO NOTHING
RETURNING id;

-- name: GetObservationByNaturalKey :one
SELECT id, contributor_ref, client_submission_id, station_id, fuel_product,
    unit, amount_milli_brl, raw_price_text, condition_kind, qualifier_key,
    evidence_id, received_at, claimed_captured_at, supersedes_id, policy_version,
    location_verdict, location_proximity, location_reason
FROM community_observations
WHERE contributor_ref = @contributor_ref AND client_submission_id = @client_submission_id;

-- name: GetObservation :one
SELECT id, contributor_ref, client_submission_id, station_id, fuel_product,
    unit, amount_milli_brl, raw_price_text, condition_kind, qualifier_key,
    evidence_id, received_at, claimed_captured_at, supersedes_id, policy_version,
    location_verdict, location_proximity, location_reason
FROM community_observations
WHERE id = @id;

-- Last site for teleport review (P16-T03B): the contributor's most
-- recent observation station and receipt time. Absence means no
-- baseline: first observations never teleport.
-- name: LastObservationSite :one
SELECT station_id, received_at
FROM community_observations
WHERE contributor_ref = @contributor_ref
ORDER BY received_at DESC, id::text DESC
LIMIT 1;

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

-- name: ListByContributor :many
SELECT id, contributor_ref, client_submission_id, station_id, fuel_product,
    unit, amount_milli_brl, raw_price_text, condition_kind, qualifier_key,
    evidence_id, received_at, claimed_captured_at, supersedes_id, policy_version,
    location_verdict, location_proximity, location_reason
FROM community_observations
WHERE contributor_ref = @contributor_ref
    AND (@has_cursor::boolean = FALSE OR
        (received_at, id::text) < (@after_time, @after_id::text))
ORDER BY received_at DESC, id::text DESC
LIMIT @limit_plus_one;

-- Erasure (P07-T04, B-BR-016, ADR-008): unlink contributor links in the
-- narrow privacy workflow while immutable price facts stay for history.
-- Observations keep product/price/provenance with an anonymized
-- reference; reporter references unlink per row so pair uniqueness
-- survives. Only these queries may rewrite contributor references;
-- normal writes never touch them.

-- name: UnlinkObservations :execrows
UPDATE community_observations
SET contributor_ref = @anon_ref
WHERE contributor_ref = @contributor_ref;

-- name: UnlinkConfirmations :execrows
UPDATE community_confirmations
SET contributor_ref = 'erased-' || id::text
WHERE contributor_ref = @contributor_ref;

-- name: UnlinkDisputes :execrows
UPDATE community_disputes
SET contributor_ref = 'erased-' || id::text
WHERE contributor_ref = @contributor_ref;

-- Retention metric (P07-T05): oldest stored fact for the retention
-- report. The 24-month observation purge needs an FK-consistent
-- cascade design before it deletes; until then this metric keeps the
-- horizon visible without touching history.

-- name: OldestObservation :one
SELECT received_at
FROM community_observations
ORDER BY received_at ASC
LIMIT 1;
