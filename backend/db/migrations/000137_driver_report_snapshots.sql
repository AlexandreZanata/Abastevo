-- DRV-11 bounded, revisioned private driver report snapshots.
CREATE TABLE driver_report_snapshots (
 account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 id UUID NOT NULL,
 vehicle_id UUID NOT NULL,
 period TEXT NOT NULL CHECK(period IN ('DAY','WEEK','MONTH','CUSTOM')),
 from_day DATE NOT NULL,
 to_day DATE NOT NULL,
 timezone TEXT NOT NULL CHECK(length(timezone) BETWEEN 1 AND 128),
 platform TEXT NOT NULL CHECK(platform IN ('','UBER','99','INDRIVE','OTHER')),
 basis TEXT NOT NULL CHECK(basis IN ('ODOMETER','GPS_ESTIMATE','PAID_REPORTED','MANUAL_TRIP')),
 source_revision TEXT NOT NULL CHECK(source_revision ~ '^[0-9a-f]{64}$'),
 version BIGINT NOT NULL DEFAULT 1 CHECK(version>=1),
 payload JSONB NOT NULL CHECK(jsonb_typeof(payload)='object'),
 generated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(account_id,id),
 UNIQUE(account_id,vehicle_id,period,from_day,to_day,timezone,platform,basis),
 CHECK(to_day>from_day)
);
CREATE INDEX driver_report_snapshot_key ON driver_report_snapshots(account_id,vehicle_id,from_day,to_day);
CREATE INDEX driver_report_snapshot_expiry ON driver_report_snapshots(generated_at);

CREATE FUNCTION driver_erase_deleted_owner_reports() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 DELETE FROM driver_report_snapshots WHERE account_id=NEW.id;
 RETURN NEW;
END $$;
CREATE TRIGGER driver_erase_deleted_owner_reports AFTER UPDATE OF status ON accounts
 FOR EACH ROW WHEN(NEW.status='deleted' AND OLD.status IS DISTINCT FROM NEW.status)
 EXECUTE FUNCTION driver_erase_deleted_owner_reports();
