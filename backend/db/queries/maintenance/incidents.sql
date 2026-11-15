-- name: IncidentReportOperation :one
-- The operation already holding a report identity (EMP-CHECK-003).
SELECT operation_id::text AS operation_id FROM org_incident_reports
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND id=CAST(sqlc.arg(id)::text AS uuid);

-- name: IncidentReporterEmployee :one
SELECT employee_id::text AS employee_id FROM org_memberships
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND id=CAST(sqlc.arg(membership_id)::text AS uuid);

-- name: IncidentAssetState :one
SELECT state FROM org_assets WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND id=CAST(sqlc.arg(id)::text AS uuid);

-- name: InsertIncidentReport :exec
INSERT INTO org_incident_reports(org_id,id,operation_id,payload_digest,report_type,incident_kind,asset_id,unit_id,cost_centre_id,trip_id,description,location,blocks_use,
 reporter_account_id,reporter_membership_id,reporter_employee_id,enrollment_id,occurred_at,occurred_submicro,entry_zone,device_offset_seconds,state,revision)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(id)::text AS uuid),CAST(sqlc.arg(operation_id)::text AS uuid),sqlc.arg(payload_digest)::text,
 sqlc.arg(report_type)::text,sqlc.narg(incident_kind)::text,CAST(sqlc.arg(asset_id)::text AS uuid),CAST(sqlc.narg(unit_id)::text AS uuid),
 CAST(sqlc.narg(cost_centre_id)::text AS uuid),CAST(sqlc.narg(trip_id)::text AS uuid),sqlc.arg(description)::text,sqlc.narg(location)::text,sqlc.arg(blocks_use)::boolean,
 CAST(sqlc.arg(reporter_account_id)::text AS uuid),CAST(sqlc.arg(reporter_membership_id)::text AS uuid),CAST(sqlc.arg(reporter_employee_id)::text AS uuid),
 CAST(sqlc.arg(enrollment_id)::text AS uuid),sqlc.arg(occurred_at)::timestamptz,sqlc.arg(occurred_submicro)::smallint,sqlc.arg(entry_zone)::text,
 sqlc.arg(device_offset_seconds)::bigint,'REPORTED',1);

-- name: GetIncidentReport :one
SELECT id::text AS id,operation_id::text AS operation_id,payload_digest,report_type,COALESCE(incident_kind,'')::text AS incident_kind,asset_id::text AS asset_id,
 COALESCE(unit_id::text,'')::text AS unit_id,COALESCE(cost_centre_id::text,'')::text AS cost_centre_id,COALESCE(trip_id::text,'')::text AS trip_id,description,location,blocks_use,
 reporter_account_id::text AS reporter_account_id,reporter_membership_id::text AS reporter_membership_id,reporter_employee_id::text AS reporter_employee_id,
 enrollment_id::text AS enrollment_id,occurred_at,occurred_submicro,entry_zone,device_offset_seconds,received_at,state,revision,updated_at
FROM org_incident_reports WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND id=CAST(sqlc.arg(id)::text AS uuid);

-- name: IncidentReportForUpdate :one
-- Handling serializes on the report row (B-BR-EMP-373).
SELECT id::text AS id,asset_id::text AS asset_id,state,revision,blocks_use,reporter_account_id::text AS reporter_account_id,
 reporter_membership_id::text AS reporter_membership_id,reporter_employee_id::text AS reporter_employee_id
FROM org_incident_reports WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND id=CAST(sqlc.arg(id)::text AS uuid) FOR UPDATE;

-- name: IncidentReportAsset :one
SELECT asset_id::text AS asset_id FROM org_incident_reports WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND id=CAST(sqlc.arg(id)::text AS uuid);

-- name: IncidentReportReporter :one
SELECT reporter_membership_id::text AS reporter_membership_id FROM org_incident_reports
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND id=CAST(sqlc.arg(id)::text AS uuid);

-- name: IncidentCanonicalEmployee :one
-- The canonical employee record of a merge chain (EDR-129); '' when unresolvable.
SELECT COALESCE(org_sync_review_employee_canonical(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(employee_id)::text AS uuid))::text,'')::text AS employee_id;

-- name: IncidentHandler :one
-- The current handler's employee record for the step's history row.
SELECT m.employee_id::text AS employee_id FROM org_memberships m
WHERE m.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND m.id=CAST(sqlc.arg(membership_id)::text AS uuid)
 AND m.account_id=CAST(sqlc.arg(account_id)::text AS uuid) AND m.state='ACTIVE';

-- name: IncidentTransitionByOperation :one
SELECT report_id::text AS report_id,revision,action,from_state,to_state,note,COALESCE(duplicate_of::text,'')::text AS duplicate_of,
 actor_account_id::text AS actor_account_id,request_hash,recorded_at
FROM org_incident_report_transitions WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND operation_id=CAST(sqlc.arg(operation_id)::text AS uuid);

-- name: InsertIncidentTransition :exec
INSERT INTO org_incident_report_transitions(org_id,report_id,revision,action,from_state,to_state,note,duplicate_of,actor_account_id,actor_membership_id,actor_employee_id,operation_id,request_hash)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(report_id)::text AS uuid),sqlc.arg(revision)::bigint,sqlc.arg(action)::text,sqlc.arg(from_state)::text,
 sqlc.arg(to_state)::text,sqlc.narg(note)::text,CAST(sqlc.narg(duplicate_of)::text AS uuid),CAST(sqlc.arg(actor_account_id)::text AS uuid),
 CAST(sqlc.arg(actor_membership_id)::text AS uuid),CAST(sqlc.arg(actor_employee_id)::text AS uuid),CAST(sqlc.arg(operation_id)::text AS uuid),sqlc.arg(request_hash)::text);

-- name: IncidentReportTransitions :many
SELECT revision,action,from_state,to_state,note,COALESCE(duplicate_of::text,'')::text AS duplicate_of,actor_membership_id::text AS actor_membership_id,recorded_at
FROM org_incident_report_transitions WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND report_id=CAST(sqlc.arg(report_id)::text AS uuid)
ORDER BY revision;

-- name: IncidentReportAttachments :many
-- EDR-239: the linked documents with their own storage state; never content.
SELECT l.document_id::text AS document_id,d.storage_state,l.created_at
FROM org_document_links l JOIN org_documents d ON(d.org_id,d.id)=(l.org_id,l.document_id)
WHERE l.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND l.target_kind='INCIDENT' AND l.incident_report_id=CAST(sqlc.arg(report_id)::text AS uuid)
ORDER BY l.document_id;

-- name: ListIncidentReports :many
SELECT id::text AS id FROM org_incident_reports
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND(sqlc.arg(after_id)::text='' OR id>CAST(NULLIF(sqlc.arg(after_id)::text,'') AS uuid))
ORDER BY id LIMIT sqlc.arg(row_limit)::int;

-- name: ListOwnIncidentReports :many
SELECT id::text AS id FROM org_incident_reports
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND reporter_membership_id=CAST(sqlc.arg(membership_id)::text AS uuid)
 AND(sqlc.arg(after_id)::text='' OR id>CAST(NULLIF(sqlc.arg(after_id)::text,'') AS uuid))
ORDER BY id LIMIT sqlc.arg(row_limit)::int;

-- name: IncidentTripReports :many
-- EDR-240: the accepted reports of one trip, as timeline facts.
SELECT id::text AS id,report_type,reporter_membership_id::text AS reporter_membership_id,occurred_at,occurred_submicro,entry_zone,device_offset_seconds,received_at
FROM org_incident_reports WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND trip_id=CAST(sqlc.arg(trip_id)::text AS uuid)
ORDER BY occurred_at,occurred_submicro,id;
