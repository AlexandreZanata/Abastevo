-- 000127: durable cross-provider recurring-source switches. The old automatic
-- source must be confirmed fenced before the new source may activate; the target
-- quote and paid-through boundary are frozen for stale-quote checks.

CREATE TABLE payment_source_switches (
    id UUID PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES accounts(id),
    offer_id TEXT NOT NULL CHECK (octet_length(offer_id) BETWEEN 1 AND 128),
    environment TEXT NOT NULL CHECK (environment IN ('test','production')),
    merchant TEXT NOT NULL CHECK (octet_length(merchant) BETWEEN 1 AND 128),
    operation_key TEXT NOT NULL CHECK (octet_length(operation_key) BETWEEN 1 AND 128),
    old_source TEXT NOT NULL CHECK (old_source IN ('stripe_card','asaas_pix_automatic','asaas_pix_manual')),
    old_offer_id TEXT NOT NULL CHECK (octet_length(old_offer_id) BETWEEN 1 AND 128),
    new_source TEXT NOT NULL CHECK (new_source IN ('stripe_card','asaas_pix_automatic')),
    old_local_id TEXT CHECK (old_local_id IS NULL OR octet_length(old_local_id) BETWEEN 1 AND 128),
    old_remote_id TEXT CHECK (old_remote_id IS NULL OR octet_length(old_remote_id) BETWEEN 1 AND 255),
    target_catalog_version TEXT NOT NULL CHECK (octet_length(target_catalog_version) BETWEEN 1 AND 128),
    target_amount_centavos BIGINT NOT NULL CHECK (target_amount_centavos > 0 AND target_amount_centavos <= 100000000000),
    target_capabilities TEXT[] NOT NULL CHECK (cardinality(target_capabilities) > 0),
    current_end TIMESTAMPTZ,
    first_debit_at TIMESTAMPTZ,
    state TEXT NOT NULL CHECK (state IN ('PENDING','CANCELING','READY','CONSUMED','CANCELED')),
    old_cancellation TEXT NOT NULL CHECK (old_cancellation IN ('CONFIRMED','PENDING','UNKNOWN','NOT_APPLICABLE')),
    worker_id TEXT,
    lease_until TIMESTAMPTZ,
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts BETWEEN 0 AND 20),
    last_error TEXT NOT NULL DEFAULT '' CHECK (octet_length(last_error) <= 500),
    requested_at TIMESTAMPTZ NOT NULL,
    last_event_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT payment_source_switches_operation_unique
        UNIQUE (account_id,environment,merchant,operation_key),
    CONSTRAINT payment_source_switches_canceling_lease CHECK (
        state <> 'CANCELING' OR lease_until IS NOT NULL
    ),
    CONSTRAINT payment_source_switches_ready CHECK (
        state <> 'READY' OR (old_cancellation = 'CONFIRMED' AND first_debit_at IS NOT NULL)
        OR (old_cancellation = 'NOT_APPLICABLE' AND first_debit_at IS NOT NULL)
    )
);
CREATE UNIQUE INDEX payment_source_switches_one_open
    ON payment_source_switches(account_id,offer_id)
    WHERE state IN ('PENDING','CANCELING','READY');
CREATE INDEX payment_source_switches_owner_idx
    ON payment_source_switches(account_id,offer_id,updated_at DESC);
