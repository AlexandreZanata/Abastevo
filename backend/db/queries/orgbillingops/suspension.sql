-- name: ReadSuspensionApproval :one
SELECT subscription_id::text AS subscription_id,expected_revision,valid_from,valid_until
FROM org_billing_suspension_approvals
WHERE org_id=sqlc.arg(org_id)::text::uuid AND id=sqlc.arg(approval_id)::text::uuid
  AND operator_id=sqlc.arg(operator_id)::text::uuid;

-- name: HasReviewedSuspension :one
SELECT EXISTS(
    SELECT 1 FROM org_billing_subscription_events
    WHERE org_id=sqlc.arg(org_id)::text::uuid AND subscription_id=sqlc.arg(subscription_id)::text::uuid
      AND revision=sqlc.arg(revision)::bigint AND event_kind='SUSPEND_ACCESS'
      AND evidence_reference=sqlc.arg(approval_id)::text
)::boolean;
