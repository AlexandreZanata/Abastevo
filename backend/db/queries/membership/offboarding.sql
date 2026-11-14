-- Current, actual responsibilities only. Unsupported producers are not empty inventories.
-- name: OffboardingItems :many
SELECT key::text AS key,source_revision FROM(
 SELECT 'SETTING:'||d.responsibility||':'||d.slot AS key,d.settings_revision AS source_revision
 FROM org_settings_designations d JOIN org_settings_heads h ON(h.org_id,h.revision)=(d.org_id,d.settings_revision)
 WHERE d.org_id=CAST(@org_id::text AS uuid) AND d.member_id=CAST(@id::text AS uuid)
 UNION ALL
 SELECT 'TEAM_LINK:'||l.id::text,1::bigint FROM org_team_links l JOIN org_memberships m ON(m.org_id,m.employee_id)=(l.org_id,l.employee_id)
 WHERE m.org_id=CAST(@org_id::text AS uuid) AND m.id=CAST(@id::text AS uuid) AND l.ended_at IS NULL
) actual ORDER BY key LIMIT 501;
-- name: TerminateOffboardingEmployee :execrows
UPDATE org_employee_identities SET state='TERMINATED',revision=revision+1
WHERE org_id=CAST(@org_id::text AS uuid) AND id=CAST(@id::text AS uuid) AND revision = @expected_revision::bigint AND state IN('ACTIVE','INACTIVE');
-- name: OffboardingCommand :one
SELECT actor_account_id::text AS actor_id,kind,request_hash,reason,result,occurred_at FROM org_offboarding_commands WHERE org_id=CAST(@org_id::text AS uuid) AND operation_id=CAST(@operation_id::text AS uuid);
-- name: InsertOffboardingCommand :one
INSERT INTO org_offboarding_commands(org_id,operation_id,actor_account_id,membership_id,kind,request_hash,reason,result)
VALUES(CAST(@org_id::text AS uuid),CAST(@operation_id::text AS uuid),CAST(@actor_id::text AS uuid),CAST(@id::text AS uuid),@kind::text,@request_hash::text,@reason::text,@result::jsonb) RETURNING occurred_at;
-- name: InsertOffboardingNotice :exec
INSERT INTO org_offboarding_notices(org_id,operation_id) VALUES(CAST(@org_id::text AS uuid),CAST(@operation_id::text AS uuid));
-- Retained pending manager flags remain visible after source reductions.
-- name: ReadOffboardingHandover :one
SELECT jsonb_build_object('items',result->'items')::jsonb AS result FROM org_offboarding_commands WHERE org_id=CAST(@org_id::text AS uuid) AND membership_id=CAST(@id::text AS uuid);
