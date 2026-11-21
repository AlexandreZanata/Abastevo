-- name: AddTelematicsEvidence :execrows
INSERT INTO org_telematics_evidence (org_id,connection_id,batch_id,trip_id,tracking_session_id,
    provider_code,payload_hash,precise,approximate,insufficient,received_at)
VALUES(sqlc.arg('org_id')::text::uuid,sqlc.arg('connection_id')::text::uuid,sqlc.arg('batch_id')::text::uuid,
       sqlc.arg('trip_id')::text::uuid,sqlc.arg('session_id')::text::uuid,sqlc.arg('provider_code')::text,
       sqlc.arg('payload_hash')::text,sqlc.arg('precise')::integer,sqlc.arg('approximate')::integer,
       sqlc.arg('insufficient')::integer,sqlc.arg('received_at')::timestamptz)
ON CONFLICT DO NOTHING;

-- name: ReadTelematicsEvidence :one
SELECT provider_code,payload_hash,precise,approximate,insufficient,received_at FROM org_telematics_evidence
WHERE org_id=sqlc.arg('org_id')::text::uuid AND connection_id=sqlc.arg('connection_id')::text::uuid
  AND batch_id=sqlc.arg('batch_id')::text::uuid;
