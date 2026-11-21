-- EMP-SUBS-004: Approved company offers are written separately from personal
-- checkout offers. Charge bindings and receipts are private organization data.
CREATE TABLE enterprise_billing_offers (
    id UUID PRIMARY KEY CHECK (id <> '00000000-0000-0000-0000-000000000000'::uuid),
    product_code TEXT NOT NULL,
    plan_code TEXT NOT NULL,
    plan_revision BIGINT NOT NULL,
    channel TEXT NOT NULL CHECK (channel IN ('stripe_card','asaas_pix_manual','asaas_pix_automatic')),
    environment TEXT NOT NULL CHECK (environment IN ('test','production')),
    merchant TEXT NOT NULL CHECK (octet_length(merchant) BETWEEN 1 AND 255 AND merchant ~ '^[!-~]+$'),
    provider_offer_ref TEXT NOT NULL CHECK (octet_length(provider_offer_ref) BETWEEN 1 AND 255 AND provider_offer_ref ~ '^[!-~]+$'),
    amount_centavos BIGINT NOT NULL CHECK (amount_centavos BETWEEN 1 AND 100000000000),
    term TEXT NOT NULL CHECK (term = 'P1M'),
    approval_ref TEXT NOT NULL CHECK (octet_length(approval_ref) BETWEEN 1 AND 200 AND approval_ref ~ '^[!-~]+$'),
    FOREIGN KEY (product_code, plan_code, plan_revision)
        REFERENCES enterprise_plan_catalog(product_code, plan_code, revision)
);

CREATE TABLE org_billing_charge_bindings (
    org_id UUID NOT NULL REFERENCES organizations(org_id),
    provider TEXT NOT NULL CHECK (provider IN ('stripe','asaas')),
    environment TEXT NOT NULL CHECK (environment IN ('test','production')),
    merchant TEXT NOT NULL CHECK (octet_length(merchant) BETWEEN 1 AND 255 AND merchant ~ '^[!-~]+$'),
    resource_ref TEXT NOT NULL CHECK (octet_length(resource_ref) BETWEEN 1 AND 255 AND resource_ref ~ '^[!-~]+$'),
    subscription_id UUID NOT NULL CHECK (subscription_id <> '00000000-0000-0000-0000-000000000000'::uuid),
    offer_id UUID NOT NULL REFERENCES enterprise_billing_offers(id),
    customer_ref TEXT NOT NULL CHECK (octet_length(customer_ref) BETWEEN 1 AND 255 AND customer_ref ~ '^[!-~]+$'),
    contract_ref TEXT NOT NULL CHECK (octet_length(contract_ref) BETWEEN 1 AND 255 AND contract_ref ~ '^[!-~]+$'),
    term_start TIMESTAMPTZ NOT NULL CHECK (isfinite(term_start)),
    term_end TIMESTAMPTZ NOT NULL CHECK (isfinite(term_end) AND term_end > term_start),
    PRIMARY KEY (org_id, provider, environment, merchant, resource_ref),
    UNIQUE (provider, environment, merchant, resource_ref),
    UNIQUE (org_id, provider, environment, merchant, resource_ref, subscription_id)
);

CREATE TABLE org_billing_settlement_receipts (
    org_id UUID NOT NULL REFERENCES organizations(org_id),
    provider TEXT NOT NULL,
    environment TEXT NOT NULL,
    merchant TEXT NOT NULL,
    event_id TEXT NOT NULL CHECK (octet_length(event_id) BETWEEN 1 AND 255 AND event_id ~ '^[!-~]+$'),
    payload_hash TEXT NOT NULL CHECK (payload_hash ~ '^[a-f0-9]{64}$'),
    resource_ref TEXT NOT NULL,
    charge_ref TEXT NOT NULL CHECK (octet_length(charge_ref) BETWEEN 1 AND 255 AND charge_ref ~ '^[!-~]+$'),
    subscription_id UUID NOT NULL,
    applied_revision BIGINT NOT NULL,
    settled_at TIMESTAMPTZ NOT NULL CHECK (isfinite(settled_at)),
    PRIMARY KEY (provider, environment, merchant, event_id),
    UNIQUE (provider, environment, merchant, charge_ref),
    FOREIGN KEY (org_id, provider, environment, merchant, resource_ref, subscription_id)
        REFERENCES org_billing_charge_bindings(org_id, provider, environment, merchant, resource_ref, subscription_id),
    FOREIGN KEY (org_id, subscription_id, applied_revision)
        REFERENCES org_billing_subscription_events(org_id, subscription_id, revision)
);

ALTER TABLE org_billing_charge_bindings ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_billing_charge_bindings FORCE ROW LEVEL SECURITY;
CREATE POLICY org_billing_charge_bindings_scope ON org_billing_charge_bindings
    USING (org_id = NULLIF(current_setting('anpfuel.org_id', true), '')::uuid)
    WITH CHECK (org_id = NULLIF(current_setting('anpfuel.org_id', true), '')::uuid);
ALTER TABLE org_billing_settlement_receipts ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_billing_settlement_receipts FORCE ROW LEVEL SECURITY;
CREATE POLICY org_billing_settlement_receipts_scope ON org_billing_settlement_receipts
    USING (org_id = NULLIF(current_setting('anpfuel.org_id', true), '')::uuid)
    WITH CHECK (org_id = NULLIF(current_setting('anpfuel.org_id', true), '')::uuid);
REVOKE ALL ON enterprise_billing_offers, org_billing_charge_bindings, org_billing_settlement_receipts FROM PUBLIC;

CREATE FUNCTION organization_billing_keep_history() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
    RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'organization billing proof is immutable';
END $$;
CREATE TRIGGER enterprise_billing_offers_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON enterprise_billing_offers
    FOR EACH STATEMENT EXECUTE FUNCTION organization_billing_keep_history();
CREATE TRIGGER org_billing_charge_bindings_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON org_billing_charge_bindings
    FOR EACH STATEMENT EXECUTE FUNCTION organization_billing_keep_history();
CREATE TRIGGER org_billing_settlement_receipts_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON org_billing_settlement_receipts
    FOR EACH STATEMENT EXECUTE FUNCTION organization_billing_keep_history();
