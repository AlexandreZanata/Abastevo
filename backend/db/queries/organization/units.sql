-- name: GetOperatingUnit :one
SELECT id::text AS id,name,code,time_zone,COALESCE(establishment_id::text,'')::text AS establishment_id,
 municipality_code,state,revision FROM org_units WHERE org_id=CAST(@org_id::text AS uuid) AND id=CAST(@id::text AS uuid);

-- name: ListOperatingUnits :many
SELECT id::text AS id,name,code,time_zone,COALESCE(establishment_id::text,'')::text AS establishment_id,
 municipality_code,state,revision FROM org_units WHERE org_id=CAST(@org_id::text AS uuid)
 AND (@state_filter::text='' OR state= @state_filter::text)
 AND (NULLIF(@after_id::text,'') IS NULL OR id>CAST(NULLIF(@after_id::text,'') AS uuid))
ORDER BY id LIMIT @row_limit::int;

-- name: OperatingUnitDefaults :one
SELECT time_zone FROM organizations WHERE org_id=CAST(@org_id::text AS uuid) AND has_snapshot;

-- name: OperatingUnitArchiveFacts :one
SELECT (SELECT count(*) FROM org_units WHERE org_id=CAST(@org_id::text AS uuid) AND state='ACTIVE')::bigint AS active_units,
 CASE WHEN (SELECT count(*) FROM org_units WHERE org_id=CAST(@org_id::text AS uuid) AND state='ACTIVE')>1
 THEN org_unit_current_usage(CAST(@org_id::text AS uuid),CAST(@id::text AS uuid))
 ELSE '{"assets":0,"employees":0,"budgets":0,"operations":0}'::jsonb END::jsonb AS usage;

-- name: InsertOperatingUnit :exec
INSERT INTO org_units(org_id,id,name,code,time_zone,establishment_id,municipality_code,state)
VALUES(CAST(@org_id::text AS uuid),CAST(@id::text AS uuid),@name::text,@code::text,@time_zone::text,
 CAST(NULLIF(@establishment_id::text,'') AS uuid),@municipality_code::text,'ACTIVE');

-- name: ChangeOperatingUnit :execrows
UPDATE org_units SET name= @name::text,code= @code::text,time_zone= @time_zone::text,
 establishment_id=CAST(NULLIF(@establishment_id::text,'') AS uuid),municipality_code= @municipality_code::text,
 state= @state::text,revision=revision+1
WHERE org_id=CAST(@org_id::text AS uuid) AND id=CAST(@id::text AS uuid) AND revision= @expected_revision::bigint;

-- name: OperatingUnitCommandReceipt :one
SELECT actor_id::text AS actor_id,unit_id::text AS unit_id,kind,request_hash,result_state,result_revision
FROM org_unit_commands WHERE org_id=CAST(@org_id::text AS uuid) AND operation_id=CAST(@operation_id::text AS uuid);

-- name: InsertOperatingUnitReceipt :exec
INSERT INTO org_unit_commands(org_id,operation_id,actor_id,unit_id,kind,request_hash,result_state,result_revision)
VALUES(CAST(@org_id::text AS uuid),CAST(@operation_id::text AS uuid),CAST(@actor_id::text AS uuid),CAST(@unit_id::text AS uuid),
 @kind::text,@request_hash::text,@result_state::text,@result_revision::bigint);
