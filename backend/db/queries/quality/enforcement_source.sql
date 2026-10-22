-- name: RecordEnforcementSourceContext :execrows
INSERT INTO quality_enforcement_source_revisions
(revision_id,source_profile,source_file,date_system,input_count,accepted_count,duplicate_count,first_document_day,last_document_day)
SELECT r.id, @source_profile::text,@source_file::text,@date_system::text,@input_count::int,@accepted_count::int,@duplicate_count::int,@first_document_day::date,@last_document_day::date
FROM quality_revisions r JOIN quality_editions e ON e.id = r.edition_id
WHERE r.id = @revision_id AND r.status = 'staging' AND e.source = 'enforcement' AND e.stream = 'enforcement-v1'
FOR SHARE OF r;

-- Bounded parameterized chunks; the revision lock lives through the transaction.
-- name: StageEnforcementSourceRecords :execrows
WITH staging AS MATERIALIZED (
 SELECT r.id,c.first_document_day,c.last_document_day FROM quality_revisions r
 JOIN quality_editions e ON e.id = r.edition_id
 JOIN quality_enforcement_source_revisions c ON c.revision_id = r.id
 WHERE r.id = @revision_id AND r.status = 'staging' AND e.source = 'enforcement' AND e.stream = 'enforcement-v1'
 FOR SHARE OF r
)
INSERT INTO quality_enforcement_source_records
(id,revision_id,native_key,subject_cnpj,subject_kind,uf,municipality,segment,document_day,document_number,procedure,result,evidence_ref,fingerprint,first_document_day,last_document_day)
SELECT x.id,s.id,x.native_key,nullif(x.subject_cnpj,''),x.subject_kind,x.uf,x.municipality,x.segment,x.document_day,x.document_number,x.procedure,x.result,x.evidence_ref,x.fingerprint,s.first_document_day,s.last_document_day
FROM staging s CROSS JOIN jsonb_to_recordset(@records::jsonb) AS x(id uuid,native_key text,subject_cnpj text,subject_kind text,uf text,municipality text,segment text,document_day date,document_number text,procedure text,result text,evidence_ref text,fingerprint text);

-- name: EnforcementSourceContext :one
SELECT * FROM quality_enforcement_source_revisions WHERE revision_id = @revision_id;

-- name: CountEnforcementSourceRecords :one
SELECT count(*) FROM quality_enforcement_source_records WHERE revision_id = @revision_id;

-- name: EnforcementSourcePage :many
SELECT id,native_key,subject_kind,uf,municipality,segment,document_day,document_number,procedure,result,evidence_ref
FROM quality_enforcement_source_records
WHERE revision_id = @revision_id AND ((native_key,id::text) > (@after_native::text,@after_id::text) OR @after_id::text = '')
ORDER BY native_key,id::text LIMIT @limit_plus_one::int;

-- name: EnforcementSourceByID :one
SELECT id,native_key,subject_kind,uf,municipality,segment,document_day,document_number,procedure,result,evidence_ref
FROM quality_enforcement_source_records WHERE revision_id = @revision_id AND id = @id;

-- name: EnforcementSourceFilteredPage :many
SELECT id,native_key,subject_kind,uf,municipality,segment,document_day,document_number,procedure,result,evidence_ref
FROM quality_enforcement_source_records
WHERE revision_id = @revision_id
 AND (uf = sqlc.narg(uf)::text OR sqlc.narg(uf)::text IS NULL)
 AND (municipality = sqlc.narg(municipality)::text OR sqlc.narg(municipality)::text IS NULL)
 AND (segment = sqlc.narg(segment)::text OR sqlc.narg(segment)::text IS NULL)
 AND (subject_kind = sqlc.narg(subject_kind)::text OR sqlc.narg(subject_kind)::text IS NULL)
 AND (document_day >= sqlc.narg(from_day)::date OR sqlc.narg(from_day)::date IS NULL)
 AND (document_day <= sqlc.narg(to_day)::date OR sqlc.narg(to_day)::date IS NULL)
 AND ((native_key,id::text) > (@after_native::text,@after_id::text) OR @after_id::text = '')
ORDER BY native_key,id::text LIMIT @limit_plus_one::int;

-- name: EnforcementSourceCoverage :one
SELECT count(*) AS observations,
 count(DISTINCT document_number) AS cases,
 min(document_day) AS first_day,
 max(document_day) AS last_day
FROM quality_enforcement_source_records WHERE revision_id = @revision_id;
