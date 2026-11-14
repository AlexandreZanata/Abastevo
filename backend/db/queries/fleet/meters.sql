-- name: MeterCurrentDBTime :one
SELECT clock_timestamp()::timestamptz AS at;

-- name: InsertMeter :exec
INSERT INTO org_meters(org_id,id,asset_id,kind,label,modulus,installed_at,installed_submicro,created_operation_id)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(id)::text AS uuid),CAST(sqlc.arg(asset_id)::text AS uuid),sqlc.arg(kind)::text,sqlc.arg(label)::text,NULLIF(sqlc.arg(modulus)::bigint,0),sqlc.arg(installed_at)::timestamptz,sqlc.arg(installed_submicro)::smallint,CAST(sqlc.arg(operation_id)::text AS uuid));

-- name: InsertMeterSegment :exec
INSERT INTO org_meter_segments(org_id,id,meter_id,kind,opened_at,opened_submicro,opening_reading_id)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(id)::text AS uuid),CAST(sqlc.arg(meter_id)::text AS uuid),'INSTALLATION',sqlc.arg(opened_at)::timestamptz,sqlc.arg(opened_submicro)::smallint,CAST(sqlc.arg(opening_reading_id)::text AS uuid));

-- name: GetMeter :one
SELECT m.id::text AS id,m.asset_id::text AS asset_id,m.kind,m.label,COALESCE(m.modulus,0)::bigint AS modulus,m.installed_at,m.installed_submicro,m.removed_at,m.removed_submicro,s.id::text AS segment_id,s.opened_at,s.opened_submicro,s.closed_at,s.closed_submicro
FROM org_meters m JOIN org_meter_segments s ON(s.org_id,s.meter_id)=(m.org_id,m.id)
WHERE m.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND m.asset_id=CAST(sqlc.arg(asset_id)::text AS uuid) AND m.id=CAST(sqlc.arg(id)::text AS uuid) AND s.closed_at IS NULL;

-- name: LockMeter :one
SELECT id::text AS id FROM org_meters WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND asset_id=CAST(sqlc.arg(asset_id)::text AS uuid) AND id=CAST(sqlc.arg(id)::text AS uuid) FOR UPDATE;

-- name: ListAssetMeterIDs :many
SELECT id::text AS id FROM org_meters WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND asset_id=CAST(sqlc.arg(asset_id)::text AS uuid) AND(sqlc.arg(after_id)::text='' OR id>CAST(NULLIF(sqlc.arg(after_id)::text,'') AS uuid)) ORDER BY id LIMIT sqlc.arg(row_limit)::int;

-- name: ActualInServiceAssetMeters :many
SELECT id::text AS id FROM org_meters WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND asset_id=CAST(sqlc.arg(asset_id)::text AS uuid) AND(installed_at,installed_submicro)<=(sqlc.arg(at)::timestamptz,0) AND removed_at IS NULL ORDER BY id;

-- name: InsertMeterReading :exec
INSERT INTO org_meter_readings(org_id,id,meter_id,segment_id,value,occurred_at,occurred_submicro,entry_zone,device_offset_seconds,author_membership_id,capture_kind,enrollment_id,web_session_id,operation_id,request_hash,origin,method,state)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(id)::text AS uuid),CAST(sqlc.arg(meter_id)::text AS uuid),CAST(sqlc.arg(segment_id)::text AS uuid),sqlc.arg(value)::bigint,sqlc.arg(occurred_at)::timestamptz,sqlc.arg(occurred_submicro)::smallint,sqlc.arg(entry_zone)::text,sqlc.arg(device_offset_seconds)::bigint,CAST(sqlc.arg(author_membership_id)::text AS uuid),sqlc.arg(capture_kind)::text,CAST(NULLIF(sqlc.arg(enrollment_id)::text,'') AS uuid),CAST(NULLIF(sqlc.arg(web_session_id)::text,'') AS uuid),CAST(sqlc.arg(operation_id)::text AS uuid),sqlc.arg(request_hash)::text,sqlc.arg(origin)::text,sqlc.arg(method)::text,sqlc.arg(state)::text);

-- name: GetMeterReadingByOperation :one
SELECT r.id::text AS id,r.meter_id::text AS meter_id,r.segment_id::text AS segment_id,r.value,r.occurred_at,r.occurred_submicro,r.received_at,r.entry_zone,r.device_offset_seconds,r.author_membership_id::text AS author_membership_id,r.capture_kind,COALESCE(r.enrollment_id::text,r.web_session_id::text)::text AS capture_id,r.operation_id::text AS operation_id,r.request_hash,r.origin,r.method,r.state
FROM org_meter_readings r WHERE r.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND r.meter_id=CAST(sqlc.arg(meter_id)::text AS uuid) AND r.operation_id=CAST(sqlc.arg(operation_id)::text AS uuid);

-- name: LatestValidMeterReading :one
SELECT r.id::text AS id,r.meter_id::text AS meter_id,r.segment_id::text AS segment_id,r.value,r.occurred_at,r.occurred_submicro,r.received_at,r.entry_zone,r.device_offset_seconds,r.author_membership_id::text AS author_membership_id,r.capture_kind,COALESCE(r.enrollment_id::text,r.web_session_id::text)::text AS capture_id,r.operation_id::text AS operation_id,r.request_hash,r.origin,r.method,r.state
FROM org_meter_readings r WHERE r.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND r.meter_id=CAST(sqlc.arg(meter_id)::text AS uuid) AND r.segment_id=CAST(sqlc.arg(segment_id)::text AS uuid) AND r.state='VALID'
ORDER BY r.occurred_at DESC,r.occurred_submicro DESC,r.received_at DESC,r.id DESC LIMIT 1;

-- name: MeterCommandReceipt :one
SELECT actor_id::text AS actor_id,asset_id::text AS asset_id,meter_id::text AS meter_id,segment_id::text AS segment_id,opening_reading_id::text AS opening_reading_id,request_hash,asset_revision FROM org_meter_commands WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND operation_id=CAST(sqlc.arg(operation_id)::text AS uuid);

-- name: InsertMeterCommand :exec
INSERT INTO org_meter_commands(org_id,operation_id,actor_id,asset_id,meter_id,segment_id,opening_reading_id,request_hash,asset_revision)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(operation_id)::text AS uuid),CAST(sqlc.arg(actor_id)::text AS uuid),CAST(sqlc.arg(asset_id)::text AS uuid),CAST(sqlc.arg(meter_id)::text AS uuid),CAST(sqlc.arg(segment_id)::text AS uuid),CAST(sqlc.arg(opening_reading_id)::text AS uuid),sqlc.arg(request_hash)::text,sqlc.arg(asset_revision)::bigint);

-- name: LockMeterAsset :one
SELECT id::text AS id FROM org_assets WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND id=CAST(sqlc.arg(asset_id)::text AS uuid) FOR UPDATE;

-- name: GetMeterReading :one
SELECT r.id::text AS id,r.meter_id::text AS meter_id,r.segment_id::text AS segment_id,r.value,r.occurred_at,r.occurred_submicro,r.received_at,r.entry_zone,r.device_offset_seconds,r.author_membership_id::text AS author_membership_id,r.capture_kind,COALESCE(r.enrollment_id::text,r.web_session_id::text)::text AS capture_id,r.operation_id::text AS operation_id,r.request_hash,r.origin,r.method,r.state
FROM org_meter_readings r WHERE r.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND r.meter_id=CAST(sqlc.arg(meter_id)::text AS uuid) AND r.segment_id=CAST(sqlc.arg(segment_id)::text AS uuid) AND r.id=CAST(sqlc.arg(id)::text AS uuid);
