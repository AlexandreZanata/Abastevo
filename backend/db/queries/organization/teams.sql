-- name: GetTeam :one
SELECT id::text AS id,name,COALESCE(unit_id::text,'')::text AS unit_id,state,revision
FROM org_teams WHERE org_id=CAST(@org_id::text AS uuid) AND id=CAST(@id::text AS uuid);

-- name: ListTeams :many
SELECT id::text AS id,name,COALESCE(unit_id::text,'')::text AS unit_id,state,revision
FROM org_teams WHERE org_id=CAST(@org_id::text AS uuid)
 AND(@state_filter::text='' OR state= @state_filter::text)
 AND(@after_id::text='' OR id>NULLIF(@after_id::text,'')::uuid)
ORDER BY id LIMIT @row_limit::int;

-- name: TeamCurrentUsage :one
SELECT org_team_current_usage(CAST(@org_id::text AS uuid),CAST(@id::text AS uuid))::jsonb AS usage;

-- name: InsertTeam :exec
INSERT INTO org_teams(org_id,id,name,unit_id,state) VALUES(CAST(@org_id::text AS uuid),CAST(@id::text AS uuid),@name::text,NULLIF(@unit_id::text,'')::uuid,'ACTIVE');

-- name: ChangeTeam :execrows
UPDATE org_teams SET name= @name::text,unit_id=NULLIF(@unit_id::text,'')::uuid,state= @state::text,revision=revision+1
WHERE org_id=CAST(@org_id::text AS uuid) AND id=CAST(@id::text AS uuid) AND revision= @expected_revision::bigint;

-- name: TeamCommandReceipt :one
SELECT actor_id::text AS actor_id,team_id::text AS team_id,COALESCE(link_id::text,'')::text AS link_id,kind,request_hash,result_state,result_revision
FROM org_team_commands WHERE org_id=CAST(@org_id::text AS uuid) AND operation_id=CAST(@operation_id::text AS uuid);

-- name: InsertTeamReceipt :exec
INSERT INTO org_team_commands(org_id,operation_id,actor_id,team_id,link_id,kind,request_hash,result_state,result_revision)
VALUES(CAST(@org_id::text AS uuid),CAST(@operation_id::text AS uuid),CAST(@actor_id::text AS uuid),CAST(@team_id::text AS uuid),NULLIF(@link_id::text,'')::uuid,@kind::text,@request_hash::text,@result_state::text,@result_revision::bigint);

-- name: InsertTeamLink :exec
INSERT INTO org_team_links(org_id,id,team_id,employee_id,created_operation_id)
VALUES(CAST(@org_id::text AS uuid),CAST(@id::text AS uuid),CAST(@team_id::text AS uuid),CAST(@employee_id::text AS uuid),CAST(@operation_id::text AS uuid));

-- name: CurrentTeamLink :one
SELECT id::text AS id,team_id::text AS team_id,employee_id::text AS employee_id,started_at,ended_at,end_reason
FROM org_team_links WHERE org_id=CAST(@org_id::text AS uuid) AND team_id=CAST(@team_id::text AS uuid) AND employee_id=CAST(@employee_id::text AS uuid) AND ended_at IS NULL;

-- name: EndTeamLink :execrows
UPDATE org_team_links SET ended_at=clock_timestamp(),end_reason='REMOVED',closed_operation_id=CAST(@operation_id::text AS uuid)
WHERE org_id=CAST(@org_id::text AS uuid) AND id=CAST(@id::text AS uuid) AND ended_at IS NULL;

-- name: ListTeamLinks :many
SELECT id::text AS id,team_id::text AS team_id,employee_id::text AS employee_id,started_at,ended_at,end_reason
FROM org_team_links WHERE org_id=CAST(@org_id::text AS uuid) AND team_id=CAST(@team_id::text AS uuid)
 AND(@state_filter::text='' OR(@state_filter::text='ACTIVE' AND ended_at IS NULL) OR(@state_filter::text='ENDED' AND ended_at IS NOT NULL))
 AND(@after_id::text='' OR id>NULLIF(@after_id::text,'')::uuid)
ORDER BY id LIMIT @row_limit::int;
