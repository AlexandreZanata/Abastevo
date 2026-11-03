-- SNA-02B: all source/correction access remains Directory-owned.
-- name: LockStationFacts :one
SELECT s.id,s.display_name,s.address,s.legal_name,s.trade_name,s.address_source,
 s.facts_source_clock,s.facts_source_checksum,
 EXISTS(SELECT 1 FROM registry_assertions a WHERE a.station_id=s.id
  AND a.display_name=s.display_name AND a.source IN ('registry-csv','registry-api','station-prep')
  AND EXISTS(SELECT 1 FROM registry_source_runs r WHERE r.state='complete' AND r.source=a.source
   AND (r.id=a.run_id OR EXISTS(SELECT 1 FROM registry_run_assertions m WHERE m.assertion_id=a.id AND m.run_id=r.id))
   AND (a.source<>'station-prep' OR (a.latitude IS NULL AND a.longitude IS NULL AND jsonb_typeof(a.address)='object' AND a.address ? 'municipio_ibge' AND a.address ? 'uf' AND r.prepared_manifest_sha256 <> '' AND r.snapshot_identity LIKE 'station-prep:%:registry-13col')))) AS source_name_owned,
 EXISTS(SELECT 1 FROM registry_assertions a WHERE a.station_id=s.id
  AND a.address=s.address AND a.source IN ('registry-csv','registry-api','station-prep')
  AND EXISTS(SELECT 1 FROM registry_source_runs r WHERE r.state='complete' AND r.source=a.source
   AND (r.id=a.run_id OR EXISTS(SELECT 1 FROM registry_run_assertions m WHERE m.assertion_id=a.id AND m.run_id=r.id))
   AND (a.source<>'station-prep' OR (a.latitude IS NULL AND a.longitude IS NULL AND jsonb_typeof(a.address)='object' AND a.address ? 'municipio_ibge' AND a.address ? 'uf' AND r.prepared_manifest_sha256 <> '' AND r.snapshot_identity LIKE 'station-prep:%:registry-13col')))) AS source_address_owned
FROM directory_stations s WHERE s.id= @id FOR UPDATE;

-- name: SourceStationFacts :one
SELECT a.source_key,a.checksum,a.display_name,a.legal_name,a.trade_name,
 a.address,a.address_source,r.finished_at AS source_clock
FROM registry_assertions a
JOIN LATERAL (
 SELECT run.finished_at FROM registry_source_runs run
 WHERE run.state='complete' AND run.source=a.source AND run.finished_at IS NOT NULL
 AND (run.id=a.run_id OR EXISTS(SELECT 1 FROM registry_run_assertions m WHERE m.assertion_id=a.id AND m.run_id=run.id))
 AND (a.source IN ('registry-csv','registry-api') OR (a.source='station-prep' AND a.latitude IS NULL AND a.longitude IS NULL AND jsonb_typeof(a.address)='object' AND a.address ? 'municipio_ibge' AND a.address ? 'uf' AND run.prepared_manifest_sha256 <> '' AND run.snapshot_identity LIKE 'station-prep:%:registry-13col'))
 ORDER BY run.finished_at DESC LIMIT 1
) r ON TRUE
JOIN directory_identifiers i ON i.station_id= @station_id AND i.kind='CNPJ'
 AND i.normalized_value=a.source_key AND i.valid_to IS NULL
WHERE a.id= @assertion_id AND a.station_id= @station_id AND a.superseded_by IS NULL;

-- name: AcceptedStationName :one
SELECT COALESCE(c.proposed_value->>'value','')::text AS value
FROM station_corrections c
WHERE c.station_id= @station_id AND c.field='display_name' AND c.status='accepted'
 AND c.decided_at IS NOT NULL AND jsonb_typeof(c.proposed_value->'value')='string'
 AND btrim(c.proposed_value->>'value')<>''
 AND EXISTS(SELECT 1 FROM station_correction_decisions d WHERE d.correction_id=c.id AND d.decision='accepted')
ORDER BY c.decided_at DESC,c.id DESC LIMIT 1;

-- name: UpdateStationSourceFacts :exec
UPDATE directory_stations SET legal_name= @legal_name,trade_name= @trade_name,
 display_name= @display_name,address= @address::jsonb,address_source= @address_source,
 facts_source_clock= @source_clock,facts_source_checksum= @source_checksum
WHERE id= @id;

-- name: UpdateStationAcceptedName :exec
UPDATE directory_stations SET display_name= @display_name WHERE id= @id;

-- name: CreateStationFactReprojectionCampaign :one
INSERT INTO directory_station_fact_reprojection_campaigns(campaign_id,profile_version,operator_ref,reason)
VALUES(@campaign_id,1,@operator_ref,@reason)
ON CONFLICT(campaign_id) DO NOTHING RETURNING campaign_id,profile_version,operator_ref,reason,created_at;

-- name: GetStationFactReprojectionCampaign :one
SELECT campaign_id,profile_version,operator_ref,reason,created_at
FROM directory_station_fact_reprojection_campaigns WHERE campaign_id= @campaign_id;

-- name: ListStationFactReprojectionPage :many
SELECT s.id FROM directory_stations s
WHERE s.id> @after_id AND EXISTS(
 SELECT 1 FROM registry_assertions a JOIN directory_identifiers i
 ON i.station_id=s.id AND i.kind='CNPJ' AND i.valid_to IS NULL AND i.normalized_value=a.source_key
 WHERE a.station_id=s.id AND a.superseded_by IS NULL
 AND EXISTS(SELECT 1 FROM registry_source_runs r WHERE r.state='complete' AND r.source=a.source
  AND (r.id=a.run_id OR EXISTS(SELECT 1 FROM registry_run_assertions m WHERE m.assertion_id=a.id AND m.run_id=r.id))
  AND (a.source IN ('registry-csv','registry-api') OR (a.source='station-prep' AND a.latitude IS NULL AND a.longitude IS NULL AND jsonb_typeof(a.address)='object' AND a.address ? 'municipio_ibge' AND a.address ? 'uf' AND r.prepared_manifest_sha256<>'' AND r.snapshot_identity LIKE 'station-prep:%:registry-13col'))))
ORDER BY s.id LIMIT @page_limit::int;

-- name: LatestStationFactAssertion :one
SELECT a.id FROM registry_assertions a
JOIN directory_identifiers i ON i.station_id= @station_id AND i.kind='CNPJ' AND i.valid_to IS NULL AND i.normalized_value=a.source_key
JOIN LATERAL(
 SELECT r.finished_at FROM registry_source_runs r WHERE r.state='complete' AND r.source=a.source AND r.finished_at IS NOT NULL
 AND (r.id=a.run_id OR EXISTS(SELECT 1 FROM registry_run_assertions m WHERE m.assertion_id=a.id AND m.run_id=r.id))
 AND (a.source IN ('registry-csv','registry-api') OR (a.source='station-prep' AND a.latitude IS NULL AND a.longitude IS NULL AND jsonb_typeof(a.address)='object' AND a.address ? 'municipio_ibge' AND a.address ? 'uf' AND r.prepared_manifest_sha256<>'' AND r.snapshot_identity LIKE 'station-prep:%:registry-13col'))
 ORDER BY r.finished_at DESC LIMIT 1
) r ON TRUE
WHERE a.station_id= @station_id AND a.superseded_by IS NULL
ORDER BY r.finished_at DESC,a.checksum DESC LIMIT 1;
