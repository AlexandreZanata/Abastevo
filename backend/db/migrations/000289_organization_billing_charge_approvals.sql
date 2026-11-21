-- EMP-SUBS-004: independent, organization-scoped review for an existing charge.
CREATE TABLE org_billing_charge_approvals (
    org_id UUID NOT NULL REFERENCES organizations(org_id),
    id UUID NOT NULL CHECK (id <> '00000000-0000-0000-0000-000000000000'::uuid),
    operator_id UUID NOT NULL CHECK (operator_id <> '00000000-0000-0000-0000-000000000000'::uuid),
    subscription_id UUID NOT NULL CHECK (subscription_id <> '00000000-0000-0000-0000-000000000000'::uuid),
    offer_id UUID NOT NULL REFERENCES enterprise_billing_offers(id),
    resource_ref TEXT NOT NULL CHECK (octet_length(resource_ref) BETWEEN 1 AND 255 AND resource_ref ~ '^[!-~]+$'),
    customer_ref TEXT NOT NULL CHECK (octet_length(customer_ref) BETWEEN 1 AND 255 AND customer_ref ~ '^[!-~]+$'),
    contract_ref TEXT NOT NULL CHECK (octet_length(contract_ref) BETWEEN 1 AND 255 AND contract_ref ~ '^[!-~]+$'),
    asaas_external_reference TEXT NOT NULL DEFAULT '' CHECK (octet_length(asaas_external_reference) <= 81),
    term_start TIMESTAMPTZ NOT NULL CHECK (isfinite(term_start)),
    term_end TIMESTAMPTZ NOT NULL CHECK (isfinite(term_end) AND term_end > term_start),
    approval_ref TEXT NOT NULL CHECK (approval_ref ~ '^[A-Za-z0-9][A-Za-z0-9._:/-]{0,199}$'),
    valid_from TIMESTAMPTZ NOT NULL CHECK (isfinite(valid_from)),
    valid_until TIMESTAMPTZ NOT NULL CHECK (isfinite(valid_until) AND valid_until > valid_from),
    PRIMARY KEY (org_id, id),
    UNIQUE (org_id, approval_ref)
);
ALTER TABLE org_billing_charge_approvals ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_billing_charge_approvals FORCE ROW LEVEL SECURITY;
CREATE POLICY org_billing_charge_approvals_scope ON org_billing_charge_approvals
    USING (org_id = NULLIF(current_setting('anpfuel.org_id', true), '')::uuid)
    WITH CHECK (org_id = NULLIF(current_setting('anpfuel.org_id', true), '')::uuid);
REVOKE ALL ON org_billing_charge_approvals FROM PUBLIC;
CREATE TRIGGER org_billing_charge_approvals_immutable
    BEFORE UPDATE OR DELETE OR TRUNCATE ON org_billing_charge_approvals
    FOR EACH STATEMENT EXECUTE FUNCTION organization_billing_keep_history();

-- Prior immutable bindings and their settlement receipts remain readable.
ALTER TABLE org_billing_charge_bindings
    ADD COLUMN approval_id UUID,
    ADD COLUMN binding_operation_id UUID,
    ADD CONSTRAINT org_billing_charge_binding_link CHECK ((approval_id IS NULL) = (binding_operation_id IS NULL)),
    ADD FOREIGN KEY (org_id, approval_id) REFERENCES org_billing_charge_approvals(org_id, id),
    ADD FOREIGN KEY (org_id, binding_operation_id) REFERENCES org_audit_events(org_id, operation_id);
CREATE UNIQUE INDEX org_billing_charge_binding_approval ON org_billing_charge_bindings(org_id, approval_id)
    WHERE approval_id IS NOT NULL;
CREATE UNIQUE INDEX org_billing_charge_binding_operation ON org_billing_charge_bindings(org_id, binding_operation_id)
    WHERE binding_operation_id IS NOT NULL;
CREATE POLICY org_billing_charge_bindings_reviewed ON org_billing_charge_bindings
    AS RESTRICTIVE FOR INSERT WITH CHECK (approval_id IS NOT NULL AND binding_operation_id IS NOT NULL);

CREATE FUNCTION org_billing_charge_binding_proof() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
DECLARE binding_time TIMESTAMPTZ := clock_timestamp();
BEGIN
    IF NEW.approval_id IS NULL THEN
        RETURN NEW;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM public.org_billing_charge_approvals a
        JOIN public.enterprise_billing_offers o ON o.id = a.offer_id
        JOIN public.organizations company ON company.org_id = a.org_id
        JOIN public.org_audit_events audit
          ON (audit.org_id, audit.operation_id) = (NEW.org_id, NEW.binding_operation_id)
        WHERE (a.org_id, a.id) = (NEW.org_id, NEW.approval_id)
          AND company.has_snapshot AND company.state <> 'CLOSED'
          AND a.valid_from <= binding_time AND binding_time < a.valid_until
          AND (a.offer_id, a.subscription_id, a.resource_ref, a.customer_ref, a.contract_ref,
               a.asaas_external_reference, a.term_start, a.term_end)
              = (NEW.offer_id, NEW.subscription_id, NEW.resource_ref, NEW.customer_ref, NEW.contract_ref,
                 NEW.asaas_external_reference, NEW.term_start, NEW.term_end)
          AND (o.environment, o.merchant, CASE o.channel WHEN 'stripe_card' THEN 'stripe' ELSE 'asaas' END)
              = (NEW.environment, NEW.merchant, NEW.provider)
          AND CASE o.channel
                WHEN 'stripe_card' THEN a.asaas_external_reference = ''
                WHEN 'asaas_pix_manual' THEN a.asaas_external_reference <> '' AND a.resource_ref = a.contract_ref
                WHEN 'asaas_pix_automatic' THEN a.asaas_external_reference <> ''
              END
          AND (audit.actor_id, audit.target_id) = (a.operator_id, a.subscription_id)
          AND audit.action = 'subs.billing.charge.bound' AND audit.target_type = 'subscription'
          AND audit.delegation_id IS NULL AND audit.reason_code = ''
          AND audit.request_hash = encode(sha256(convert_to(
              'subs.billing.charge.bound/v1/' || a.org_id::text || '/' || a.id::text, 'UTF8')), 'hex')
    ) THEN
        RAISE EXCEPTION USING ERRCODE = '23514', CONSTRAINT = 'org_billing_charge_binding_review',
            MESSAGE = 'billing charge review unavailable';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER org_billing_charge_binding_proof BEFORE INSERT ON org_billing_charge_bindings
    FOR EACH ROW EXECUTE FUNCTION org_billing_charge_binding_proof();

CREATE FUNCTION org_billing_charge_audit_proof() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM public.org_billing_charge_bindings b
        WHERE (b.org_id, b.binding_operation_id) = (NEW.org_id, NEW.operation_id)
    ) THEN
        RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'billing charge binding unavailable';
    END IF;
    RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER org_billing_charge_audit_proof AFTER INSERT ON org_audit_events
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW WHEN (NEW.action = 'subs.billing.charge.bound')
    EXECUTE FUNCTION org_billing_charge_audit_proof();
