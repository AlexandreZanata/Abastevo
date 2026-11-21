-- name: ReadChargeApproval :one
SELECT a.subscription_id::text AS subscription_id,
       (company.has_snapshot AND company.state <> 'CLOSED'
        AND a.valid_from <= statement_timestamp() AND statement_timestamp() < a.valid_until)::boolean AS can_bind
FROM org_billing_charge_approvals a
JOIN organizations company USING (org_id)
WHERE a.org_id = sqlc.arg(org_id)::text::uuid
  AND a.id = sqlc.arg(approval_id)::text::uuid
  AND a.operator_id = sqlc.arg(operator_id)::text::uuid;

-- name: RecordReviewedChargeBinding :execrows
INSERT INTO org_billing_charge_bindings
    (org_id, provider, environment, merchant, resource_ref, subscription_id, offer_id, customer_ref, contract_ref,
     term_start, term_end, asaas_external_reference, approval_id, binding_operation_id)
SELECT a.org_id, CASE o.channel WHEN 'stripe_card' THEN 'stripe' ELSE 'asaas' END,
       o.environment, o.merchant, a.resource_ref, a.subscription_id, o.id, a.customer_ref, a.contract_ref,
       a.term_start, a.term_end, a.asaas_external_reference, a.id, sqlc.arg(operation_id)::text::uuid
FROM org_billing_charge_approvals a JOIN enterprise_billing_offers o ON o.id = a.offer_id
WHERE a.org_id = sqlc.arg(org_id)::text::uuid AND a.id = sqlc.arg(approval_id)::text::uuid
ON CONFLICT DO NOTHING;

-- name: HasReviewedChargeBinding :one
SELECT EXISTS (
    SELECT 1 FROM org_billing_charge_bindings
    WHERE org_id = sqlc.arg(org_id)::text::uuid AND approval_id = sqlc.arg(approval_id)::text::uuid
      AND binding_operation_id = sqlc.arg(operation_id)::text::uuid
);
