-- name: LockSuspensionRoot :one
SELECT org_id::text AS org_id FROM organizations WHERE org_id=CAST(@org_id::text AS uuid) AND has_snapshot FOR UPDATE;

-- name: OpenSuspensionPeriod :one
INSERT INTO org_suspension_periods(org_id,id,reason,started_revision,started_session_id)
VALUES(CAST(@org_id::text AS uuid),CAST(@id::text AS uuid),@reason::text,@revision::bigint,CAST(@session_id::text AS uuid)) RETURNING started_at;

-- name: ResolveSuspensionPeriod :one
UPDATE org_suspension_periods SET ended_revision= @revision::bigint,ended_session_id=CAST(@session_id::text AS uuid)
WHERE org_id=CAST(@org_id::text AS uuid) AND ended_at IS NULL AND reason= @reason::text
RETURNING id::text AS id,ended_at;

-- name: InsertSuspensionNotice :exec
INSERT INTO org_suspension_notices(org_id,root_revision,period_id,kind) VALUES(CAST(@org_id::text AS uuid),@revision::bigint,CAST(@period_id::text AS uuid),@kind::text);

-- name: SuspensionIntakeClock :one
SELECT clock_timestamp()::timestamptz AS received_at;

-- name: SuspensionIntakePeriods :many
SELECT started_at,ended_at FROM org_suspension_periods
WHERE org_id=CAST(@org_id::text AS uuid) AND(ended_at IS NULL OR ended_at> @earliest::timestamptz)
ORDER BY started_at,id LIMIT 101;

-- name: GetOpenSuspensionPeriod :one
SELECT id::text AS id,reason FROM org_suspension_periods WHERE org_id=CAST(@org_id::text AS uuid) AND ended_at IS NULL;
