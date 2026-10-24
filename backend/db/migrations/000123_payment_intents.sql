-- 000123: durable payment intents, remote-attempt fences, settlements, grants,
-- cycles and provider-event inbox. Outbox work reuses job_queue (000007) in the
-- same transaction. 000122 is owned by the MAP effort; this file reserves 123.
-- No provider, secret, price or live sale is enabled by this schema.

CREATE TABLE payment_customers (
    account_id UUID NOT NULL REFERENCES accounts(id),
    provider TEXT NOT NULL CHECK (provider IN ('stripe','asaas')),
    environment TEXT NOT NULL CHECK (environment IN ('test','production')),
    merchant TEXT NOT NULL CHECK (octet_length(merchant) BETWEEN 1 AND 128),
    remote_customer_id TEXT NOT NULL CHECK (octet_length(remote_customer_id) BETWEEN 1 AND 200),
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (account_id,provider,environment,merchant),
    CONSTRAINT payment_customers_remote_unique UNIQUE (provider,environment,merchant,remote_customer_id)
);

CREATE TABLE payment_orders (
    id UUID PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES accounts(id),
    offer_id TEXT NOT NULL CHECK (octet_length(offer_id) BETWEEN 1 AND 128),
    catalog_version TEXT NOT NULL CHECK (octet_length(catalog_version) BETWEEN 1 AND 128),
    channel TEXT NOT NULL CHECK (channel IN ('stripe_card','asaas_pix_manual','asaas_pix_automatic')),
    amount_centavos BIGINT NOT NULL CHECK (amount_centavos > 0 AND amount_centavos <= 100000000000),
    currency TEXT NOT NULL CHECK (currency = 'BRL'),
    capabilities TEXT[] NOT NULL CHECK (cardinality(capabilities) > 0),
    state TEXT NOT NULL CHECK (state IN ('QUOTED','PENDING','PAID','EXPIRED','CANCELED')),
    quoted_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    paid_at TIMESTAMPTZ,
    settlement_id UUID,
    revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT payment_orders_window CHECK (expires_at > quoted_at),
    CONSTRAINT payment_orders_paid_evidence CHECK (
        state <> 'PAID' OR (paid_at IS NOT NULL AND settlement_id IS NOT NULL)
    )
);
CREATE INDEX payment_orders_owner_idx ON payment_orders(account_id,created_at DESC,id);
CREATE INDEX payment_orders_open_idx ON payment_orders(expires_at,id)
    WHERE state IN ('QUOTED','PENDING');
CREATE UNIQUE INDEX payment_orders_settlement_unique ON payment_orders(settlement_id)
    WHERE settlement_id IS NOT NULL;

CREATE TABLE payment_operations (
    id UUID PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES accounts(id),
    provider TEXT NOT NULL CHECK (provider IN ('stripe','asaas')),
    environment TEXT NOT NULL CHECK (environment IN ('test','production')),
    merchant TEXT NOT NULL CHECK (octet_length(merchant) BETWEEN 1 AND 128),
    operation_key TEXT NOT NULL CHECK (octet_length(operation_key) BETWEEN 1 AND 128),
    kind TEXT NOT NULL CHECK (kind IN (
        'create_customer','create_order','create_attempt','record_settlement',
        'refund','cancel','switch','reconcile','webhook'
    )),
    payload_hash TEXT NOT NULL CHECK (payload_hash ~ '^[0-9a-f]{64}$'),
    order_id UUID REFERENCES payment_orders(id),
    state TEXT NOT NULL CHECK (state IN (
        'PENDING','DISPATCHED','SUCCEEDED','FAILED','OUTCOME_UNKNOWN','QUARANTINED','CANCELED'
    )),
    generation BIGINT NOT NULL DEFAULT 1 CHECK (generation > 0),
    attempt_no INTEGER NOT NULL DEFAULT 1 CHECK (attempt_no BETWEEN 1 AND 5),
    lease_until TIMESTAMPTZ,
    remote_id TEXT CHECK (remote_id IS NULL OR octet_length(remote_id) BETWEEN 1 AND 255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    completed_at TIMESTAMPTZ,
    CONSTRAINT payment_operations_key UNIQUE (account_id,provider,environment,merchant,operation_key),
    CONSTRAINT payment_operations_lease CHECK (
        state NOT IN ('PENDING','DISPATCHED','OUTCOME_UNKNOWN') OR lease_until IS NOT NULL
    )
);
CREATE INDEX payment_operations_order_idx ON payment_operations(order_id) WHERE order_id IS NOT NULL;
CREATE INDEX payment_operations_lease_idx ON payment_operations(lease_until)
    WHERE state IN ('PENDING','DISPATCHED','OUTCOME_UNKNOWN');

CREATE TABLE payment_attempts (
    id UUID PRIMARY KEY,
    operation_id UUID NOT NULL REFERENCES payment_operations(id) ON DELETE CASCADE,
    order_id UUID NOT NULL REFERENCES payment_orders(id),
    attempt_no INTEGER NOT NULL CHECK (attempt_no BETWEEN 1 AND 5),
    generation BIGINT NOT NULL CHECK (generation > 0),
    provider TEXT NOT NULL CHECK (provider IN ('stripe','asaas')),
    environment TEXT NOT NULL CHECK (environment IN ('test','production')),
    merchant TEXT NOT NULL CHECK (octet_length(merchant) BETWEEN 1 AND 128),
    payload_hash TEXT NOT NULL CHECK (payload_hash ~ '^[0-9a-f]{64}$'),
    state TEXT NOT NULL CHECK (state IN (
        'CREATED','DISPATCHED','SUCCEEDED','FAILED','UNKNOWN','QUARANTINED','CANCELED'
    )),
    remote_id TEXT CHECK (remote_id IS NULL OR octet_length(remote_id) BETWEEN 1 AND 255),
    lease_until TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    completed_at TIMESTAMPTZ,
    CONSTRAINT payment_attempts_no UNIQUE (operation_id,attempt_no),
    CONSTRAINT payment_attempts_remote_unique UNIQUE (provider,environment,merchant,remote_id)
);
CREATE INDEX payment_attempts_order_idx ON payment_attempts(order_id,created_at DESC);

CREATE TABLE payment_settlements (
    id UUID PRIMARY KEY,
    order_id UUID NOT NULL REFERENCES payment_orders(id),
    attempt_id UUID NOT NULL REFERENCES payment_attempts(id),
    account_id UUID NOT NULL REFERENCES accounts(id),
    offer_id TEXT NOT NULL CHECK (octet_length(offer_id) BETWEEN 1 AND 128),
    channel TEXT NOT NULL CHECK (channel IN ('stripe_card','asaas_pix_manual','asaas_pix_automatic')),
    provider TEXT NOT NULL CHECK (provider IN ('stripe','asaas')),
    environment TEXT NOT NULL CHECK (environment IN ('test','production')),
    merchant TEXT NOT NULL CHECK (octet_length(merchant) BETWEEN 1 AND 128),
    remote_payment_id TEXT NOT NULL CHECK (octet_length(remote_payment_id) BETWEEN 1 AND 255),
    provider_status TEXT NOT NULL CHECK (octet_length(provider_status) BETWEEN 1 AND 128),
    authority TEXT NOT NULL CHECK (authority = 'provider_verified'),
    amount_centavos BIGINT NOT NULL CHECK (amount_centavos > 0 AND amount_centavos <= 100000000000),
    currency TEXT NOT NULL CHECK (currency = 'BRL'),
    state TEXT NOT NULL CHECK (state IN ('SETTLED','PARTIALLY_REFUNDED','REFUNDED','REVERSED')),
    outcome TEXT NOT NULL CHECK (outcome IN ('GRANTED','EXCESS')),
    settled_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT payment_settlements_remote_unique UNIQUE (provider,environment,merchant,remote_payment_id)
);
CREATE UNIQUE INDEX payment_settlements_one_granted
    ON payment_settlements(order_id) WHERE outcome = 'GRANTED';
CREATE INDEX payment_settlements_order_idx ON payment_settlements(order_id,settled_at DESC);
CREATE INDEX payment_settlements_owner_idx ON payment_settlements(account_id,settled_at DESC);

CREATE TABLE payment_grants (
    id UUID PRIMARY KEY,
    order_id UUID NOT NULL UNIQUE REFERENCES payment_orders(id),
    settlement_id UUID NOT NULL UNIQUE REFERENCES payment_settlements(id),
    remote_payment_id TEXT NOT NULL CHECK (octet_length(remote_payment_id) BETWEEN 1 AND 255),
    account_id UUID NOT NULL REFERENCES accounts(id),
    offer_id TEXT NOT NULL CHECK (octet_length(offer_id) BETWEEN 1 AND 128),
    source TEXT NOT NULL CHECK (source IN ('stripe_card','asaas_pix_manual','asaas_pix_automatic')),
    paid_centavos BIGINT NOT NULL CHECK (paid_centavos > 0 AND paid_centavos <= 100000000000),
    currency TEXT NOT NULL CHECK (currency = 'BRL'),
    start_at TIMESTAMPTZ NOT NULL,
    end_at TIMESTAMPTZ NOT NULL,
    anchor_day SMALLINT NOT NULL CHECK (anchor_day BETWEEN 1 AND 31),
    policy_version INTEGER NOT NULL CHECK (policy_version > 0),
    capabilities TEXT[] NOT NULL CHECK (cardinality(capabilities) > 0),
    state TEXT NOT NULL CHECK (state IN ('ACTIVE','REVOKED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    revoked_at TIMESTAMPTZ,
    CONSTRAINT payment_grants_period CHECK (end_at > start_at)
);
CREATE UNIQUE INDEX payment_grants_active_start
    ON payment_grants(account_id,offer_id,start_at) WHERE state = 'ACTIVE';
CREATE INDEX payment_grants_owner_idx ON payment_grants(account_id,offer_id,end_at DESC);

CREATE TABLE payment_cycles (
    id UUID PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES accounts(id),
    offer_id TEXT NOT NULL CHECK (octet_length(offer_id) BETWEEN 1 AND 128),
    source TEXT NOT NULL CHECK (source IN ('stripe_card','asaas_pix_manual','asaas_pix_automatic')),
    period_start TIMESTAMPTZ NOT NULL,
    period_end TIMESTAMPTZ NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('PENDING','SETTLED','FAILED','CANCELED')),
    settlement_id UUID UNIQUE REFERENCES payment_settlements(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT payment_cycles_period CHECK (period_end > period_start),
    CONSTRAINT payment_cycles_settlement CHECK (
        (state = 'SETTLED' AND settlement_id IS NOT NULL) OR
        (state <> 'SETTLED' AND settlement_id IS NULL)
    ),
    CONSTRAINT payment_cycles_key UNIQUE (account_id,offer_id,period_start)
);

CREATE TABLE payment_inbox (
    provider TEXT NOT NULL CHECK (provider IN ('stripe','asaas')),
    environment TEXT NOT NULL CHECK (environment IN ('test','production')),
    merchant TEXT NOT NULL CHECK (octet_length(merchant) BETWEEN 1 AND 128),
    event_id TEXT NOT NULL CHECK (octet_length(event_id) BETWEEN 1 AND 255),
    payload_hash TEXT NOT NULL CHECK (payload_hash ~ '^[0-9a-f]{64}$'),
    event_type TEXT NOT NULL CHECK (octet_length(event_type) BETWEEN 1 AND 128),
    resource_type TEXT NOT NULL CHECK (octet_length(resource_type) BETWEEN 1 AND 64),
    resource_id TEXT NOT NULL DEFAULT '' CHECK (octet_length(resource_id) <= 255),
    state TEXT NOT NULL DEFAULT 'PENDING' CHECK (state IN ('PENDING','PROCESSED','CONFLICT')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts BETWEEN 0 AND 100),
    received_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    processed_at TIMESTAMPTZ,
    last_error TEXT NOT NULL DEFAULT '' CHECK (octet_length(last_error) <= 500),
    PRIMARY KEY (provider,environment,merchant,event_id)
);
CREATE INDEX payment_inbox_pending_idx ON payment_inbox(received_at,event_id)
    WHERE state = 'PENDING';
