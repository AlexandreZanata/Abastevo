-- DRV-10 versioned owner reconciliation decisions for overlapping earnings.
CREATE TABLE driver_reconciliation_decisions (
 account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 id UUID NOT NULL,
 version BIGINT NOT NULL CHECK(version>=1),
 kind TEXT NOT NULL CHECK(kind IN ('SUMMARY_DETAIL','PAYOUT_EARNINGS')),
 platform TEXT NOT NULL CHECK(platform IN ('UBER','99','INDRIVE','OTHER')),
 from_at TIMESTAMPTZ NOT NULL CHECK(isfinite(from_at)),
 to_at TIMESTAMPTZ NOT NULL CHECK(isfinite(to_at)),
 primary_id UUID NOT NULL,
 primary_revision BIGINT NOT NULL CHECK(primary_revision>=1),
 payout_source_hash TEXT CHECK(payout_source_hash IS NULL OR payout_source_hash ~ '^[0-9a-f]{64}$'),
 content_sha256 TEXT NOT NULL CHECK(content_sha256 ~ '^[0-9a-f]{64}$'),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(account_id,id,version),
 CHECK(to_at>from_at),
 CHECK((kind='SUMMARY_DETAIL' AND payout_source_hash IS NULL)
    OR (kind='PAYOUT_EARNINGS' AND payout_source_hash IS NOT NULL))
);
CREATE INDEX driver_reconciliation_history ON driver_reconciliation_decisions(account_id,id,version DESC);

CREATE TABLE driver_reconciliation_records (
 account_id UUID NOT NULL,
 decision_id UUID NOT NULL,
 version BIGINT NOT NULL,
 record_id UUID NOT NULL,
 record_revision BIGINT NOT NULL CHECK(record_revision>=1),
 PRIMARY KEY(account_id,decision_id,version,record_id),
 FOREIGN KEY(account_id,decision_id,version) REFERENCES driver_reconciliation_decisions(account_id,id,version) ON DELETE CASCADE
);

CREATE TABLE driver_reconciliation_commands (
 account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 operation_id UUID NOT NULL,
 request_sha256 TEXT NOT NULL CHECK(request_sha256 ~ '^[0-9a-f]{64}$'),
 decision_id UUID NOT NULL,
 version BIGINT NOT NULL CHECK(version>=1),
 PRIMARY KEY(account_id,operation_id)
);

CREATE FUNCTION driver_erase_deleted_owner_reconciliation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 DELETE FROM driver_reconciliation_commands WHERE account_id=NEW.id;
 DELETE FROM driver_reconciliation_records WHERE account_id=NEW.id;
 DELETE FROM driver_reconciliation_decisions WHERE account_id=NEW.id;
 RETURN NEW;
END $$;
CREATE TRIGGER driver_erase_deleted_owner_reconciliation AFTER UPDATE OF status ON accounts
 FOR EACH ROW WHEN(NEW.status='deleted' AND OLD.status IS DISTINCT FROM NEW.status)
 EXECUTE FUNCTION driver_erase_deleted_owner_reconciliation();
