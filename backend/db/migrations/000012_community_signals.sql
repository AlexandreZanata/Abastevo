-- 000012: community observation signals (P06-T01). Private derived
-- bands per observation for consensus input: proximity, recency,
-- capture, photo and regional classes plus risk codes and a review
-- flag. Bands only — this table and its owned queries must never gain
-- coordinate, meter-level or payload columns; exact inputs live in
-- memory during derivation and never persist (privacy by schema, proven
-- by test). The projection is mutable and rebuildable: upserts converge
-- recomputation without history forks.
CREATE TABLE community_observation_signals (
    observation_id UUID PRIMARY KEY REFERENCES community_observations (id) ON DELETE RESTRICT,
    proximity_band TEXT NOT NULL,
    recency_band TEXT NOT NULL,
    capture_flag TEXT NOT NULL,
    photo_signal TEXT NOT NULL,
    duplicate_count INTEGER NOT NULL DEFAULT 0 CHECK (duplicate_count >= 0),
    regional_band TEXT NOT NULL,
    risk_codes TEXT[] NOT NULL DEFAULT '{}',
    needs_review BOOLEAN NOT NULL DEFAULT FALSE,
    policy_version TEXT NOT NULL,
    computed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX community_observation_signals_review_idx
    ON community_observation_signals (needs_review, computed_at DESC);
