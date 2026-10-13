-- PRM07-T02 owned rules and inbox. Append-only; rules never restore OS
-- permission or revoked device authority. Basic free alerts stay
-- independent. Inbox statuses extend in later tasks without schema churn.
CREATE TABLE alert_rules(
 account_id UUID NOT NULL REFERENCES accounts(id),
 rule_id UUID NOT NULL,
 payload JSONB NOT NULL CHECK(jsonb_typeof(payload)='object'),
 revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0),
 enabled BOOLEAN NOT NULL DEFAULT true,
 deleted BOOLEAN NOT NULL DEFAULT false,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(account_id, rule_id)
);
CREATE INDEX alert_rules_live_idx ON alert_rules(account_id, rule_id) WHERE NOT deleted;
CREATE TABLE alert_operations(
 account_id UUID NOT NULL REFERENCES accounts(id),
 operation_id UUID NOT NULL,
 payload_hash TEXT NOT NULL CHECK(payload_hash ~ '^[0-9a-f]{64}$'),
 receipt JSONB NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(account_id, operation_id)
);
CREATE TABLE alert_inbox(
 account_id UUID NOT NULL REFERENCES accounts(id),
 inbox_id UUID NOT NULL,
 rule_id UUID NOT NULL,
 payload JSONB NOT NULL CHECK(jsonb_typeof(payload)='object'),
 status TEXT NOT NULL DEFAULT 'unread',
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(account_id, inbox_id)
);
CREATE INDEX alert_inbox_rule_idx ON alert_inbox(account_id, rule_id);
