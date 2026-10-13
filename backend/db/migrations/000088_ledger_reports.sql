-- PRM06-T03 timezone-aware monthly report summaries. Append-only; reports
-- derive from ledger_records and never store raw credentials or photos.
-- Totals cover only frozen categories and windows; voided and deleted rows
-- are excluded. Worker restarts cannot publish superseded revisions.
CREATE TABLE ledger_reports(
 account_id UUID NOT NULL REFERENCES accounts(id),
 year INTEGER NOT NULL CHECK(year BETWEEN 2000 AND 2100),
 month INTEGER NOT NULL CHECK(month BETWEEN 1 AND 12),
 timezone TEXT NOT NULL CHECK(length(timezone) BETWEEN 1 AND 64),
 totals JSONB NOT NULL CHECK(jsonb_typeof(totals)='object'),
 revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0),
 base_updated_at TIMESTAMPTZ,
 generated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(account_id, year, month, timezone)
);
