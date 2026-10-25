-- 000126: Pix Automático provider-owned cycles, instruction tracking and
-- account eligibility. The application never creates recurring debit
-- instructions; Asaas SUBSCRIPTION remains the only cycle owner.

ALTER TABLE payment_mandates
    ADD COLUMN payment_creation_mode TEXT NOT NULL DEFAULT 'SUBSCRIPTION'
        CHECK (payment_creation_mode = 'SUBSCRIPTION'),
    ADD COLUMN retry_policy TEXT NOT NULL DEFAULT 'NOT_ALLOWED'
        CHECK (retry_policy = 'NOT_ALLOWED'),
    ADD COLUMN cancel_requested_at TIMESTAMPTZ,
    ADD COLUMN cancel_reason TEXT
        CHECK (cancel_reason IS NULL OR octet_length(cancel_reason) BETWEEN 1 AND 64),
    ADD COLUMN cancelled_at TIMESTAMPTZ;

ALTER TABLE payment_inbox
    ADD COLUMN resource_status TEXT NOT NULL DEFAULT ''
        CHECK (octet_length(resource_status) <= 64);

-- One nonterminal Pix Automático source per account/offer. Switching is PAY-14;
-- duplicate concurrent authorizations are refused here.
CREATE UNIQUE INDEX payment_mandates_one_nonterminal
    ON payment_mandates(account_id,offer_id)
    WHERE status IN ('CREATED','ACTIVE');

CREATE TABLE payment_pix_auto_instructions (
    id UUID PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES accounts(id),
    offer_id TEXT NOT NULL CHECK (octet_length(offer_id) BETWEEN 1 AND 128),
    environment TEXT NOT NULL CHECK (environment IN ('test','production')),
    merchant TEXT NOT NULL CHECK (octet_length(merchant) BETWEEN 1 AND 128),
    provider TEXT NOT NULL CHECK (provider = 'asaas'),
    remote_instruction_id TEXT NOT NULL CHECK (octet_length(remote_instruction_id) BETWEEN 1 AND 255),
    remote_authorization_id TEXT NOT NULL CHECK (octet_length(remote_authorization_id) BETWEEN 1 AND 255),
    remote_subscription_id TEXT CHECK (remote_subscription_id IS NULL OR octet_length(remote_subscription_id) BETWEEN 1 AND 255),
    remote_payment_id TEXT CHECK (remote_payment_id IS NULL OR octet_length(remote_payment_id) BETWEEN 1 AND 255),
    status TEXT NOT NULL CHECK (status IN ('AWAITING_REQUEST','SCHEDULED','DONE','REFUSED','CANCELLED')),
    purpose TEXT NOT NULL CHECK (purpose IN ('SCHEDULE','RETRY_AFTER_DUE_DATE')),
    retry_attempt INTEGER NOT NULL DEFAULT 0 CHECK (retry_attempt BETWEEN 0 AND 3),
    due_date DATE NOT NULL,
    refusal_reason TEXT CHECK (refusal_reason IS NULL OR octet_length(refusal_reason) BETWEEN 1 AND 128),
    last_event_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT payment_pix_auto_instructions_remote_unique
        UNIQUE (provider,environment,merchant,remote_instruction_id)
);
CREATE INDEX payment_pix_auto_instructions_mandate_idx
    ON payment_pix_auto_instructions(provider,environment,merchant,remote_authorization_id,due_date);

CREATE TABLE payment_pix_auto_eligibility (
    provider TEXT NOT NULL CHECK (provider = 'asaas'),
    environment TEXT NOT NULL CHECK (environment IN ('test','production')),
    merchant TEXT NOT NULL CHECK (octet_length(merchant) BETWEEN 1 AND 128),
    status TEXT NOT NULL CHECK (status IN ('ELIGIBLE','INELIGIBLE')),
    reason TEXT NOT NULL DEFAULT '' CHECK (octet_length(reason) <= 256),
    last_event_at TIMESTAMPTZ NOT NULL,
    observed_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (provider,environment,merchant)
);
