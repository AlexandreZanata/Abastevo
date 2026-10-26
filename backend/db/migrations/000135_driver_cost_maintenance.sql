-- DRV-09 additive private cost profiles, maintenance schedules and erasure fences.
CREATE TABLE driver_cost_profiles (
 account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 vehicle_id UUID NOT NULL,
 version BIGINT NOT NULL CHECK(version>=1),
 ownership TEXT NOT NULL CHECK(ownership IN ('OWNED','LEASED','FINANCED')),
 timezone TEXT NOT NULL CHECK(length(timezone) BETWEEN 1 AND 128),
 from_day DATE NOT NULL,
 until_day DATE,
 content_sha256 TEXT NOT NULL CHECK(content_sha256 ~ '^[0-9a-f]{64}$'),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(account_id,vehicle_id,version),
 CHECK(until_day IS NULL OR until_day>from_day)
);
CREATE INDEX driver_cost_profile_versions ON driver_cost_profiles(account_id,vehicle_id,version DESC);

CREATE TABLE driver_cost_profile_items (
 account_id UUID NOT NULL,
 vehicle_id UUID NOT NULL,
 version BIGINT NOT NULL,
 cost_id TEXT NOT NULL CHECK(cost_id ~ '^[A-Za-z0-9._-]{1,64}$'),
 kind TEXT NOT NULL CHECK(kind IN ('LEASE','DEPRECIATION','PRINCIPAL','INTEREST','FEE','MAINTENANCE','RESERVE')),
 provenance TEXT NOT NULL CHECK(provenance IN ('ACTUAL','MODELED')),
 ledger_source_id UUID,
 source_revision BIGINT,
 from_day DATE NOT NULL,
 until_day DATE,
 amount_centavos BIGINT CHECK(amount_centavos BETWEEN 0 AND 1000000000000),
 PRIMARY KEY(account_id,vehicle_id,version,cost_id),
 FOREIGN KEY(account_id,vehicle_id,version) REFERENCES driver_cost_profiles(account_id,vehicle_id,version) ON DELETE CASCADE,
 CHECK(until_day IS NULL OR until_day>from_day),
 CHECK((provenance='MODELED' AND ledger_source_id IS NULL AND source_revision IS NULL)
    OR (provenance='ACTUAL' AND ledger_source_id IS NOT NULL AND source_revision>=1))
);

CREATE TABLE driver_cost_commands (
 account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 operation_id UUID NOT NULL,
 request_sha256 TEXT NOT NULL CHECK(request_sha256 ~ '^[0-9a-f]{64}$'),
 vehicle_id UUID NOT NULL,
 version BIGINT NOT NULL CHECK(version>=1),
 PRIMARY KEY(account_id,operation_id)
);

CREATE TABLE driver_cost_owner_state (
 account_id UUID PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
 epoch BIGINT NOT NULL DEFAULT 0 CHECK(epoch>=0)
);

CREATE TABLE driver_cost_tombstones (
 account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 vehicle_id UUID NOT NULL,
 version BIGINT NOT NULL CHECK(version>=1),
 erased_content_sha256 TEXT NOT NULL CHECK(erased_content_sha256 ~ '^[0-9a-f]{64}$'),
 erased_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(account_id,vehicle_id,version)
);

CREATE TABLE driver_maintenance_schedules (
 account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 id UUID NOT NULL,
 version BIGINT NOT NULL CHECK(version>=1),
 vehicle_id UUID NOT NULL,
 timezone TEXT NOT NULL CHECK(length(timezone) BETWEEN 1 AND 128),
 kind TEXT NOT NULL CHECK(kind IN ('OIL','TIRES','BRAKES','FILTER','INSPECTION','OTHER')),
 basis TEXT NOT NULL CHECK(basis IN ('ODOMETER','DATE','ODOMETER_OR_DATE')),
 effective_from DATE NOT NULL,
 effective_until DATE,
 interval_metres BIGINT CHECK(interval_metres BETWEEN 0 AND 1000000000000),
 interval_days BIGINT CHECK(interval_days BETWEEN 0 AND 36500),
 odometer_segment BIGINT NOT NULL CHECK(odometer_segment>=1),
 baseline_metres BIGINT CHECK(baseline_metres BETWEEN 0 AND 1000000000000),
 baseline_day DATE,
 next_due_metres BIGINT CHECK(next_due_metres BETWEEN 0 AND 1000000000000),
 next_due_day DATE,
 content_sha256 TEXT NOT NULL CHECK(content_sha256 ~ '^[0-9a-f]{64}$'),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(account_id,id,version),
 CHECK(effective_until IS NULL OR effective_until>effective_from),
 CHECK((basis='ODOMETER' AND interval_metres IS NOT NULL)
    OR (basis='DATE' AND interval_days IS NOT NULL)
    OR (basis='ODOMETER_OR_DATE' AND (interval_metres IS NOT NULL OR interval_days IS NOT NULL)))
);
CREATE INDEX driver_maintenance_versions ON driver_maintenance_schedules(account_id,vehicle_id,version DESC);

CREATE TABLE driver_maintenance_commands (
 account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 operation_id UUID NOT NULL,
 request_sha256 TEXT NOT NULL CHECK(request_sha256 ~ '^[0-9a-f]{64}$'),
 schedule_id UUID NOT NULL,
 version BIGINT NOT NULL CHECK(version>=1),
 PRIMARY KEY(account_id,operation_id)
);

CREATE TABLE driver_maintenance_owner_state (
 account_id UUID PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
 epoch BIGINT NOT NULL DEFAULT 0 CHECK(epoch>=0)
);

CREATE TABLE driver_maintenance_tombstones (
 account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 schedule_id UUID NOT NULL,
 version BIGINT NOT NULL CHECK(version>=1),
 erased_content_sha256 TEXT NOT NULL CHECK(erased_content_sha256 ~ '^[0-9a-f]{64}$'),
 erased_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(account_id,schedule_id,version)
);

CREATE FUNCTION driver_erase_deleted_owner_cost() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 DELETE FROM driver_cost_profile_items WHERE account_id=NEW.id;
 DELETE FROM driver_cost_commands WHERE account_id=NEW.id;
 DELETE FROM driver_cost_tombstones WHERE account_id=NEW.id;
 DELETE FROM driver_cost_profiles WHERE account_id=NEW.id;
 DELETE FROM driver_maintenance_commands WHERE account_id=NEW.id;
 DELETE FROM driver_maintenance_tombstones WHERE account_id=NEW.id;
 DELETE FROM driver_maintenance_schedules WHERE account_id=NEW.id;
 RETURN NEW;
END $$;
CREATE TRIGGER driver_erase_deleted_owner_cost AFTER UPDATE OF status ON accounts
 FOR EACH ROW WHEN(NEW.status='deleted' AND OLD.status IS DISTINCT FROM NEW.status)
 EXECUTE FUNCTION driver_erase_deleted_owner_cost();
