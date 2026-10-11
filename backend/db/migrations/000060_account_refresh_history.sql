-- 000060: bounded hashed refresh rotation history (SEC07-A, AUD-08).
-- Proven-reuse detection without revoking on random invalid tokens.
-- Only salted verifiers persist; plaintext tokens never reach storage.
-- Append-only; one row per superseded verifier, pruned to the newest
-- domain.MaxRefreshHistory (=8) on every rotation.
CREATE TABLE account_session_refresh_history (
    family_id UUID NOT NULL REFERENCES account_session_families (id) ON DELETE CASCADE,
    refresh_salt TEXT NOT NULL,
    refresh_hash TEXT NOT NULL,
    rotated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (family_id, refresh_hash)
);
CREATE INDEX account_session_refresh_history_family_idx
    ON account_session_refresh_history (family_id, rotated_at DESC);
