-- Owned by community. Projection writes serialize on a transactional
-- advisory lock per price key before loading votes: a stale worker can
-- never overwrite a fresher projection, and versions only move forward
-- under the lock. Reads hit the indexed current row, never history.

-- name: LockPriceKey :exec
SELECT pg_advisory_xact_lock(hashtext(@key));

-- name: EligibleAnchors :many
SELECT o.id, o.contributor_ref, o.client_submission_id, o.station_id,
    o.fuel_product, o.unit, o.amount_milli_brl, o.raw_price_text,
    o.condition_kind, o.qualifier_key, o.evidence_id, o.received_at,
    o.claimed_captured_at, o.supersedes_id, o.policy_version
FROM community_observations o
WHERE o.station_id = @station_id
    AND o.fuel_product = @fuel_product
    AND o.unit = @unit
    AND o.condition_kind = @condition_kind
    AND o.qualifier_key = @qualifier_key
    AND o.received_at >= @cutoff AND o.received_at <= @now
    AND o.hidden_at IS NULL AND o.deleted_at IS NULL
    AND (SELECT d.to_state FROM community_observation_decisions d
         WHERE d.observation_id = o.id
         ORDER BY d.sequence DESC LIMIT 1) = 'VALIDATED'
ORDER BY o.received_at ASC, o.id ASC;

-- name: ConfirmationsForAnchors :many
SELECT c.id, c.observation_id, c.contributor_ref, c.client_submission_id,
    c.received_at, c.policy_version
FROM community_confirmations c
WHERE c.observation_id = ANY(@anchor_ids::uuid[])
ORDER BY c.received_at ASC, c.id ASC;

-- name: UpsertProjection :execrows
INSERT INTO community_current_prices AS p
    (station_id, fuel_product, unit, condition_kind, qualifier_key,
     amount_milli_brl, availability, confidence,
     representative_observation_id, independent_supporters,
     confirmation_count, anchor_received_at, expires_at,
     next_recompute_at, computed_at, projection_version,
     algorithm_version, policy_config_version)
VALUES (@station_id, @fuel_product, @unit, @condition_kind,
    @qualifier_key, @amount_milli_brl, @availability, @confidence,
    @representative_observation_id, @independent_supporters,
    @confirmation_count, @anchor_received_at, @expires_at,
    @next_recompute_at, @computed_at, @projection_version,
    @algorithm_version, @policy_config_version)
ON CONFLICT (station_id, fuel_product, unit, condition_kind,
    qualifier_key) DO UPDATE SET
    amount_milli_brl = EXCLUDED.amount_milli_brl,
    availability = EXCLUDED.availability,
    confidence = EXCLUDED.confidence,
    representative_observation_id = EXCLUDED.representative_observation_id,
    independent_supporters = EXCLUDED.independent_supporters,
    confirmation_count = EXCLUDED.confirmation_count,
    anchor_received_at = EXCLUDED.anchor_received_at,
    expires_at = EXCLUDED.expires_at,
    next_recompute_at = EXCLUDED.next_recompute_at,
    computed_at = EXCLUDED.computed_at,
    projection_version = EXCLUDED.projection_version,
    algorithm_version = EXCLUDED.algorithm_version,
    policy_config_version = EXCLUDED.policy_config_version
WHERE p.projection_version < EXCLUDED.projection_version;

-- name: GetProjection :one
SELECT p.station_id, p.fuel_product, p.unit, p.condition_kind, p.qualifier_key,
    p.amount_milli_brl, p.availability, p.confidence,
    p.representative_observation_id, p.independent_supporters,
    p.confirmation_count, p.anchor_received_at, p.expires_at,
    p.next_recompute_at, p.computed_at, p.projection_version,
    p.algorithm_version, p.policy_config_version
FROM community_current_prices p
WHERE p.station_id = @station_id
    AND p.fuel_product = @fuel_product
    AND p.unit = @unit
    AND p.condition_kind = @condition_kind
    AND p.qualifier_key = @qualifier_key
 AND (p.representative_observation_id IS NULL OR EXISTS (
  SELECT 1 FROM community_observations visible WHERE visible.id=p.representative_observation_id
   AND visible.hidden_at IS NULL AND visible.deleted_at IS NULL
   AND COALESCE((SELECT d.to_state FROM community_observation_decisions d WHERE d.observation_id=visible.id ORDER BY d.sequence DESC LIMIT 1),'') <> 'REJECTED'
 ))
;

-- name: DueProjections :many
SELECT station_id, fuel_product, unit, condition_kind, qualifier_key,
    next_recompute_at
FROM community_current_prices
WHERE next_recompute_at IS NOT NULL AND next_recompute_at <= @now
ORDER BY next_recompute_at ASC
LIMIT @batch;

-- name: UpsertProjectionInputs :execrows
INSERT INTO community_projection_inputs AS i
    (projection_key, version, input_cutoff, supporting_event_ids,
     reason_codes, computed_at)
VALUES (@projection_key, @version, @input_cutoff, @supporting_event_ids,
    @reason_codes, @computed_at)
ON CONFLICT (projection_key) DO UPDATE SET
    version = EXCLUDED.version,
    input_cutoff = EXCLUDED.input_cutoff,
    supporting_event_ids = EXCLUDED.supporting_event_ids,
    reason_codes = EXCLUDED.reason_codes,
    computed_at = EXCLUDED.computed_at
WHERE i.version < EXCLUDED.version;

-- Internal version reads must include invisible representatives. Public filtering
-- must never reset the monotonic version used under LockPriceKey.
-- name: ProjectionVersion :one
SELECT projection_version FROM community_current_prices
WHERE station_id = @station_id AND fuel_product = @fuel_product AND unit = @unit
 AND condition_kind = @condition_kind AND qualifier_key = @qualifier_key;
