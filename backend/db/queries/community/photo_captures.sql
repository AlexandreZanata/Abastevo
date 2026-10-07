-- name: InsertPhotoCapture :one
INSERT INTO community_photo_captures
(id, contributor_ref, key_id, client_capture_id, station_id, issued_at, camera_expires_at, expires_at, policy_version)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
ON CONFLICT (contributor_ref, client_capture_id) DO UPDATE SET client_capture_id = EXCLUDED.client_capture_id
WHERE community_photo_captures.key_id = EXCLUDED.key_id AND community_photo_captures.station_id = EXCLUDED.station_id
RETURNING *;

-- name: OwnedPhotoCapture :one
SELECT * FROM community_photo_captures WHERE id = $1 AND contributor_ref = $2;

-- name: BindPhotoCapture :execrows
UPDATE community_photo_captures SET evidence_session_id = sqlc.arg(session_id), captured_at = sqlc.arg(captured_at)
WHERE id = sqlc.arg(id) AND contributor_ref = sqlc.arg(contributor_ref) AND key_id = sqlc.arg(key_id)
AND expires_at > sqlc.arg(now_at)
AND sqlc.arg(captured_at) >= issued_at AND sqlc.arg(captured_at) < camera_expires_at
AND (evidence_session_id IS NULL OR (evidence_session_id = sqlc.arg(session_id) AND captured_at = sqlc.arg(captured_at)));

-- name: PurgePhotoCaptures :execrows
DELETE FROM community_photo_captures WHERE expires_at <= $1;
