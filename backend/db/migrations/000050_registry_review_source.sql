-- 000050: review registry source (RST-06). Append-only: extend the source
-- enum with audited location-review runs. A review run holds the single
-- reviewed assertion promoted from corroborated candidate evidence;
-- publication still flows through the existing complete-run
-- reconciliation, and official-lookup reads keep their current source
-- scope until a tested publication change says otherwise.
ALTER TABLE registry_source_runs
    DROP CONSTRAINT registry_source_runs_source_check,
    ADD CONSTRAINT registry_source_runs_source_check
        CHECK (source IN ('registry-csv', 'registry-api', 'station-prep', 'review'));
