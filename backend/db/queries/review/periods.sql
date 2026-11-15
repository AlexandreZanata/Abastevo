-- name: PeriodRootLock :one
SELECT state FROM organizations WHERE org_id= CAST(sqlc.arg(org_id)::text AS UUID) AND has_snapshot FOR UPDATE;

-- name: PeriodRootShare :one
SELECT state FROM organizations WHERE org_id= CAST(sqlc.arg(org_id)::text AS UUID) AND has_snapshot FOR SHARE;

-- name: PeriodGateRoot :one
SELECT COALESCE(time_zone, '')::text AS time_zone FROM organizations WHERE org_id= CAST(sqlc.arg(org_id)::text AS UUID) AND has_snapshot FOR KEY SHARE;

-- name: PeriodClock :one
SELECT clock_timestamp()::timestamptz AS checked_at;

-- name: PeriodUnitZone :one
SELECT time_zone FROM org_units WHERE org_id= CAST(sqlc.arg(org_id)::text AS UUID) AND id= CAST(sqlc.arg(id)::text AS UUID);

-- name: PeriodHead :one
SELECT CAST(id AS text) AS id, state, revision FROM org_finance_periods
WHERE org_id= CAST(sqlc.arg(org_id)::text AS UUID) AND period_month= CAST(sqlc.arg(period_month)::text AS DATE);

-- name: PeriodTransitionByOperation :one
SELECT to_char(period_month, 'YYYY-MM')::text AS month, CAST(period_id AS text) AS period_id, revision, kind, to_state, CAST(actor_account_id AS text) AS actor_account_id, request_hash, recorded_at
FROM org_finance_period_transitions WHERE org_id= CAST(sqlc.arg(org_id)::text AS UUID) AND operation_id= CAST(sqlc.arg(operation_id)::text AS UUID);

-- name: InsertPeriodTransition :one
INSERT INTO org_finance_period_transitions(org_id, period_month, period_id, revision, kind, from_state, to_state, authority, reason, actor_account_id, actor_membership_id, operation_id, request_hash)
VALUES(CAST(sqlc.arg(org_id)::text AS UUID), CAST(sqlc.arg(period_month)::text AS DATE), CAST(sqlc.arg(period_id)::text AS UUID), sqlc.arg(revision)::bigint, sqlc.arg(kind)::text, sqlc.arg(from_state)::text, sqlc.arg(to_state)::text, sqlc.arg(authority)::text, sqlc.arg(reason)::text,
 CAST(sqlc.arg(actor_account_id)::text AS UUID), CAST(sqlc.arg(actor_membership_id)::text AS UUID), CAST(sqlc.arg(operation_id)::text AS UUID), sqlc.arg(request_hash)::text)
RETURNING recorded_at;

-- name: PeriodTransitions :many
SELECT revision, kind, from_state, to_state, authority, reason, CAST(period_id AS text) AS period_id, CAST(actor_account_id AS text) AS actor_account_id, CAST(actor_membership_id AS text) AS actor_membership_id, CAST(operation_id AS text) AS operation_id, recorded_at
FROM org_finance_period_transitions WHERE org_id= CAST(sqlc.arg(org_id)::text AS UUID) AND period_month= CAST(sqlc.arg(period_month)::text AS DATE) ORDER BY revision;

-- name: PeriodTransitionAt :one
SELECT revision, kind, from_state, to_state, authority, reason, CAST(period_id AS text) AS period_id, CAST(actor_account_id AS text) AS actor_account_id, CAST(actor_membership_id AS text) AS actor_membership_id, CAST(operation_id AS text) AS operation_id, recorded_at
FROM org_finance_period_transitions WHERE org_id= CAST(sqlc.arg(org_id)::text AS UUID) AND period_month= CAST(sqlc.arg(period_month)::text AS DATE) AND revision= sqlc.arg(revision)::bigint;

-- name: ListPeriodHeads :many
SELECT to_char(period_month, 'YYYY-MM')::text AS month, CAST(id AS text) AS id, state, revision FROM org_finance_periods
WHERE org_id= CAST(sqlc.arg(org_id)::text AS UUID) AND(sqlc.arg(after_month)::text='' OR period_month< CAST(NULLIF(sqlc.arg(after_month)::text, '') || '-01' AS DATE)) AND(sqlc.arg(state)::text='' OR state= sqlc.arg(state)::text)
ORDER BY period_month DESC LIMIT sqlc.arg(row_limit)::int;
