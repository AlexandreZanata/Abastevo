-- PRM07-T05 opaque push-token enrollment per account. Tokens stay out of
-- logs and payloads; sends address one enrolled unrevoked token. Inbox rows
-- remain the fallback when the provider is unavailable. Real provider sends
-- stay PROOF_OWED; no SDK or credential leaves the restricted configuration.
CREATE TABLE alert_push_tokens(
 account_id UUID NOT NULL REFERENCES accounts(id),
 token TEXT NOT NULL CHECK(length(token) BETWEEN 1 AND 256),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 revoked_at TIMESTAMPTZ,
 PRIMARY KEY(account_id, token)
);
