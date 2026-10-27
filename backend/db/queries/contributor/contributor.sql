-- Owned by contributor. Public reads expose only consented display fields;
-- claim codes are one-time ownership proofs and are cleared on every terminal
-- state. Expiry is evaluated at read time.

-- name: ExpireClaims :exec
UPDATE contributor_claims
SET state = 'EXPIRED', decided_at = @now, code = ''
WHERE account_id = @account_id AND state = 'PENDING' AND expires_at <= @now;

-- name: CountClaimsSince :one
SELECT count(*)::int FROM contributor_claims
WHERE account_id = @account_id AND created_at > @since;

-- name: GetOpenClaim :one
SELECT * FROM contributor_claims
WHERE account_id = @account_id AND state = 'PENDING' AND expires_at > @now
FOR UPDATE;

-- name: GetClaimByOperation :one
SELECT * FROM contributor_claims
WHERE account_id = @account_id AND operation_id = @operation_id;

-- name: InsertClaim :one
INSERT INTO contributor_claims
    (id, account_id, operation_id, github_login, code, state, proof_status,
     created_at, expires_at)
VALUES (@id, @account_id, @operation_id, @github_login, @code, 'PENDING',
    'UNVERIFIED', @created_at, @expires_at)
RETURNING *;

-- name: ListOwnClaims :many
SELECT * FROM contributor_claims
WHERE account_id = @account_id
ORDER BY created_at DESC, id DESC
LIMIT 100;

-- name: LockClaim :one
SELECT * FROM contributor_claims WHERE id = @id FOR UPDATE;

-- name: SetClaimProof :one
UPDATE contributor_claims
SET proof_status = @proof_status, gist_id = @gist_id
WHERE id = @id AND account_id = @account_id AND state = 'PENDING'
  AND expires_at > @now
RETURNING *;

-- name: WithdrawClaim :one
UPDATE contributor_claims
SET state = 'WITHDRAWN', decided_at = @now, code = ''
WHERE id = @id AND account_id = @account_id AND state = 'PENDING'
RETURNING *;

-- name: DecideClaim :one
UPDATE contributor_claims
SET state = @state, decision_reason = @reason, override = @override,
    decided_at = @now, code = ''
WHERE id = @id AND state = 'PENDING' AND expires_at > @now
RETURNING *;

-- name: ListAdminClaims :many
SELECT * FROM contributor_claims
WHERE (sqlc.narg('state')::text IS NULL OR state = sqlc.narg('state')::text)
  AND (sqlc.narg('before_created')::timestamptz IS NULL OR
       (created_at, id) < (sqlc.narg('before_created')::timestamptz, sqlc.narg('before_id')::uuid))
ORDER BY created_at DESC, id DESC
LIMIT @row_limit;

-- name: ListClaimsByAccount :many
SELECT * FROM contributor_claims
WHERE account_id = @account_id
ORDER BY created_at DESC, id DESC
LIMIT 20;

-- name: UpsertProfile :one
INSERT INTO contributor_profiles
    (account_id, display_name, visibility, public_consent, show_roles_publicly)
VALUES (@account_id, @display_name, @visibility, @public_consent, @show_roles_publicly)
ON CONFLICT (account_id) DO UPDATE
SET display_name = EXCLUDED.display_name,
    visibility = EXCLUDED.visibility,
    public_consent = EXCLUDED.public_consent,
    show_roles_publicly = EXCLUDED.show_roles_publicly,
    updated_at = clock_timestamp()
RETURNING *;

-- name: GetProfile :one
SELECT * FROM contributor_profiles WHERE account_id = @account_id;

-- name: DeleteProfileLinks :exec
DELETE FROM contributor_links WHERE account_id = @account_id;

-- name: InsertProfileLink :exec
INSERT INTO contributor_links (account_id, platform, handle)
VALUES (@account_id, @platform, @handle);

-- name: ListProfileLinks :many
SELECT platform, handle FROM contributor_links
WHERE account_id = @account_id
ORDER BY created_at, platform;

-- name: CountProfileEditsSince :one
SELECT count(*)::int FROM contributor_profile_edits
WHERE account_id = @account_id AND edited_at > @since;

-- name: InsertProfileEdit :exec
INSERT INTO contributor_profile_edits (account_id, edited_at)
VALUES (@account_id, @edited_at);

-- name: SetProfileHidden :one
UPDATE contributor_profiles
SET hidden_by_admin = @hidden, hidden_reason = @reason, updated_at = clock_timestamp()
WHERE account_id = @account_id
RETURNING *;

-- name: GetActiveLinkByLogin :one
SELECT * FROM contributor_github_links
WHERE github_login = @github_login AND revoked_at IS NULL;

-- name: GetActiveLinkByAccount :one
SELECT * FROM contributor_github_links
WHERE account_id = @account_id AND revoked_at IS NULL;

-- name: InsertGitHubLink :one
INSERT INTO contributor_github_links (account_id, github_login, linked_at)
VALUES (@account_id, @github_login, @linked_at)
RETURNING *;

-- name: ListActiveGitHubLinks :many
SELECT account_id, github_login FROM contributor_github_links
WHERE revoked_at IS NULL
ORDER BY github_login;

-- name: ListOpenClaims :many
SELECT id, account_id, github_login, state, proof_status, created_at, expires_at
FROM contributor_claims
WHERE state = 'PENDING' AND expires_at > @now
ORDER BY created_at DESC, id DESC;

-- name: GetSnapshot :one
SELECT repo, logins, etag, fetched_at
FROM github_contributor_snapshots WHERE repo = @repo;

-- name: UpsertSnapshot :one
INSERT INTO github_contributor_snapshots (repo, logins, etag, fetched_at)
VALUES (@repo, @logins::jsonb, @etag, @fetched_at)
ON CONFLICT (repo) DO UPDATE
SET logins = EXCLUDED.logins, etag = EXCLUDED.etag, fetched_at = EXCLUDED.fetched_at
RETURNING repo, logins, etag, fetched_at;

-- name: PublicContributors :many
SELECT p.account_id, p.display_name,
    (COALESCE(first_grant.since, p.created_at))::timestamptz AS contributor_since,
    gl.github_login,
    COALESCE(links.links, '[]'::jsonb) AS links
FROM contributor_profiles p
JOIN accounts a ON a.id = p.account_id AND a.status = 'active'
JOIN LATERAL (
    SELECT min(g.start_at) AS since FROM entitlement_grants g
    WHERE g.account_id = p.account_id AND g.tier = 'CONTRIBUTOR'
) first_grant ON true
LEFT JOIN contributor_github_links gl ON gl.account_id = p.account_id AND gl.revoked_at IS NULL
LEFT JOIN LATERAL (
    SELECT jsonb_agg(jsonb_build_object('platform', platform, 'handle', handle)
        ORDER BY created_at, platform) AS links
    FROM contributor_links WHERE account_id = p.account_id
) links ON true
WHERE p.visibility = 'PUBLIC' AND p.public_consent AND NOT p.hidden_by_admin
  AND EXISTS (
      SELECT 1 FROM entitlement_grants g
      WHERE g.account_id = p.account_id AND g.tier = 'CONTRIBUTOR'
        AND g.state = 'ACTIVE' AND g.start_at <= @now
        AND (g.end_at IS NULL OR g.end_at > @now)
  )
  AND (sqlc.narg('after_since')::timestamptz IS NULL OR
       (COALESCE(first_grant.since, p.created_at), p.account_id) >
       (sqlc.narg('after_since')::timestamptz, sqlc.narg('after_id')::uuid))
ORDER BY COALESCE(first_grant.since, p.created_at) ASC, p.account_id ASC
LIMIT @row_limit;

-- name: ExpireAllClaims :exec
UPDATE contributor_claims
SET state = 'EXPIRED', decided_at = @now, code = ''
WHERE state = 'PENDING' AND expires_at <= @now;
