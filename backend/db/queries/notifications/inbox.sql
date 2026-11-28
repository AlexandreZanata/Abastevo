-- Owned by notifications (B-BR-NOTIF-02/03). Every read and write is
-- scoped by the authenticated account; the only foreign relation is the
-- recipient's accounts row, read to skip accounts that are not active.

-- Producers call this inside the transaction of the event they describe.
-- name: InsertNotification :execrows
INSERT INTO account_notifications (id, account_id, event_key, kind, payload, created_at, expires_at)
SELECT sqlc.arg(id)::uuid, a.id, sqlc.arg(event_key)::text, sqlc.arg(kind)::text,
    sqlc.arg(payload)::jsonb, sqlc.arg(created_at)::timestamptz, sqlc.arg(expires_at)::timestamptz
FROM accounts a
WHERE a.id = sqlc.arg(account_id)::uuid AND a.status = 'active'
ON CONFLICT (account_id, event_key) DO NOTHING;

-- Bounded oldest-first purge; concurrent writers skip each other's rows.
-- name: PurgeExpiredNotifications :execrows
DELETE FROM account_notifications
WHERE id IN (
    SELECT n.id FROM account_notifications n
    WHERE n.expires_at <= sqlc.arg(now)::timestamptz
    ORDER BY n.expires_at
    LIMIT sqlc.arg(batch)::int
    FOR UPDATE SKIP LOCKED
);

-- name: ListNotifications :many
SELECT id, kind, payload, created_at, read_at
FROM account_notifications
WHERE account_id = sqlc.arg(account_id)::uuid
    AND expires_at > sqlc.arg(now)::timestamptz
    AND (NOT sqlc.arg(has_cursor)::boolean
        OR (created_at, id) < (sqlc.arg(before_created_at)::timestamptz, sqlc.arg(before_id)::uuid))
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit)::int;

-- name: CountUnreadNotifications :one
SELECT count(*)::bigint
FROM account_notifications
WHERE account_id = sqlc.arg(account_id)::uuid
    AND read_at IS NULL
    AND expires_at > sqlc.arg(now)::timestamptz;

-- Returns how many of the caller's live notifications the selection
-- covers, all of them read afterwards, so a replay answers the same.
-- name: MarkNotificationsRead :one
WITH target AS (
    SELECT t.id FROM account_notifications t
    WHERE t.account_id = sqlc.arg(account_id)::uuid
        AND t.id = ANY(sqlc.arg(ids)::uuid[])
        AND t.expires_at > sqlc.arg(now)::timestamptz
), marked AS (
    UPDATE account_notifications m SET read_at = sqlc.arg(now)::timestamptz
    WHERE m.account_id = sqlc.arg(account_id)::uuid
        AND m.id IN (SELECT id FROM target)
        AND m.read_at IS NULL
    RETURNING m.id
)
SELECT count(*)::bigint FROM target;

-- name: MarkAllNotificationsRead :one
WITH target AS (
    SELECT t.id FROM account_notifications t
    WHERE t.account_id = sqlc.arg(account_id)::uuid
        AND t.expires_at > sqlc.arg(now)::timestamptz
), marked AS (
    UPDATE account_notifications m SET read_at = sqlc.arg(now)::timestamptz
    WHERE m.account_id = sqlc.arg(account_id)::uuid
        AND m.id IN (SELECT id FROM target)
        AND m.read_at IS NULL
    RETURNING m.id
)
SELECT count(*)::bigint FROM target;

-- Account erasure removes the inbox in bounded batches.
-- name: EraseAccountNotifications :execrows
DELETE FROM account_notifications
WHERE id IN (
    SELECT e.id FROM account_notifications e
    WHERE e.account_id = sqlc.arg(account_id)::uuid
    LIMIT sqlc.arg(batch)::int
);
