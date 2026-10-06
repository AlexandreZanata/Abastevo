-- Owned by stationprofile (claims, P30-T03). Account-bound private
-- requests with server-bound declarations. Competing claims stay
-- private rows; idempotency converges per owner key; only one active
-- declaration per claim (reissue supersedes in one guarded update).

-- name: CreateClaim :one
INSERT INTO profile_claims (id, account_id, station_id, operator_cnpj, operator_source, role, scopes, policy_version, state, client_key)
VALUES (@id, @account_id, @station_id, @operator_cnpj, @operator_source, @role, @scopes, @policy_version, 'draft', @client_key)
ON CONFLICT (account_id, client_key) WHERE client_key <> '' DO NOTHING
RETURNING id, account_id, station_id, operator_cnpj, operator_source, role, scopes, policy_version, state, client_key, created_at, updated_at;

-- name: GetClaim :one
SELECT id, account_id, station_id, operator_cnpj, operator_source, role, scopes, policy_version, state, client_key, created_at, updated_at
FROM profile_claims
WHERE id = @id;

-- name: GetClaimByKey :one
SELECT id, account_id, station_id, operator_cnpj, operator_source, role, scopes, policy_version, state, client_key, created_at, updated_at
FROM profile_claims
WHERE account_id = @account_id AND client_key = @client_key AND client_key <> '';

-- name: ListOwnedClaims :many
SELECT id, account_id, station_id, operator_cnpj, operator_source, role, scopes, policy_version, state, client_key, created_at, updated_at
FROM profile_claims
WHERE account_id = @account_id
ORDER BY created_at DESC
LIMIT @page_limit::int OFFSET @page_offset::int;

-- name: CountOpenClaims :one
SELECT count(*) FROM profile_claims
WHERE account_id = @account_id AND state NOT IN ('approved', 'denied', 'cancelled', 'expired');

-- name: SetClaimState :execrows
UPDATE profile_claims
SET state = @state, updated_at = now()
WHERE id = @id AND state = @expected_state;

-- name: SetClaimReviewState :execrows
UPDATE profile_claims
SET state = @state, updated_at = now()
WHERE id = @id AND state IN ('draft', 'awaiting_proof', 'checking', 'needs_information', 'in_review');

-- name: CreateDeclaration :one
INSERT INTO claim_declarations (id, claim_id, version, nonce_digest, expected_digest, declaration, expires_at)
VALUES (@id, @claim_id, @version, @nonce_digest, @expected_digest, @declaration, @expires_at)
RETURNING id, claim_id, version, nonce_digest, expected_digest, declaration, state, attempts, expires_at, consumed_at, created_at;

-- name: SupersedeDeclarations :execrows
UPDATE claim_declarations
SET state = 'superseded'
WHERE claim_id = @claim_id AND state = 'active';

-- name: ActiveDeclaration :one
SELECT id, claim_id, version, nonce_digest, expected_digest, declaration, state, attempts, expires_at, consumed_at, created_at
FROM claim_declarations
WHERE claim_id = @claim_id AND state = 'active'
ORDER BY version DESC
LIMIT 1;

-- name: LatestDeclaration :one
SELECT id, claim_id, version, nonce_digest, expected_digest, declaration, state, attempts, expires_at, consumed_at, created_at
FROM claim_declarations
WHERE claim_id = @claim_id
ORDER BY version DESC
LIMIT 1;

-- name: GetDeclaration :one
SELECT id, claim_id, version, nonce_digest, expected_digest, declaration, state, attempts, expires_at, consumed_at, created_at
FROM claim_declarations
WHERE id = @id;

-- name: BumpDeclarationAttempts :one
UPDATE claim_declarations
SET attempts = attempts + 1
WHERE id = @id AND state = 'active'
RETURNING id, attempts;

-- name: ConsumeDeclaration :execrows
UPDATE claim_declarations
SET state = 'consumed', consumed_at = now()
WHERE id = @id AND state = 'active';

-- name: ExpireDeclaration :execrows
UPDATE claim_declarations
SET state = 'expired'
WHERE id = @id AND state = 'active';
