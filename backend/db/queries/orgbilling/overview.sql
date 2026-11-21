-- name: IsPlanTermSettled :one
SELECT EXISTS (
    SELECT 1 FROM org_billing_settlement_receipts r
    JOIN org_billing_subscription_events e
      ON (e.org_id,e.subscription_id,e.revision) = (r.org_id,r.subscription_id,r.applied_revision)
    WHERE e.org_id=CAST(sqlc.arg('org_id')::text AS uuid) AND e.subscription_id=CAST(sqlc.arg('subscription_id')::text AS uuid)
      AND e.product_code=sqlc.arg('product_code')::text AND e.plan_code=sqlc.arg('plan_code')::text
      AND e.plan_revision=sqlc.arg('plan_revision')::bigint
      AND e.term_start=sqlc.arg('term_start')::timestamptz AND e.term_end=sqlc.arg('term_end')::timestamptz
)::boolean AS settled;

-- name: ListReviewedNextPlans :many
SELECT DISTINCT o.product_code,o.plan_code,o.plan_revision,b.term_start
FROM org_billing_charge_bindings b
JOIN enterprise_billing_offers o ON o.id=b.offer_id
WHERE b.org_id=CAST(sqlc.arg('org_id')::text AS uuid) AND b.subscription_id=CAST(sqlc.arg('subscription_id')::text AS uuid)
  AND o.product_code=sqlc.arg('product_code')::text
  AND (o.plan_code,o.plan_revision)<>(sqlc.arg('plan_code')::text,sqlc.arg('plan_revision')::bigint)
  AND b.term_start=sqlc.arg('next_start')::timestamptz AND b.term_end>sqlc.arg('observed_at')::timestamptz
ORDER BY b.term_start,o.plan_code,o.plan_revision
LIMIT 2;
