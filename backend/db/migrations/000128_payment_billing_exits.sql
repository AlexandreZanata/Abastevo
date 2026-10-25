-- 000128: durable owner/account billing exits. Stopping renewal is separate
-- from expiry, payment/refund and account erasure. No provider call or refund
-- is implied by recording an intent.

CREATE TABLE payment_billing_exits (
    id UUID PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES accounts(id),
    operation_key TEXT NOT NULL CHECK (octet_length(operation_key) BETWEEN 1 AND 128),
    reason TEXT NOT NULL CHECK (reason IN ('OWNER_REQUEST','ACCOUNT_DELETION')),
    state TEXT NOT NULL CHECK (state IN ('PENDING','DONE','REVIEW')),
    requested_at TIMESTAMPTZ NOT NULL,
    last_event_at TIMESTAMPTZ NOT NULL,
    last_error TEXT NOT NULL DEFAULT '' CHECK (octet_length(last_error) <= 500),
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT payment_billing_exits_operation_unique UNIQUE (account_id,operation_key)
);
CREATE INDEX payment_billing_exits_owner_idx
    ON payment_billing_exits(account_id,requested_at DESC);

CREATE TABLE payment_billing_exit_sources (
    id UUID PRIMARY KEY,
    exit_id UUID NOT NULL REFERENCES payment_billing_exits(id) ON DELETE CASCADE,
    account_id UUID NOT NULL REFERENCES accounts(id),
    source TEXT NOT NULL CHECK (source IN ('stripe_card','asaas_pix_automatic')),
    local_id TEXT NOT NULL CHECK (octet_length(local_id) BETWEEN 1 AND 128),
    remote_id TEXT NOT NULL CHECK (octet_length(remote_id) BETWEEN 1 AND 255),
    offer_id TEXT NOT NULL CHECK (octet_length(offer_id) BETWEEN 1 AND 128),
    provider TEXT NOT NULL CHECK (provider IN ('stripe','asaas')),
    environment TEXT NOT NULL CHECK (environment IN ('test','production')),
    merchant TEXT NOT NULL CHECK (octet_length(merchant) BETWEEN 1 AND 128),
    state TEXT NOT NULL CHECK (state IN ('PENDING','CANCELING','DONE','REVIEW')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts BETWEEN 0 AND 20),
    lease_until TIMESTAMPTZ,
    last_error TEXT NOT NULL DEFAULT '' CHECK (octet_length(last_error) <= 500),
    last_event_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT payment_billing_exit_sources_unique UNIQUE (exit_id,source,local_id),
    CONSTRAINT payment_billing_exit_sources_lease CHECK (
        state <> 'CANCELING' OR lease_until IS NOT NULL
    )
);
CREATE INDEX payment_billing_exit_sources_pending_idx
    ON payment_billing_exit_sources(state,lease_until,exit_id);
