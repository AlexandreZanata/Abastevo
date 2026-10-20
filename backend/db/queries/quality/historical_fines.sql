-- name: RecordHistoricalFineContext :one
WITH staging AS MATERIALIZED (
 SELECT r.id FROM quality_revisions r JOIN quality_editions e ON e.id=r.edition_id
 WHERE r.id = @revision_id AND r.status='staging' AND e.source='sanctions' AND e.stream='sanction-v1' AND e.edition = @source_period
 FOR SHARE OF r
)
INSERT INTO quality_historical_fine_revisions(revision_id,input_contract,source_period,source_file,header_record,source_clock_precision,source_clock_value,source_record_count,metadata_count)
SELECT id,@input_contract,@source_period,@source_file,@header_record::int,@source_clock_precision,@source_clock_value,@source_record_count::int,@metadata_count::int FROM staging
ON CONFLICT DO NOTHING RETURNING revision_id;

-- name: StageHistoricalFineObservation :one
WITH staging AS MATERIALIZED (
 SELECT r.id,c.input_contract FROM quality_revisions r
 JOIN quality_editions e ON e.id=r.edition_id
 JOIN quality_historical_fine_revisions c ON c.revision_id=r.id AND c.source_period=e.edition
 WHERE r.id = @revision_id AND r.status='staging' AND e.source='sanctions' AND e.stream='sanction-v1'
  AND quality_history_record(@evidence_ref,c.source_file) BETWEEN c.header_record+1 AND c.header_record+c.source_record_count
 FOR SHARE OF r
)
INSERT INTO quality_sanction_events(id,revision_id,native_key,proceeding_ref,party_cnpj,amount_minor,status,evidence_ref,
 source_contract,history_revision_id,reference_label,reference_ref,source_transit_on,unlabelled_field,
 department,due_on,amount_text,paid_text,paid_minor)
SELECT @id,staging.id,@native_key,@proceeding_ref,@party_cnpj,@amount_minor::bigint,@status,@evidence_ref,
 staging.input_contract,staging.id,@reference_label,@reference_ref,sqlc.narg(source_transit_on)::date,@unlabelled_field,
 @department,@due_on::date,@amount_text,@paid_text,sqlc.narg(paid_minor)::bigint FROM staging
ON CONFLICT DO NOTHING RETURNING id;

-- name: StageHistoricalFineNote :one
WITH staging AS MATERIALIZED (
 SELECT r.id,c.source_period FROM quality_revisions r
 JOIN quality_editions e ON e.id=r.edition_id
 JOIN quality_historical_fine_revisions c ON c.revision_id=r.id AND c.source_period=e.edition
 WHERE r.id = @revision_id AND r.status='staging' AND e.source='sanctions' AND e.stream='sanction-v1'
  AND quality_history_record(@evidence_ref,c.source_file) BETWEEN c.header_record+1 AND c.header_record+c.source_record_count
 FOR SHARE OF r
)
INSERT INTO quality_historical_fine_notes(revision_id,source_period,evidence_ref,kind,note_text)
SELECT id,source_period,@evidence_ref,@kind,@note_text FROM staging
ON CONFLICT(revision_id,evidence_ref) DO NOTHING RETURNING evidence_ref;

-- name: HistoricalFineContext :one
SELECT * FROM quality_historical_fine_revisions WHERE revision_id = @revision_id;

-- name: HistoricalFineNotes :many
SELECT evidence_ref,kind,note_text FROM quality_historical_fine_notes WHERE revision_id = @revision_id ORDER BY evidence_ref;
