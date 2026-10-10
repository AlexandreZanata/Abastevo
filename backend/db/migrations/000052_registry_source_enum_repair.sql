-- 000052: repair run source enum (CI fix). Append-only: 000049/000050
-- rewrote the check from 000031 without the 000033 DOU values,
-- breaking dou-editions/dou-acts staging. This restores the full
-- lineage in one set: registry, DOU, station-prep and review sources.
-- Lesson recorded: evolve CHECKs from their latest definition, never
-- from the creating migration.
ALTER TABLE registry_source_runs
    DROP CONSTRAINT registry_source_runs_source_check,
    ADD CONSTRAINT registry_source_runs_source_check
        CHECK (source IN ('registry-csv', 'registry-api', 'dou-editions', 'dou-acts', 'station-prep', 'review'));
