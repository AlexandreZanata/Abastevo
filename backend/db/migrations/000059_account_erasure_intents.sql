-- 000059: durable account-erasure intents (SEC04-B, AUD-05). The
-- account DeleteAccount transaction captures contributor bindings
-- BEFORE dropping them and records one intent row per linked
-- contributor in the same transaction: revocation and intent are
-- atomic, so a crash can never unlink without a repairable intent.
-- The worker-side erasure job consumes these rows through the
-- privacy deletion ledger; replay converges on the natural key.
-- Append-only; never rewrite earlier files.
CREATE TABLE account_erasure_intents (
    account_id UUID NOT NULL REFERENCES accounts (id),
    contributor_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (account_id, contributor_id)
);
CREATE INDEX account_erasure_intents_account_idx
    ON account_erasure_intents (account_id);
