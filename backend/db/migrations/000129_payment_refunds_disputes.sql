-- 000129: refunds, disputes/MED and provider holds. Refunds bind to the
-- original settled payment; concurrent reservations cannot exceed the
-- refundable principal. Disputes revoke only the matched unexpired grant.
-- Unknown remote money stays an explicit provider hold, never access.

ALTER TABLE payment_settlements
    ADD COLUMN remote_charge_id TEXT
        CHECK (remote_charge_id IS NULL OR octet_length(remote_charge_id) BETWEEN 1 AND 255);

CREATE TABLE payment_refunds (
    id UUID PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES accounts(id),
    offer_id TEXT NOT NULL CHECK (octet_length(offer_id) BETWEEN 1 AND 128),
    order_id UUID NOT NULL REFERENCES payment_orders(id),
    grant_id UUID REFERENCES payment_grants(id),
    settlement_id UUID NOT NULL REFERENCES payment_settlements(id),
    provider TEXT NOT NULL CHECK (provider IN ('stripe','asaas')),
    environment TEXT NOT NULL CHECK (environment IN ('test','production')),
    merchant TEXT NOT NULL CHECK (octet_length(merchant) BETWEEN 1 AND 128),
    operation_key TEXT NOT NULL CHECK (octet_length(operation_key) BETWEEN 1 AND 128),
    remote_payment_id TEXT NOT NULL CHECK (octet_length(remote_payment_id) BETWEEN 1 AND 255),
    remote_refund_id TEXT CHECK (remote_refund_id IS NULL OR octet_length(remote_refund_id) BETWEEN 1 AND 255),
    amount_centavos BIGINT NOT NULL CHECK (amount_centavos > 0 AND amount_centavos <= 100000000000),
    state TEXT NOT NULL CHECK (state IN ('REQUESTED','PENDING','SUCCEEDED','FAILED','DENIED')),
    reason TEXT NOT NULL CHECK (octet_length(reason) BETWEEN 1 AND 256),
    requested_by TEXT NOT NULL CHECK (octet_length(requested_by) BETWEEN 1 AND 128),
    approved_by TEXT CHECK (approved_by IS NULL OR octet_length(approved_by) BETWEEN 1 AND 128),
    requested_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    resolved_at TIMESTAMPTZ,
    last_error TEXT NOT NULL DEFAULT '' CHECK (octet_length(last_error) <= 500),
    CONSTRAINT payment_refunds_operation_unique
        UNIQUE (account_id,provider,environment,merchant,operation_key)
);
CREATE UNIQUE INDEX payment_refunds_remote_unique
    ON payment_refunds(provider,environment,merchant,remote_refund_id)
    WHERE remote_refund_id IS NOT NULL;
CREATE INDEX payment_refunds_grant_idx ON payment_refunds(grant_id,requested_at DESC)
    WHERE grant_id IS NOT NULL;
CREATE INDEX payment_refunds_settlement_idx ON payment_refunds(settlement_id);

CREATE TABLE payment_disputes (
    id UUID PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES accounts(id),
    offer_id TEXT NOT NULL CHECK (octet_length(offer_id) BETWEEN 1 AND 128),
    grant_id UUID REFERENCES payment_grants(id),
    settlement_id UUID NOT NULL REFERENCES payment_settlements(id),
    provider TEXT NOT NULL CHECK (provider IN ('stripe','asaas')),
    environment TEXT NOT NULL CHECK (environment IN ('test','production')),
    merchant TEXT NOT NULL CHECK (octet_length(merchant) BETWEEN 1 AND 128),
    remote_dispute_id TEXT NOT NULL CHECK (octet_length(remote_dispute_id) BETWEEN 1 AND 255),
    remote_payment_id TEXT CHECK (remote_payment_id IS NULL OR octet_length(remote_payment_id) BETWEEN 1 AND 255),
    remote_charge_id TEXT CHECK (remote_charge_id IS NULL OR octet_length(remote_charge_id) BETWEEN 1 AND 255),
    amount_centavos BIGINT NOT NULL CHECK (amount_centavos > 0 AND amount_centavos <= 100000000000),
    state TEXT NOT NULL CHECK (state IN ('PENDING','UNDER_REVIEW','WON','LOST','CLOSED')),
    opened_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    resolved_at TIMESTAMPTZ,
    last_event_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT payment_disputes_remote_unique
        UNIQUE (provider,environment,merchant,remote_dispute_id)
);
CREATE INDEX payment_disputes_grant_idx ON payment_disputes(grant_id)
    WHERE grant_id IS NOT NULL;

CREATE TABLE payment_provider_holds (
    id UUID PRIMARY KEY,
    provider TEXT NOT NULL CHECK (provider IN ('stripe','asaas')),
    environment TEXT NOT NULL CHECK (environment IN ('test','production')),
    merchant TEXT NOT NULL CHECK (octet_length(merchant) BETWEEN 1 AND 128),
    remote_hold_id TEXT NOT NULL CHECK (octet_length(remote_hold_id) BETWEEN 1 AND 255),
    remote_payment_id TEXT CHECK (remote_payment_id IS NULL OR octet_length(remote_payment_id) BETWEEN 1 AND 255),
    amount_centavos BIGINT CHECK (amount_centavos IS NULL OR (amount_centavos > 0 AND amount_centavos <= 100000000000)),
    reason TEXT NOT NULL DEFAULT '' CHECK (octet_length(reason) <= 256),
    state TEXT NOT NULL CHECK (state IN ('OPEN','REVIEWED','DISMISSED')),
    observed_at TIMESTAMPTZ NOT NULL,
    reviewed_at TIMESTAMPTZ,
    reviewed_by TEXT CHECK (reviewed_by IS NULL OR octet_length(reviewed_by) BETWEEN 1 AND 128),
    review_reason TEXT NOT NULL DEFAULT '' CHECK (octet_length(review_reason) <= 256),
    CONSTRAINT payment_provider_holds_remote_unique
        UNIQUE (provider,environment,merchant,remote_hold_id)
);
CREATE INDEX payment_provider_holds_open_idx ON payment_provider_holds(observed_at,id)
    WHERE state = 'OPEN';

CREATE TABLE payment_operator_actions (
    id UUID PRIMARY KEY,
    actor TEXT NOT NULL CHECK (octet_length(actor) BETWEEN 1 AND 128),
    action TEXT NOT NULL CHECK (action IN ('refund.request','refund.replay','hold.review')),
    target_id UUID NOT NULL,
    account_id UUID REFERENCES accounts(id),
    reason TEXT NOT NULL CHECK (octet_length(reason) BETWEEN 1 AND 256),
    amount_centavos BIGINT CHECK (amount_centavos IS NULL OR amount_centavos > 0),
    result TEXT NOT NULL CHECK (octet_length(result) BETWEEN 1 AND 128),
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX payment_operator_actions_target_idx
    ON payment_operator_actions(target_id,created_at DESC);
