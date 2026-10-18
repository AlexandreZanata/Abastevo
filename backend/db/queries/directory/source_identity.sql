-- Directory-owned read-only source identity; no business validity inference.
-- Two canonical IDs suffice to prove ambiguity, regardless of retained row count.
-- name: SourceExactIdentities :many
SELECT s.id AS station_id, s.status='active' AS station_active,
 EXISTS(SELECT 1 FROM directory_identifiers active
   WHERE active.kind='CNPJ' AND active.normalized_value=sqlc.arg(cnpj) AND active.station_id=s.id AND active.valid_to IS NULL) AS is_current
FROM (SELECT DISTINCT station_id FROM directory_identifiers
 WHERE kind='CNPJ' AND normalized_value=sqlc.arg(cnpj) ORDER BY station_id LIMIT 2) candidates
JOIN directory_stations s ON s.id=candidates.station_id ORDER BY s.id;

-- name: SourceHasOtherBranch :one
SELECT EXISTS(SELECT 1 FROM directory_identifiers
 WHERE kind='CNPJ' AND normalized_value LIKE @base::text || '%' AND normalized_value<>sqlc.arg(cnpj) LIMIT 1);

-- name: LockSourceBindings :many
SELECT i.normalized_value AS cnpj, s.id AS station_id
FROM unnest(@cnpjs::text[]) WITH ORDINALITY expected(cnpj,ordinal)
JOIN unnest(@station_ids::uuid[]) WITH ORDINALITY stations(station_id,ordinal) ON stations.ordinal=expected.ordinal
JOIN directory_identifiers i ON i.kind='CNPJ' AND i.normalized_value=expected.cnpj AND i.valid_to IS NULL
JOIN directory_stations s ON s.id=i.station_id AND s.id=stations.station_id AND s.status='active'
ORDER BY i.normalized_value,s.id FOR SHARE OF i,s;

-- name: SourceBoundHistory :many
SELECT expected.cnpj::text AS cnpj,candidates.station_id
FROM unnest(@cnpjs::text[]) expected(cnpj)
CROSS JOIN LATERAL(SELECT DISTINCT station_id FROM directory_identifiers
 WHERE kind='CNPJ' AND normalized_value=expected.cnpj ORDER BY station_id LIMIT 2) candidates
ORDER BY expected.cnpj,candidates.station_id;

-- name: SourceEvidenceBatch :many
-- Bounded set-based evidence for revalidation sweeps: same two-candidate
-- policy as SourceExactIdentities, one round trip per chunk.
SELECT expected.cnpj::text AS cnpj, candidates.station_id,
 s.status='active' AS station_active,
 EXISTS(SELECT 1 FROM directory_identifiers active
   WHERE active.kind='CNPJ' AND active.normalized_value=expected.cnpj
     AND active.station_id=candidates.station_id AND active.valid_to IS NULL) AS is_current
FROM unnest(@cnpjs::text[]) expected(cnpj)
JOIN LATERAL(SELECT DISTINCT station_id FROM directory_identifiers
  WHERE kind='CNPJ' AND normalized_value=expected.cnpj ORDER BY station_id LIMIT 2) candidates ON TRUE
JOIN directory_stations s ON s.id=candidates.station_id
ORDER BY expected.cnpj,candidates.station_id;
