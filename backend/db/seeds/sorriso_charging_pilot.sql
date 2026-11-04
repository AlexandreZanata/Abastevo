-- Explicit reviewed Sorriso edition. Never run as part of migrations.
-- Source evidence: docs/backend/evidence/evc/SORRISO_CHARGING_PILOT.md.
BEGIN;
SET LOCAL statement_timeout = '10s';

DO $seed$
DECLARE
    profile CONSTANT text := 'charging-mt-sorriso-pilot-v1';
    edition CONSTANT text := '2026-11-04';
    batch CONSTANT uuid := '7cf5edec-2a66-5be2-9cd1-18f1b35cd943';
    places CONSTANT jsonb := '[
        {"id":"67c3d1f0-5898-54f0-bca3-689e9b48b099","external_id":"nova-rota-sau-17",
         "name":"Nova Rota do Oeste — SAU 17","latitude":"-12.3035","longitude":"-55.5743","access":"PUBLIC"},
        {"id":"451a0ea1-2a8c-5ed3-98ae-a6d11753dbdb","external_id":"byd-disbrava-sorriso",
         "name":"BYD Disbrava Sorriso","latitude":"-12.5708","longitude":"-55.7258","access":"UNKNOWN"}
    ]';
    total CONSTANT integer := jsonb_array_length(places);
    manifest CONSTANT text := encode(sha256(convert_to(places::text, 'UTF8')), 'hex');
BEGIN
    IF coalesce(current_setting('abastevo.sorriso_pilot_target', true), '') NOT IN ('test', 'staging') THEN
        RAISE EXCEPTION 'Sorriso pilot requires an explicit test or staging target';
    END IF;
    PERFORM pg_advisory_xact_lock(hashtext(profile));
    INSERT INTO charging_batches (id, format, profile_id, source_revision, manifest_sha256, input_count, accepted_count)
    VALUES (batch, 'charging-prepared-v1', profile, edition, manifest, total, total)
    ON CONFLICT (id) DO NOTHING;
    IF NOT EXISTS (
        SELECT 1 FROM charging_batches
        WHERE id = batch AND profile_id = profile AND source_revision = edition AND manifest_sha256 = manifest
          AND input_count = total AND accepted_count = total AND format = 'charging-prepared-v1'
    ) OR EXISTS (SELECT 1 FROM charging_batches WHERE profile_id = profile AND id <> batch) THEN
        RAISE EXCEPTION 'Sorriso pilot batch differs from the reviewed edition';
    END IF;
    INSERT INTO charging_places (id, batch_id, entity_key, external_id, name, country,
                                 latitude, longitude, public_access, reported_points)
    SELECT p.id, batch, encode(sha256(convert_to(profile || ':' || p.external_id, 'UTF8')), 'hex'),
           p.external_id, p.name, 'BR', p.latitude, p.longitude, p.access, 0
    FROM jsonb_to_recordset(places) AS p(id uuid, external_id text, name text, latitude text, longitude text, access text)
    ON CONFLICT (id) DO NOTHING;
    IF (SELECT count(*) FROM charging_places WHERE batch_id = batch) <> total OR EXISTS (
        SELECT 1
        FROM jsonb_to_recordset(places) AS p(id uuid, external_id text, name text, latitude text, longitude text, access text)
        LEFT JOIN charging_places stored ON stored.id = p.id AND stored.batch_id = batch
        WHERE stored.id IS NULL OR (stored.external_id, stored.name, stored.country, stored.latitude,
               stored.longitude, stored.public_access, stored.reported_points, stored.entity_key)
          IS DISTINCT FROM (p.external_id, p.name, 'BR', p.latitude, p.longitude, p.access, 0,
               encode(sha256(convert_to(profile || ':' || p.external_id, 'UTF8')), 'hex'))
    ) THEN
        RAISE EXCEPTION 'Sorriso pilot places differ from the reviewed edition';
    END IF;
END;
$seed$;

COMMIT;
