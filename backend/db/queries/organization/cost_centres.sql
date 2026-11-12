-- name: GetCostCentre :one
SELECT id::text AS id,name,code,state,revision FROM org_cost_centres WHERE org_id=CAST(@org_id::text AS uuid) AND id=CAST(@id::text AS uuid);

-- name: GetCostCentreByHistoricalCode :one
SELECT c.id::text AS id,c.name,c.code,c.state,c.revision FROM org_cost_centre_codes k
JOIN org_cost_centres c ON (c.org_id,c.id)=(k.org_id,k.centre_id)
WHERE k.org_id=CAST(@org_id::text AS uuid) AND k.code_key=lower(@code::text);

-- name: ListCostCentres :many
SELECT id::text AS id,name,code,state,revision FROM org_cost_centres WHERE org_id=CAST(@org_id::text AS uuid)
 AND (@state_filter::text='' OR state= @state_filter::text)
 AND(NULLIF(@after_id::text,'') IS NULL OR id>CAST(NULLIF(@after_id::text,'') AS uuid))
ORDER BY id LIMIT @row_limit::int;

-- name: CostCentreCurrentUsage :one
SELECT org_cost_centre_current_usage(CAST(@org_id::text AS uuid),CAST(@id::text AS uuid))::jsonb AS usage;

-- name: InsertCostCentre :exec
INSERT INTO org_cost_centres(org_id,id,name,code,state) VALUES(CAST(@org_id::text AS uuid),CAST(@id::text AS uuid),@name::text,@code::text,'ACTIVE');

-- name: ChangeCostCentre :execrows
UPDATE org_cost_centres SET name= @name::text,code= @code::text,state= @state::text,revision=revision+1
WHERE org_id=CAST(@org_id::text AS uuid) AND id=CAST(@id::text AS uuid) AND revision= @expected_revision::bigint;

-- name: CostCentreCommandReceipt :one
SELECT actor_id::text AS actor_id,centre_id::text AS centre_id,kind,request_hash,result_state,result_revision
FROM org_cost_centre_commands WHERE org_id=CAST(@org_id::text AS uuid) AND operation_id=CAST(@operation_id::text AS uuid);

-- name: InsertCostCentreReceipt :exec
INSERT INTO org_cost_centre_commands(org_id,operation_id,actor_id,centre_id,kind,request_hash,result_state,result_revision)
VALUES(CAST(@org_id::text AS uuid),CAST(@operation_id::text AS uuid),CAST(@actor_id::text AS uuid),CAST(@centre_id::text AS uuid),@kind::text,@request_hash::text,@result_state::text,@result_revision::bigint);
