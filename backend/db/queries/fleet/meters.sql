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
-- B-BR-EMP-284: the current state is the latest history revision, else the
-- recorded classification; only a currently VALID reading is the current value.
SELECT r.id::text AS id,r.meter_id::text AS meter_id,r.segment_id::text AS segment_id,r.value,r.occurred_at,r.occurred_submicro,r.received_at,r.entry_zone,r.device_offset_seconds,r.author_membership_id::text AS author_membership_id,r.capture_kind,COALESCE(r.enrollment_id::text,r.web_session_id::text)::text AS capture_id,r.operation_id::text AS operation_id,r.request_hash,r.origin,r.method,'VALID'::text AS state
FROM org_meter_readings r WHERE r.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND r.meter_id=CAST(sqlc.arg(meter_id)::text AS uuid) AND r.segment_id=CAST(sqlc.arg(segment_id)::text AS uuid)
AND COALESCE((SELECT s.state FROM org_meter_reading_states s WHERE(s.org_id,s.meter_id,s.reading_id)=(r.org_id,r.meter_id,r.id) ORDER BY s.revision DESC LIMIT 1),r.state)='VALID'
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

-- name: MeterSegmentSeries :many
-- The whole segment in BR-METER-008 order with each reading's current state.
SELECT r.id::text AS id,r.value,r.occurred_at,r.occurred_submicro,r.received_at,
 COALESCE(c.state,r.state)::text AS state,COALESCE(c.flags,'{}'::text[])::text[] AS flags,COALESCE(c.revision,0)::int AS revision
FROM org_meter_readings r LEFT JOIN LATERAL(SELECT s.state,s.flags,s.revision FROM org_meter_reading_states s WHERE(s.org_id,s.meter_id,s.reading_id)=(r.org_id,r.meter_id,r.id) ORDER BY s.revision DESC LIMIT 1) c ON true
WHERE r.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND r.meter_id=CAST(sqlc.arg(meter_id)::text AS uuid) AND r.segment_id=CAST(sqlc.arg(segment_id)::text AS uuid)
ORDER BY r.occurred_at,r.occurred_submicro,r.received_at,r.id;

-- name: InsertMeterReadingState :exec
INSERT INTO org_meter_reading_states(org_id,meter_id,reading_id,revision,state,flags,retroactive,cause,operation_id,actor_membership_id,reason,superseded_by)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(meter_id)::text AS uuid),CAST(sqlc.arg(reading_id)::text AS uuid),sqlc.arg(revision)::int,sqlc.arg(state)::text,sqlc.arg(flags)::text[],sqlc.arg(retroactive)::boolean,sqlc.arg(cause)::text,CAST(sqlc.arg(operation_id)::text AS uuid),
 CAST(NULLIF(sqlc.arg(actor_membership_id)::text,'') AS uuid),NULLIF(sqlc.arg(reason)::text,''),CAST(NULLIF(sqlc.arg(superseded_by)::text,'') AS uuid));

-- name: MeterReadingCurrent :one
-- EMP-METER-004: one reading of an open segment with its current state.
SELECT r.id::text AS id,r.origin,r.operation_id::text AS operation_id,COALESCE(c.state,r.state)::text AS state,COALESCE(c.flags,'{}'::text[])::text[] AS flags,COALESCE(c.revision,0)::int AS revision
FROM org_meter_readings r LEFT JOIN LATERAL(SELECT s.state,s.flags,s.revision FROM org_meter_reading_states s WHERE(s.org_id,s.meter_id,s.reading_id)=(r.org_id,r.meter_id,r.id) ORDER BY s.revision DESC LIMIT 1) c ON true
WHERE r.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND r.meter_id=CAST(sqlc.arg(meter_id)::text AS uuid) AND r.segment_id=CAST(sqlc.arg(segment_id)::text AS uuid) AND r.id=CAST(sqlc.arg(id)::text AS uuid);

-- name: MeterStatesByOperation :many
SELECT reading_id::text AS reading_id,revision,state,flags::text[] AS flags,cause,COALESCE(superseded_by::text,'')::text AS superseded_by
FROM org_meter_reading_states WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND meter_id=CAST(sqlc.arg(meter_id)::text AS uuid) AND operation_id=CAST(sqlc.arg(operation_id)::text AS uuid)
ORDER BY reading_id,revision;

-- name: MeterOperationStateCount :one
SELECT count(*)::bigint AS n FROM org_meter_reading_states WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND operation_id=CAST(sqlc.arg(operation_id)::text AS uuid);

-- name: MeterReadingHistoryPage :many
-- B-BR-EMP-291: the open segment in series order with each reading's current
-- state, its supersession links and the reason of its latest person decision.
SELECT r.id::text AS id,r.meter_id::text AS meter_id,r.segment_id::text AS segment_id,r.value,r.occurred_at,r.occurred_submicro,r.received_at,r.entry_zone,r.device_offset_seconds,r.author_membership_id::text AS author_membership_id,r.capture_kind,COALESCE(r.enrollment_id::text,r.web_session_id::text)::text AS capture_id,r.operation_id::text AS operation_id,r.request_hash,r.origin,r.method,r.state,
 COALESCE(c.state,r.state)::text AS current_state,COALESCE(c.flags,'{}'::text[])::text[] AS flags,COALESCE(c.retroactive,false)::boolean AS retroactive,COALESCE(c.revision,0)::int AS revision,
 COALESCE((SELECT v.superseded_by::text FROM org_meter_reading_states v WHERE(v.org_id,v.meter_id,v.reading_id)=(r.org_id,r.meter_id,r.id) AND v.superseded_by IS NOT NULL),'')::text AS superseded_by,
 COALESCE((SELECT v.reading_id::text FROM org_meter_reading_states v WHERE(v.org_id,v.meter_id,v.superseded_by)=(r.org_id,r.meter_id,r.id)),'')::text AS supersedes,
 COALESCE((SELECT v.reason FROM org_meter_reading_states v WHERE(v.org_id,v.meter_id,v.reading_id)=(r.org_id,r.meter_id,r.id) AND v.reason IS NOT NULL ORDER BY v.revision DESC LIMIT 1),'')::text AS reason
FROM org_meter_readings r LEFT JOIN LATERAL(SELECT s.state,s.flags,s.retroactive,s.revision FROM org_meter_reading_states s WHERE(s.org_id,s.meter_id,s.reading_id)=(r.org_id,r.meter_id,r.id) ORDER BY s.revision DESC LIMIT 1) c ON true
WHERE r.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND r.meter_id=CAST(sqlc.arg(meter_id)::text AS uuid) AND r.segment_id=CAST(sqlc.arg(segment_id)::text AS uuid)
AND(sqlc.arg(state)::text='' OR COALESCE(c.state,r.state)=sqlc.arg(state)::text)
AND(sqlc.arg(after_id)::text='' OR(r.occurred_at,r.occurred_submicro,r.received_at,r.id)>(SELECT a.occurred_at,a.occurred_submicro,a.received_at,a.id FROM org_meter_readings a WHERE a.org_id=r.org_id AND a.meter_id=r.meter_id AND a.id=CAST(NULLIF(sqlc.arg(after_id)::text,'') AS uuid)))
ORDER BY r.occurred_at,r.occurred_submicro,r.received_at,r.id LIMIT sqlc.arg(row_limit)::int;
