-- EMP-TRIP-008 missions (migration 000227), owned by the trip module.

-- name: MissionRootLock :one
SELECT state FROM organizations WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND has_snapshot FOR UPDATE;

-- name: MissionCommandReceipt :one
SELECT operation_id::text,actor_id::text,mission_id::text,kind,request_hash,result_revision,result_state,COALESCE(warning_code,'')::text AS warning_code,
 COALESCE(warning_availability,'')::text AS warning_availability,
 (SELECT COALESCE(t.assignee_employee_id::text,'') FROM org_trip_mission_transitions t WHERE t.org_id=c.org_id AND t.mission_id=c.mission_id AND t.revision=c.result_revision)::text AS assignee_employee_id
FROM org_trip_mission_commands c WHERE c.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND c.operation_id=CAST(sqlc.arg(operation_id)::text AS uuid);

-- name: MissionUnitState :one
SELECT state FROM org_units WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND id=CAST(sqlc.arg(id)::text AS uuid);

-- name: MissionAssetFacts :one
SELECT state,kind FROM org_assets WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND id=CAST(sqlc.arg(id)::text AS uuid);

-- name: MissionAssigneeFacts :one
SELECT e.state,(e.merged_into IS NOT NULL)::boolean AS merged,COALESCE((SELECT m.id::text FROM org_memberships m WHERE m.org_id=e.org_id AND m.employee_id=e.id AND m.state='ACTIVE' AND m.account_id IS NOT NULL),'')::text AS membership_id
FROM org_employee_identities e WHERE e.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND e.id=CAST(sqlc.arg(id)::text AS uuid);

-- name: InsertMission :exec
INSERT INTO org_trip_missions(org_id,id,unit_id,asset_id,title,starts_at,ends_at,state,revision,created_by,created_by_membership_id,create_operation_id)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(id)::text AS uuid),CAST(sqlc.arg(unit_id)::text AS uuid),CAST(sqlc.narg(asset_id)::text AS uuid),sqlc.arg(title)::text,sqlc.arg(starts_at)::timestamptz,sqlc.arg(ends_at)::timestamptz,'DRAFT',1,CAST(sqlc.arg(created_by)::text AS uuid),CAST(sqlc.arg(created_by_membership_id)::text AS uuid),CAST(sqlc.arg(create_operation_id)::text AS uuid));

-- name: InsertMissionTransition :exec
INSERT INTO org_trip_mission_transitions(org_id,mission_id,revision,kind,from_state,to_state,source,actor_account_id,actor_membership_id,operation_id,enrollment_id,assignee_employee_id,assignee_membership_id,reason)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(mission_id)::text AS uuid),sqlc.arg(revision)::bigint,sqlc.arg(kind)::text,sqlc.narg(from_state)::text,sqlc.arg(to_state)::text,sqlc.arg(source)::text,CAST(sqlc.arg(actor_account_id)::text AS uuid),CAST(sqlc.arg(actor_membership_id)::text AS uuid),
 CAST(sqlc.arg(operation_id)::text AS uuid),CAST(sqlc.narg(enrollment_id)::text AS uuid),CAST(sqlc.narg(assignee_employee_id)::text AS uuid),CAST(sqlc.narg(assignee_membership_id)::text AS uuid),sqlc.narg(reason)::text);

-- name: AdvanceMission :execrows
UPDATE org_trip_missions SET state=sqlc.arg(state)::text,revision=revision+1,assignee_employee_id=CAST(sqlc.narg(assignee_employee_id)::text AS uuid),assignee_membership_id=CAST(sqlc.narg(assignee_membership_id)::text AS uuid),
 answer=sqlc.narg(answer)::text,answer_reason=sqlc.narg(answer_reason)::text
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND id=CAST(sqlc.arg(id)::text AS uuid) AND revision=sqlc.arg(revision)::bigint;

-- name: InsertMissionCommand :exec
INSERT INTO org_trip_mission_commands(org_id,operation_id,actor_id,mission_id,kind,request_hash,result_revision,result_state,warning_code,warning_availability)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(operation_id)::text AS uuid),CAST(sqlc.arg(actor_id)::text AS uuid),CAST(sqlc.arg(mission_id)::text AS uuid),sqlc.arg(kind)::text,sqlc.arg(request_hash)::text,sqlc.arg(result_revision)::bigint,sqlc.arg(result_state)::text,sqlc.narg(warning_code)::text,sqlc.narg(warning_availability)::text);

-- name: MissionForUpdate :one
SELECT id::text,unit_id::text,COALESCE(asset_id::text,'')::text AS asset_id,title,starts_at,ends_at,state,revision,COALESCE(assignee_employee_id::text,'')::text AS assignee_employee_id,
 COALESCE(assignee_membership_id::text,'')::text AS assignee_membership_id,COALESCE(answer,'')::text AS answer,answer_reason,created_at,updated_at
FROM org_trip_missions WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND id=CAST(sqlc.arg(id)::text AS uuid) FOR UPDATE;

-- name: GetMission :one
SELECT id::text,unit_id::text,COALESCE(asset_id::text,'')::text AS asset_id,title,starts_at,ends_at,state,revision,COALESCE(assignee_employee_id::text,'')::text AS assignee_employee_id,
 COALESCE(assignee_membership_id::text,'')::text AS assignee_membership_id,COALESCE(answer,'')::text AS answer,answer_reason,created_at,updated_at
FROM org_trip_missions WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND id=CAST(sqlc.arg(id)::text AS uuid);

-- name: ListMissions :many
SELECT id::text,unit_id::text,COALESCE(asset_id::text,'')::text AS asset_id,title,starts_at,ends_at,state,revision,COALESCE(assignee_employee_id::text,'')::text AS assignee_employee_id,
 COALESCE(assignee_membership_id::text,'')::text AS assignee_membership_id,COALESCE(answer,'')::text AS answer,answer_reason,created_at,updated_at
FROM org_trip_missions WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND(sqlc.arg(after_id)::text='' OR id>CAST(NULLIF(sqlc.arg(after_id)::text,'') AS uuid))
ORDER BY id LIMIT sqlc.arg(page_limit)::integer;

-- name: ListOwnMissions :many
SELECT id::text,unit_id::text,COALESCE(asset_id::text,'')::text AS asset_id,title,starts_at,ends_at,state,revision,COALESCE(assignee_employee_id::text,'')::text AS assignee_employee_id,
 COALESCE(assignee_membership_id::text,'')::text AS assignee_membership_id,COALESCE(answer,'')::text AS answer,answer_reason,created_at,updated_at
FROM org_trip_missions WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND assignee_membership_id=CAST(sqlc.arg(membership_id)::text AS uuid) AND(sqlc.arg(after_id)::text='' OR id>CAST(NULLIF(sqlc.arg(after_id)::text,'') AS uuid))
ORDER BY id LIMIT sqlc.arg(page_limit)::integer;

-- name: MissionResponse :one
SELECT mission_id::text,operation_id::text,actor_membership_id::text,actor_account_id::text,COALESCE(enrollment_id::text,'')::text AS enrollment_id,revision,kind,reason
FROM org_trip_mission_transitions WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND operation_id=CAST(sqlc.arg(operation_id)::text AS uuid) AND source='DEVICE';

-- EMP-TRIP-009 planning revisions (migration 000231).

-- name: MissionCostCentreState :one
SELECT state FROM org_cost_centres WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND id=CAST(sqlc.arg(id)::text AS uuid);

-- name: InsertMissionPlan :exec
INSERT INTO org_trip_mission_plans(org_id,mission_id,revision,operation_id,destination_text,destination_unit_id,reason,cost_centre_id,load_kind,cargo_description,passenger_count,planned_by,planned_by_membership_id)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(mission_id)::text AS uuid),sqlc.arg(revision)::bigint,CAST(sqlc.arg(operation_id)::text AS uuid),sqlc.narg(destination_text)::text,
 CAST(sqlc.narg(destination_unit_id)::text AS uuid),sqlc.arg(reason)::text,CAST(sqlc.narg(cost_centre_id)::text AS uuid),sqlc.arg(load_kind)::text,sqlc.narg(cargo_description)::text,
 sqlc.narg(passenger_count)::integer,CAST(sqlc.arg(planned_by)::text AS uuid),CAST(sqlc.arg(planned_by_membership_id)::text AS uuid));

-- name: MissionPlans :many
SELECT DISTINCT ON(mission_id) mission_id::text,revision,destination_text,COALESCE(destination_unit_id::text,'')::text AS destination_unit_id,reason,COALESCE(cost_centre_id::text,'')::text AS cost_centre_id,
 load_kind,cargo_description,passenger_count,recorded_at
FROM org_trip_mission_plans WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND mission_id=ANY(CAST(sqlc.arg(mission_ids)::text[] AS uuid[]))
ORDER BY mission_id,revision DESC;
