-- name: LockDataRightsReviewWrite :exec
SELECT pg_catalog.pg_advisory_xact_lock(pg_catalog.hashtextextended('enterprise-data-source-rights', 0));

-- name: ReadDataRightsOperation :one
SELECT source_code,source_digest,revision,status,decision_ref,license_ref,
       COALESCE(snapshot_digest,'')::text AS snapshot_digest,
       COALESCE(official_revision::text,'')::text AS official_revision,
       COALESCE(recorded_by = session_user,false)::boolean AS same_actor
FROM enterprise_data_source_reviews
WHERE operation_id=sqlc.arg(operation_id)::text::uuid;

-- name: ReadLatestSourceReview :one
SELECT revision,status FROM enterprise_data_source_reviews
WHERE source_code=sqlc.arg(source_code)::text AND source_digest=sqlc.arg(source_digest)::text
ORDER BY revision DESC LIMIT 1;

-- name: InsertDataRightsOperation :exec
INSERT INTO enterprise_data_source_reviews
    (operation_id,source_code,source_digest,revision,status,decision_ref,license_ref,attribution,snapshot_digest,official_revision)
VALUES (sqlc.arg(operation_id)::text::uuid,sqlc.arg(source_code)::text,sqlc.arg(source_digest)::text,
        sqlc.arg(revision)::bigint,sqlc.arg(status)::text,sqlc.arg(decision_ref)::text,sqlc.arg(license_ref)::text,
        sqlc.arg(attribution)::text,NULLIF(sqlc.arg(snapshot_digest)::text,''),NULLIF(sqlc.arg(official_revision)::text,'')::uuid);
