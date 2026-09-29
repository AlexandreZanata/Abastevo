-- 000013: community confirmations and disputes (P06-T02). Immutable
-- support votes and structured reports. Confirmations converge retries
-- on the contributor natural key and refuse duplicate support through
-- the observation+contributor unique pair: one contributor never
-- amplifies one observation. Disputes converge retries the same way and
-- deduplicate active repeated reports through a partial unique index on
-- reporter+target+reason while OPEN; resolution history lands with
-- moderation (P07). Reports never edit facts or erase prices.
CREATE TABLE community_confirmations (
    id UUID PRIMARY KEY,
    observation_id UUID NOT NULL REFERENCES community_observations (id) ON DELETE RESTRICT,
    contributor_ref TEXT NOT NULL,
    client_submission_id TEXT NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    policy_version TEXT NOT NULL
);
CREATE UNIQUE INDEX community_confirmations_pair_unique
    ON community_confirmations (observation_id, contributor_ref);
CREATE UNIQUE INDEX community_confirmations_natural_unique
    ON community_confirmations (contributor_ref, client_submission_id);
CREATE INDEX community_confirmations_observation_idx
    ON community_confirmations (observation_id, received_at DESC);
CREATE TABLE community_disputes (
    id UUID PRIMARY KEY,
    target_observation_id UUID NOT NULL REFERENCES community_observations (id) ON DELETE RESTRICT,
    contributor_ref TEXT NOT NULL,
    client_submission_id TEXT NOT NULL,
    reason TEXT NOT NULL CONSTRAINT community_disputes_reason_check CHECK (reason IN
        ('PRICE_CHANGED', 'WRONG_STATION', 'WRONG_PRODUCT', 'WRONG_CONDITION', 'EVIDENCE_MISMATCH', 'OTHER')),
    detail TEXT NOT NULL DEFAULT '',
    replacement_id UUID REFERENCES community_observations (id) ON DELETE RESTRICT,
    status TEXT NOT NULL DEFAULT 'OPEN' CONSTRAINT community_disputes_status_check CHECK (status IN
        ('OPEN', 'RESOLVED', 'REJECTED')),
    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    policy_version TEXT NOT NULL
);
CREATE UNIQUE INDEX community_disputes_natural_unique
    ON community_disputes (contributor_ref, client_submission_id);
CREATE UNIQUE INDEX community_disputes_open_unique
    ON community_disputes (contributor_ref, target_observation_id, reason)
    WHERE status = 'OPEN';
CREATE INDEX community_disputes_target_idx
    ON community_disputes (target_observation_id, status, received_at DESC);
