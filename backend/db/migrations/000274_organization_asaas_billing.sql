-- EMP-SUBS-004: provider-retrieved Asaas reference and source payment date.
ALTER TABLE org_billing_charge_bindings
    ADD COLUMN asaas_external_reference TEXT NOT NULL DEFAULT '',
    ADD CONSTRAINT org_billing_asaas_reference_scope CHECK (
        asaas_external_reference='' OR (
            provider='asaas' AND length(asaas_external_reference)=81
            AND split_part(asaas_external_reference,':',1)='abv_org'
            AND split_part(asaas_external_reference,':',2)=org_id::text
            AND split_part(asaas_external_reference,':',3) ~ '^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$'
            AND split_part(asaas_external_reference,':',3)<>'00000000-0000-0000-0000-000000000000'
        )
    );
CREATE UNIQUE INDEX org_billing_asaas_checkout_reference
    ON org_billing_charge_bindings(environment,merchant,asaas_external_reference)
    WHERE provider='asaas' AND asaas_external_reference<>'';

ALTER TABLE org_billing_settlement_receipts
    ADD COLUMN provider_paid_on DATE,
    ADD CONSTRAINT org_billing_asaas_source_date CHECK (
        provider_paid_on IS NULL OR (provider='asaas' AND isfinite(provider_paid_on))
    );
