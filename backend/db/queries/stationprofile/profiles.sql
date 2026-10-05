-- Owned by stationprofile (P30-T02). The profile is a read-only
-- public projection over the canonical Directory station: no second
-- identity, no private proof/person data, no grant implication.
-- Operator revisions link station to effective CNPJ with validity;
-- only one open revision per station (partial unique index).

-- name: EnsureUnclaimedProfile :one
INSERT INTO station_profiles (station_id, policy_version, projection)
VALUES (@station_id, @policy_version, '{}')
ON CONFLICT (station_id) DO NOTHING
RETURNING station_id, policy_version, projection, revision, updated_at;

-- name: GetProfile :one
SELECT station_id, policy_version, projection, revision, updated_at
FROM station_profiles
WHERE station_id = @station_id;

-- name: RecordOperatorRevision :one
INSERT INTO station_operator_revisions (id, station_id, cnpj, source, source_reference)
VALUES (@id, @station_id, @cnpj, @source, @source_reference)
RETURNING id, station_id, cnpj, source, source_reference, valid_from, valid_to;

-- name: CloseOperatorRevision :execrows
UPDATE station_operator_revisions
SET valid_to = now()
WHERE id = @id AND valid_to IS NULL;

-- name: CurrentOperatorRevision :one
SELECT id, station_id, cnpj, source, source_reference, valid_from, valid_to
FROM station_operator_revisions
WHERE station_id = @station_id AND valid_to IS NULL
ORDER BY valid_from DESC
LIMIT 1;
