-- FQ-05W: parent archive identity and the exact monthly member are distinct.
-- Existing frozen resource provenance remains untouched and explicitly memberless.
ALTER TABLE quality_resources ADD COLUMN member_name TEXT NOT NULL DEFAULT '';
ALTER TABLE quality_resources ADD COLUMN member_sha256 TEXT;
ALTER TABLE quality_resources ADD COLUMN member_bytes BIGINT;
ALTER TABLE quality_resources ADD CONSTRAINT quality_resources_member_check CHECK(
 (member_name='' AND member_sha256 IS NULL AND member_bytes IS NULL) OR
 (length(member_name) BETWEEN 1 AND 256 AND member_sha256 IS NOT NULL AND member_sha256 ~ '^[0-9a-f]{64}$' AND member_bytes IS NOT NULL AND member_bytes>0));
ALTER TABLE quality_revision_resources ADD COLUMN member_name TEXT NOT NULL DEFAULT '';
ALTER TABLE quality_revision_resources ADD COLUMN member_sha256 TEXT;
ALTER TABLE quality_revision_resources ADD COLUMN member_bytes BIGINT;
ALTER TABLE quality_revision_resources ADD CONSTRAINT quality_revision_resources_member_check CHECK(
 (member_name='' AND member_sha256 IS NULL AND member_bytes IS NULL) OR
 (length(member_name) BETWEEN 1 AND 256 AND member_sha256 IS NOT NULL AND member_sha256 ~ '^[0-9a-f]{64}$' AND member_bytes IS NOT NULL AND member_bytes>0));

ALTER TABLE quality_resources DROP CONSTRAINT quality_resources_pkey;
ALTER TABLE quality_resources ADD PRIMARY KEY(edition_id,sha256,member_name);
ALTER TABLE quality_revision_resources DROP CONSTRAINT quality_revision_resources_pkey;
ALTER TABLE quality_revision_resources ADD PRIMARY KEY(revision_id,sha256,url,member_name);

-- Compare the locked current row itself, not a subquery snapshot that may miss
-- a competing revision committed while this statement waited for the row lock.
ALTER TABLE quality_current_revisions ADD COLUMN edition TEXT;
UPDATE quality_current_revisions c SET edition=e.edition
FROM quality_revisions r JOIN quality_editions e ON e.id=r.edition_id
WHERE r.id=c.revision_id;
ALTER TABLE quality_current_revisions ALTER COLUMN edition SET NOT NULL;
