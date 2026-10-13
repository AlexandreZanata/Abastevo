-- PRM05-T02 private owned ledger. Append-only; no financial fact is written
-- to public community tables. Voided records are retained but excluded from
-- calculations. Deleted rows are tombstones with fenced UUIDs via
-- cloud_erased_ids semantics in later tasks; vehicle ownership references
-- cloud_records for the same account. Archived vehicles remain writable
-- history; foreign vehicles are refused.
CREATE TABLE ledger_records(
 account_id UUID NOT NULL REFERENCES accounts(id),
 dataset TEXT NOT NULL CHECK(dataset IN ('refuel','expense')),
 id UUID NOT NULL,
 vehicle_id UUID NOT NULL,
 occurred_at TIMESTAMPTZ NOT NULL,
 payload JSONB NOT NULL CHECK(jsonb_typeof(payload)='object'),
 revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0),
 voided BOOLEAN NOT NULL DEFAULT false,
 deleted BOOLEAN NOT NULL DEFAULT false,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(account_id, dataset, id)
);
CREATE INDEX ledger_records_period_idx ON ledger_records(account_id, occurred_at);
CREATE INDEX ledger_records_vehicle_period_idx ON ledger_records(account_id, vehicle_id, occurred_at);
CREATE INDEX ledger_records_live_idx ON ledger_records(account_id, dataset, id) WHERE NOT deleted;
CREATE TABLE ledger_operations(
 account_id UUID NOT NULL REFERENCES accounts(id),
 operation_id UUID NOT NULL,
 payload_hash TEXT NOT NULL CHECK(payload_hash ~ '^[0-9a-f]{64}$'),
 receipt JSONB NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(account_id, operation_id)
);
CREATE INDEX ledger_operations_retention_idx ON ledger_operations(account_id, created_at);
