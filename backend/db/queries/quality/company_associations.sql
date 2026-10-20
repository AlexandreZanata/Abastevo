-- Private scoped storage ports, not public station attribution. Runtime must
-- additionally fence current full-CNPJ bindings via Directory's owned port.
-- Both stages lock/recheck revision state through publication/withdrawal.

-- name: RecordCompanyAssociationContext :one
WITH staging AS MATERIALIZED (
 SELECT r.id FROM quality_revisions r JOIN quality_editions e ON e.id=r.edition_id
 WHERE r.id= @revision_id AND r.status='staging' AND e.source='sanctions' AND e.stream='sanction-v1'
  AND (EXISTS(SELECT 1 FROM quality_native_fine_revisions n WHERE n.revision_id=r.id)
    OR EXISTS(SELECT 1 FROM quality_historical_fine_revisions h WHERE h.revision_id=r.id))
  AND (SELECT count(*) FROM quality_sanction_events v WHERE v.revision_id=r.id)= @observation_count::int
  AND NOT EXISTS(SELECT 1 FROM quality_sanction_events v WHERE v.revision_id=r.id AND v.source_contract='sanction-prepared-8-v1')
 FOR SHARE OF r
)
INSERT INTO quality_company_association_revisions(revision_id,policy_version,association_scope,decision_day,historical_business_window_known,observation_count)
SELECT id,@policy_version,@association_scope,@decision_day::date,@historical_business_window_known::bool,@observation_count::int FROM staging
ON CONFLICT DO NOTHING RETURNING revision_id;

-- name: StageCompanyAssociation :one
WITH staging AS MATERIALIZED (
 SELECT r.id FROM quality_revisions r JOIN quality_editions e ON e.id=r.edition_id
 JOIN quality_company_association_revisions c ON c.revision_id=r.id
 JOIN quality_sanction_events v ON v.revision_id=r.id AND v.native_key= @record_key
 WHERE r.id= @revision_id AND r.status='staging' AND e.source='sanctions' AND e.stream='sanction-v1'
  AND v.source_contract!='sanction-prepared-8-v1'
 FOR SHARE OF r
)
INSERT INTO quality_company_associations(revision_id,record_key,resolution_kind,reason,station_id)
SELECT id,@record_key,@resolution_kind,@reason,sqlc.narg(station_id)::uuid FROM staging
ON CONFLICT DO NOTHING RETURNING record_key;

-- name: CompanyAssociationContext :one
SELECT * FROM quality_company_association_revisions WHERE revision_id= @revision_id;

-- name: CompanyAssociationDecisions :many
SELECT a.record_key,a.resolution_kind,a.reason,a.station_id,v.party_cnpj
FROM quality_company_associations a
JOIN quality_sanction_events v ON v.revision_id=a.revision_id AND v.native_key=a.record_key
WHERE a.revision_id= @revision_id ORDER BY a.record_key;

-- name: CompanyAssociationCounts :one
SELECT count(*) AS total,
 count(*) FILTER(WHERE resolution_kind='bound') AS bound_count,
 count(*) FILTER(WHERE resolution_kind='unmatched') AS unmatched_count,
 count(*) FILTER(WHERE resolution_kind='ambiguous') AS ambiguous_count,
 count(*) FILTER(WHERE resolution_kind='wrong_branch') AS wrong_branch_count,
 count(*) FILTER(WHERE resolution_kind='succession_review') AS succession_count
FROM quality_company_associations WHERE revision_id= @revision_id;
