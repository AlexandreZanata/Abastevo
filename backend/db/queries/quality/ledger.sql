-- Owned by quality (FQ-04a assay ledger). Modules must not import another
-- module's generated package; cross-module reads use owned queries
-- documented in DATA_MODEL. Every publish path runs inside one transaction;
-- readers only see published revisions through quality_current_revisions.

-- name: GetEditionByKey :one
SELECT id, source, stream, edition, predecessor, created_at
FROM quality_editions
WHERE source = @source AND stream = @stream AND edition = @edition;

-- name: CreateEdition :one
INSERT INTO quality_editions (id, source, stream, edition, predecessor)
VALUES (@id, @source, @stream, @edition, @predecessor)
RETURNING id;

-- name: AddResource :exec
INSERT INTO quality_resources (edition_id, url, sha256, bytes)
VALUES (@edition_id, @url, @sha256, @bytes)
ON CONFLICT (edition_id, sha256) DO NOTHING;

-- name: GetManifest :one
SELECT edition_id, input, accepted, duplicates, quarantined, manifest_hash, recorded_at
FROM quality_manifests
WHERE edition_id = @edition_id;

-- name: RecordManifest :one
INSERT INTO quality_manifests (edition_id, input, accepted, duplicates, quarantined, manifest_hash)
VALUES (@edition_id, @input, @accepted, @duplicates, @quarantined, @manifest_hash)
ON CONFLICT (edition_id) DO NOTHING
RETURNING edition_id;

-- name: UpdateManifest :exec
UPDATE quality_manifests
SET input = @input, accepted = @accepted, duplicates = @duplicates,
    quarantined = @quarantined, manifest_hash = @manifest_hash,
    recorded_at = now()
WHERE edition_id = @edition_id;

-- name: CreateRevision :one
INSERT INTO quality_revisions (id, edition_id, supersedes_revision_id)
VALUES (@id, @edition_id, @supersedes_revision_id)
RETURNING id;

-- name: GetRevision :one
SELECT id, edition_id, status, supersedes_revision_id, published_at, created_at
FROM quality_revisions
WHERE id = @id;

-- name: SetRevisionPublished :exec
UPDATE quality_revisions
SET status = 'published', published_at = now()
WHERE id = @id AND status = 'staging';

-- name: SetRevisionSuperseded :exec
UPDATE quality_revisions
SET status = 'superseded'
WHERE id = @id AND status = 'published';

-- name: StageAssay :one
INSERT INTO quality_assays
    (id, revision_id, sample_ref, assay_ref, source_cnpj, observed_on, fuel,
     result_text, unit, conformity, evidence_ref, fingerprint)
VALUES (@id, @revision_id, @sample_ref, @assay_ref, @source_cnpj, @observed_on,
    @fuel, @result_text, @unit, @conformity, @evidence_ref, @fingerprint)
ON CONFLICT (revision_id, fingerprint) DO NOTHING
RETURNING id;

-- name: CountRevisionAssays :one
SELECT count(*) FROM quality_assays WHERE revision_id = @revision_id;

-- name: ListRevisionAssays :many
SELECT id, revision_id, sample_ref, assay_ref, source_cnpj, observed_on, fuel,
    result_text, unit, conformity, evidence_ref, fingerprint
FROM quality_assays
WHERE revision_id = @revision_id
ORDER BY sample_ref ASC, assay_ref ASC, id ASC;

-- name: StageLink :one
INSERT INTO quality_links (id, revision_id, record_key, station_id, automatic, reason, active)
VALUES (@id, @revision_id, @record_key, @station_id, @automatic, @reason, @active)
ON CONFLICT (revision_id, record_key, station_id) DO NOTHING
RETURNING id;

-- name: DeactivateLinks :exec
UPDATE quality_links
SET active = FALSE
WHERE revision_id = @revision_id AND record_key = @record_key AND active;

-- name: GetPublishedRevisionByEdition :one
SELECT id, edition_id, status, supersedes_revision_id, published_at, created_at
FROM quality_revisions
WHERE edition_id = @edition_id AND status = 'published'
ORDER BY published_at DESC, created_at DESC
LIMIT 1;

-- name: SetCurrentRevision :exec
INSERT INTO quality_current_revisions (source, stream, revision_id)
VALUES (@source, @stream, @revision_id)
ON CONFLICT (source, stream)
DO UPDATE SET revision_id = EXCLUDED.revision_id, switched_at = now();

-- name: GetCurrentRevision :one
SELECT revision_id FROM quality_current_revisions
WHERE source = @source AND stream = @stream;

-- name: StageEnforcementEvent :one
INSERT INTO quality_enforcement_events
    (id, revision_id, native_key, scope, status, subject_cnpj, occurred_on,
     as_of, evidence_ref)
VALUES (@id, @revision_id, @native_key, @scope, @status, @subject_cnpj,
    @occurred_on, @as_of, @evidence_ref)
ON CONFLICT (revision_id, native_key) DO NOTHING
RETURNING id;

-- name: CountRevisionEnforcement :one
SELECT count(*) FROM quality_enforcement_events WHERE revision_id = @revision_id;

-- name: ListRevisionEnforcement :many
SELECT id, revision_id, native_key, scope, status, subject_cnpj, occurred_on,
    as_of, evidence_ref
FROM quality_enforcement_events
WHERE revision_id = @revision_id
ORDER BY native_key ASC, id ASC;

-- name: StageSanctionEvent :one
INSERT INTO quality_sanction_events
    (id, revision_id, native_key, proceeding_ref, party_cnpj, amount_minor,
     status, decided_on, as_of, evidence_ref)
VALUES (@id, @revision_id, @native_key, @proceeding_ref, @party_cnpj,
    @amount_minor, @status, @decided_on, @as_of, @evidence_ref)
ON CONFLICT (revision_id, native_key) DO NOTHING
RETURNING id;

-- name: CountRevisionSanctions :one
SELECT count(*) FROM quality_sanction_events WHERE revision_id = @revision_id;

-- name: ListRevisionSanctions :many
SELECT id, revision_id, native_key, proceeding_ref, party_cnpj, amount_minor,
    status, decided_on, as_of, evidence_ref
FROM quality_sanction_events
WHERE revision_id = @revision_id
ORDER BY native_key ASC, id ASC;

-- name: UpsertPeriod :exec
INSERT INTO quality_periods (edition_id, period_label, period_start, period_end)
VALUES (@edition_id, @period_label, @period_start, @period_end)
ON CONFLICT (edition_id, period_label)
DO UPDATE SET period_start = EXCLUDED.period_start, period_end = EXCLUDED.period_end;

-- name: ListEditionPeriods :many
SELECT edition_id, period_label, period_start, period_end
FROM quality_periods
WHERE edition_id = @edition_id
ORDER BY period_start ASC, period_label ASC;
