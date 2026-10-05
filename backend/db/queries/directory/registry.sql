-- Owned by directory (registry staging, P25-T02). Staging only: publishers
-- read complete runs in P25-T04; failed/quarantined runs stay invisible.

-- name: CreateRegistryRun :one
INSERT INTO registry_source_runs (id, source, snapshot_identity, checksum, parser_version)
VALUES (@id, @source, @snapshot_identity, @checksum, @parser_version)
ON CONFLICT (source, snapshot_identity) DO NOTHING
RETURNING id, source, snapshot_identity, checksum, parser_version, state,
    accepted, duplicates, rejected, error_code, started_at, finished_at;

-- name: GetRegistryRun :one
SELECT id, source, snapshot_identity, checksum, parser_version, state,
    accepted, duplicates, rejected, error_code, started_at, finished_at
FROM registry_source_runs
WHERE source = @source AND snapshot_identity = @snapshot_identity;

-- name: FinishRegistryRun :execrows
UPDATE registry_source_runs
SET state = @state, accepted = @accepted, duplicates = @duplicates,
    rejected = @rejected, error_code = @error_code, finished_at = now()
WHERE id = @id AND state = 'running';

-- name: StageRegistryAssertion :one
INSERT INTO registry_assertions (
    id, run_id, source, source_key, checksum, display_name, address,
    municipality_code, state, auth_state, operation, eligibility,
    location_quality, source_reference, effective_date,
    latitude, longitude, crs
) VALUES (
    @id, @run_id, @source, @source_key, @checksum, @display_name,
    @address, @municipality_code, @state, @auth_state, @operation,
    @eligibility, @location_quality, @source_reference, @effective_date,
    @latitude, @longitude, @crs
)
ON CONFLICT (source, source_key, checksum) DO NOTHING
RETURNING id;

-- name: CountRegistryAssertions :one
SELECT count(*) FROM registry_assertions WHERE run_id = @run_id;

-- name: CountCompleteRegistryRuns :one
SELECT count(*) FROM registry_source_runs WHERE source = @source AND state = 'complete';

-- name: GetRegistryRunByID :one
SELECT id, source, snapshot_identity, checksum, parser_version, state,
    accepted, duplicates, rejected, error_code, started_at, finished_at
FROM registry_source_runs
WHERE id = @id;

-- name: ListRegistryAssertions :many
SELECT id, run_id, source, source_key, checksum, display_name, address,
    municipality_code, state, auth_state, operation, eligibility,
    location_quality, source_reference, latitude, longitude, crs,
    station_id
FROM registry_assertions
WHERE run_id = @run_id
ORDER BY source_key, checksum;

-- name: SetAssertionStation :execrows
UPDATE registry_assertions
SET station_id = @station_id
WHERE id = @id AND station_id IS NULL;

-- name: UpdateStationStatus :execrows
UPDATE directory_stations
SET status = @status
WHERE id = @id AND status <> @status
  -- Revocation sticks: a sourced suspended/revoked status is never
  -- cleared back to active by a later snapshot; reactivation is an
  -- audited operator decision, not an import side effect.
  AND NOT (status IN ('suspended', 'revoked') AND @status = 'active');

-- name: LastCompleteRegistryRun :one
SELECT id, source, snapshot_identity, checksum, parser_version, state,
    accepted, duplicates, rejected, error_code, started_at, finished_at
FROM registry_source_runs
WHERE source = @source AND state = 'complete'
ORDER BY finished_at DESC NULLS LAST, started_at DESC
LIMIT 1;

-- name: SetAssertionSuperseded :execrows
UPDATE registry_assertions
SET superseded_by = @superseded_by
WHERE id = @id AND superseded_by IS NULL;

-- Owned by directory (station suggestions, P27-T01). Private intake:
-- one row per owner idempotency key; owner-only reads; decisions land
-- in T02 (approved/rejected), cancellation stays owner-side.

-- name: CreateSuggestion :one
INSERT INTO station_suggestions (id, account_id, client_submission_id, proposal, evidence_ref)
VALUES (@id, @account_id, @client_submission_id, @proposal, @evidence_ref)
ON CONFLICT (account_id, client_submission_id) DO NOTHING
RETURNING id, account_id, client_submission_id, proposal, evidence_ref,
    state, created_at, decided_at;

-- name: GetSuggestion :one
SELECT id, account_id, client_submission_id, proposal, evidence_ref,
    state, created_at, decided_at
FROM station_suggestions
WHERE id = @id;

-- name: GetSuggestionByKey :one
SELECT id, account_id, client_submission_id, proposal, evidence_ref,
    state, created_at, decided_at
FROM station_suggestions
WHERE account_id = @account_id AND client_submission_id = @client_submission_id;

-- name: ListOwnedSuggestions :many
SELECT id, account_id, client_submission_id, proposal, evidence_ref,
    state, created_at, decided_at
FROM station_suggestions
WHERE account_id = @account_id
ORDER BY created_at DESC
LIMIT @page_limit::int OFFSET @page_offset::int;

-- name: CountRecentSuggestions :one
SELECT count(*) FROM station_suggestions
WHERE account_id = @account_id AND created_at > now() - make_interval(days => 1);

-- name: CancelSuggestion :execrows
UPDATE station_suggestions
SET state = 'cancelled'
WHERE id = @id AND account_id = @account_id AND state = 'pending';
