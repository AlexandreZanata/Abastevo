-- Deterministic removal of geometric penalties only; never create media/evidence or rewrite audit history.
UPDATE community_observation_signals
SET proximity_band = 'UNKNOWN',
    risk_codes = ARRAY(SELECT code FROM unnest(risk_codes) AS code
      WHERE code NOT IN ('missing-claim','unknown-station-point','poor-gps-accuracy','far-from-station','teleport-suspect')),
    needs_review = EXISTS(SELECT 1 FROM unnest(risk_codes) AS code
      WHERE code NOT IN ('missing-claim','unknown-station-point','poor-gps-accuracy','far-from-station','teleport-suspect')),
    policy_version = 'signals-community-v2', computed_at = now()
WHERE policy_version <> 'signals-community-v2';

-- Existing bounded consensus-boundary sweeper rebuilds retained keys with the new algorithm.
-- Do not insert unbounded jobs or rewrite historical inputs/results during migration.
UPDATE community_current_prices SET next_recompute_at = now()
WHERE algorithm_version <> 'consensus-community-v2';
