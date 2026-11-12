-- name: EnsureSettings :one
SELECT org_settings_bootstrap(CAST(@org_id::text AS uuid))::bigint AS revision;

-- name: GetSettingsRevision :one
SELECT revision,effective_at,settings_values,source,COALESCE(author_account_id::text,'')::text AS author_account_id,
 COALESCE(author_membership_id::text,'')::text AS author_membership_id,reason,COALESCE(operation_id::text,'')::text AS operation_id,
 COALESCE(source_membership_id::text,'')::text AS source_membership_id,COALESCE(source_membership_revision,0)::bigint AS source_membership_revision
FROM org_settings_versions WHERE org_id=CAST(@org_id::text AS uuid) AND revision= @revision::bigint;

-- name: ListSettingsRevisions :many
SELECT revision,effective_at,settings_values,source,COALESCE(author_account_id::text,'')::text AS author_account_id,
 COALESCE(author_membership_id::text,'')::text AS author_membership_id,reason,COALESCE(operation_id::text,'')::text AS operation_id,
 COALESCE(source_membership_id::text,'')::text AS source_membership_id,COALESCE(source_membership_revision,0)::bigint AS source_membership_revision
FROM org_settings_versions WHERE org_id=CAST(@org_id::text AS uuid) AND(@before_revision::bigint=0 OR revision < @before_revision::bigint)
ORDER BY revision DESC LIMIT @row_limit::int;

-- name: InsertSettingsRevision :exec
INSERT INTO org_settings_versions(org_id,revision,settings_values,source,author_account_id,author_membership_id,reason,operation_id)
VALUES(CAST(@org_id::text AS uuid),@revision::bigint,@settings_values::jsonb,'COMMAND',CAST(@actor_id::text AS uuid),CAST(@member_id::text AS uuid),@reason::text,CAST(@operation_id::text AS uuid));

-- name: PublishSettingsRevision :execrows
UPDATE org_settings_heads SET revision=revision+1 WHERE org_id=CAST(@org_id::text AS uuid) AND revision= @expected_revision::bigint;

-- name: SettingsCommandReceipt :one
SELECT c.actor_id::text AS actor_id,c.request_hash,c.settings_revision,c.result_root_revision,v.effective_at
FROM org_settings_commands c JOIN org_settings_versions v ON(v.org_id,v.revision)=(c.org_id,c.settings_revision)
WHERE c.org_id=CAST(@org_id::text AS uuid) AND c.operation_id=CAST(@operation_id::text AS uuid);

-- name: InsertSettingsReceipt :exec
INSERT INTO org_settings_commands(org_id,operation_id,actor_id,request_hash,settings_revision,result_root_revision)
VALUES(CAST(@org_id::text AS uuid),CAST(@operation_id::text AS uuid),CAST(@actor_id::text AS uuid),@request_hash::text,@settings_revision::bigint,@result_root_revision::bigint);

-- name: SettingsRootRevision :one
SELECT revision FROM organizations WHERE org_id=CAST(@org_id::text AS uuid) AND has_snapshot;

-- name: SettingsIntakeClock :one
SELECT clock_timestamp()::timestamptz AS received_at,h.revision FROM organizations o JOIN org_settings_heads h ON h.org_id=o.org_id
WHERE o.org_id=CAST(@org_id::text AS uuid) AND o.has_snapshot AND o.state<>'CLOSED' FOR UPDATE OF o;
