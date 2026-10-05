-- Owned by stationprofile (proof intake, P30-T04). Hash-bound
-- immutable metadata; bytes live in private object storage behind
-- server-generated keys. One proof binds per declaration; expiry and
-- deletion purge bytes while rows stay as audit.

-- name: CreateProof :one
INSERT INTO claim_proofs (id, claim_id, declaration_id, sha256, bytes_size, format, evidence_kind, object_key, expires_at)
VALUES (@id, @claim_id, @declaration_id, @sha256, @bytes_size, @format, @evidence_kind, @object_key, @expires_at)
ON CONFLICT (declaration_id) WHERE status NOT IN ('rejected', 'deleted') DO NOTHING
RETURNING id, claim_id, declaration_id, sha256, bytes_size, format, evidence_kind, object_key, status, expires_at, created_at;

-- name: GetProofByHash :one
SELECT id, claim_id, declaration_id, sha256, bytes_size, format, evidence_kind, object_key, status, expires_at, created_at
FROM claim_proofs
WHERE claim_id = @claim_id AND sha256 = @sha256 AND status NOT IN ('rejected', 'deleted');

-- name: ListExpiredProofs :many
SELECT id, claim_id, declaration_id, sha256, bytes_size, format, evidence_kind, object_key, status, expires_at, created_at
FROM claim_proofs
WHERE status NOT IN ('expired', 'deleted') AND expires_at <= now()
ORDER BY expires_at ASC
LIMIT @page_limit::int;

-- name: MarkProofExpired :execrows
UPDATE claim_proofs
SET status = 'expired'
WHERE id = @id AND status NOT IN ('expired', 'deleted');

-- name: MarkProofDeleted :execrows
UPDATE claim_proofs
SET status = 'deleted'
WHERE id = @id AND status NOT IN ('deleted');

-- name: GetProof :one
SELECT id, claim_id, declaration_id, sha256, bytes_size, format, evidence_kind, object_key, status, expires_at, created_at
FROM claim_proofs
WHERE id = @id;

-- name: SetProofStatus :execrows
UPDATE claim_proofs
SET status = @status
WHERE id = @id AND status = 'received';
