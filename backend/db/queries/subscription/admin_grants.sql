-- Admin-granted tiers join the Premium projection as a local, independent
-- source. Capabilities are derived from the code catalog; expiry is read-time.

-- name: AdminGrants :many
SELECT tier, end_at FROM entitlement_grants
WHERE account_id = sqlc.arg('account_id')::uuid
  AND state = 'ACTIVE' AND policy_version = 1
  AND start_at <= sqlc.arg('now')::timestamptz
  AND (end_at IS NULL OR end_at > sqlc.arg('now')::timestamptz)
ORDER BY end_at NULLS LAST, tier;
