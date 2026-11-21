-- EMP-SUBS-004: a provider reversal names a canonical charge, not an
-- organization. Its exact immutable receipt can resolve the private scope.
CREATE POLICY org_billing_receipt_by_canonical_charge
ON org_billing_settlement_receipts FOR SELECT
USING (
    COALESCE(pg_catalog.current_setting('anpfuel.org_id',true),'') = ''
    AND pg_catalog.current_setting('anpfuel.company_reversal_locator',true) =
        pg_catalog.jsonb_build_array(provider,environment,merchant,charge_ref)::text
);
