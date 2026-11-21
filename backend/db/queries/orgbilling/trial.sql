-- name: GetTrialGrant :one
SELECT product_code,plan_code,plan_revision,eligible_from,eligible_until,duration_seconds,approval_ref
FROM org_billing_trial_grants
WHERE org_id=CAST(sqlc.arg('org_id')::text AS uuid) AND id=CAST(sqlc.arg('grant_id')::text AS uuid);

-- name: LockTrialOrganization :one
SELECT state::text FROM organizations
WHERE org_id=CAST(sqlc.arg('org_id')::text AS uuid) AND has_snapshot
FOR SHARE;

-- name: TrialClock :one
SELECT pg_catalog.clock_timestamp()::timestamptz AS observed_at;

-- name: NonterminalSubscriptionForProduct :one
SELECT EXISTS (
    SELECT 1 FROM (
        SELECT DISTINCT ON (subscription_id) state
        FROM org_billing_subscription_events
        WHERE org_id=CAST(sqlc.arg('org_id')::text AS uuid) AND product_code=sqlc.arg('product_code')::text
        ORDER BY subscription_id,revision DESC
    ) current_subscriptions WHERE state <> 'CANCELLED'
)::boolean AS present;

-- name: GetTrialStart :one
SELECT term_start,term_end,evidence_reference FROM org_billing_subscription_events
WHERE org_id=CAST(sqlc.arg('org_id')::text AS uuid) AND subscription_id=CAST(sqlc.arg('grant_id')::text AS uuid)
  AND revision=1 AND event_kind='START_TRIAL' AND event_source='TRIAL_POLICY';
