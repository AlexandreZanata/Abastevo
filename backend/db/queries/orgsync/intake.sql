-- name: ReadOperation :one
SELECT org_id::text AS org_id,operation_id::text AS operation_id,owner_account_id::text AS owner_account_id,api_origin,membership_id::text AS membership_id,enrollment_id::text AS enrollment_id,operation_type,operation_schema,entity_type,entity_id::text AS entity_id,payload_digest
FROM org_sync_operations WHERE org_id=CAST(@org_id::text AS uuid) AND operation_id=CAST(@operation_id::text AS uuid) FOR UPDATE;
-- name: ReserveOperation :execrows
INSERT INTO org_sync_operations(org_id,operation_id,owner_account_id,api_origin,membership_id,enrollment_id,operation_type,operation_schema,entity_type,entity_id,payload_digest)
VALUES(CAST(@org_id::text AS uuid),CAST(@operation_id::text AS uuid),CAST(@owner_account_id::text AS uuid),@api_origin::text,CAST(@membership_id::text AS uuid),CAST(@enrollment_id::text AS uuid),@operation_type::text,@operation_schema::bigint,@entity_type::text,CAST(@entity_id::text AS uuid),@payload_digest::text)
ON CONFLICT(org_id,operation_id) DO NOTHING;
-- name: ReadAcknowledgement :one
SELECT k.canonical_ack,k.result,k.code,k.entity_revision,COALESCE(k.review_item_id::text,'')::text AS review_item_id,k.received_at,k.warnings,k.audit_hash,
 a.actor_id::text AS audit_actor_id,a.action AS audit_action,a.target_type AS audit_target_type,a.target_id::text AS audit_target_id,a.request_hash AS source_audit_hash
FROM org_sync_acknowledgements k JOIN org_audit_events a USING(org_id,operation_id)
WHERE k.org_id=CAST(@org_id::text AS uuid) AND k.operation_id=CAST(@operation_id::text AS uuid);
-- name: InsertAcknowledgement :exec
INSERT INTO org_sync_acknowledgements(org_id,operation_id,result,code,entity_revision,review_item_id,received_at,warnings,canonical_ack,audit_hash)
VALUES(CAST(@org_id::text AS uuid),CAST(@operation_id::text AS uuid),@result::text,@code::text,sqlc.narg('entity_revision')::bigint,NULLIF(@review_item_id::text,'')::uuid,@received_at::timestamptz,@warnings::text[],@canonical_ack::bytea,@audit_hash::text);
-- name: InsertEffect :exec
INSERT INTO org_sync_effects(org_id,operation_id,entity_revision)VALUES(CAST(@org_id::text AS uuid),CAST(@operation_id::text AS uuid),@entity_revision::bigint);
-- No claim/dependency row locks or foreign placeholders; completed proofs are immutable.
-- name: ReadyDependencies :one
SELECT count(*)::bigint AS ready FROM org_sync_effects WHERE org_id=CAST(@org_id::text AS uuid) AND operation_id=ANY(@operation_ids::uuid[]);
