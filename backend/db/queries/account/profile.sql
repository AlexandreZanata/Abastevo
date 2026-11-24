-- Account profile and recovery. Recovery-address plaintext and challenge
-- secrets do not reach these statements.

-- name: InsertNamedAccount :exec
INSERT INTO accounts (id, alias, display_name, status, created_at)
VALUES (@id, @alias, @display_name, @status, @created_at);

-- name: GetNamedAccount :one
SELECT id, alias, status, created_at, display_name
FROM accounts WHERE id = @id;

-- name: LockNamedAccount :one
SELECT id, alias, status, created_at, display_name
FROM accounts WHERE id = @id FOR UPDATE;

-- name: UpdateDisplayName :exec
UPDATE accounts SET display_name = @display_name WHERE id = @id;

-- name: InsertRecoveryAddress :execrows
INSERT INTO account_addresses (address_hash, account_id, linked_at, email_hint)
VALUES (@address_hash, @account_id, @linked_at, @email_hint)
ON CONFLICT (address_hash) DO NOTHING;

-- name: ReadRecoveryAddress :one
SELECT address_hash, email_hint FROM account_addresses
WHERE account_id = @account_id
ORDER BY linked_at DESC, address_hash
LIMIT 1;

-- name: FindKeyCredentialByAccount :one
SELECT account_id, username_hash, key_lookup, key_salt, key_hash, created_at
FROM account_key_credentials WHERE account_id = @account_id;

-- name: HasAccountKey :one
SELECT EXISTS (SELECT 1 FROM account_key_credentials WHERE account_id = @account_id);

-- name: ReplaceKeyCredential :exec
INSERT INTO account_key_credentials (
    account_id, username_hash, key_lookup, key_salt, key_hash, created_at)
VALUES (@account_id, @username_hash, @key_lookup, @key_salt, @key_hash, @created_at)
ON CONFLICT (account_id) DO UPDATE SET
    key_lookup = EXCLUDED.key_lookup,
    key_salt = EXCLUDED.key_salt,
    key_hash = EXCLUDED.key_hash,
    created_at = EXCLUDED.created_at;

-- name: InsertRecoveryAudit :exec
INSERT INTO account_recovery_audit (id, account_id, action, occurred_at)
VALUES (@id, @account_id, @action, @occurred_at);

-- name: PurgeRecoveryAudit :execrows
DELETE FROM account_recovery_audit WHERE id IN (
    SELECT pending.id FROM account_recovery_audit pending WHERE pending.occurred_at <= @cutoff
    ORDER BY pending.occurred_at, pending.id LIMIT @batch_size
);

-- name: OldestRecoveryAudit :one
SELECT occurred_at FROM account_recovery_audit
WHERE occurred_at <= @cutoff ORDER BY occurred_at, id LIMIT 1;
