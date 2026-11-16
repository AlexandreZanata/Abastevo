-- name: ChecklistRunOperation :one
-- The operation already holding a run identity (EMP-CHECK-002).
SELECT operation_id::text AS operation_id FROM org_checklist_runs
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND id=CAST(sqlc.arg(id)::text AS uuid);

-- name: ChecklistVersionStamps :many
SELECT version,published_at FROM org_checklist_template_versions
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND template_id=CAST(sqlc.arg(template_id)::text AS uuid) ORDER BY version;

-- name: ChecklistPolicyHistory :many
SELECT revision,mode,validity_seconds,effective_at FROM org_checklist_policy_revisions
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND template_id=CAST(sqlc.arg(template_id)::text AS uuid) ORDER BY revision;

-- name: InsertChecklistRun :exec
INSERT INTO org_checklist_runs(org_id,id,operation_id,payload_digest,asset_id,unit_id,cost_centre_id,context,template_id,template_version,policy_revision,
 server_policy_revision,started_at,occurred_at,occurred_submicro,entry_zone,device_offset_seconds,device_outcome,recheck_outcome,outcome,outcome_differs,
 template_superseded,acknowledged,author_account_id,author_membership_id,author_employee_id,enrollment_id,revision)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(id)::text AS uuid),CAST(sqlc.arg(operation_id)::text AS uuid),sqlc.arg(payload_digest)::text,
 CAST(sqlc.arg(asset_id)::text AS uuid),CAST(sqlc.narg(unit_id)::text AS uuid),CAST(sqlc.narg(cost_centre_id)::text AS uuid),sqlc.arg(context)::text,
 CAST(sqlc.arg(template_id)::text AS uuid),sqlc.arg(template_version)::int,sqlc.arg(policy_revision)::int,sqlc.arg(server_policy_revision)::int,
 sqlc.arg(started_at)::timestamptz,sqlc.arg(occurred_at)::timestamptz,sqlc.arg(occurred_submicro)::smallint,sqlc.arg(entry_zone)::text,
 sqlc.arg(device_offset_seconds)::bigint,sqlc.arg(device_outcome)::text,sqlc.arg(recheck_outcome)::text,sqlc.arg(recheck_outcome)::text,
 sqlc.arg(outcome_differs)::boolean,sqlc.arg(template_superseded)::boolean,sqlc.arg(acknowledged)::boolean,CAST(sqlc.arg(author_account_id)::text AS uuid),
 CAST(sqlc.arg(author_membership_id)::text AS uuid),CAST(sqlc.arg(author_employee_id)::text AS uuid),CAST(sqlc.arg(enrollment_id)::text AS uuid),1);

-- name: InsertChecklistRunAnswer :exec
INSERT INTO org_checklist_run_answers(org_id,run_id,template_id,template_version,position,answer,note)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(run_id)::text AS uuid),CAST(sqlc.arg(template_id)::text AS uuid),sqlc.arg(template_version)::int,
 sqlc.arg(position)::int,sqlc.arg(answer)::text,sqlc.narg(note)::text);

-- name: GetChecklistRun :one
SELECT r.id::text AS id,r.operation_id::text AS operation_id,r.payload_digest,r.asset_id::text AS asset_id,COALESCE(r.unit_id::text,'')::text AS unit_id,
 COALESCE(r.cost_centre_id::text,'')::text AS cost_centre_id,r.context,r.template_id::text AS template_id,r.template_version,r.policy_revision,r.server_policy_revision,
 r.started_at,r.occurred_at,r.occurred_submicro,r.entry_zone,r.device_offset_seconds,r.device_outcome,r.recheck_outcome,r.outcome,r.outcome_differs,
 r.template_superseded,r.acknowledged,r.author_account_id::text AS author_account_id,r.author_membership_id::text AS author_membership_id,
 r.author_employee_id::text AS author_employee_id,r.enrollment_id::text AS enrollment_id,r.received_at,r.revision,
 COALESCE(l.actor_membership_id::text,'')::text AS released_by,l.recorded_at AS released_at
FROM org_checklist_runs r LEFT JOIN org_checklist_run_releases l ON(l.org_id,l.run_id)=(r.org_id,r.id)
WHERE r.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND r.id=CAST(sqlc.arg(id)::text AS uuid);

-- name: ChecklistRunAnswers :many
SELECT a.position,a.answer,a.note,a.photo,i.critical FROM org_checklist_run_answers a
JOIN org_checklist_template_items i ON(i.org_id,i.template_id,i.version,i.position)=(a.org_id,a.template_id,a.template_version,a.position)
WHERE a.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND a.run_id=CAST(sqlc.arg(run_id)::text AS uuid) ORDER BY a.position;

-- name: ChecklistRunForUpdate :one
-- A release serializes on the run row (B-BR-EMP-442).
SELECT id::text AS id,asset_id::text AS asset_id,outcome,revision,author_account_id::text AS author_account_id,
 author_membership_id::text AS author_membership_id,author_employee_id::text AS author_employee_id
FROM org_checklist_runs WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND id=CAST(sqlc.arg(id)::text AS uuid) FOR UPDATE;

-- name: ChecklistRunReleaseByOperation :one
SELECT run_id::text AS run_id,actor_account_id::text AS actor_account_id,request_hash,recorded_at
FROM org_checklist_run_releases WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND operation_id=CAST(sqlc.arg(operation_id)::text AS uuid);

-- name: InsertChecklistRunRelease :exec
INSERT INTO org_checklist_run_releases(org_id,run_id,revision,reason,actor_account_id,actor_membership_id,actor_employee_id,operation_id,request_hash)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(run_id)::text AS uuid),2,sqlc.arg(reason)::text,CAST(sqlc.arg(actor_account_id)::text AS uuid),
 CAST(sqlc.arg(actor_membership_id)::text AS uuid),CAST(sqlc.arg(actor_employee_id)::text AS uuid),CAST(sqlc.arg(operation_id)::text AS uuid),sqlc.arg(request_hash)::text);

-- name: ChecklistBlockingRuns :many
-- EDR-285: the BLOCKING runs of an asset whose period (declared completion
-- until the release) contains the instant.
SELECT r.id::text AS id,r.occurred_at,l.recorded_at AS released_at FROM org_checklist_runs r
LEFT JOIN org_checklist_run_releases l ON(l.org_id,l.run_id)=(r.org_id,r.id)
WHERE r.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND r.asset_id=CAST(sqlc.arg(asset_id)::text AS uuid) AND r.recheck_outcome='BLOCKING'
 AND r.occurred_at<=sqlc.arg(at)::timestamptz AND(l.recorded_at IS NULL OR sqlc.arg(at)::timestamptz<l.recorded_at)
ORDER BY r.occurred_at,r.id;

-- name: ChecklistRunGateFacts :one
-- EMP-TRIP-019 (EDR-334): a run as a trip boundary reads it, with the
-- recording of its release; no row lock (the caller holds the root lock).
SELECT r.id::text AS id,r.asset_id::text AS asset_id,r.context,r.template_id::text AS template_id,r.author_membership_id::text AS author_membership_id,
 r.occurred_at,r.occurred_submicro,r.recheck_outcome,l.recorded_at AS released_at
FROM org_checklist_runs r LEFT JOIN org_checklist_run_releases l ON(l.org_id,l.run_id)=(r.org_id,r.id)
WHERE r.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND r.id=CAST(sqlc.arg(id)::text AS uuid);
