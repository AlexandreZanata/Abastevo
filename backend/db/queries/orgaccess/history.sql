-- name: RecordedAssignmentPeriods :many
-- Complete retained periods are evidence for source-owned intake disposition,
-- never current authority. Do not replace this with current-only candidates.
SELECT id::text AS id,membership_id::text AS membership_id,role,scope_type,scope_target::text AS scope_target,
 valid_from,valid_to,assigned_by::text AS assigned_by,reason,revision,ended_at,created_at
FROM org_role_assignments
WHERE org_id=CAST(@org_id::text AS uuid) AND membership_id=CAST(@membership_id::text AS uuid)
ORDER BY id;
