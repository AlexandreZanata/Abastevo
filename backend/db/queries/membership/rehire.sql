-- name: RehireLivePeriods :one
SELECT count(*)::bigint AS live_periods FROM org_memberships WHERE org_id=CAST(@org_id::text AS uuid) AND employee_id=CAST(@employee_id::text AS uuid) AND state<>'ENDED';
-- name: ReopenTerminatedEmployee :execrows
UPDATE org_employee_identities e SET state='ACTIVE',revision=revision+1
WHERE e.org_id=CAST(@org_id::text AS uuid) AND e.id=CAST(@employee_id::text AS uuid) AND e.revision = @expected_revision::bigint AND e.state='TERMINATED' AND e.merged_into IS NULL
 AND NOT EXISTS(SELECT 1 FROM org_memberships m WHERE m.org_id=e.org_id AND m.employee_id=e.id AND m.state<>'ENDED');
-- name: RehireCommand :one
SELECT actor_account_id::text AS actor_id,employee_id::text AS employee_id,request_hash,reason,previous_revision,result_revision,result_state,occurred_at
FROM org_employee_rehire_commands WHERE org_id=CAST(@org_id::text AS uuid) AND operation_id=CAST(@operation_id::text AS uuid);
-- name: InsertRehireCommand :one
INSERT INTO org_employee_rehire_commands(org_id,operation_id,actor_account_id,employee_id,request_hash,reason,previous_revision,result_revision,result_state)
VALUES(CAST(@org_id::text AS uuid),CAST(@operation_id::text AS uuid),CAST(@actor_id::text AS uuid),CAST(@employee_id::text AS uuid),@request_hash::text,@reason::text,@previous_revision::bigint,@result_revision::bigint,'ACTIVE')RETURNING occurred_at;
