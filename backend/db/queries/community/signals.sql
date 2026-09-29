-- Owned by community. Derived signal projection: bands and reason codes
-- only, never coordinates or meter-level claims (see the privacy
-- column test). Recomputable: upserts converge by observation.

-- name: UpsertSignals :one
INSERT INTO community_observation_signals
    (observation_id, proximity_band, recency_band, capture_flag,
     photo_signal, duplicate_count, regional_band, risk_codes,
     needs_review, policy_version, computed_at)
VALUES (@observation_id, @proximity_band, @recency_band, @capture_flag,
    @photo_signal, @duplicate_count, @regional_band, @risk_codes,
    @needs_review, @policy_version, @computed_at)
ON CONFLICT (observation_id) DO UPDATE SET
    proximity_band = EXCLUDED.proximity_band,
    recency_band = EXCLUDED.recency_band,
    capture_flag = EXCLUDED.capture_flag,
    photo_signal = EXCLUDED.photo_signal,
    duplicate_count = EXCLUDED.duplicate_count,
    regional_band = EXCLUDED.regional_band,
    risk_codes = EXCLUDED.risk_codes,
    needs_review = EXCLUDED.needs_review,
    policy_version = EXCLUDED.policy_version,
    computed_at = EXCLUDED.computed_at
RETURNING observation_id;

-- name: GetSignals :one
SELECT observation_id, proximity_band, recency_band, capture_flag,
    photo_signal, duplicate_count, regional_band, risk_codes,
    needs_review, policy_version, computed_at
FROM community_observation_signals
WHERE observation_id = @observation_id;
