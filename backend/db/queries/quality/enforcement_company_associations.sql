-- Private scoped storage ports for enforcement company associations, not
-- public station attribution. Runtime must additionally fence current
-- full-CNPJ bindings via Directory's owned port. Both stages lock/recheck
-- revision state through publication/withdrawal.

-- name: RecordEnforcementCompanyAssociationContext :one
WITH staging AS MATERIALIZED (
 SELECT r.id FROM quality_revisions r JOIN quality_editions e ON e.id=r.edition_id
 WHERE r.id= @revision_id AND r.status='staging' AND e.source='enforcement' AND e.stream='enforcement-v1'
  AND EXISTS(SELECT 1 FROM quality_enforcement_source_revisions n WHERE n.revision_id=r.id)
  AND (SELECT count(*) FROM quality_enforcement_source_records v WHERE v.revision_id=r.id)= @observation_count::int
 FOR SHARE OF r
)
INSERT INTO quality_enforcement_company_association_revisions(revision_id,policy_version,association_scope,decision_day,historical_business_window_known,observation_count)
SELECT id,@policy_version,@association_scope,@decision_day::date,@historical_business_window_known::bool,@observation_count::int FROM staging
ON CONFLICT DO NOTHING RETURNING revision_id;

-- name: StageEnforcementCompanyAssociation :one
WITH staging AS MATERIALIZED (
 SELECT r.id FROM quality_revisions r JOIN quality_editions e ON e.id=r.edition_id
 JOIN quality_enforcement_company_association_revisions c ON c.revision_id=r.id
 JOIN quality_enforcement_source_records v ON v.revision_id=r.id AND v.native_key= @record_key
 WHERE r.id= @revision_id AND r.status='staging' AND e.source='enforcement' AND e.stream='enforcement-v1'
 FOR SHARE OF r
)
INSERT INTO quality_enforcement_company_associations(revision_id,record_key,resolution_kind,reason,station_id)
SELECT id,@record_key,@resolution_kind,@reason,sqlc.narg(station_id)::uuid FROM staging
ON CONFLICT DO NOTHING RETURNING record_key;

-- name: EnforcementCompanyAssociationContext :one
SELECT * FROM quality_enforcement_company_association_revisions WHERE revision_id= @revision_id;

-- name: EnforcementCompanyAssociationDecisions :many
SELECT a.record_key,a.resolution_kind,a.reason,a.station_id,v.subject_cnpj
FROM quality_enforcement_company_associations a
JOIN quality_enforcement_source_records v ON v.revision_id=a.revision_id AND v.native_key=a.record_key
WHERE a.revision_id= @revision_id ORDER BY a.record_key;

-- name: EnforcementCompanyAssociationCounts :one
SELECT count(*) AS total,
 count(*) FILTER(WHERE resolution_kind='bound') AS bound_count,
 count(*) FILTER(WHERE resolution_kind='unmatched') AS unmatched_count,
 count(*) FILTER(WHERE resolution_kind='ambiguous') AS ambiguous_count,
 count(*) FILTER(WHERE resolution_kind='wrong_branch') AS wrong_branch_count,
 count(*) FILTER(WHERE resolution_kind='succession_review') AS succession_count
FROM quality_enforcement_company_associations WHERE revision_id= @revision_id;

-- name: EnforcementStationPage :many
SELECT s.id,s.display_name,s.municipality_code,s.state,s.status,
 count(a.record_key) AS record_count
FROM quality_enforcement_company_associations a
JOIN directory_stations s ON s.id = a.station_id
JOIN quality_enforcement_source_records v ON v.revision_id = a.revision_id AND v.native_key = a.record_key
WHERE a.revision_id = @revision_id
 AND a.resolution_kind = 'bound'
 AND s.status = 'active'
 AND (s.state = sqlc.narg(state)::text OR sqlc.narg(state)::text IS NULL)
 AND (v.segment = sqlc.narg(segment)::text OR sqlc.narg(segment)::text IS NULL)
 AND (v.document_day >= sqlc.narg(from_day)::date OR sqlc.narg(from_day)::date IS NULL)
 AND (v.document_day <= sqlc.narg(to_day)::date OR sqlc.narg(to_day)::date IS NULL)
 AND (s.id::text > @after_id::text OR @after_id::text = '')
GROUP BY s.id,s.display_name,s.municipality_code,s.state,s.status
ORDER BY s.id::text LIMIT @limit_plus_one::int;

-- name: EnforcementStationCoverage :one
SELECT count(DISTINCT a.station_id) AS stations,
 count(DISTINCT a.record_key) AS records
FROM quality_enforcement_company_associations a
JOIN directory_stations s ON s.id = a.station_id
WHERE a.revision_id = @revision_id
 AND a.resolution_kind = 'bound'
 AND s.status = 'active';
