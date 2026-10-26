-- MAP-21: optional dated community operational assertions, never live status.
-- Legacy observations remain byte-for-byte intact; no default/backfill/expiry.
ALTER TABLE charging_connection_groups ADD CONSTRAINT charging_group_place_identity UNIQUE(id,place_id);
ALTER TABLE charging_observations
 ADD COLUMN operational_event TEXT,
 ADD COLUMN operational_scope TEXT,
 ADD COLUMN operational_group_id UUID,
 ADD COLUMN operational_observed_at TIMESTAMPTZ,
 ADD CONSTRAINT charging_operational_shape CHECK (
  CASE WHEN operational_event IS NULL THEN
   operational_scope IS NULL AND operational_group_id IS NULL AND operational_observed_at IS NULL
  ELSE operational_event IN ('WORKED','DID_NOT_WORK','IN_USE','ACCESS_RESTRICTED')
   AND place_id IS NOT NULL AND operational_scope IS NOT NULL AND operational_observed_at IS NOT NULL
   AND ((operational_scope='SITE' AND operational_group_id IS NULL)
     OR (operational_scope IN ('EQUIPMENT','CONNECTOR') AND operational_group_id IS NOT NULL)) END),
 ADD CONSTRAINT charging_operational_actual_group FOREIGN KEY(operational_group_id,place_id)
  REFERENCES charging_connection_groups(id,place_id) ON DELETE RESTRICT;
CREATE INDEX charging_operational_history ON charging_observations(place_id,operational_observed_at DESC)
 WHERE operational_event IS NOT NULL AND NOT deleted AND hidden_at IS NULL;

-- Owner+operation receipts bind retries to exact bodies and accepted revisions.
-- No session/access token, GPS or raw body is retained; account erasure cascades.
CREATE TABLE charging_observation_commands (
 account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 operation_id UUID NOT NULL,
 target_id UUID NOT NULL REFERENCES charging_observations(id) ON DELETE CASCADE,
 kind TEXT NOT NULL CHECK(kind IN ('SUBMIT','CORRECT','WITHDRAW')),
 request_sha256 TEXT NOT NULL CHECK(request_sha256 ~ '^[0-9a-f]{64}$'),
 result_revision BIGINT NOT NULL CHECK(result_revision>0),
 PRIMARY KEY(account_id,operation_id)
);
