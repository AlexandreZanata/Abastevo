-- PRM01-T04: authenticated provider receipts, never client entitlement events.
CREATE TABLE subscription_provider_notifications(
 environment TEXT NOT NULL CHECK(environment IN ('test','production')),
 message_id TEXT NOT NULL CHECK(length(message_id) BETWEEN 1 AND 128),
 payload_hash TEXT NOT NULL CHECK(payload_hash ~ '^[0-9a-f]{64}$'),
 token_digest TEXT NOT NULL CHECK(token_digest ~ '^[0-9a-f]{64}$'),
 received_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(environment,message_id)
);
CREATE INDEX subscription_notifications_received_idx ON subscription_provider_notifications(received_at);
