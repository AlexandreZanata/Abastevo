-- DRV-07 additive private earnings; integer BRL and opaque source identities.
CREATE TABLE driver_earning_owner_state (
 account_id UUID PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
 epoch BIGINT NOT NULL DEFAULT 0 CHECK(epoch>=0)
);
CREATE TABLE driver_earnings (
 account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 id UUID NOT NULL, vehicle_id UUID NOT NULL,
 platform TEXT NOT NULL CHECK(platform IN ('UBER','99','INDRIVE','OTHER')),
 source_hash TEXT NOT NULL CHECK(source_hash ~ '^[0-9a-f]{64}$'),
 coverage_kind TEXT NOT NULL CHECK(coverage_kind IN ('DETAIL','SUMMARY')),
 currency TEXT NOT NULL CHECK(currency='BRL'),
 from_at TIMESTAMPTZ NOT NULL CHECK(isfinite(from_at)),
 to_at TIMESTAMPTZ NOT NULL CHECK(isfinite(to_at)),
 timezone TEXT NOT NULL CHECK(length(timezone) BETWEEN 1 AND 128),
 basis TEXT NOT NULL CHECK(basis IN ('DETAILED','INCLUSIVE_NET')),
 gross_centavos BIGINT CHECK(gross_centavos BETWEEN 0 AND 1000000000000),
 fees_centavos BIGINT CHECK(fees_centavos BETWEEN 0 AND 1000000000000),
 tips_centavos BIGINT CHECK(tips_centavos BETWEEN 0 AND 1000000000000),
 bonus_centavos BIGINT CHECK(bonus_centavos BETWEEN 0 AND 1000000000000),
 refund_centavos BIGINT CHECK(refund_centavos BETWEEN 0 AND 1000000000000),
 net_centavos BIGINT CHECK(net_centavos BETWEEN 0 AND 1000000000000),
 earned_centavos BIGINT GENERATED ALWAYS AS (CASE WHEN basis='DETAILED' THEN gross_centavos-fees_centavos+tips_centavos+bonus_centavos-refund_centavos ELSE net_centavos END) STORED CHECK(earned_centavos BETWEEN -1000000000000 AND 1000000000000),
 content_sha256 TEXT NOT NULL CHECK(content_sha256 ~ '^[0-9a-f]{64}$'),
 revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0),
 PRIMARY KEY(account_id,id), UNIQUE(account_id,platform,source_hash),
 CHECK(to_at>from_at),
 CHECK((basis='DETAILED' AND net_centavos IS NULL AND gross_centavos IS NOT NULL AND fees_centavos IS NOT NULL AND tips_centavos IS NOT NULL AND bonus_centavos IS NOT NULL AND refund_centavos IS NOT NULL)
 OR (basis='INCLUSIVE_NET' AND net_centavos IS NOT NULL AND gross_centavos IS NULL AND fees_centavos IS NULL AND tips_centavos IS NULL AND bonus_centavos IS NULL AND refund_centavos IS NULL))
);
CREATE INDEX driver_earnings_coverage ON driver_earnings(account_id,platform,from_at,to_at);
CREATE TABLE driver_earning_revisions (
 account_id UUID NOT NULL, record_id UUID NOT NULL, revision BIGINT NOT NULL CHECK(revision>0),
 schema_version INTEGER NOT NULL DEFAULT 1 CHECK(schema_version=1),
 payload JSONB NOT NULL CHECK(jsonb_typeof(payload)='object'),
 PRIMARY KEY(account_id,record_id,revision),
 FOREIGN KEY(account_id,record_id) REFERENCES driver_earnings(account_id,id) ON DELETE CASCADE
);
CREATE TABLE driver_earning_adjustments (
 account_id UUID NOT NULL, id UUID NOT NULL,
 source_hash TEXT NOT NULL CHECK(source_hash ~ '^[0-9a-f]{64}$'),
 target_id UUID NOT NULL, target_revision BIGINT NOT NULL CHECK(target_revision>0),
 kind TEXT NOT NULL CHECK(kind IN ('REVERSAL','CORRECTION')),
 delta_centavos BIGINT NOT NULL CHECK(delta_centavos<>0 AND delta_centavos BETWEEN -1000000000000 AND 1000000000000),
 content_sha256 TEXT NOT NULL CHECK(content_sha256 ~ '^[0-9a-f]{64}$'),
 PRIMARY KEY(account_id,id), UNIQUE(account_id,source_hash),
 FOREIGN KEY(account_id,target_id,target_revision) REFERENCES driver_earning_revisions(account_id,record_id,revision) ON DELETE CASCADE,
 CHECK(kind<>'REVERSAL' OR delta_centavos<0)
);
CREATE INDEX driver_adjustments_target ON driver_earning_adjustments(account_id,target_id,target_revision,id);
CREATE TABLE driver_earning_commands (
 account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 operation_id UUID NOT NULL, request_sha256 TEXT NOT NULL CHECK(request_sha256 ~ '^[0-9a-f]{64}$'),
 record_id UUID NOT NULL, revision BIGINT NOT NULL CHECK(revision>0), epoch BIGINT NOT NULL CHECK(epoch>=0),
 PRIMARY KEY(account_id,operation_id)
);
CREATE TABLE driver_earning_tombstones (
 account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 record_id UUID NOT NULL, platform TEXT NOT NULL,
 source_hash TEXT NOT NULL CHECK(source_hash ~ '^[0-9a-f]{64}$'),
 erased_revision BIGINT NOT NULL CHECK(erased_revision>0),
 PRIMARY KEY(account_id,record_id), UNIQUE(account_id,platform,source_hash)
);
CREATE FUNCTION driver_erase_deleted_owner() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 INSERT INTO driver_earning_owner_state(account_id,epoch) VALUES(NEW.id,1)
 ON CONFLICT(account_id) DO UPDATE SET epoch=driver_earning_owner_state.epoch+1;
 DELETE FROM driver_earning_adjustments WHERE account_id=NEW.id;
 DELETE FROM driver_earning_revisions WHERE account_id=NEW.id;
 DELETE FROM driver_earnings WHERE account_id=NEW.id;
 DELETE FROM driver_earning_commands WHERE account_id=NEW.id;
 DELETE FROM driver_earning_tombstones WHERE account_id=NEW.id;
 RETURN NEW;
END $$;
CREATE TRIGGER driver_erase_deleted_owner AFTER UPDATE OF status ON accounts
 FOR EACH ROW WHEN(NEW.status='deleted' AND OLD.status IS DISTINCT FROM NEW.status)
 EXECUTE FUNCTION driver_erase_deleted_owner();
