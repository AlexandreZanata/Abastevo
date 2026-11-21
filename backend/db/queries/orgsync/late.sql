-- EMP-OFFL-009. One immutable late decision per late acknowledgement.
-- name: ReadLateIntake :one
SELECT membership_id::text AS membership_id,enrollment_id::text AS enrollment_id,end_source,ended_at,window_days,COALESCE(settings_revision,0)::bigint AS settings_revision,
 declared_at,declared_submicro,received_at,disposition,result,code,policy
FROM org_sync_late_intakes WHERE org_id=CAST(@org_id::text AS uuid) AND operation_id=CAST(@operation_id::text AS uuid);
-- name: InsertLateIntake :exec
INSERT INTO org_sync_late_intakes(org_id,operation_id,membership_id,enrollment_id,end_source,ended_at,window_days,settings_revision,declared_at,declared_submicro,received_at,disposition,result,code)
VALUES(CAST(@org_id::text AS uuid),CAST(@operation_id::text AS uuid),CAST(@membership_id::text AS uuid),CAST(@enrollment_id::text AS uuid),@end_source::text,@ended_at::timestamptz,@window_days::smallint,
 NULLIF(@settings_revision::bigint,0),@declared_at::timestamptz,@declared_submicro::smallint,@received_at::timestamptz,@disposition::text,@result::text,@code::text);
-- EMP-OFFL-012. One immutable state decision per state acknowledgement.
-- name: ReadStateIntake :one
SELECT condition,disposition,COALESCE(capability,'')::text AS capability,COALESCE(capability_revision,0)::bigint AS capability_revision,plan_changed_at,
 declared_at,declared_submicro,received_at,result,code,policy
FROM org_sync_state_intakes WHERE org_id=CAST(@org_id::text AS uuid) AND operation_id=CAST(@operation_id::text AS uuid);
-- name: InsertStateIntake :exec
INSERT INTO org_sync_state_intakes(org_id,operation_id,condition,disposition,capability,capability_revision,plan_changed_at,declared_at,declared_submicro,received_at,result,code,subscription_id,subscription_revision)
VALUES(CAST(@org_id::text AS uuid),CAST(@operation_id::text AS uuid),@condition::text,@disposition::text,NULLIF(@capability::text,''),NULLIF(@capability_revision::bigint,0),
 sqlc.narg('plan_changed_at')::timestamptz,@declared_at::timestamptz,@declared_submicro::smallint,@received_at::timestamptz,@result::text,@code::text,
 CAST(NULLIF(@subscription_id::text,'') AS uuid),NULLIF(@subscription_revision::bigint,0));

-- name: InactiveSubscriptionIntakeProof :one
SELECT prior.subscription_id::text AS subscription_id, prior.subscription_revision::bigint AS subscription_revision
FROM org_sync_subscription_entitled_at(
  CAST(sqlc.arg('org_id')::text AS uuid), sqlc.arg('capability')::text, sqlc.arg('declared_at')::timestamptz
) AS prior(subscription_id, subscription_revision)
WHERE sqlc.arg('declared_at')::timestamptz < sqlc.arg('received_at')::timestamptz
  AND NOT EXISTS (SELECT 1 FROM org_sync_subscription_entitled_at(
    CAST(sqlc.arg('org_id')::text AS uuid), sqlc.arg('capability')::text, sqlc.arg('received_at')::timestamptz
  ));
