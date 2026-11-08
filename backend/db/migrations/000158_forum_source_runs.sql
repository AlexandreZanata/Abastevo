-- 000158: Forum source ingestion runs and checkpoints (FOR-12, B-BR-FOR-02/03/12/16).
-- Append-only schema for candidate news collection, conditional GET checkpoints and execution leases.

CREATE TABLE forum_source_runs (
    id UUID PRIMARY KEY,
    source_id UUID NOT NULL REFERENCES forum_sources(id) ON DELETE RESTRICT,
    status TEXT NOT NULL CHECK (status IN ('RUNNING', 'SUCCESS', 'NOT_MODIFIED', 'FAILED', 'BLOCKED')),
    etag TEXT NOT NULL DEFAULT '',
    last_modified TEXT NOT NULL DEFAULT '',
    items_discovered INT NOT NULL DEFAULT 0,
    items_ingested INT NOT NULL DEFAULT 0,
    error_message TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ
);

CREATE INDEX forum_source_runs_source_started_idx
    ON forum_source_runs (source_id, started_at DESC);

CREATE TABLE forum_source_checkpoints (
    source_id UUID PRIMARY KEY REFERENCES forum_sources(id) ON DELETE RESTRICT,
    etag TEXT NOT NULL DEFAULT '',
    last_modified TEXT NOT NULL DEFAULT '',
    last_run_id UUID REFERENCES forum_source_runs(id) ON DELETE SET NULL,
    last_success_at TIMESTAMPTZ,
    last_status TEXT NOT NULL DEFAULT 'PENDING' CHECK (last_status IN ('PENDING', 'RUNNING', 'SUCCESS', 'NOT_MODIFIED', 'FAILED', 'BLOCKED')),
    consecutive_failures INT NOT NULL DEFAULT 0 CHECK (consecutive_failures >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
