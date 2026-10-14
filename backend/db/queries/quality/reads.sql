-- Owned by quality (FQ-08 official reads). All reads resolve the current
-- published revision first and scope every row to it, so one request sees
-- one frozen lease even across concurrent publications. No read exposes
-- source CNPJs or internal notes; station scoping for assays flows only
-- through active links. Events page per stream without station filter:
-- station attribution for events needs resolved links, owed separately.

-- name: LeaseByStream :one
SELECT r.id AS revision_id, r.status, r.published_at,
    e.id AS edition_id, e.source, e.stream, e.edition, e.predecessor,
    m.input, m.accepted, m.duplicates, m.quarantined, m.manifest_hash
FROM quality_current_revisions AS c
JOIN quality_revisions AS r ON r.id = c.revision_id
JOIN quality_editions AS e ON e.id = r.edition_id
JOIN quality_manifests AS m ON m.edition_id = e.id
WHERE c.source = @source AND c.stream = @stream;

-- name: SummaryByRevision :many
SELECT a.fuel, a.conformity, count(*) AS n, max(a.observed_on)::date AS last_observed
FROM quality_assays AS a
JOIN quality_links AS l ON l.revision_id = a.revision_id
    AND l.active AND l.station_id = @station_id
    AND l.record_key = @stream || ':' || a.sample_ref
WHERE a.revision_id = @revision_id
    AND (@fuel::text = '' OR a.fuel = @fuel)
GROUP BY a.fuel, a.conformity
ORDER BY a.fuel ASC, a.conformity ASC;

-- name: AssayPageByRevision :many
SELECT a.id, a.sample_ref, a.assay_ref, a.observed_on, a.fuel,
    a.result_text, a.unit, a.conformity, a.evidence_ref
FROM quality_assays AS a
JOIN quality_links AS l ON l.revision_id = a.revision_id
    AND l.active AND l.station_id = @station_id
    AND l.record_key = @stream || ':' || a.sample_ref
WHERE a.revision_id = @revision_id
    AND (@fuel::text = '' OR a.fuel = @fuel)
    AND ((a.observed_on, a.id::text) > (@after_date::date, @after_id::text)
        OR @after_id::text = '')
ORDER BY a.observed_on ASC, a.id::text ASC
LIMIT @limit_plus_one::int;

-- name: SampleAssaysByRevision :many
SELECT a.id, a.sample_ref, a.assay_ref, a.observed_on, a.fuel,
    a.result_text, a.unit, a.conformity, a.evidence_ref
FROM quality_assays AS a
WHERE a.revision_id = @revision_id AND a.sample_ref = @sample_ref
ORDER BY a.assay_ref ASC, a.id::text ASC;

-- name: SampleLinksByRevision :many
SELECT station_id, automatic, reason, active
FROM quality_links
WHERE revision_id = @revision_id AND record_key = @record_key
ORDER BY active DESC;

-- name: ActionPageByRevision :many
SELECT id, native_key, scope, status, subject_cnpj, occurred_on, as_of, evidence_ref
FROM quality_enforcement_events
WHERE revision_id = @revision_id
    AND (@status::text = '' OR status = @status)
    AND (@scope::text = '' OR scope = @scope)
    AND ((native_key, id::text) > (@after_native::text, @after_id::text)
        OR @after_id::text = '')
ORDER BY native_key ASC, id::text ASC
LIMIT @limit_plus_one::int;

-- name: SanctionPageByRevision :many
SELECT id, native_key, proceeding_ref, party_cnpj, amount_minor, status,
    decided_on, as_of, evidence_ref
FROM quality_sanction_events
WHERE revision_id = @revision_id
    AND (@status::text = '' OR status = @status)
    AND ((native_key, id::text) > (@after_native::text, @after_id::text)
        OR @after_id::text = '')
ORDER BY native_key ASC, id::text ASC
LIMIT @limit_plus_one::int;

-- name: EnforcementByID :one
SELECT id, revision_id, native_key, scope, status, subject_cnpj, occurred_on,
    as_of, evidence_ref
FROM quality_enforcement_events
WHERE id = @id AND revision_id = @revision_id;

-- name: SanctionByID :one
SELECT id, revision_id, native_key, proceeding_ref, party_cnpj, amount_minor,
    status, decided_on, as_of, evidence_ref
FROM quality_sanction_events
WHERE id = @id AND revision_id = @revision_id;
