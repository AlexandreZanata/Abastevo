-- 000033: DOU edition sources for staged runs (P26-T02). Append-only
-- constraint evolution: the run ledger admits DOU edition/act snapshots
-- alongside registry snapshots. Assertions stay publisher-gated by run
-- state exactly like registry rows; no live source is configured here.
ALTER TABLE registry_source_runs
    DROP CONSTRAINT registry_source_runs_source_check;
ALTER TABLE registry_source_runs
    ADD CONSTRAINT registry_source_runs_source_check
        CHECK (source IN ('registry-csv', 'registry-api', 'dou-editions', 'dou-acts'));
