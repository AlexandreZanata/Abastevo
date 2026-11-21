-- name: GetSubscription :one
SELECT product_code, plan_code, plan_revision, state, prior_state, revision, term_start, term_end, occurred_at
FROM org_billing_subscription_events
WHERE org_id = CAST(sqlc.arg('org_id')::text AS uuid)
  AND subscription_id = CAST(sqlc.arg('subscription_id')::text AS uuid)
ORDER BY revision DESC
LIMIT 1;

-- name: RecordSubscriptionEvent :execrows
INSERT INTO org_billing_subscription_events
(org_id, subscription_id, revision, product_code, plan_code, plan_revision,
 from_state, state, prior_state, event_kind, event_source, evidence_reference,
 occurred_at, term_start, term_end)
VALUES (CAST(sqlc.arg('org_id')::text AS uuid), CAST(sqlc.arg('subscription_id')::text AS uuid), @revision,
        @product_code, @plan_code, @plan_revision, @from_state, @state, @prior_state, @event_kind, @event_source,
        @evidence_reference, @occurred_at, @term_start, @term_end)
ON CONFLICT (org_id, subscription_id, revision) DO NOTHING;
