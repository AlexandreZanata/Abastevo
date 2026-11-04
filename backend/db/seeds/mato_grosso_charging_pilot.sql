-- Render through scripts/render-mt-ev-pilot.py. Never part of migrations.
SET LOCAL statement_timeout = '15s';

DO $seed$
DECLARE
    manifest CONSTANT jsonb := current_setting('abastevo.mt_ev_manifest')::jsonb;
    profile CONSTANT text := manifest->>'profile';
    edition CONSTANT text := manifest->>'revision';
    batch CONSTANT uuid := CASE profile
        WHEN 'charging-mt-top10-pilot-v1' THEN '27c19cf6-cce4-5455-894d-be7be1f47469'
        WHEN 'charging-mt-sorriso-supplement-v1' THEN '75b1de98-a227-54fc-b600-74856588ef17'
        WHEN 'charging-mt-cuiaba-supplement-v1' THEN 'b203cddc-9737-5f19-ba94-81799788b05f'
    END;
    total CONSTANT integer := jsonb_array_length(manifest->'places');
    fingerprint CONSTANT text := encode(sha256(convert_to(manifest::text, 'UTF8')), 'hex');
BEGIN
    IF coalesce(current_setting('abastevo.sorriso_pilot_target', true), '') NOT IN ('test', 'staging')
        OR batch IS NULL
        OR edition IS DISTINCT FROM '2026-11-04'
        OR total IS NULL OR total NOT BETWEEN 1 AND 100 THEN
        RAISE EXCEPTION 'Mato Grosso pilot requires its reviewed edition and a test or staging target';
    END IF;
    PERFORM pg_advisory_xact_lock(hashtext(profile));
    INSERT INTO charging_batches (id, format, profile_id, source_revision, manifest_sha256, input_count, accepted_count)
    VALUES (batch, 'charging-prepared-v1', profile, edition, fingerprint, total, total)
    ON CONFLICT (id) DO NOTHING;
    IF NOT EXISTS (
        SELECT 1 FROM charging_batches WHERE id = batch AND profile_id = profile
          AND source_revision = edition AND manifest_sha256 = fingerprint
          AND format = 'charging-prepared-v1' AND input_count = total AND accepted_count = total
    ) OR EXISTS (SELECT 1 FROM charging_batches WHERE profile_id = profile AND id <> batch) THEN
        RAISE EXCEPTION 'Mato Grosso pilot batch differs from the reviewed edition';
    END IF;

    CREATE TEMP TABLE mt_ev_expected ON COMMIT DROP AS
    SELECT p.id, p.external_id, p.name, p.latitude, p.longitude, p.access,
           encode(sha256(convert_to(profile || ':' || p.external_id, 'UTF8')), 'hex') AS entity_key
    FROM jsonb_to_recordset(manifest->'places') AS p(
        id uuid, external_id text, name text, latitude text, longitude text, access text);
    INSERT INTO charging_places (id, batch_id, entity_key, external_id, name, country,
                                 latitude, longitude, public_access, reported_points)
    SELECT id, batch, entity_key, external_id, name, 'BR', latitude, longitude, access, 0
    FROM mt_ev_expected
    ON CONFLICT (id) DO NOTHING;
    IF (SELECT count(*) FROM charging_places WHERE batch_id = batch) <> total OR EXISTS (
        SELECT 1 FROM mt_ev_expected p LEFT JOIN charging_places s ON s.id = p.id AND s.batch_id = batch
        WHERE (s.external_id, s.name, s.country, s.latitude, s.longitude, s.public_access, s.reported_points, s.entity_key)
          IS DISTINCT FROM (p.external_id, p.name, 'BR', p.latitude, p.longitude, p.access, 0, p.entity_key)
    ) THEN
        RAISE EXCEPTION 'Mato Grosso pilot places differ from the reviewed edition';
    END IF;
END;
$seed$;
