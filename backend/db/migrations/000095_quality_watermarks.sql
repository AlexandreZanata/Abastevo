-- FQ-05 discovery watermarks. Append-only; one cursor per source/stream
-- records the last completed edition import. Failed, skipped or partial
-- runs never move the cursor, so replays resume behind it and the current
-- publication stays untouched.
CREATE TABLE quality_watermarks(
 source TEXT NOT NULL,
 stream TEXT NOT NULL,
 last_edition TEXT NOT NULL CHECK(length(last_edition) BETWEEN 1 AND 32),
 last_hash TEXT NOT NULL CHECK(length(last_hash) = 64),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(source, stream)
);
