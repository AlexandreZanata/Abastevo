-- Owned by account. Code verifiers and session families for FREE email
-- signup/login (P13-T02B). Only salted hashes persist; plaintext codes and
-- tokens never reach these statements.

-- name: InsertAccount :exec
INSERT INTO accounts (id, alias, status, created_at)
VALUES (@id, @alias, @status, @created_at);

-- name: InsertAddress :exec
INSERT INTO account_addresses (address_hash, account_id, linked_at)
VALUES (@address_hash, @account_id, @linked_at)
ON CONFLICT (address_hash) DO NOTHING;

-- name: FindAccountByAddress :one
SELECT a.id, a.alias, a.status, a.created_at
FROM accounts a
JOIN account_addresses l ON l.account_id = a.id
WHERE l.address_hash = @address_hash;

-- name: InsertCode :exec
INSERT INTO account_email_codes
    (id, address_hash, salt, hash, issued_at, attempts, consumed_at)
VALUES (@id, @address_hash, @salt, @hash, @issued_at, 0, NULL);

-- name: CountCodesSince :one
SELECT count(*)::bigint
FROM account_email_codes
WHERE address_hash = @address_hash AND issued_at >= @since;

-- name: LatestCode :one
SELECT id, address_hash, salt, hash, issued_at, attempts, consumed_at
FROM account_email_codes
WHERE address_hash = @address_hash
ORDER BY issued_at DESC
LIMIT 1;

-- name: LockLatestCode :one
SELECT id, address_hash, salt, hash, issued_at, attempts, consumed_at
FROM account_email_codes
WHERE address_hash = @address_hash
ORDER BY issued_at DESC
LIMIT 1
FOR UPDATE;

-- Consume lanes lock every code row of the address newest-first; the Go
-- side matches by hash (stable id tiebreak), so same-second issuance never
-- misattributes a guess to the wrong row.

-- name: LockAddressCodes :many
SELECT id, address_hash, salt, hash, issued_at, attempts, consumed_at
FROM account_email_codes
WHERE address_hash = @address_hash
ORDER BY issued_at DESC, id DESC
FOR UPDATE;

-- name: BumpCodeAttempts :exec
UPDATE account_email_codes SET attempts = attempts + 1 WHERE id = @id;

-- name: ConsumeCode :exec
UPDATE account_email_codes SET consumed_at = @now WHERE id = @id;

-- name: InsertFamily :exec
INSERT INTO account_session_families
    (id, account_id, refresh_salt, refresh_hash, access_salt, access_hash,
     access_expires, issued_at, revoked_at)
VALUES (@id, @account_id, @refresh_salt, @refresh_hash, @access_salt,
    @access_hash, @access_expires, @issued_at, NULL);

-- name: GetFamily :one
SELECT id, account_id, refresh_salt, refresh_hash, access_salt,
    access_hash, access_expires, issued_at, revoked_at
FROM account_session_families
WHERE id = @id;

-- name: LockFamily :one
SELECT id, account_id, refresh_salt, refresh_hash, access_salt,
    access_hash, access_expires, issued_at, revoked_at
FROM account_session_families
WHERE id = @id
FOR UPDATE;

-- name: RotateFamily :exec
UPDATE account_session_families
SET refresh_salt = @refresh_salt, refresh_hash = @refresh_hash,
    access_salt = @access_salt, access_hash = @access_hash,
    access_expires = @access_expires
WHERE id = @id;

-- name: RevokeFamily :exec
UPDATE account_session_families SET revoked_at = @now WHERE id = @id;

-- name: RevokeAccountFamilies :exec
UPDATE account_session_families SET revoked_at = @now
WHERE account_id = @account_id AND revoked_at IS NULL;

-- Provider links and OIDC nonces (P13-T03B). Only salt-free opaque
-- subjects persist; email is display/relay only and never a merge key.

-- name: GetAccountByID :one
SELECT id, alias, status, created_at
FROM accounts
WHERE id = @id;

-- name: FindProviderOwner :one
SELECT account_id, provider, issuer, subject, email, linked_at
FROM account_provider_links
WHERE provider = @provider AND subject = @subject;

-- name: GetProviderLink :one
SELECT account_id, provider, issuer, subject, email, linked_at
FROM account_provider_links
WHERE account_id = @account_id AND provider = @provider;

-- name: ListProviderLinks :many
SELECT account_id, provider, issuer, subject, email, linked_at
FROM account_provider_links
WHERE account_id = @account_id
ORDER BY provider;

-- name: CountAddressesByAccount :one
SELECT count(*)::bigint
FROM account_addresses
WHERE account_id = @account_id;

-- name: InsertProviderLink :exec
INSERT INTO account_provider_links
    (account_id, provider, issuer, subject, email, linked_at)
VALUES (@account_id, @provider, @issuer, @subject, @email, @linked_at);

-- name: UpdateProviderLink :exec
UPDATE account_provider_links
SET issuer = @issuer, subject = @subject, email = @email,
    linked_at = @linked_at
WHERE account_id = @account_id AND provider = @provider;

-- name: DeleteProviderLink :exec
DELETE FROM account_provider_links
WHERE account_id = @account_id AND provider = @provider;

-- name: InsertNonce :one
INSERT INTO account_oidc_nonces (nonce, consumed_at)
VALUES (@nonce, @consumed_at)
ON CONFLICT (nonce) DO NOTHING
RETURNING nonce;
