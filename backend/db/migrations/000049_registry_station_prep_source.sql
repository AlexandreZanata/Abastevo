-- 000049: station-prep registry source (RST-05). Append-only: extend the
-- source enum with the Rust-prepared batch source. Existing rows are
-- unaffected (the new set is a superset). Prepared batches stage under
-- their own source with per-input snapshots; publication still flows
-- through the existing complete-run reconciliation, and official-lookup
-- reads keep their current source scope until a tested publication
-- change says otherwise.
ALTER TABLE registry_source_runs
    DROP CONSTRAINT registry_source_runs_source_check,
    ADD CONSTRAINT registry_source_runs_source_check
        CHECK (source IN ('registry-csv', 'registry-api', 'station-prep'));
