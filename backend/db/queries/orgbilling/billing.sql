-- name: GetChargeBinding :one
SELECT b.subscription_id::text AS subscription_id, b.customer_ref, b.contract_ref, b.asaas_external_reference,
       b.term_start, b.term_end, o.product_code, o.plan_code, o.plan_revision,
       o.channel, o.environment, o.merchant, o.provider_offer_ref,
       o.amount_centavos, o.term, o.approval_ref
FROM org_billing_charge_bindings b
JOIN enterprise_billing_offers o ON o.id = b.offer_id
WHERE b.org_id = CAST(sqlc.arg('org_id')::text AS uuid)
  AND b.provider = @provider AND b.environment = @environment
  AND b.merchant = @merchant AND b.resource_ref = @resource_ref;

-- name: GetSettlementReceipt :one
SELECT event_id, payload_hash, resource_ref, charge_ref, subscription_id::text AS subscription_id,
       applied_revision, settled_at, provider_paid_on
FROM org_billing_settlement_receipts
WHERE org_id = CAST(sqlc.arg('org_id')::text AS uuid)
  AND provider = @provider AND environment = @environment AND merchant = @merchant
  AND (event_id = @event_id OR charge_ref = @charge_ref)
ORDER BY (event_id = @event_id) DESC
LIMIT 1;

-- name: RecordSettlementReceipt :execrows
INSERT INTO org_billing_settlement_receipts
(org_id, provider, environment, merchant, event_id, payload_hash, resource_ref,
 charge_ref, subscription_id, applied_revision, settled_at, provider_paid_on)
VALUES (CAST(sqlc.arg('org_id')::text AS uuid), @provider, @environment, @merchant,
        @event_id, @payload_hash, @resource_ref, @charge_ref, CAST(sqlc.arg('subscription_id')::text AS uuid),
        @applied_revision, @settled_at, sqlc.narg(provider_paid_on)::date)
ON CONFLICT DO NOTHING;

-- name: LockBillingCharge :exec
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg('charge_identity')::text, 0));
