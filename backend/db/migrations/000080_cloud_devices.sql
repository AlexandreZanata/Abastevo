CREATE TABLE cloud_devices(
 id UUID PRIMARY KEY,
 account_id UUID NOT NULL REFERENCES accounts(id),
 family_id UUID NOT NULL REFERENCES account_session_families(id),
 contributor_id UUID NOT NULL REFERENCES identity_contributors(id),
 key_id UUID NOT NULL REFERENCES identity_keys(id),
 origin TEXT NOT NULL CHECK(length(origin) BETWEEN 1 AND 255),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 last_success_at TIMESTAMPTZ,
 revoked_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX cloud_device_binding_idx ON cloud_devices(account_id,family_id,key_id,origin) WHERE revoked_at IS NULL;
CREATE INDEX cloud_devices_owner_idx ON cloud_devices(account_id,id);
CREATE TABLE cloud_device_revocations(
 device_id UUID PRIMARY KEY,
 account_id UUID NOT NULL REFERENCES accounts(id),
 revoked_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX cloud_device_revocations_owner_idx ON cloud_device_revocations(account_id,device_id);
CREATE OR REPLACE FUNCTION cloud_erase_deleted_owner() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 UPDATE cloud_records SET payload=NULL,deleted=true,revision=revision+1,updated_at=clock_timestamp() WHERE account_id=NEW.id;
 INSERT INTO cloud_device_revocations(device_id,account_id) SELECT id,account_id FROM cloud_devices WHERE account_id=NEW.id ON CONFLICT DO NOTHING;
 UPDATE cloud_devices SET revoked_at=COALESCE(revoked_at,clock_timestamp()) WHERE account_id=NEW.id;
 RETURN NEW;
END $$;
