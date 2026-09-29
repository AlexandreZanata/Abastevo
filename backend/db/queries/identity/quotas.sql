-- Owned by identity. Quota checks share the business transaction's pool
-- but never its rows: a denied check runs before expensive work starts.

-- name: ConsumeQuota :one
INSERT INTO identity_rate_windows
    (subject_digest, operation, window_start, count, expires_at)
VALUES (@subject_digest, @operation, @window_start, 1, @expires_at)
ON CONFLICT (subject_digest, operation, window_start)
DO UPDATE SET count = identity_rate_windows.count + 1
WHERE identity_rate_windows.count < @cap
RETURNING count, expires_at;

-- name: CleanupExpiredWindows :execrows
DELETE FROM identity_rate_windows WHERE expires_at < now();
