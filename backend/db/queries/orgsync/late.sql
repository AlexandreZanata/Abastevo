-- EMP-OFFL-009. One immutable late decision per late acknowledgement.
-- name: ReadLateIntake :one
SELECT membership_id::text AS membership_id,enrollment_id::text AS enrollment_id,end_source,ended_at,window_days,COALESCE(settings_revision,0)::bigint AS settings_revision,
 declared_at,declared_submicro,received_at,disposition,result,code,policy
FROM org_sync_late_intakes WHERE org_id=CAST(@org_id::text AS uuid) AND operation_id=CAST(@operation_id::text AS uuid);
-- name: InsertLateIntake :exec
INSERT INTO org_sync_late_intakes(org_id,operation_id,membership_id,enrollment_id,end_source,ended_at,window_days,settings_revision,declared_at,declared_submicro,received_at,disposition,result,code)
VALUES(CAST(@org_id::text AS uuid),CAST(@operation_id::text AS uuid),CAST(@membership_id::text AS uuid),CAST(@enrollment_id::text AS uuid),@end_source::text,@ended_at::timestamptz,@window_days::smallint,
 NULLIF(@settings_revision::bigint,0),@declared_at::timestamptz,@declared_submicro::smallint,@received_at::timestamptz,@disposition::text,@result::text,@code::text);
