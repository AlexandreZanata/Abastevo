-- Owned by quality (FQ-08 official reads). All reads resolve the current
-- published or explicitly selected historical revision and scope every row to
-- it, so one request sees
-- one frozen lease even across concurrent publications. No read exposes
-- source CNPJs or internal notes; station scoping for assays flows only
-- through active links. Events page per stream without station filter:
-- station attribution for events needs resolved links, owed separately.

-- name: LeaseByStream :one
SELECT r.id AS revision_id, r.status, r.published_at, r.supersedes_revision_id,
    e.id AS edition_id, e.source, e.stream, e.edition, e.predecessor,
    COALESCE(m.input,0)::bigint AS input, COALESCE(m.accepted,0)::bigint AS accepted,
    COALESCE(m.duplicates,0)::bigint AS duplicates, COALESCE(m.quarantined,0)::bigint AS quarantined,
    COALESCE(m.manifest_hash,'')::text AS manifest_hash, m.provenance_state, true::boolean AS is_current_stream
FROM quality_current_revisions AS c
JOIN quality_revisions AS r ON r.id = c.revision_id
JOIN quality_editions AS e ON e.id = r.edition_id
JOIN quality_revision_provenance AS m ON m.revision_id = r.id
WHERE c.source = @source AND c.stream = @stream;

-- name: RevisionLeaseByStream :one
SELECT r.id AS revision_id, r.status, r.published_at, r.supersedes_revision_id,
    e.id AS edition_id, e.source, e.stream, e.edition, e.predecessor,
    COALESCE(m.input,0)::bigint AS input, COALESCE(m.accepted,0)::bigint AS accepted,
    COALESCE(m.duplicates,0)::bigint AS duplicates, COALESCE(m.quarantined,0)::bigint AS quarantined,
    COALESCE(m.manifest_hash,'')::text AS manifest_hash, m.provenance_state, EXISTS(SELECT 1 FROM quality_current_revisions c WHERE c.revision_id=r.id)::boolean AS is_current_stream
FROM quality_revisions AS r
JOIN quality_editions AS e ON e.id = r.edition_id
JOIN quality_revision_provenance AS m ON m.revision_id = r.id
WHERE e.source = @source AND e.stream = @stream AND r.id= @revision_id
 AND r.status IN ('published','superseded','withdrawn');

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
    a.result_text, a.unit, a.conformity, a.evidence_ref, a.source_observed_at
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
    a.result_text, a.unit, a.conformity, a.evidence_ref, a.source_observed_at
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

-- name: RevisionCatalog :many
SELECT r.id,r.status,r.published_at,r.supersedes_revision_id,e.edition,m.provenance_state,
 EXISTS(SELECT 1 FROM quality_current_revisions c WHERE c.revision_id=r.id)::boolean AS is_current_stream
FROM quality_revisions r JOIN quality_editions e ON e.id=r.edition_id
JOIN quality_revision_provenance m ON m.revision_id=r.id
WHERE e.source= @source AND e.stream= @stream
AND r.status IN ('published','superseded','withdrawn') AND r.id> @after_id
ORDER BY r.id LIMIT @limit_plus_one::int;

-- name: ReadRevisionResources :many
SELECT url,sha256,bytes,member_name,member_sha256,member_bytes FROM quality_revision_resources WHERE revision_id= @revision_id ORDER BY url;
