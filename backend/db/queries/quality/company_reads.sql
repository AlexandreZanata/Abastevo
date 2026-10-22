-- Company-scoped facts use only a separately resolved current-company scope.
-- No source CNPJ leaves these private ports; Directory verification/locking is
-- root-composed and never implemented through cross-module SQL here.

-- name: CompanyStationCandidates :many
SELECT DISTINCT v.party_cnpj::text AS cnpj
FROM quality_company_associations a
JOIN quality_company_association_revisions c ON c.revision_id=a.revision_id
JOIN quality_sanction_events v ON v.revision_id=a.revision_id AND v.native_key=a.record_key
WHERE a.revision_id = @revision_id AND a.station_id = @station_id AND a.resolution_kind='bound'
 AND c.policy_version='current-company-v1' AND c.association_scope='CURRENT_COMPANY_ONLY'
 AND NOT c.historical_business_window_known
 AND v.source_contract!='sanction-prepared-8-v1'
ORDER BY cnpj;

-- name: CompanySanctionPage :many
SELECT v.id, v.native_key, v.proceeding_ref, v.party_cnpj, v.amount_minor, v.status, v.decided_on, v.as_of, v.evidence_ref, v.source_contract, v.extraction_month, v.report_year, v.duf_ref, v.department, v.due_on, v.amount_text, v.paid_text, v.paid_minor, v.history_revision_id, v.reference_label, v.reference_ref, v.source_transit_on, v.unlabelled_field
FROM quality_company_associations a
JOIN quality_company_association_revisions c ON c.revision_id=a.revision_id
JOIN quality_sanction_events v ON v.revision_id=a.revision_id AND v.native_key=a.record_key
WHERE a.revision_id = @revision_id AND a.station_id = @station_id AND a.resolution_kind='bound'
 AND c.policy_version='current-company-v1' AND c.association_scope='CURRENT_COMPANY_ONLY'
 AND NOT c.historical_business_window_known
 AND v.source_contract!='sanction-prepared-8-v1'
 AND v.party_cnpj=ANY(@allowed_cnpjs::text[])
 AND (@status::text = '' OR v.status = @status)
 AND ((a.record_key,v.id::text) > (@after_native::text,@after_id::text) OR @after_id::text = '')
ORDER BY a.record_key ASC,v.id::text ASC LIMIT @limit_plus_one::int;

-- name: CompanySanctionByID :one
SELECT v.id, v.revision_id, v.native_key, v.proceeding_ref, v.party_cnpj, v.amount_minor, v.status, v.decided_on, v.as_of, v.evidence_ref, v.source_contract, v.extraction_month, v.report_year, v.duf_ref, v.department, v.due_on, v.amount_text, v.paid_text, v.paid_minor, v.history_revision_id, v.reference_label, v.reference_ref, v.source_transit_on, v.unlabelled_field
FROM quality_company_associations a
JOIN quality_company_association_revisions c ON c.revision_id=a.revision_id
JOIN quality_sanction_events v ON v.revision_id=a.revision_id AND v.native_key=a.record_key
WHERE a.revision_id = @revision_id AND a.station_id = @station_id AND a.resolution_kind='bound'
 AND c.policy_version='current-company-v1' AND c.association_scope='CURRENT_COMPANY_ONLY'
 AND NOT c.historical_business_window_known
 AND v.source_contract!='sanction-prepared-8-v1'
 AND v.party_cnpj=ANY(@allowed_cnpjs::text[]) AND v.id = @id;

-- name: LockCompanyReadRevision :one
SELECT r.id FROM quality_revisions r
JOIN quality_editions e ON e.id=r.edition_id
WHERE r.id=sqlc.arg(revision_id)::uuid AND e.source='sanctions' AND e.stream='sanction-v1'
AND r.status IN ('published','superseded')
FOR SHARE OF r;
