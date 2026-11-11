-- name: GetRoot :one
SELECT org_id::text AS org_id, has_snapshot,
 COALESCE(display_name,'')::text AS display_name,
 COALESCE(time_zone,'')::text AS time_zone,
 COALESCE(kind_tags,ARRAY[]::text[])::text[] AS kind_tags,
 COALESCE(state,'')::text AS state, revision,
 COALESCE(suspension_reason,'')::text AS suspension_reason
FROM organizations WHERE org_id=CAST(sqlc.arg('org_id')::text AS uuid);

-- name: InsertRoot :execrows
INSERT INTO organizations(org_id,has_snapshot,display_name,time_zone,kind_tags,state,revision,suspension_reason)
VALUES(CAST(sqlc.arg('org_id')::text AS uuid),true,
 @display_name::text,@time_zone::text,@kind_tags::text[],'PENDING_SETUP',1,'')
ON CONFLICT(org_id) DO NOTHING;

-- name: AdvanceRoot :execrows
UPDATE organizations SET display_name = @display_name::text,time_zone = @time_zone::text,
 kind_tags = @kind_tags::text[],state = @state::text,revision = @revision::bigint,
 suspension_reason = @suspension_reason::text
WHERE org_id=CAST(sqlc.arg('org_id')::text AS uuid) AND has_snapshot
 AND revision = @expected_revision::bigint AND state<>'CLOSED';
