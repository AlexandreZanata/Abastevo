-- DRV-08 additive private shifts, recovery gaps and aggregate distance observations.
CREATE TABLE driver_shifts (
 account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 id UUID NOT NULL,
 origin TEXT NOT NULL CHECK(length(origin) BETWEEN 1 AND 128),
 vehicle_id UUID NOT NULL,
 timezone TEXT NOT NULL CHECK(length(timezone) BETWEEN 1 AND 128),
 state TEXT NOT NULL CHECK(state IN ('READY','RUNNING','PAUSED','CLOSED','RECOVERY_REQUIRED')),
 version BIGINT NOT NULL CHECK(version>=1),
 last_boot_id TEXT CHECK(last_boot_id ~ '^[A-Za-z0-9._-]{1,64}$'),
 last_elapsed_millis BIGINT CHECK(last_elapsed_millis>=0),
 last_utc_millis BIGINT,
 active_millis BIGINT NOT NULL DEFAULT 0 CHECK(active_millis BETWEEN 0 AND 1000000000000),
 manually_reviewed_millis BIGINT NOT NULL DEFAULT 0 CHECK(manually_reviewed_millis BETWEEN 0 AND 1000000000000),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(account_id,id),
 CHECK((last_boot_id IS NULL AND last_elapsed_millis IS NULL AND last_utc_millis IS NULL)
    OR (last_boot_id IS NOT NULL AND last_elapsed_millis IS NOT NULL AND last_utc_millis IS NOT NULL)),
 CHECK(manually_reviewed_millis<=active_millis),
 CHECK(state<>'READY' OR (last_boot_id IS NULL AND active_millis=0))
);
CREATE UNIQUE INDEX driver_one_nonclosed_shift ON driver_shifts(account_id) WHERE state<>'CLOSED';
CREATE INDEX driver_shift_history ON driver_shifts(account_id,created_at DESC,id);

CREATE TABLE driver_shift_gaps (
 account_id UUID NOT NULL,
 shift_id UUID NOT NULL,
 id UUID NOT NULL,
 reason TEXT NOT NULL CHECK(reason IN ('boot-changed','clock-discontinuity','checkpoint-gap','process-recovery')),
 previous_boot_id TEXT NOT NULL CHECK(previous_boot_id ~ '^[A-Za-z0-9._-]{1,64}$'),
 previous_elapsed_millis BIGINT NOT NULL CHECK(previous_elapsed_millis>=0),
 previous_utc_millis BIGINT NOT NULL,
 next_boot_id TEXT NOT NULL CHECK(next_boot_id ~ '^[A-Za-z0-9._-]{1,64}$'),
 next_elapsed_millis BIGINT NOT NULL CHECK(next_elapsed_millis>=0),
 next_utc_millis BIGINT NOT NULL,
 reviewed BOOLEAN NOT NULL DEFAULT false,
 reviewed_work_millis BIGINT CHECK(reviewed_work_millis BETWEEN 0 AND 1000000000000),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(account_id,shift_id,id),
 CHECK(reviewed OR reviewed_work_millis IS NULL),
 FOREIGN KEY(account_id,shift_id) REFERENCES driver_shifts(account_id,id) ON DELETE CASCADE
);

CREATE TABLE driver_shift_commands (
 account_id UUID NOT NULL,
 shift_id UUID NOT NULL,
 operation_id UUID NOT NULL,
 fingerprint TEXT NOT NULL CHECK(fingerprint ~ '^[0-9a-f]{64}$'),
 accepted_version BIGINT NOT NULL CHECK(accepted_version>0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(account_id,shift_id,operation_id),
 FOREIGN KEY(account_id,shift_id) REFERENCES driver_shifts(account_id,id) ON DELETE CASCADE
);
CREATE INDEX driver_shift_receipts_version ON driver_shift_commands(account_id,shift_id,accepted_version);

CREATE TABLE driver_distance_observations (
 account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 id UUID NOT NULL,
 origin TEXT NOT NULL CHECK(length(origin) BETWEEN 1 AND 128),
 vehicle_id UUID NOT NULL,
 segment TEXT NOT NULL CHECK(segment ~ '^[A-Za-z0-9._:-]{1,64}$'),
 basis TEXT NOT NULL CHECK(basis IN ('ODOMETER','GPS_ESTIMATE','PAID_REPORTED','MANUAL_TRIP','ROUTE_PREDICTION')),
 from_at TIMESTAMPTZ NOT NULL CHECK(isfinite(from_at)),
 to_at TIMESTAMPTZ NOT NULL CHECK(isfinite(to_at)),
 timezone TEXT NOT NULL CHECK(length(timezone) BETWEEN 1 AND 128),
 metres BIGINT CHECK(metres BETWEEN 0 AND 1000000000000),
 complete BOOLEAN NOT NULL,
 has_gap BOOLEAN NOT NULL,
 quality TEXT NOT NULL CHECK(quality IN ('MEASURED','ESTIMATED','REPORTED','UNKNOWN')),
 quality_version INTEGER CHECK(quality_version>0),
 quality_note TEXT CHECK(quality_note IS NULL OR quality_note ~ '^[a-z0-9-]{1,64}$'),
 start_odometer_metres BIGINT CHECK(start_odometer_metres BETWEEN 0 AND 1000000000000),
 end_odometer_metres BIGINT CHECK(end_odometer_metres BETWEEN 0 AND 1000000000000),
 content_sha256 TEXT NOT NULL CHECK(content_sha256 ~ '^[0-9a-f]{64}$'),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(account_id,id),
 CHECK(to_at>from_at),
 CHECK(NOT (complete AND has_gap)),
 CHECK(NOT complete OR metres IS NOT NULL),
 CHECK((basis='ODOMETER' AND start_odometer_metres IS NOT NULL AND end_odometer_metres IS NOT NULL
        AND end_odometer_metres>=start_odometer_metres
        AND (metres IS NULL OR metres=end_odometer_metres-start_odometer_metres))
    OR (basis<>'ODOMETER' AND start_odometer_metres IS NULL AND end_odometer_metres IS NULL)),
 CHECK((basis IN ('ODOMETER','MANUAL_TRIP') AND quality='MEASURED')
    OR (basis IN ('GPS_ESTIMATE','ROUTE_PREDICTION') AND quality='ESTIMATED')
    OR (basis='PAID_REPORTED' AND quality='REPORTED')),
 CHECK(basis<>'GPS_ESTIMATE' OR quality_version IS NOT NULL)
);
CREATE INDEX driver_distance_windows ON driver_distance_observations(account_id,vehicle_id,basis,from_at,to_at);
CREATE INDEX driver_distance_report ON driver_distance_observations(account_id,vehicle_id,from_at,id);

CREATE TABLE driver_distance_commands (
 account_id UUID NOT NULL,
 operation_id UUID NOT NULL,
 request_sha256 TEXT NOT NULL CHECK(request_sha256 ~ '^[0-9a-f]{64}$'),
 record_id UUID NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(account_id,operation_id),
 FOREIGN KEY(account_id,record_id) REFERENCES driver_distance_observations(account_id,id) ON DELETE CASCADE
);

CREATE FUNCTION driver_erase_deleted_owner_movement() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 DELETE FROM driver_distance_commands WHERE account_id=NEW.id;
 DELETE FROM driver_distance_observations WHERE account_id=NEW.id;
 DELETE FROM driver_shift_commands WHERE account_id=NEW.id;
 DELETE FROM driver_shift_gaps WHERE account_id=NEW.id;
 DELETE FROM driver_shifts WHERE account_id=NEW.id;
 RETURN NEW;
END $$;
CREATE TRIGGER driver_erase_deleted_owner_movement AFTER UPDATE OF status ON accounts
 FOR EACH ROW WHEN(NEW.status='deleted' AND OLD.status IS DISTINCT FROM NEW.status)
 EXECUTE FUNCTION driver_erase_deleted_owner_movement();
