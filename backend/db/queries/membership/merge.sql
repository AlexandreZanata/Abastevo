-- name: MergeEmployeeIdentity :execrows
UPDATE org_employee_identities SET state='TERMINATED',merged_into=CAST(@survivor_id::text AS uuid),revision=revision+1
WHERE org_id=CAST(@org_id::text AS uuid) AND id=CAST(@source_id::text AS uuid) AND revision = @expected_revision::bigint AND merged_into IS NULL;
-- name: EmployeeMergeCommand :one
SELECT actor_account_id::text AS actor_id,source_employee_id::text AS source_id,survivor_employee_id::text AS survivor_id,request_hash,review_digest,reason,source_revision,survivor_revision,survivor_state,team_effects,occurred_at
FROM org_employee_merge_commands WHERE org_id=CAST(@org_id::text AS uuid) AND operation_id=CAST(@operation_id::text AS uuid);
-- name: InsertEmployeeMergeCommand :one
INSERT INTO org_employee_merge_commands(org_id,operation_id,actor_account_id,source_employee_id,survivor_employee_id,request_hash,review_digest,reason,source_revision,survivor_revision,survivor_state,team_effects)
VALUES(CAST(@org_id::text AS uuid),CAST(@operation_id::text AS uuid),CAST(@actor_id::text AS uuid),CAST(@source_id::text AS uuid),CAST(@survivor_id::text AS uuid),@request_hash::text,@review_digest::text,@reason::text,@source_revision::bigint,@survivor_revision::bigint,@survivor_state::text,@team_effects::jsonb)RETURNING occurred_at;
