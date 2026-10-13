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

-- name: DueInbox :many
SELECT account_id,inbox_id,rule_id,payload FROM alert_inbox WHERE status='evaluated' ORDER BY created_at,inbox_id LIMIT $1 FOR UPDATE SKIP LOCKED;

-- name: MarkInbox :execrows
UPDATE alert_inbox SET status=$3,payload=$4 WHERE account_id=$1 AND inbox_id=$2 AND status='evaluated';

-- name: MarkInboxSent :execrows
UPDATE alert_inbox SET status='sent',payload=$3 WHERE account_id=$1 AND inbox_id=$2 AND status='dispatched';

-- name: LastDispatch :one
SELECT MAX(created_at)::timestamptz FROM alert_inbox WHERE account_id=$1 AND rule_id=$2 AND status='dispatched';

-- name: EnrollPushToken :exec
INSERT INTO alert_push_tokens(account_id,token,revoked_at) VALUES($1,$2,NULL) ON CONFLICT(account_id,token) DO UPDATE SET revoked_at=NULL;

-- name: RevokePushToken :execrows
UPDATE alert_push_tokens SET revoked_at=clock_timestamp() WHERE account_id=$1 AND token=$2 AND revoked_at IS NULL;

-- name: GetPushToken :one
SELECT revoked_at FROM alert_push_tokens WHERE account_id=$1 AND token=$2;

-- name: GetInboxRow :one
SELECT rule_id,payload,status FROM alert_inbox WHERE account_id=$1 AND inbox_id=$2 FOR UPDATE;

-- name: CommunityPrices :many
SELECT o.id,o.station_id,o.fuel_product,o.unit,o.amount_milli_brl,o.condition_kind,o.received_at FROM community_observations o
WHERE o.station_id=ANY($1::uuid[]) AND o.fuel_product=$2 AND o.unit=$3 AND o.received_at>$4
AND (SELECT d.to_state FROM community_observation_decisions d WHERE d.observation_id=o.id ORDER BY d.sequence DESC LIMIT 1)='VALIDATED'
ORDER BY o.station_id,o.received_at DESC LIMIT $5;

-- name: OfficialPrices :many
SELECT p.id,p.station_id,p.fuel_product,p.unit,p.amount_milli_brl,p.collected_on FROM official_station_prices p
WHERE p.station_id=ANY($1::uuid[]) AND p.fuel_product=$2 AND p.unit=$3 AND p.collected_on>$4 ORDER BY p.station_id,p.collected_on DESC LIMIT $5;

-- name: InsertInboxCandidate :execrows
INSERT INTO alert_inbox(account_id,inbox_id,rule_id,payload,status) VALUES($1,$2,$3,$4,'evaluated') ON CONFLICT DO NOTHING;

-- name: CommunityBaseline :many
SELECT DISTINCT ON (o.station_id) o.id,o.station_id,o.fuel_product,o.unit,o.amount_milli_brl,o.condition_kind,o.received_at FROM community_observations o
WHERE o.station_id=ANY($1::uuid[]) AND o.fuel_product=$2 AND o.unit=$3 AND o.received_at<=$4
AND (SELECT d.to_state FROM community_observation_decisions d WHERE d.observation_id=o.id ORDER BY d.sequence DESC LIMIT 1)='VALIDATED'
ORDER BY o.station_id,o.received_at DESC;

-- name: OfficialBaseline :many
SELECT DISTINCT ON (p.station_id) p.id,p.station_id,p.fuel_product,p.unit,p.amount_milli_brl,p.collected_on FROM official_station_prices p
WHERE p.station_id=ANY($1::uuid[]) AND p.fuel_product=$2 AND p.unit=$3 AND p.collected_on<=$4 ORDER BY p.station_id,p.collected_on DESC;

-- name: EraseAlertRules :exec
UPDATE alert_rules SET payload='{}',deleted=true,revision=revision+1,updated_at=clock_timestamp() WHERE account_id=$1 AND NOT deleted;

-- name: EraseAlertInbox :exec
DELETE FROM alert_inbox WHERE account_id=$1;

-- name: PurgeAlertOperations :exec
DELETE FROM alert_operations WHERE account_id=$1;
