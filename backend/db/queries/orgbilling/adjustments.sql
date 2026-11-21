-- name: ReadAdjustmentApproval :one
SELECT subscription_id::text AS subscription_id,subscription_revision,kind,amount_centavos,currency,valid_from,valid_until
FROM org_billing_adjustment_approvals
WHERE (org_id,id,operator_id)=(sqlc.arg('org_id')::text::uuid,sqlc.arg('approval_id')::text::uuid,sqlc.arg('operator_id')::text::uuid);

-- name: RecordAdjustmentPosting :execrows
INSERT INTO org_billing_adjustment_postings(org_id,operation_id,approval_id,delta_centavos)
VALUES(sqlc.arg('org_id')::text::uuid,sqlc.arg('operation_id')::text::uuid,sqlc.arg('approval_id')::text::uuid,sqlc.arg('delta_centavos')::bigint)
ON CONFLICT DO NOTHING;

-- name: ReadAdjustmentPosting :one
SELECT delta_centavos,posted_at FROM org_billing_adjustment_postings
WHERE (org_id,operation_id,approval_id)=(sqlc.arg('org_id')::text::uuid,sqlc.arg('operation_id')::text::uuid,sqlc.arg('approval_id')::text::uuid);
