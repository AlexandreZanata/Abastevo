-- PRM05-T03 explicit odometer segments and missing-fill declarations.
-- Append-only; segments open UNKNOWN boundaries on reset or replacement.
-- Gaps are informational incompleteness markers for later consumption
-- UNKNOWN; they never block inserts. Histories survive vehicle archive;
-- cascades require preview and consistent erasure in PRM05-T05.
CREATE TABLE ledger_segments(
 account_id UUID NOT NULL REFERENCES accounts(id),
 vehicle_id UUID NOT NULL,
 segment_no BIGINT NOT NULL CHECK(segment_no>1),
 reason TEXT NOT NULL CHECK(reason IN ('reset','replacement')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(account_id, vehicle_id, segment_no)
);
CREATE INDEX ledger_segments_vehicle_idx ON ledger_segments(account_id, vehicle_id);
CREATE TABLE ledger_gaps(
 account_id UUID NOT NULL REFERENCES accounts(id),
 gap_id UUID NOT NULL,
 vehicle_id UUID NOT NULL,
 after_id UUID,
 before_id UUID,
 note TEXT NOT NULL DEFAULT '' CHECK(length(note)<=280),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(account_id, gap_id)
);
CREATE INDEX ledger_gaps_vehicle_idx ON ledger_gaps(account_id, vehicle_id);
ALTER TABLE ledger_records ADD COLUMN segment_no BIGINT NOT NULL DEFAULT 1 CHECK(segment_no>=1);
CREATE INDEX ledger_records_segment_idx ON ledger_records(account_id, vehicle_id, segment_no);
