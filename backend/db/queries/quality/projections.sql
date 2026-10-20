-- FQ-13 deterministic projections. Derived rows are written only for the
-- current published revision of a stream and only when the basis or digest
-- actually changed; stale bases are refused by the current-pointer fence.

-- name: LockProjectionBasis :one
SELECT revision_id FROM quality_current_revisions
WHERE source = @source AND stream = @stream
FOR SHARE;

-- name: ListLinkedAssayRows :many
SELECT l.station_id, a.sample_ref, a.assay_ref, a.result_text, a.conformity,
       a.observed_on, a.fuel
FROM quality_links l
JOIN quality_assays a ON a.revision_id = l.revision_id
 AND l.record_key = 'pmqc-assay-v1:' || a.sample_ref
WHERE l.revision_id = @revision_id AND l.active AND l.station_id IS NOT NULL
ORDER BY l.station_id, a.sample_ref, a.assay_ref;

-- name: StageStationFuelProjection :execrows
INSERT INTO quality_station_fuel_projections(
 station_id, fuel, methodology, policy,
 basis_source, basis_stream, basis_revision, basis_published_at,
 input_digest, samples, visits, conforming, nonconforming, unresolved,
 oldest_observed_on, newest_observed_on, score_state, score)
SELECT @station_id, @fuel, @methodology, @policy,
 @basis_source, @basis_stream, r.id, r.published_at,
 @input_digest, @samples, @visits, @conforming, @nonconforming, @unresolved,
 @oldest_observed_on, @newest_observed_on, @score_state, NULL
FROM quality_revisions r
WHERE r.id = @basis_revision
 AND EXISTS(SELECT 1 FROM quality_current_revisions c
   WHERE c.source = @basis_source AND c.stream = @basis_stream AND c.revision_id = r.id)
ON CONFLICT (station_id, fuel, methodology) DO UPDATE SET
 policy = EXCLUDED.policy,
 basis_source = EXCLUDED.basis_source,
 basis_stream = EXCLUDED.basis_stream,
 basis_revision = EXCLUDED.basis_revision,
 basis_published_at = EXCLUDED.basis_published_at,
 input_digest = EXCLUDED.input_digest,
 samples = EXCLUDED.samples,
 visits = EXCLUDED.visits,
 conforming = EXCLUDED.conforming,
 nonconforming = EXCLUDED.nonconforming,
 unresolved = EXCLUDED.unresolved,
 oldest_observed_on = EXCLUDED.oldest_observed_on,
 newest_observed_on = EXCLUDED.newest_observed_on,
 score_state = EXCLUDED.score_state,
 score = EXCLUDED.score,
 generation = quality_station_fuel_projections.generation + 1,
 computed_at = now()
WHERE EXISTS(SELECT 1 FROM quality_current_revisions c
   WHERE c.source = EXCLUDED.basis_source AND c.stream = EXCLUDED.basis_stream
     AND c.revision_id = EXCLUDED.basis_revision)
 AND (quality_station_fuel_projections.basis_revision <> EXCLUDED.basis_revision
      OR quality_station_fuel_projections.input_digest <> EXCLUDED.input_digest);

-- name: ReadStationFuelProjections :many
SELECT station_id, fuel, methodology, policy, basis_source, basis_stream,
       basis_revision, input_digest, samples, visits, conforming, nonconforming,
       unresolved, oldest_observed_on, newest_observed_on, score_state, score,
       generation, computed_at
FROM quality_station_fuel_projections
WHERE basis_revision = @revision_id
ORDER BY station_id, fuel;

-- name: ReadStationDimensions :many
SELECT fuel, policy, input_digest, samples, visits, conforming, nonconforming,
 unresolved, oldest_observed_on, newest_observed_on, computed_at
FROM quality_station_fuel_projections
WHERE station_id = @station_id AND basis_revision = @revision_id
 AND methodology = @methodology AND (@fuel::text = '' OR fuel = @fuel)
ORDER BY fuel LIMIT 8;
