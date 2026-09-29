-- 000008: community observations and decisions (P04-T03). Append-only.
-- Observations are immutable facts: no UPDATE or DELETE path exists in the
-- owned queries, and corrections append new rows with supersedes links.
-- The natural key (contributor_ref, client_submission_id) makes retries
-- converge on one row. Decisions append the validation history with one
-- sequence per observation; state itself is always derived, never stored.
-- Least-privilege database roles land with provisioning (P08); until then
-- the repository exposes no mutation method at all.
CREATE TABLE community_observations (
    id UUID PRIMARY KEY,
    contributor_ref TEXT NOT NULL,
    client_submission_id TEXT NOT NULL,
    station_id UUID NOT NULL REFERENCES directory_stations (id) ON DELETE RESTRICT,
    fuel_product TEXT NOT NULL,
    unit TEXT NOT NULL,
    amount_milli_brl BIGINT NOT NULL CHECK (amount_milli_brl >= 1),
    raw_price_text TEXT NOT NULL DEFAULT '',
    condition_kind TEXT NOT NULL,
    qualifier_key TEXT NOT NULL,
    evidence_id UUID,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    claimed_captured_at TIMESTAMPTZ,
    supersedes_id UUID REFERENCES community_observations (id) ON DELETE RESTRICT,
    policy_version TEXT NOT NULL
);
CREATE UNIQUE INDEX community_observations_natural_unique
    ON community_observations (contributor_ref, client_submission_id);
CREATE INDEX community_observations_price_key_idx
    ON community_observations (station_id, fuel_product, unit, received_at DESC);
CREATE INDEX community_observations_contributor_idx
    ON community_observations (contributor_ref, received_at DESC);
CREATE TABLE community_observation_decisions (
    id UUID PRIMARY KEY,
    observation_id UUID NOT NULL REFERENCES community_observations (id) ON DELETE RESTRICT,
    sequence BIGINT NOT NULL,
    to_state TEXT NOT NULL,
    reason_codes TEXT[] NOT NULL DEFAULT '{}',
    policy_version TEXT NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    actor_ref TEXT NOT NULL DEFAULT '',
    CONSTRAINT community_observation_decisions_state_check CHECK (to_state IN
        ('RECEIVED', 'VALIDATING', 'VALIDATED', 'REJECTED'))
);
CREATE UNIQUE INDEX community_observation_decisions_sequence_unique
    ON community_observation_decisions (observation_id, sequence);
