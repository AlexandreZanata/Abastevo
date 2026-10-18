-- FQ-05W6: durable Directory-revalidation scan progress. The cursor is
-- operational scan state, separate from import watermarks: it only orders
-- which published bindings a bounded sweep checks next and never changes
-- the meaning of completed-import, lease or historical source clocks.
-- Rows are checked against current Directory evidence; a divergence
-- enqueues a bounded fresh import that publishes a corrected association
-- revision while every earlier revision stays reachable.
CREATE TABLE quality_revalidation_cursors(
 kind TEXT PRIMARY KEY CHECK(length(kind) BETWEEN 1 AND 64),
 cursor_edition UUID REFERENCES quality_editions(id) ON DELETE RESTRICT,
 cursor_key TEXT CHECK(cursor_key IS NULL OR length(cursor_key) BETWEEN 1 AND 256),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
