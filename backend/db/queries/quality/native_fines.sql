-- Native observations have their own staging operation. A shared revision lock
-- fences the staging-state check against publication; no published/foreign stream
-- can gain new facts through this operation. Runtime composition follows separately.
-- name: StageNativeFineObservation :one
WITH staging AS MATERIALIZED (
 SELECT r.id FROM quality_revisions r
 JOIN quality_editions e ON e.id = r.edition_id
 WHERE r.id = @revision_id AND r.status = 'staging'
  AND e.source = 'sanctions' AND e.stream = 'sanction-v1'
 FOR SHARE OF r
)
INSERT INTO quality_sanction_events
 (id, revision_id, native_key, proceeding_ref, party_cnpj, amount_minor,
  status, evidence_ref, source_contract, extraction_month, report_year,
  duf_ref, department, due_on, amount_text, paid_text, paid_minor)
SELECT @id, staging.id, @native_key, @proceeding_ref, @party_cnpj,
 @amount_minor::bigint, @status, @evidence_ref, 'anp-sicom-cp850-9-v1',
 @extraction_month::text, @report_year::int, @duf_ref::text,
 @department::text, @due_on::date, @amount_text::text, @paid_text::text,
 @paid_minor::bigint
FROM staging
ON CONFLICT (revision_id, native_key) DO NOTHING
RETURNING id;

-- name: RecordNativeFineClock :execrows
INSERT INTO quality_native_fine_revisions(revision_id,report_year,extraction_month)
SELECT r.id,@report_year::int,@extraction_month::text
FROM quality_revisions r JOIN quality_editions e ON e.id = r.edition_id
WHERE r.id= @revision_id AND r.status = 'staging'
 AND e.source = 'sanctions' AND e.stream = 'sanction-v1'
FOR SHARE OF r;

-- name: NativeFineRevisionClock :one
SELECT report_year,extraction_month FROM quality_native_fine_revisions
WHERE revision_id= @revision_id;
