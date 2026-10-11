-- 000054: community helpfulness votes and tallies (CS02A). Append-only.
-- One live helpfulness vote per contributor+observation+decision sequence:
-- changes rewrite Choice in place, removals tombstone via deleted_at, and
-- lost-response retries converge on the contributor+submission natural key.
-- Tallies refresh in the same transaction as the vote write and rebuild
-- from live rows at any time. Observations stay immutable facts; the two
-- nullable visibility columns below are set only by moderation/author
-- flows (CS05/CS09), never by voting. No historical row is deleted.
ALTER TABLE community_observations
    ADD COLUMN hidden_at TIMESTAMPTZ,
    ADD COLUMN deleted_at TIMESTAMPTZ;
CREATE TABLE community_helpfulness_votes (
    id UUID PRIMARY KEY,
    contributor_ref TEXT NOT NULL,
    observation_id UUID NOT NULL REFERENCES community_observations (id) ON DELETE RESTRICT,
    decision_sequence BIGINT NOT NULL CONSTRAINT community_helpfulness_sequence_check CHECK (decision_sequence >= 1),
    choice TEXT NOT NULL CONSTRAINT community_helpfulness_choice_check CHECK (choice IN ('HELPFUL', 'NOT_HELPFUL')),
    client_submission_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX community_helpfulness_live_unique
    ON community_helpfulness_votes (contributor_ref, observation_id, decision_sequence)
    WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX community_helpfulness_natural_unique
    ON community_helpfulness_votes (contributor_ref, client_submission_id);
CREATE INDEX community_helpfulness_target_idx
    ON community_helpfulness_votes (observation_id, decision_sequence)
    WHERE deleted_at IS NULL;
CREATE TABLE community_helpfulness_tallies (
    observation_id UUID NOT NULL REFERENCES community_observations (id) ON DELETE RESTRICT,
    decision_sequence BIGINT NOT NULL CONSTRAINT community_helpfulness_tally_sequence_check CHECK (decision_sequence >= 1),
    helpful BIGINT NOT NULL DEFAULT 0 CONSTRAINT community_helpfulness_helpful_check CHECK (helpful >= 0),
    not_helpful BIGINT NOT NULL DEFAULT 0 CONSTRAINT community_helpfulness_not_helpful_check CHECK (not_helpful >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (observation_id, decision_sequence)
);
