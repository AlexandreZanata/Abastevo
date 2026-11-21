-- name: SetCanonicalChargeLocator :exec
SELECT pg_catalog.set_config('anpfuel.company_reversal_locator',
    pg_catalog.jsonb_build_array(sqlc.arg(provider)::text,sqlc.arg(environment)::text,
        sqlc.arg(merchant)::text,sqlc.arg(charge_ref)::text)::text, true);

-- name: LocateCompanySettlement :one
SELECT org_id::text FROM org_billing_settlement_receipts
WHERE provider=sqlc.arg(provider)::text AND environment=sqlc.arg(environment)::text
  AND merchant=sqlc.arg(merchant)::text AND charge_ref=sqlc.arg(charge_ref)::text;

-- name: ReadCompanyReversibleSettlement :one
SELECT r.subscription_id::text AS subscription_id,
       b.term_start,b.term_end,b.asaas_external_reference,o.amount_centavos
FROM org_billing_settlement_receipts r
JOIN org_billing_charge_bindings b
  ON (b.org_id,b.provider,b.environment,b.merchant,b.resource_ref,b.subscription_id) =
     (r.org_id,r.provider,r.environment,r.merchant,r.resource_ref,r.subscription_id)
JOIN enterprise_billing_offers o ON o.id=b.offer_id
WHERE r.org_id=sqlc.arg(org_id)::text::uuid AND r.provider=sqlc.arg(provider)::text
  AND r.environment=sqlc.arg(environment)::text AND r.merchant=sqlc.arg(merchant)::text AND r.charge_ref=sqlc.arg(charge_ref)::text;

-- name: CompanyReversalAlreadyObserved :one
SELECT EXISTS (
    SELECT 1 FROM org_billing_subscription_events
    WHERE org_id=sqlc.arg(org_id)::text::uuid AND subscription_id=sqlc.arg(subscription_id)::text::uuid
      AND event_kind='REPORT_PAST_DUE' AND evidence_reference=sqlc.arg(evidence_reference)::text
)::boolean;
