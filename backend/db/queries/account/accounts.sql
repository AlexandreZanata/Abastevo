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

-- SEC07-A bounded hashed rotation history (AUD-08): one row per
-- superseded refresh verifier, newest first, pruned on rotation.
-- Plaintext tokens never reach these statements.

-- name: InsertRefreshHistory :exec
INSERT INTO account_session_refresh_history
    (family_id, refresh_salt, refresh_hash, rotated_at)
VALUES (@family_id, @refresh_salt, @refresh_hash, @rotated_at)
ON CONFLICT (family_id, refresh_hash) DO NOTHING;

-- name: ListRefreshHistory :many
SELECT refresh_salt, refresh_hash
FROM account_session_refresh_history
WHERE family_id = @family_id
ORDER BY rotated_at DESC, refresh_hash
LIMIT @cap;

-- name: PruneRefreshHistory :execrows
DELETE FROM account_session_refresh_history h
WHERE h.family_id = @family_id AND h.refresh_hash NOT IN (
    SELECT refresh_hash FROM account_session_refresh_history
    WHERE family_id = @family_id
    ORDER BY rotated_at DESC, refresh_hash
    LIMIT @keep
);

-- Provider links and OIDC nonces (P13-T03B). Only salt-free opaque
-- subjects persist; email is display/relay only and never a merge key.

-- name: GetAccountByID :one
SELECT id, alias, status, created_at
FROM accounts
WHERE id = @id;

-- FQ-09: protect signed private commands against concurrent deletion/unbinding.
-- name: LockQualityAccount :one
SELECT id, alias, status, created_at FROM accounts WHERE id = @id FOR SHARE;

-- name: LockQualityBinding :one
SELECT account_id, contributor_id, key_fingerprint, bound_at
FROM account_contributor_bindings
WHERE account_id = @account_id AND contributor_id = @contributor_id FOR SHARE;

-- Account status lifecycle (P13-T04A). Suspension and deletion revoke
-- sessions via RevokeAccountFamilies in the same transaction; deletion
-- additionally drops address and provider bindings while the account
-- row stays as an audit record.

-- name: SetAccountStatus :exec
UPDATE accounts SET status = @status WHERE id = @id;

-- name: DeleteAccountAddresses :exec
DELETE FROM account_addresses WHERE account_id = @account_id;

-- name: DeleteAccountProviders :exec
DELETE FROM account_provider_links WHERE account_id = @account_id;

-- SEC04-C credential minimization: key verifiers never survive the
-- owner. The account audit row stays; the secrets that authenticate
-- as it go with deletion.

-- name: DeleteAccountKeys :exec
DELETE FROM account_key_credentials WHERE account_id = @account_id;

-- Contributor bindings and link audit (P13-T04B). One contributor binds
-- at most one account; the application refuses stolen-ID binds and the
-- UNIQUE guard closes the concurrent race.

-- name: GetBinding :one
SELECT account_id, contributor_id, key_fingerprint, bound_at
FROM account_contributor_bindings
WHERE account_id = @account_id AND contributor_id = @contributor_id;

-- name: ListBindings :many
SELECT account_id, contributor_id, key_fingerprint, bound_at
FROM account_contributor_bindings
WHERE account_id = @account_id
ORDER BY contributor_id;

-- name: FindBindingOwner :one
SELECT account_id, contributor_id, key_fingerprint, bound_at
FROM account_contributor_bindings
WHERE contributor_id = @contributor_id;

-- name: InsertBinding :exec
INSERT INTO account_contributor_bindings
    (account_id, contributor_id, key_fingerprint, bound_at)
VALUES (@account_id, @contributor_id, @key_fingerprint, @bound_at);

-- name: UpdateBinding :exec
UPDATE account_contributor_bindings
SET key_fingerprint = @key_fingerprint, bound_at = @bound_at
WHERE account_id = @account_id AND contributor_id = @contributor_id;

-- name: DeleteBinding :exec
DELETE FROM account_contributor_bindings
WHERE account_id = @account_id AND contributor_id = @contributor_id;

-- name: DeleteAccountBindings :exec
DELETE FROM account_contributor_bindings WHERE account_id = @account_id;

-- name: InsertBindingAudit :exec
INSERT INTO account_binding_audit
    (id, account_id, contributor_id, action, occurred_at)
VALUES (@id, @account_id, @contributor_id, @action, @occurred_at);

-- name: ListBindingAudit :many
SELECT id, account_id, contributor_id, action, occurred_at
FROM account_binding_audit
WHERE account_id = @account_id
ORDER BY occurred_at, id;

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

-- Key-account credentials: unique username and key lookups; the salted
-- verifier authenticates after the lookup finds the candidate row.

-- name: InsertKeyCredential :exec
INSERT INTO account_key_credentials
    (account_id, username_hash, key_lookup, key_salt, key_hash, created_at)
VALUES (@account_id, @username_hash, @key_lookup, @key_salt, @key_hash, @created_at);

-- name: FindKeyCredentialByUsername :one
SELECT account_id, username_hash, key_lookup, key_salt, key_hash, created_at
FROM account_key_credentials
WHERE username_hash = @username_hash;

-- name: FindKeyCredentialByLookup :one
SELECT account_id, username_hash, key_lookup, key_salt, key_hash, created_at
FROM account_key_credentials
WHERE key_lookup = @key_lookup;

-- SEC01-C admission accounting (B-BR-A01/A02/A04/A05). Counts only;
-- no raw addresses/usernames travel, only hashes and totals.

-- name: CountAllCodesSince :one
SELECT count(*)::bigint
FROM account_email_codes
WHERE issued_at >= @since;

-- name: CountKeyAccountsSince :one
SELECT count(*)::bigint
FROM account_key_admission_events
WHERE admitted_at >= @since_unix;

-- name: RecordKeyAccountAdmission :exec
INSERT INTO account_key_admission_events(admitted_at) VALUES (@admitted_at);

-- name: PruneKeyAccountAdmissions :execrows
WITH expired AS (
 SELECT adm.id FROM account_key_admission_events adm WHERE adm.admitted_at < @before_unix
 ORDER BY adm.admitted_at, adm.id LIMIT 1000 FOR UPDATE SKIP LOCKED
)
DELETE FROM account_key_admission_events e USING expired WHERE e.id = expired.id;

-- name: CountLiveFamiliesByAccount :one
SELECT count(*)::bigint
FROM account_session_families
WHERE account_id = @account_id
  AND revoked_at IS NULL
  AND issued_at >= @live_since;

-- SEC01-C bounded retention (FREE_ACCOUNT_ACCESS inventory): purge
-- code verifiers and absolutely-expired families past the caller's
-- cutoff. Callers pass now-(TTL+grace); audit inside the window stays.

-- name: PurgeExpiredCodes :execrows
DELETE FROM account_email_codes
WHERE issued_at < @before;

-- name: PurgeExpiredFamilies :execrows
DELETE FROM account_session_families
WHERE issued_at < @before;

-- SEC01-D quota rollback: compensate one issued code when downstream
-- delivery fails, so failed sends consume no budget rows.

-- name: DeleteCodeByID :execrows
DELETE FROM account_email_codes
WHERE id = @id;

-- SEC04-B durable erasure intents: recorded in the DeleteAccount
-- transaction before bindings drop, so revocation and intent are
-- atomic. Replay converges on the natural key.

-- name: InsertErasureIntent :execrows
INSERT INTO account_erasure_intents (account_id, contributor_id, created_at)
VALUES (@account_id, @contributor_id, @created_at)
ON CONFLICT (account_id, contributor_id) DO NOTHING;

-- name: ListErasureIntentsByAccount :many
SELECT account_id, contributor_id, created_at
FROM account_erasure_intents
WHERE account_id = @account_id
ORDER BY contributor_id;

-- name: ActiveQualityOwners :many
SELECT id FROM accounts WHERE id=ANY(@ids::uuid[]) AND status='active';
