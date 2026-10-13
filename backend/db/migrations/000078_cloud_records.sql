CREATE TABLE cloud_records(
 account_id UUID NOT NULL REFERENCES accounts(id),
 dataset TEXT NOT NULL CHECK(dataset IN ('vehicles')),
 id UUID NOT NULL,
 schema_version INTEGER NOT NULL DEFAULT 1 CHECK(schema_version=1),
 revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0),
 payload JSONB,
 deleted BOOLEAN NOT NULL DEFAULT false,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(account_id,dataset,id),
 CHECK((deleted AND payload IS NULL) OR (NOT deleted AND jsonb_typeof(payload)='object'))
);
CREATE INDEX cloud_records_live_idx ON cloud_records(account_id,dataset,id) WHERE NOT deleted;
CREATE FUNCTION cloud_erase_deleted_owner() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 UPDATE cloud_records SET payload=NULL,deleted=true,revision=revision+1,updated_at=clock_timestamp() WHERE account_id=NEW.id;
 RETURN NEW;
END $$;
CREATE TRIGGER cloud_erase_deleted_owner AFTER UPDATE OF status ON accounts
FOR EACH ROW WHEN(NEW.status='deleted' AND OLD.status IS DISTINCT FROM NEW.status)
EXECUTE FUNCTION cloud_erase_deleted_owner();
