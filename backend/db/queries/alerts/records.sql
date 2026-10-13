-- Ownership joins are reviewed module ports: rules check station existence
-- via directory_stations without importing adapters.
-- name: RuleStation :one
SELECT id FROM directory_stations WHERE id=$1;

-- name: GetRule :one
SELECT payload,revision,enabled,deleted FROM alert_rules WHERE account_id=$1 AND rule_id=$2 FOR UPDATE;

-- name: CountRules :one
SELECT count(*) FROM alert_rules WHERE account_id=$1 AND NOT deleted;

-- name: SaveRule :one
INSERT INTO alert_rules(account_id,rule_id,payload,enabled)
VALUES(sqlc.arg(account_id)::uuid,sqlc.arg(rule_id)::uuid,sqlc.arg(payload)::jsonb,sqlc.arg(enabled)::boolean)
ON CONFLICT(account_id,rule_id) DO UPDATE SET payload=EXCLUDED.payload,enabled=EXCLUDED.enabled,revision=alert_rules.revision+1,updated_at=clock_timestamp()
WHERE NOT alert_rules.deleted
RETURNING revision,enabled;

-- name: SetRuleEnabled :one
UPDATE alert_rules SET enabled=sqlc.arg(enabled)::boolean,revision=revision+1,updated_at=clock_timestamp()
WHERE account_id=sqlc.arg(account_id)::uuid AND rule_id=sqlc.arg(rule_id)::uuid AND NOT deleted RETURNING revision;

-- name: DeleteRule :one
UPDATE alert_rules SET deleted=true,revision=revision+1,updated_at=clock_timestamp()
WHERE account_id=$1 AND rule_id=$2 AND NOT deleted RETURNING revision;

-- name: GetAlertOperation :one
SELECT payload_hash,receipt FROM alert_operations WHERE account_id=$1 AND operation_id=$2;

-- name: SaveAlertOperation :exec
INSERT INTO alert_operations(account_id,operation_id,payload_hash,receipt) VALUES($1,$2,$3,$4);

-- name: ListInbox :many
SELECT inbox_id,rule_id,payload,status FROM alert_inbox WHERE account_id=$1 ORDER BY created_at,inbox_id LIMIT $2;

-- name: ListRules :many
SELECT rule_id,revision,enabled,payload FROM alert_rules WHERE account_id=$1 AND NOT deleted ORDER BY rule_id;

-- name: EraseAlertRules :exec
UPDATE alert_rules SET payload='{}',deleted=true,revision=revision+1,updated_at=clock_timestamp() WHERE account_id=$1 AND NOT deleted;

-- name: EraseAlertInbox :exec
DELETE FROM alert_inbox WHERE account_id=$1;

-- name: PurgeAlertOperations :exec
DELETE FROM alert_operations WHERE account_id=$1;
