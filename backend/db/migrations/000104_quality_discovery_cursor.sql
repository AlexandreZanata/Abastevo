-- FQ-05W7: durable live-discovery scan progress. The cursor records which
-- official archive a bounded discovery run inspected last, so consecutive
-- runs walk the whole listing one archive at a time instead of restarting
-- from the first resource. It is operational scan state, separate from
-- completed-import watermarks, publication keys and lease clocks.
CREATE TABLE quality_discovery_cursors(
 kind TEXT PRIMARY KEY CHECK(length(kind) BETWEEN 1 AND 64),
 cursor_url TEXT NOT NULL CHECK(length(cursor_url) BETWEEN 1 AND 1024),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
