-- 000131: retire stale official revisions as superseded instead of letting a
-- late finish repoint the survey window. Numbers 000122-000130 are reserved
-- by the payments line on dev; this file takes the next coordinated number.
-- Append-only: the previous status set is preserved, one value added.
ALTER TABLE official_revisions DROP CONSTRAINT official_revisions_status_check;
ALTER TABLE official_revisions ADD CONSTRAINT official_revisions_status_check
    CHECK (status IN ('staging', 'published', 'failed', 'needs-review', 'superseded'));
