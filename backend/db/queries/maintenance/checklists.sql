-- name: ChecklistRootLock :one
-- Template commands hold the organization root until commit (EDR-214).
SELECT state FROM organizations WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND has_snapshot FOR UPDATE;

-- name: ChecklistClock :one
SELECT clock_timestamp()::timestamptz AS at;

-- name: ChecklistCommandReceipt :one
SELECT actor_id::text AS actor_id,template_id::text AS template_id,kind,request_hash,result_revision,result_version,result_policy_revision,result_state
FROM org_checklist_template_commands WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND operation_id=CAST(sqlc.arg(operation_id)::text AS uuid);

-- name: InsertChecklistCommand :exec
INSERT INTO org_checklist_template_commands(org_id,operation_id,actor_id,template_id,kind,request_hash,result_revision,result_version,result_policy_revision,result_state)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(operation_id)::text AS uuid),CAST(sqlc.arg(actor_id)::text AS uuid),CAST(sqlc.arg(template_id)::text AS uuid),
 sqlc.arg(kind)::text,sqlc.arg(request_hash)::text,sqlc.arg(result_revision)::bigint,sqlc.arg(result_version)::int,sqlc.arg(result_policy_revision)::int,sqlc.arg(result_state)::text);

-- name: InsertChecklistTemplate :exec
INSERT INTO org_checklist_templates(org_id,id,context,state,revision,version,policy_revision,operation_id,created_by)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(id)::text AS uuid),sqlc.arg(context)::text,'ACTIVE',1,1,1,CAST(sqlc.arg(operation_id)::text AS uuid),CAST(sqlc.arg(created_by)::text AS uuid));

-- name: ChecklistTemplateForUpdate :one
SELECT context,state,revision,version,policy_revision FROM org_checklist_templates
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND id=CAST(sqlc.arg(id)::text AS uuid) FOR UPDATE;

-- name: AdvanceChecklistVersion :execrows
UPDATE org_checklist_templates SET revision=revision+1,version=version+1
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND id=CAST(sqlc.arg(id)::text AS uuid) AND state='ACTIVE' AND revision=sqlc.arg(revision)::bigint;

-- name: AdvanceChecklistPolicy :execrows
UPDATE org_checklist_templates SET revision=revision+1,policy_revision=policy_revision+1
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND id=CAST(sqlc.arg(id)::text AS uuid) AND state='ACTIVE' AND revision=sqlc.arg(revision)::bigint;

-- name: ArchiveChecklistTemplate :execrows
UPDATE org_checklist_templates SET revision=revision+1,state='ARCHIVED',archive_operation_id=CAST(sqlc.arg(archive_operation_id)::text AS uuid),archived_by=CAST(sqlc.arg(archived_by)::text AS uuid)
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND id=CAST(sqlc.arg(id)::text AS uuid) AND state='ACTIVE' AND revision=sqlc.arg(revision)::bigint;

-- name: InsertChecklistVersion :exec
INSERT INTO org_checklist_template_versions(org_id,template_id,version,name,target_kind,target_count,item_count,operation_id,published_by)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(template_id)::text AS uuid),sqlc.arg(version)::int,sqlc.arg(name)::text,sqlc.arg(target_kind)::text,
 sqlc.arg(target_count)::int,sqlc.arg(item_count)::int,CAST(sqlc.arg(operation_id)::text AS uuid),CAST(sqlc.arg(published_by)::text AS uuid));

-- name: InsertChecklistItems :exec
INSERT INTO org_checklist_template_items(org_id,template_id,version,position,label,critical,optional,photo_on_not_ok)
SELECT CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(template_id)::text AS uuid),sqlc.arg(version)::int,i.position::int,i.label,
 (sqlc.arg(criticals)::boolean[])[i.position],(sqlc.arg(optionals)::boolean[])[i.position],(sqlc.arg(photos)::boolean[])[i.position]
FROM unnest(sqlc.arg(labels)::text[]) WITH ORDINALITY AS i(label,position)
ORDER BY i.position;

-- name: InsertChecklistTargets :exec
INSERT INTO org_checklist_template_targets(org_id,template_id,version,category_id,asset_id)
SELECT CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(template_id)::text AS uuid),sqlc.arg(version)::int,
 CASE WHEN sqlc.arg(categories)::boolean THEN CAST(t.id AS uuid) END,CASE WHEN sqlc.arg(categories)::boolean THEN NULL ELSE CAST(t.id AS uuid) END
FROM unnest(sqlc.arg(ids)::text[]) AS t(id) ORDER BY t.id;

-- name: InsertChecklistPolicy :exec
INSERT INTO org_checklist_policy_revisions(org_id,template_id,revision,mode,validity_seconds,operation_id,set_by)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(template_id)::text AS uuid),sqlc.arg(revision)::int,sqlc.arg(mode)::text,sqlc.arg(validity_seconds)::int,
 CAST(sqlc.arg(operation_id)::text AS uuid),CAST(sqlc.arg(set_by)::text AS uuid));

-- name: ChecklistKnownCategories :one
SELECT count(*)::bigint AS known FROM org_asset_categories WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND id=ANY(CAST(sqlc.arg(ids)::text[] AS uuid[]));

-- name: ChecklistTargetAssets :many
SELECT id::text AS id,state FROM org_assets WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND id=ANY(CAST(sqlc.arg(ids)::text[] AS uuid[]));

-- name: ChecklistTargetTaken :one
-- B-BR-EMP-332: another ACTIVE template of the context names the
-- organization default or one of the identifiers in its current version.
SELECT EXISTS(SELECT 1 FROM org_checklist_templates o JOIN org_checklist_template_versions v ON(v.org_id,v.template_id,v.version)=(o.org_id,o.id,o.version)
 WHERE o.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND o.id<>CAST(sqlc.arg(template_id)::text AS uuid) AND o.state='ACTIVE' AND o.context=sqlc.arg(context)::text
 AND(sqlc.arg(target_kind)::text='ORGANIZATION' AND v.target_kind='ORGANIZATION'
  OR EXISTS(SELECT 1 FROM org_checklist_template_targets t WHERE(t.org_id,t.template_id,t.version)=(v.org_id,v.template_id,v.version)
   AND(sqlc.arg(target_kind)::text='CATEGORIES' AND t.category_id=ANY(CAST(sqlc.arg(ids)::text[] AS uuid[]))
    OR sqlc.arg(target_kind)::text='ASSETS' AND t.asset_id=ANY(CAST(sqlc.arg(ids)::text[] AS uuid[]))))))::boolean AS taken;

-- name: ListChecklistTemplates :many
SELECT o.id::text AS id,o.context,o.state,o.revision,o.version,o.policy_revision,v.name,v.target_kind,o.created_at,o.archived_at
FROM org_checklist_templates o JOIN org_checklist_template_versions v ON(v.org_id,v.template_id,v.version)=(o.org_id,o.id,o.version)
WHERE o.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND(sqlc.arg(after_id)::text='' OR o.id>CAST(NULLIF(sqlc.arg(after_id)::text,'') AS uuid))
ORDER BY o.id LIMIT sqlc.arg(row_limit)::int;

-- name: ChecklistTemplateHead :one
SELECT context,state,revision,version,policy_revision,created_at,archived_at FROM org_checklist_templates
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND id=CAST(sqlc.arg(id)::text AS uuid);

-- name: ChecklistVersion :one
SELECT name,target_kind,target_count,item_count,published_at FROM org_checklist_template_versions
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND template_id=CAST(sqlc.arg(template_id)::text AS uuid) AND version=sqlc.arg(version)::int;

-- name: ChecklistVersionItems :many
SELECT position,label,critical,optional,photo_on_not_ok FROM org_checklist_template_items
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND template_id=CAST(sqlc.arg(template_id)::text AS uuid) AND version=sqlc.arg(version)::int ORDER BY position;

-- name: ChecklistVersionTargets :many
SELECT COALESCE(category_id,asset_id)::text AS id FROM org_checklist_template_targets
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND template_id=CAST(sqlc.arg(template_id)::text AS uuid) AND version=sqlc.arg(version)::int ORDER BY 1;

-- name: ChecklistPolicyRevision :one
SELECT mode,validity_seconds,effective_at FROM org_checklist_policy_revisions
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND template_id=CAST(sqlc.arg(template_id)::text AS uuid) AND revision=sqlc.arg(revision)::int;

-- name: ChecklistAssetCategory :one
SELECT category_id::text AS category_id FROM org_assets WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND id=CAST(sqlc.arg(id)::text AS uuid);

-- name: ChecklistApplicableCandidates :many
-- The ACTIVE templates of the context whose current version names the asset,
-- its category or the organization default, with only the matching target.
SELECT o.id::text AS template_id,v.target_kind,COALESCE(COALESCE(t.category_id,t.asset_id)::text,'')::text AS target_id
FROM org_checklist_templates o JOIN org_checklist_template_versions v ON(v.org_id,v.template_id,v.version)=(o.org_id,o.id,o.version)
LEFT JOIN org_checklist_template_targets t ON(t.org_id,t.template_id,t.version)=(v.org_id,v.template_id,v.version)
 AND(t.asset_id=CAST(sqlc.arg(asset_id)::text AS uuid) OR t.category_id=CAST(sqlc.arg(category_id)::text AS uuid))
WHERE o.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND o.state='ACTIVE' AND o.context=sqlc.arg(context)::text
 AND(v.target_kind='ORGANIZATION' OR t.template_id IS NOT NULL)
ORDER BY o.id,2,3;
