-- 000124: Stripe provider subscription lifecycle, distinct from invoice-funded
-- paid-through grants. Append-only after payments 000123.
CREATE TABLE payment_contracts (
    id UUID PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES accounts(id),
    offer_id TEXT NOT NULL CHECK (octet_length(offer_id) BETWEEN 1 AND 128),
    source TEXT NOT NULL CHECK (source = 'stripe_card'),
    environment TEXT NOT NULL CHECK (environment IN ('test','production')),
    merchant TEXT NOT NULL CHECK (octet_length(merchant) BETWEEN 1 AND 128),
    provider TEXT NOT NULL CHECK (provider = 'stripe'),
    remote_subscription_id TEXT NOT NULL CHECK (octet_length(remote_subscription_id) BETWEEN 1 AND 255),
    remote_customer_id TEXT NOT NULL CHECK (octet_length(remote_customer_id) BETWEEN 1 AND 255),
    provider_status TEXT NOT NULL CHECK (octet_length(provider_status) BETWEEN 1 AND 64),
    cancel_at_period_end BOOLEAN NOT NULL DEFAULT false,
    last_event_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT payment_contracts_remote_unique UNIQUE (provider,environment,merchant,remote_subscription_id)
);
CREATE INDEX payment_contracts_owner_idx ON payment_contracts(account_id,offer_id,updated_at DESC);
