-- 000125: Pix Automático authorization/mandate lifecycle. The initial paid
-- period remains an invoice-funded payment_grant; this table only tracks consent
-- and future-debit authority.
CREATE TABLE payment_mandates (
    id UUID PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES accounts(id),
    offer_id TEXT NOT NULL CHECK (octet_length(offer_id) BETWEEN 1 AND 128),
    order_id UUID NOT NULL UNIQUE REFERENCES payment_orders(id),
    environment TEXT NOT NULL CHECK (environment IN ('test','production')),
    merchant TEXT NOT NULL CHECK (octet_length(merchant) BETWEEN 1 AND 128),
    provider TEXT NOT NULL CHECK (provider = 'asaas'),
    remote_authorization_id TEXT NOT NULL CHECK (octet_length(remote_authorization_id) BETWEEN 1 AND 255),
    remote_subscription_id TEXT CHECK (remote_subscription_id IS NULL OR octet_length(remote_subscription_id) BETWEEN 1 AND 255),
    contract_id TEXT NOT NULL CHECK (octet_length(contract_id) BETWEEN 1 AND 35),
    remote_customer_id TEXT NOT NULL CHECK (octet_length(remote_customer_id) BETWEEN 1 AND 255),
    status TEXT NOT NULL CHECK (status IN ('CREATED','ACTIVE','REFUSED','CANCELLED','EXPIRED')),
    frequency TEXT NOT NULL CHECK (frequency = 'MONTHLY'),
    value_centavos BIGINT NOT NULL CHECK (value_centavos > 0 AND value_centavos <= 100000000000),
    start_date DATE NOT NULL,
    finish_date DATE,
    consent_version TEXT NOT NULL CHECK (octet_length(consent_version) BETWEEN 1 AND 64),
    consent_at TIMESTAMPTZ NOT NULL,
    last_event_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT payment_mandates_remote_unique UNIQUE (provider,environment,merchant,remote_authorization_id),
    CONSTRAINT payment_mandates_contract_unique UNIQUE (provider,environment,merchant,contract_id),
    CONSTRAINT payment_mandates_finish CHECK (finish_date IS NULL OR finish_date >= start_date)
);
CREATE INDEX payment_mandates_owner_idx ON payment_mandates(account_id,offer_id,updated_at DESC);
CREATE UNIQUE INDEX payment_mandates_subscription_unique
    ON payment_mandates(provider,environment,merchant,remote_subscription_id)
    WHERE remote_subscription_id IS NOT NULL;
