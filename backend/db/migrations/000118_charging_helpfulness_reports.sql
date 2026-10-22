-- EV community projection support: helpfulness votes, reports for
-- operator review and visibility flags. Observations stay immutable
-- facts; visibility is set only by moderation flows, never by voting.
-- No payment weight, automatic certification or live-occupancy claim.
CREATE TABLE charging_helpfulness_votes (
 id UUID PRIMARY KEY,
 account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 observation_id UUID NOT NULL REFERENCES charging_observations(id) ON DELETE CASCADE,
 choice TEXT NOT NULL CHECK (choice IN ('HELPFUL','NOT_HELPFUL')),
 client_submission_id TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 deleted_at TIMESTAMPTZ,
 CHECK (client_submission_id <> ''),
 UNIQUE(account_id, client_submission_id)
);
CREATE UNIQUE INDEX charging_helpfulness_live_unique
 ON charging_helpfulness_votes (account_id, observation_id) WHERE deleted_at IS NULL;
CREATE INDEX charging_helpfulness_target_idx
 ON charging_helpfulness_votes (observation_id) WHERE deleted_at IS NULL;

CREATE TABLE charging_observation_reports (
 id UUID PRIMARY KEY,
 account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 observation_id UUID NOT NULL REFERENCES charging_observations(id) ON DELETE CASCADE,
 reason TEXT NOT NULL CHECK (reason IN ('INACCURATE','SPAM','PRIVATE_DATA','OTHER')),
 detail TEXT NOT NULL CHECK (octet_length(detail) <= 1024),
 status TEXT NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN','RESOLVED','DISMISSED')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(account_id, observation_id)
);

ALTER TABLE charging_observations ADD COLUMN hidden_at TIMESTAMPTZ;
