-- EMP-NOTIF-001 own tables; explicit membership reads bind current recipients.
-- name: CurrentScope :one
SELECT COALESCE(current_setting('anpfuel.org_id',true),'')::text AS org_id;

-- name: InsertRequest :one
INSERT INTO org_notification_requests(org_id,id,idempotency_key,source_event_id,source_event_name,source_target_type,
 source_target_id,kind,template,params,link_path,content_hash,recipient_count)
VALUES(CAST(@org_id::text AS uuid),CAST(@id::text AS uuid),@idempotency_key,CAST(@source_event_id::text AS uuid),
 @source_event_name,@source_target_type,CAST(@source_target_id::text AS uuid),@kind,@template,@params::jsonb,
 NULLIF(@link_path::text,''),@content_hash,@recipient_count)
ON CONFLICT(org_id,idempotency_key) DO NOTHING
RETURNING id::text;

-- name: RequestByKey :one
SELECT id::text,content_hash FROM org_notification_requests
WHERE org_id=CAST(@org_id::text AS uuid) AND idempotency_key= @idempotency_key;

-- name: InsertRecipient :exec
INSERT INTO org_notification_recipients(org_id,request_id,membership_id)
VALUES(CAST(@org_id::text AS uuid),CAST(@request_id::text AS uuid),CAST(@membership_id::text AS uuid));

-- name: ClaimRequests :many
UPDATE org_notification_requests SET dispatched_at=clock_timestamp()
WHERE org_id=CAST(@org_id::text AS uuid) AND source_event_id=CAST(@source_event_id::text AS uuid) AND dispatched_at IS NULL
RETURNING id::text,source_event_name,source_target_type,source_target_id::text,kind;

-- name: RequestRecipients :many
SELECT membership_id::text FROM org_notification_recipients
WHERE org_id=CAST(@org_id::text AS uuid) AND request_id=CAST(@request_id::text AS uuid)
ORDER BY membership_id;

-- name: DatabaseNow :one
SELECT clock_timestamp()::timestamptz AS now;

-- name: MembershipAccount :one
SELECT COALESCE(account_id::text,'')::text AS account_id FROM org_memberships
WHERE org_id=CAST(@org_id::text AS uuid) AND id=CAST(@membership_id::text AS uuid) AND state<>'ENDED';

-- name: Muted :one
SELECT EXISTS(SELECT 1 FROM org_notification_preferences
 WHERE org_id=CAST(@org_id::text AS uuid) AND membership_id=CAST(@membership_id::text AS uuid) AND kind= @kind AND muted) AS muted;

-- name: ConsumeBudget :one
INSERT INTO org_notification_budgets AS b(org_id,membership_id,kind,window_start,used)
VALUES(CAST(@org_id::text AS uuid),CAST(@membership_id::text AS uuid),@kind,@window_start,1)
ON CONFLICT(org_id,membership_id,kind,window_start) DO UPDATE SET used=b.used+1
WHERE b.used< @hourly_limit::integer
RETURNING used;

-- name: InsertNotice :exec
INSERT INTO org_notification_notices(org_id,id,request_id,membership_id)
VALUES(CAST(@org_id::text AS uuid),CAST(@id::text AS uuid),CAST(@request_id::text AS uuid),CAST(@membership_id::text AS uuid));

-- name: InsertDelivery :exec
INSERT INTO org_notification_deliveries(org_id,request_id,membership_id,channel,outcome,reason,notice_id)
VALUES(CAST(@org_id::text AS uuid),CAST(@request_id::text AS uuid),CAST(@membership_id::text AS uuid),'IN_APP',@outcome,
 NULLIF(@reason::text,''),CAST(NULLIF(@notice_id::text,'') AS uuid));

-- name: ListInbox :many
SELECT n.id::text AS id,n.position,n.created_at,n.read_at,r.kind,r.template,r.params,COALESCE(r.link_path,'')::text AS link_path
FROM org_notification_notices n JOIN org_notification_requests r ON r.org_id=n.org_id AND r.id=n.request_id
WHERE n.org_id=CAST(@org_id::text AS uuid) AND n.membership_id=CAST(@membership_id::text AS uuid)
 AND (NOT @unread_only::boolean OR n.read_at IS NULL)
 AND (@after_position::bigint=0 OR n.position< @after_position::bigint)
ORDER BY n.position DESC
LIMIT @row_limit;

-- name: GetNotice :one
SELECT n.id::text AS id,n.position,n.created_at,n.read_at,r.kind,r.template,r.params,COALESCE(r.link_path,'')::text AS link_path
FROM org_notification_notices n JOIN org_notification_requests r ON r.org_id=n.org_id AND r.id=n.request_id
WHERE n.org_id=CAST(@org_id::text AS uuid) AND n.id=CAST(@id::text AS uuid) AND n.membership_id=CAST(@membership_id::text AS uuid);

-- name: MarkNoticeRead :exec
UPDATE org_notification_notices SET read_at=clock_timestamp()
WHERE org_id=CAST(@org_id::text AS uuid) AND id=CAST(@id::text AS uuid) AND membership_id=CAST(@membership_id::text AS uuid)
 AND read_at IS NULL;

-- name: ReserveCommand :one
INSERT INTO org_notification_commands(org_id,operation_id,membership_id,action,target,request_hash)
VALUES(CAST(@org_id::text AS uuid),CAST(@operation_id::text AS uuid),CAST(@membership_id::text AS uuid),@action,@target,@request_hash)
ON CONFLICT(org_id,operation_id) DO NOTHING
RETURNING operation_id::text;

-- name: GetCommand :one
SELECT membership_id::text AS membership_id,action,target,request_hash FROM org_notification_commands
WHERE org_id=CAST(@org_id::text AS uuid) AND operation_id=CAST(@operation_id::text AS uuid);

-- name: ListPreferences :many
SELECT kind,muted,revision FROM org_notification_preferences
WHERE org_id=CAST(@org_id::text AS uuid) AND membership_id=CAST(@membership_id::text AS uuid)
ORDER BY kind;

-- name: LockPreference :one
SELECT muted,revision FROM org_notification_preferences
WHERE org_id=CAST(@org_id::text AS uuid) AND membership_id=CAST(@membership_id::text AS uuid) AND kind= @kind
FOR UPDATE;

-- name: InsertPreference :one
INSERT INTO org_notification_preferences(org_id,membership_id,kind,muted,revision)
VALUES(CAST(@org_id::text AS uuid),CAST(@membership_id::text AS uuid),@kind,@muted,1)
ON CONFLICT(org_id,membership_id,kind) DO NOTHING
RETURNING revision;

-- name: UpdatePreference :one
UPDATE org_notification_preferences SET muted= @muted,revision=revision+1,updated_at=clock_timestamp()
WHERE org_id=CAST(@org_id::text AS uuid) AND membership_id=CAST(@membership_id::text AS uuid) AND kind= @kind AND revision= @revision
RETURNING revision;
