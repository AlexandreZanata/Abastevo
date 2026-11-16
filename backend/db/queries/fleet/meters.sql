-- name: MeterCurrentDBTime :one
SELECT clock_timestamp()::timestamptz AS at;

-- name: InsertMeter :exec
INSERT INTO org_meters(org_id,id,asset_id,kind,label,modulus,installed_at,installed_submicro,created_operation_id)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(id)::text AS uuid),CAST(sqlc.arg(asset_id)::text AS uuid),sqlc.arg(kind)::text,sqlc.arg(label)::text,NULLIF(sqlc.arg(modulus)::bigint,0),sqlc.arg(installed_at)::timestamptz,sqlc.arg(installed_submicro)::smallint,CAST(sqlc.arg(operation_id)::text AS uuid));

-- name: InsertMeterSegment :exec
INSERT INTO org_meter_segments(org_id,id,meter_id,kind,opened_at,opened_submicro,opening_reading_id)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(id)::text AS uuid),CAST(sqlc.arg(meter_id)::text AS uuid),'INSTALLATION',sqlc.arg(opened_at)::timestamptz,sqlc.arg(opened_submicro)::smallint,CAST(sqlc.arg(opening_reading_id)::text AS uuid));

-- name: GetMeter :one
-- EMP-METER-005: segment-keyed reads below count a reading in the segment
-- B-BR-EMP-481 derives (the inlined org_meter_reading_segment rule over
-- 000198 columns, so older upgrade images still run), not its recorded one.
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
SELECT r.id::text AS id,r.meter_id::text AS meter_id,CAST(sqlc.arg(segment_id)::text AS uuid)::text AS segment_id,r.value,r.occurred_at,r.occurred_submicro,r.received_at,r.entry_zone,r.device_offset_seconds,r.author_membership_id::text AS author_membership_id,r.capture_kind,COALESCE(r.enrollment_id::text,r.web_session_id::text)::text AS capture_id,r.operation_id::text AS operation_id,r.request_hash,r.origin,r.method,'VALID'::text AS state
FROM org_meter_readings r WHERE r.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND r.meter_id=CAST(sqlc.arg(meter_id)::text AS uuid) AND (CASE WHEN r.origin='METER_REMOVAL' THEN r.segment_id ELSE(SELECT g.id FROM org_meter_segments g WHERE(g.org_id,g.meter_id)=(r.org_id,r.meter_id) AND(g.opened_at,g.opened_submicro)<=(r.occurred_at,r.occurred_submicro) AND(g.closed_at IS NULL OR(r.occurred_at,r.occurred_submicro)<(g.closed_at,g.closed_submicro)) ORDER BY g.opened_at DESC,g.opened_submicro DESC LIMIT 1) END)=CAST(sqlc.arg(segment_id)::text AS uuid)
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
SELECT r.id::text AS id,r.meter_id::text AS meter_id,CAST(sqlc.arg(segment_id)::text AS uuid)::text AS segment_id,r.value,r.occurred_at,r.occurred_submicro,r.received_at,r.entry_zone,r.device_offset_seconds,r.author_membership_id::text AS author_membership_id,r.capture_kind,COALESCE(r.enrollment_id::text,r.web_session_id::text)::text AS capture_id,r.operation_id::text AS operation_id,r.request_hash,r.origin,r.method,r.state
FROM org_meter_readings r WHERE r.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND r.meter_id=CAST(sqlc.arg(meter_id)::text AS uuid) AND (CASE WHEN r.origin='METER_REMOVAL' THEN r.segment_id ELSE(SELECT g.id FROM org_meter_segments g WHERE(g.org_id,g.meter_id)=(r.org_id,r.meter_id) AND(g.opened_at,g.opened_submicro)<=(r.occurred_at,r.occurred_submicro) AND(g.closed_at IS NULL OR(r.occurred_at,r.occurred_submicro)<(g.closed_at,g.closed_submicro)) ORDER BY g.opened_at DESC,g.opened_submicro DESC LIMIT 1) END)=CAST(sqlc.arg(segment_id)::text AS uuid) AND r.id=CAST(sqlc.arg(id)::text AS uuid);

-- name: MeterSegmentSeries :many
-- The whole segment in BR-METER-008 order with each reading's current state.
SELECT r.id::text AS id,r.value,r.occurred_at,r.occurred_submicro,r.received_at,
 COALESCE(c.state,r.state)::text AS state,COALESCE(c.flags,'{}'::text[])::text[] AS flags,COALESCE(c.revision,0)::int AS revision
FROM org_meter_readings r LEFT JOIN LATERAL(SELECT s.state,s.flags,s.revision FROM org_meter_reading_states s WHERE(s.org_id,s.meter_id,s.reading_id)=(r.org_id,r.meter_id,r.id) ORDER BY s.revision DESC LIMIT 1) c ON true
WHERE r.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND r.meter_id=CAST(sqlc.arg(meter_id)::text AS uuid) AND (CASE WHEN r.origin='METER_REMOVAL' THEN r.segment_id ELSE(SELECT g.id FROM org_meter_segments g WHERE(g.org_id,g.meter_id)=(r.org_id,r.meter_id) AND(g.opened_at,g.opened_submicro)<=(r.occurred_at,r.occurred_submicro) AND(g.closed_at IS NULL OR(r.occurred_at,r.occurred_submicro)<(g.closed_at,g.closed_submicro)) ORDER BY g.opened_at DESC,g.opened_submicro DESC LIMIT 1) END)=CAST(sqlc.arg(segment_id)::text AS uuid)
ORDER BY r.occurred_at,r.occurred_submicro,r.received_at,r.id;

-- name: InsertMeterReadingState :exec
INSERT INTO org_meter_reading_states(org_id,meter_id,reading_id,revision,state,flags,retroactive,cause,operation_id,actor_membership_id,reason,superseded_by)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(meter_id)::text AS uuid),CAST(sqlc.arg(reading_id)::text AS uuid),sqlc.arg(revision)::int,sqlc.arg(state)::text,sqlc.arg(flags)::text[],sqlc.arg(retroactive)::boolean,sqlc.arg(cause)::text,CAST(sqlc.arg(operation_id)::text AS uuid),
 CAST(NULLIF(sqlc.arg(actor_membership_id)::text,'') AS uuid),NULLIF(sqlc.arg(reason)::text,''),CAST(NULLIF(sqlc.arg(superseded_by)::text,'') AS uuid));

-- name: MeterReadingCurrent :one
-- EMP-METER-004: one reading of an open segment with its current state.
SELECT r.id::text AS id,r.origin,r.operation_id::text AS operation_id,COALESCE(c.state,r.state)::text AS state,COALESCE(c.flags,'{}'::text[])::text[] AS flags,COALESCE(c.revision,0)::int AS revision
FROM org_meter_readings r LEFT JOIN LATERAL(SELECT s.state,s.flags,s.revision FROM org_meter_reading_states s WHERE(s.org_id,s.meter_id,s.reading_id)=(r.org_id,r.meter_id,r.id) ORDER BY s.revision DESC LIMIT 1) c ON true
WHERE r.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND r.meter_id=CAST(sqlc.arg(meter_id)::text AS uuid) AND (CASE WHEN r.origin='METER_REMOVAL' THEN r.segment_id ELSE(SELECT g.id FROM org_meter_segments g WHERE(g.org_id,g.meter_id)=(r.org_id,r.meter_id) AND(g.opened_at,g.opened_submicro)<=(r.occurred_at,r.occurred_submicro) AND(g.closed_at IS NULL OR(r.occurred_at,r.occurred_submicro)<(g.closed_at,g.closed_submicro)) ORDER BY g.opened_at DESC,g.opened_submicro DESC LIMIT 1) END)=CAST(sqlc.arg(segment_id)::text AS uuid) AND r.id=CAST(sqlc.arg(id)::text AS uuid);

-- name: MeterStatesByOperation :many
SELECT reading_id::text AS reading_id,revision,state,flags::text[] AS flags,cause,COALESCE(superseded_by::text,'')::text AS superseded_by
FROM org_meter_reading_states WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND meter_id=CAST(sqlc.arg(meter_id)::text AS uuid) AND operation_id=CAST(sqlc.arg(operation_id)::text AS uuid)
ORDER BY reading_id,revision;

-- name: MeterOperationStateCount :one
SELECT count(*)::bigint AS n FROM org_meter_reading_states WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND operation_id=CAST(sqlc.arg(operation_id)::text AS uuid);

-- name: MeterReadingHistoryPage :many
-- B-BR-EMP-291: the open segment in series order with each reading's current
-- state, its supersession links and the reason of its latest person decision.
SELECT r.id::text AS id,r.meter_id::text AS meter_id,CAST(sqlc.arg(segment_id)::text AS uuid)::text AS segment_id,r.value,r.occurred_at,r.occurred_submicro,r.received_at,r.entry_zone,r.device_offset_seconds,r.author_membership_id::text AS author_membership_id,r.capture_kind,COALESCE(r.enrollment_id::text,r.web_session_id::text)::text AS capture_id,r.operation_id::text AS operation_id,r.request_hash,r.origin,r.method,r.state,
 COALESCE(c.state,r.state)::text AS current_state,COALESCE(c.flags,'{}'::text[])::text[] AS flags,COALESCE(c.retroactive,false)::boolean AS retroactive,COALESCE(c.revision,0)::int AS revision,
 COALESCE((SELECT v.superseded_by::text FROM org_meter_reading_states v WHERE(v.org_id,v.meter_id,v.reading_id)=(r.org_id,r.meter_id,r.id) AND v.superseded_by IS NOT NULL),'')::text AS superseded_by,
 COALESCE((SELECT v.reading_id::text FROM org_meter_reading_states v WHERE(v.org_id,v.meter_id,v.superseded_by)=(r.org_id,r.meter_id,r.id)),'')::text AS supersedes,
 COALESCE((SELECT v.reason FROM org_meter_reading_states v WHERE(v.org_id,v.meter_id,v.reading_id)=(r.org_id,r.meter_id,r.id) AND v.reason IS NOT NULL ORDER BY v.revision DESC LIMIT 1),'')::text AS reason
FROM org_meter_readings r LEFT JOIN LATERAL(SELECT s.state,s.flags,s.retroactive,s.revision FROM org_meter_reading_states s WHERE(s.org_id,s.meter_id,s.reading_id)=(r.org_id,r.meter_id,r.id) ORDER BY s.revision DESC LIMIT 1) c ON true
WHERE r.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND r.meter_id=CAST(sqlc.arg(meter_id)::text AS uuid) AND (CASE WHEN r.origin='METER_REMOVAL' THEN r.segment_id ELSE(SELECT g.id FROM org_meter_segments g WHERE(g.org_id,g.meter_id)=(r.org_id,r.meter_id) AND(g.opened_at,g.opened_submicro)<=(r.occurred_at,r.occurred_submicro) AND(g.closed_at IS NULL OR(r.occurred_at,r.occurred_submicro)<(g.closed_at,g.closed_submicro)) ORDER BY g.opened_at DESC,g.opened_submicro DESC LIMIT 1) END)=CAST(sqlc.arg(segment_id)::text AS uuid)
AND(sqlc.arg(state)::text='' OR COALESCE(c.state,r.state)=sqlc.arg(state)::text)
AND(sqlc.arg(after_id)::text='' OR(r.occurred_at,r.occurred_submicro,r.received_at,r.id)>(SELECT a.occurred_at,a.occurred_submicro,a.received_at,a.id FROM org_meter_readings a WHERE a.org_id=r.org_id AND a.meter_id=r.meter_id AND a.id=CAST(NULLIF(sqlc.arg(after_id)::text,'') AS uuid)))
ORDER BY r.occurred_at,r.occurred_submicro,r.received_at,r.id LIMIT sqlc.arg(row_limit)::int;

-- name: MeterSegmentChain :many
-- EMP-METER-005: every segment of a meter in chain order with its closure.
SELECT s.id::text AS id,s.kind,s.opened_at,s.opened_submicro,s.opening_reading_id::text AS opening_reading_id,COALESCE(s.opened_operation_id::text,'')::text AS opened_operation_id,
 s.closed_at,s.closed_submicro,COALESCE(s.closing_kind,'')::text AS closing_kind,s.closing_value,COALESCE(s.closing_reading_id::text,'')::text AS closing_reading_id,COALESCE(s.closing_operation_id::text,'')::text AS closing_operation_id,
 COALESCE(s.closing_previous_reading_id::text,'')::text AS closing_previous_reading_id,s.closing_implied_usage,
 COALESCE((SELECT r.value FROM org_meter_readings r WHERE(r.org_id,r.meter_id,r.id)=(s.org_id,s.meter_id,s.opening_reading_id)),0)::bigint AS opening_reading_value
FROM org_meter_segments s WHERE s.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND s.meter_id=CAST(sqlc.arg(meter_id)::text AS uuid)
ORDER BY s.opened_at,s.opened_submicro,s.closed_at NULLS LAST,s.id;

-- name: MeterReadingIsAnchor :one
SELECT EXISTS(SELECT 1 FROM org_meter_segments s WHERE s.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND s.meter_id=CAST(sqlc.arg(meter_id)::text AS uuid)
 AND(s.opening_reading_id=CAST(sqlc.arg(reading_id)::text AS uuid) OR s.closing_reading_id=CAST(sqlc.arg(reading_id)::text AS uuid)))::boolean AS anchor;

-- name: CloseMeterSegment :execrows
UPDATE org_meter_segments SET closed_at=sqlc.arg(closed_at)::timestamptz,closed_submicro=sqlc.arg(closed_submicro)::smallint,closing_kind=sqlc.arg(closing_kind)::text,
 closing_value=sqlc.narg(closing_value)::bigint,closing_reading_id=CAST(NULLIF(sqlc.arg(closing_reading_id)::text,'') AS uuid),closing_operation_id=CAST(sqlc.arg(operation_id)::text AS uuid),
 closing_previous_reading_id=CAST(sqlc.arg(previous_reading_id)::text AS uuid),closing_implied_usage=sqlc.narg(implied_usage)::bigint
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND meter_id=CAST(sqlc.arg(meter_id)::text AS uuid) AND id=CAST(sqlc.arg(id)::text AS uuid) AND closed_at IS NULL;

-- name: InsertBoundarySegment :exec
INSERT INTO org_meter_segments(org_id,id,meter_id,kind,opened_at,opened_submicro,opening_reading_id,opened_operation_id)
VALUES(CAST(sqlc.arg(org_id)::text AS uuid),CAST(sqlc.arg(id)::text AS uuid),CAST(sqlc.arg(meter_id)::text AS uuid),sqlc.arg(kind)::text,sqlc.arg(opened_at)::timestamptz,sqlc.arg(opened_submicro)::smallint,CAST(sqlc.arg(opening_reading_id)::text AS uuid),CAST(sqlc.arg(operation_id)::text AS uuid));

-- name: RemoveMeter :execrows
UPDATE org_meters SET removed_at=sqlc.arg(removed_at)::timestamptz,removed_submicro=sqlc.arg(removed_submicro)::smallint,removal_operation_id=CAST(sqlc.arg(operation_id)::text AS uuid)
WHERE org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND asset_id=CAST(sqlc.arg(asset_id)::text AS uuid) AND id=CAST(sqlc.arg(id)::text AS uuid) AND removed_at IS NULL;

-- name: MeterBoundaryByOperation :one
-- EMP-METER-005 replay: the boundary an operation recorded, rebuilt from its
-- rows only; segments is the closed meter's count up to that boundary.
SELECT s.meter_id::text AS meter_id,s.id::text AS closed_segment_id,s.closing_kind::text AS closing_kind,s.closing_value,COALESCE(s.closing_reading_id::text,'')::text AS closing_reading_id,s.closed_at,s.closed_submicro,
 s.closing_previous_reading_id::text AS previous_reading_id,s.closing_implied_usage,COALESCE(m.id::text,'')::text AS new_meter_id,n.id::text AS opened_segment_id,n.opening_reading_id::text AS opening_reading_id,
 (CASE n.kind WHEN 'ROLLOVER' THEN 0 ELSE(SELECT r.value FROM org_meter_readings r WHERE(r.org_id,r.meter_id,r.id)=(n.org_id,n.meter_id,n.opening_reading_id)) END)::bigint AS opening_value,
 (SELECT count(*) FROM org_meter_segments c WHERE(c.org_id,c.meter_id)=(s.org_id,s.meter_id) AND(c.opened_at,c.opened_submicro)<=(s.closed_at,s.closed_submicro))::bigint AS segments
FROM org_meter_segments s LEFT JOIN org_meters m ON m.org_id=s.org_id AND m.created_operation_id=s.closing_operation_id AND s.closing_kind='REMOVAL'
JOIN org_meter_segments n ON n.org_id=s.org_id AND(n.meter_id=s.meter_id AND n.opened_operation_id=s.closing_operation_id OR n.meter_id=m.id AND n.kind='INSTALLATION')
WHERE s.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND s.closing_operation_id=CAST(sqlc.arg(operation_id)::text AS uuid);

-- name: LockAssetKindMeters :many
-- EMP-METER-006: share-lock the asset's meters of one kind (every reading
-- change locks its meter for update). A locking statement keeps the snapshot
-- it started with for joined rows, so every read follows in later statements.
SELECT m.id::text AS id FROM org_meters m
WHERE m.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND m.asset_id=CAST(sqlc.arg(asset_id)::text AS uuid) AND m.kind=sqlc.arg(kind)::text
ORDER BY m.installed_at,m.installed_submicro,m.id FOR SHARE;

-- name: AssetKindMeterRevisions :many
-- The projection revision of each of the asset's meters of one kind.
SELECT m.id::text AS id,COALESCE(c.revision,0)::bigint AS revision,(c.org_id IS NOT NULL)::boolean AS projected
FROM org_meters m LEFT JOIN org_meter_current_values c ON(c.org_id,c.meter_id)=(m.org_id,m.id)
WHERE m.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND m.asset_id=CAST(sqlc.arg(asset_id)::text AS uuid) AND m.kind=sqlc.arg(kind)::text
ORDER BY m.installed_at,m.installed_submicro,m.id;

-- name: AssetKindSegmentChain :many
SELECT s.id::text AS id,s.meter_id::text AS meter_id,s.kind,s.opened_at,s.opened_submicro,
 (CASE s.kind WHEN 'ROLLOVER' THEN 0 ELSE(SELECT r.value FROM org_meter_readings r WHERE(r.org_id,r.meter_id,r.id)=(s.org_id,s.meter_id,s.opening_reading_id)) END)::bigint AS open_value,
 s.closed_at,s.closed_submicro,s.closing_value
FROM org_meter_segments s JOIN org_meters m ON(m.org_id,m.id)=(s.org_id,s.meter_id)
WHERE m.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND m.asset_id=CAST(sqlc.arg(asset_id)::text AS uuid) AND m.kind=sqlc.arg(kind)::text
ORDER BY m.installed_at,m.installed_submicro,m.id,s.opened_at,s.opened_submicro;

-- name: AssetKindValidReadings :many
-- The readings currently VALID with the segment B-BR-EMP-481 derives.
SELECT r.id::text AS id,COALESCE((CASE WHEN r.origin='METER_REMOVAL' THEN r.segment_id ELSE(SELECT g.id FROM org_meter_segments g WHERE(g.org_id,g.meter_id)=(r.org_id,r.meter_id) AND(g.opened_at,g.opened_submicro)<=(r.occurred_at,r.occurred_submicro) AND(g.closed_at IS NULL OR(r.occurred_at,r.occurred_submicro)<(g.closed_at,g.closed_submicro)) ORDER BY g.opened_at DESC,g.opened_submicro DESC LIMIT 1) END),r.segment_id)::text AS segment_id,r.value,r.occurred_at,r.occurred_submicro,r.received_at
FROM org_meter_readings r JOIN org_meters m ON(m.org_id,m.id)=(r.org_id,r.meter_id)
WHERE m.org_id=CAST(sqlc.arg(org_id)::text AS uuid) AND m.asset_id=CAST(sqlc.arg(asset_id)::text AS uuid) AND m.kind=sqlc.arg(kind)::text
AND COALESCE((SELECT s.state FROM org_meter_reading_states s WHERE(s.org_id,s.meter_id,s.reading_id)=(r.org_id,r.meter_id,r.id) ORDER BY s.revision DESC LIMIT 1),r.state)='VALID'
ORDER BY r.occurred_at,r.occurred_submicro,r.received_at,r.id;
