CREATE TABLE cloud_state(
 account_id UUID PRIMARY KEY REFERENCES accounts(id),
 epoch BIGINT NOT NULL DEFAULT 1 CHECK(epoch>0),
 revision BIGINT NOT NULL DEFAULT 0 CHECK(revision>=0),
 compacted_through BIGINT NOT NULL DEFAULT 0 CHECK(compacted_through>=0 AND compacted_through<=revision),
 consent_mask INTEGER NOT NULL DEFAULT 0 CHECK(consent_mask BETWEEN 0 AND 7),
 consent_revision BIGINT NOT NULL DEFAULT 0 CHECK(consent_revision>=0)
);
CREATE TABLE cloud_operations(
 account_id UUID NOT NULL REFERENCES accounts(id),
 operation_id UUID NOT NULL,
 payload_hash TEXT NOT NULL CHECK(payload_hash ~ '^[0-9a-f]{64}$'),
 receipt JSONB NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(account_id,operation_id)
);
CREATE INDEX cloud_operations_retention_idx ON cloud_operations(account_id,created_at);
CREATE TABLE cloud_changes(
 account_id UUID NOT NULL REFERENCES accounts(id),
 sequence BIGINT NOT NULL CHECK(sequence>0),
 epoch BIGINT NOT NULL CHECK(epoch>0),
 dataset TEXT NOT NULL CHECK(dataset IN ('vehicles','favorites','preferences')),
 id UUID NOT NULL,
 record_revision BIGINT NOT NULL CHECK(record_revision>0),
 deleted BOOLEAN NOT NULL,
 payload JSONB,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(account_id,sequence)
);
CREATE INDEX cloud_changes_record_idx ON cloud_changes(account_id,dataset,id);
CREATE TABLE cloud_erased_ids(
 account_id UUID NOT NULL REFERENCES accounts(id),
 dataset TEXT NOT NULL,
 id UUID NOT NULL,
 erased_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(account_id,dataset,id)
);
CREATE OR REPLACE FUNCTION cloud_erase_deleted_owner() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 INSERT INTO cloud_state(account_id,epoch,revision,consent_revision) VALUES(NEW.id,2,1,1)
 ON CONFLICT(account_id) DO UPDATE SET epoch=cloud_state.epoch+1,revision=cloud_state.revision+1,consent_mask=0,consent_revision=cloud_state.consent_revision+1;
 INSERT INTO cloud_erased_ids(account_id,dataset,id) SELECT account_id,dataset,id FROM cloud_records WHERE account_id=NEW.id ON CONFLICT DO NOTHING;
 UPDATE cloud_records SET payload=NULL,deleted=true,revision=revision+1,updated_at=clock_timestamp() WHERE account_id=NEW.id;
 UPDATE cloud_changes SET payload=NULL,deleted=true WHERE account_id=NEW.id;
 INSERT INTO cloud_device_revocations(device_id,account_id) SELECT id,account_id FROM cloud_devices WHERE account_id=NEW.id ON CONFLICT DO NOTHING;
 UPDATE cloud_devices SET revoked_at=COALESCE(revoked_at,clock_timestamp()) WHERE account_id=NEW.id;
 RETURN NEW;
END $$;
