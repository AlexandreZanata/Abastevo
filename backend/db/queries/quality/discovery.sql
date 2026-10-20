-- FQ-05W7 bounded live discovery. The cursor is the verified URL of the
-- archive inspected last; the next run inspects the following listed
-- archive, wrapping only after the whole listing was covered.

-- name: GetDiscoveryCursor :one
SELECT kind, cursor_url, updated_at
FROM quality_discovery_cursors WHERE kind = @kind;

-- name: SetDiscoveryCursor :exec
INSERT INTO quality_discovery_cursors (kind, cursor_url)
VALUES (@kind, @cursor_url)
ON CONFLICT (kind) DO UPDATE
 SET cursor_url = EXCLUDED.cursor_url, updated_at = now();
